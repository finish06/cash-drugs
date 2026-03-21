# Runbook: High Latency

**Alert:** `CashDrugsHighLatency`
**Severity:** warning
**SLO Impact:** Cache hit latency P95 (< 50ms target -- alert fires at 100ms, meaning SLO is already breached)

## Symptom

The alert fires when the 95th percentile HTTP request latency exceeds 100ms for 5 minutes. This is 2x the SLO target of 50ms P95. Consumers will experience noticeably slower API responses. The alert threshold at 100ms gives an early warning; the SLO is already breached at this point.

## Likely Cause

1. **LRU cache thrashing** -- cache is full and frequently evicting hot entries, forcing MongoDB lookups
2. **MongoDB slow queries** -- collection scans due to missing indexes, or WiredTiger cache pressure
3. **High concurrent load** -- many simultaneous requests saturating the concurrency limiter
4. **Upstream fetches blocking request path** -- cache misses triggering synchronous upstream fetches
5. **GC pressure** -- Go garbage collector running frequently due to high memory allocation rate
6. **Container resource contention** -- another container on the same host consuming CPU/memory

## Diagnostic Steps

### 1. Check current latency

```
# PromQL: P95 latency
histogram_quantile(0.95,
  sum by (le, instance) (
    rate(cashdrugs_http_request_duration_seconds_bucket[5m])
  )
)

# PromQL: P50 for comparison (is the whole distribution shifted, or just the tail?)
histogram_quantile(0.50,
  sum by (le, instance) (
    rate(cashdrugs_http_request_duration_seconds_bucket[5m])
  )
)
```

### 2. Check cache hit rates

```
# PromQL: hit vs miss ratio
rate(cashdrugs_cache_hits_total{outcome="hit"}[5m])
rate(cashdrugs_cache_hits_total{outcome="miss"}[5m])
rate(cashdrugs_cache_hits_total{outcome="stale"}[5m])
```

If misses are high, latency is likely due to upstream fetches on the request path.

### 3. Check LRU cache utilization

```
# PromQL: LRU memory usage
cashdrugs_lru_cache_bytes
cashdrugs_lru_cache_entries

# PromQL: LRU evictions (high rate = thrashing)
rate(cashdrugs_lru_cache_evictions_total[5m])
```

### 4. Check MongoDB performance

```bash
# Staging
docker exec cash-drugs-mongo mongosh drugs_staging --eval "db.currentOp({secs_running: {'\$gt': 1}})"
docker exec cash-drugs-mongo mongosh drugs_staging --eval "db.cache_entries.stats()"

# Check if WiredTiger cache is under pressure
docker logs cash-drugs-mongo --tail 50 --since 5m 2>&1 | grep -i "cache\|evict\|slow"
```

### 5. Check container resource usage

```bash
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" | grep cash-drugs
```

### 6. Check for GC pressure

```bash
# Look for frequent GC pauses in app logs
docker logs cash-drugs-leader --tail 200 --since 10m 2>&1 | grep -i "gc\|pause\|gomemlimit"
```

### 7. Check concurrent request load

```
# PromQL: current in-flight requests
cashdrugs_inflight_requests

# PromQL: rejected requests (concurrency limiter saturated)
rate(cashdrugs_rejected_requests_total[5m])
```

## Remediation

### Quick Fix

**If LRU cache is thrashing (high eviction rate):**

Increase the LRU cache size. This takes effect on restart.

```bash
# Staging: edit docker-compose.staging.yml, change LRU_CACHE_SIZE_MB from 128 to 256
# Then restart
docker compose -f docker-compose.staging.yml up -d drugs-leader drugs-replica
```

**If MongoDB is slow:**

```bash
# Restart MongoDB to clear any stuck operations
docker compose -f docker-compose.staging.yml restart mongodb

# Verify indexes exist
docker exec cash-drugs-mongo mongosh drugs_staging --eval "db.cache_entries.getIndexes()"
```

**If load is too high:**

The concurrency limiter should be handling this. If requests are not being rejected but latency is high, the limit may be set too high for the instance's capacity. Consider lowering `MAX_CONCURRENT_REQUESTS`:

```bash
# Staging currently at 25; try 15 to reduce contention
# Edit docker-compose.staging.yml and restart
```

### Root Cause Fix

**LRU undersized for working set:**

Review the working set size. If the top 100 warmup queries fill most of the LRU, the cache needs to be larger:

```bash
# Check warmup query count
wc -l warmup-queries.yaml
```

Increase `LRU_CACHE_SIZE_MB` in the environment. In staging, also increase `GOMEMLIMIT` proportionally and the container memory limit.

**MongoDB missing indexes:**

```bash
docker exec cash-drugs-mongo mongosh drugs_staging --eval "
  db.cache_entries.createIndex({slug: 1, query_hash: 1}, {background: true})
"
```

**Sustained high load:**

Consider scaling horizontally -- add another replica in `docker-compose.staging.yml` and add it to the nginx `least_conn` upstream.

## Escalation

- **P2** if P95 exceeds 200ms sustained -- significant degradation for consumers
- **P3** if P95 is between 100ms and 200ms -- degraded but functional
- If latency spikes correlate with scheduler runs, the scheduler may be competing with live traffic for MongoDB and LRU resources

## Prevention

- Size the LRU cache to hold the full working set (all warmup queries + common on-demand queries)
- Run warmup queries after deployment to pre-populate the LRU
- Monitor LRU eviction rate as a leading indicator -- rising evictions predict latency increases
- Keep `GOMEMLIMIT` set to ~85-90% of the container memory limit to control GC behavior
- Ensure MongoDB WiredTiger cache is sized appropriately for the dataset

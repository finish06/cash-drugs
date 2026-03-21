# Runbook: High Memory

**Alert:** `CashDrugsHighMemory`
**Severity:** warning
**SLO Impact:** Availability (99.5% -- OOM kill causes downtime until container restarts)

## Symptom

The alert fires when container RSS exceeds 80% of the container memory limit for 5 minutes. If memory continues to rise, the container will be OOM-killed by the kernel, causing a service interruption until Docker restarts it (per `restart: unless-stopped`).

Current memory limits:
- **Staging leader/replica:** 512MB (GOMEMLIMIT=450MiB)
- **Staging MongoDB:** 2GB (WiredTiger cache 1GB)
- **Production:** No explicit container limit set (host memory is the ceiling)

## Likely Cause

1. **LRU cache exceeds configured size** -- `LRU_CACHE_SIZE_MB` is a soft target; actual memory includes Go overhead, response buffers, and metadata
2. **Large upstream responses held in memory** -- paginated fetches that aggregate many pages before storing
3. **GOMEMLIMIT too close to container limit** -- Go runtime keeps allocating up to GOMEMLIMIT before aggressive GC
4. **Memory leak** -- goroutine leak or unbounded growth in a data structure
5. **Warmup loaded too much data at once** -- the top-100 drug warmup queries may spike memory on startup
6. **MongoDB container memory pressure** -- WiredTiger cache sized too large for the container limit

## Diagnostic Steps

### 1. Check current memory usage

```bash
docker stats --no-stream --format "table {{.Name}}\t{{.MemUsage}}\t{{.MemPerc}}" | grep cash-drugs
```

### 2. Check metrics

```
# PromQL: memory usage ratio
cashdrugs_container_memory_rss_bytes / cashdrugs_container_memory_limit_bytes

# PromQL: absolute RSS
cashdrugs_container_memory_rss_bytes

# PromQL: LRU cache size in bytes
cashdrugs_lru_cache_bytes
```

### 3. Check LRU cache utilization

```
# PromQL: how much of the LRU budget is used?
cashdrugs_lru_cache_bytes
cashdrugs_lru_cache_entries
```

If `cashdrugs_lru_cache_bytes` is close to `LRU_CACHE_SIZE_MB * 1048576`, the LRU is full and Go runtime overhead may push total RSS beyond the container limit.

### 4. Check for goroutine leaks

```bash
# Fetch runtime debug info (if pprof is enabled)
curl -s http://192.168.1.145:8085/debug/pprof/goroutine?debug=1 | head -5

# Or check logs for goroutine count
docker logs cash-drugs-leader --tail 100 --since 10m 2>&1 | grep -i "goroutine\|routine"
```

### 5. Check if memory grew after warmup

```
# PromQL: memory over time -- look for step increase at startup
cashdrugs_container_memory_rss_bytes[1h]
```

### 6. Check for OOM kill history

```bash
dmesg | grep -i "oom\|killed" | tail 10
docker inspect cash-drugs-leader --format '{{.State.OOMKilled}}'
docker inspect cash-drugs-replica --format '{{.State.OOMKilled}}'
```

## Remediation

### Quick Fix

**Restart the container to reclaim memory:**

```bash
# Staging
docker compose -f docker-compose.staging.yml restart drugs-leader
# or
docker compose -f docker-compose.staging.yml restart drugs-replica
```

The LRU cache will be empty after restart. Run warmup queries to repopulate or wait for organic traffic.

**Reduce LRU cache size immediately:**

If the LRU is consuming too much relative to the container limit, reduce it:

```bash
# Edit docker-compose.staging.yml: LRU_CACHE_SIZE_MB=64 (from 128)
# Then restart
docker compose -f docker-compose.staging.yml up -d drugs-leader drugs-replica
```

### Root Cause Fix

**LRU + Go overhead exceeds container limit:**

The rule of thumb: total RSS = LRU cache + ~100-150MB for Go runtime, goroutines, request buffers, and MongoDB driver. Size accordingly:

| Container Limit | GOMEMLIMIT | LRU_CACHE_SIZE_MB | Headroom |
|----------------|------------|-------------------|----------|
| 512MB | 450MiB | 128 | ~180MB for Go runtime |
| 768MB | 650MiB | 256 | ~200MB for Go runtime |
| 1024MB | 900MiB | 512 | ~250MB for Go runtime |

If the working set requires a larger LRU, increase the container memory limit proportionally:

```yaml
# docker-compose.staging.yml
drugs-leader:
  environment:
    - LRU_CACHE_SIZE_MB=256
    - GOMEMLIMIT=650MiB
  deploy:
    resources:
      limits:
        memory: 768m
```

**Goroutine leak:**

If goroutine count is growing unboundedly, this is a code bug. Check for:
- Goroutines spawned in request handlers that never exit
- Channels that are never closed
- Context cancellation not being respected

File a bug and deploy a fix.

**Warmup memory spike:**

If memory spikes during warmup and then stabilizes, increase GOMEMLIMIT to accommodate the transient peak, or reduce the number of warmup queries.

## Escalation

- **P2** if memory is above 80% and climbing -- OOM kill is imminent
- **P3** if memory is at 80% but stable -- may sustain for a while but is risky
- If the container is being OOM-killed repeatedly (crash loop), escalate to **P1** -- the service is effectively down
- Check both app containers (leader + replica in staging) -- if only one is affected, the load balancer routes around it

## Prevention

- Always set `GOMEMLIMIT` to ~85-90% of the container memory limit
- Size `LRU_CACHE_SIZE_MB` to leave at least 150MB headroom below GOMEMLIMIT for Go runtime overhead
- Monitor `cashdrugs_lru_cache_bytes` as a leading indicator -- if it is growing toward the configured limit, memory pressure will follow
- Set up a separate alert for OOM kills at the host level
- In production, set explicit container memory limits (currently absent in `docker-compose.prod.yml`)

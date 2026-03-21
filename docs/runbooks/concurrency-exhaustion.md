# Runbook: Concurrency Exhaustion

**Alert:** `CashDrugsConcurrencyExhaustion`
**Severity:** warning
**SLO Impact:** Availability (99.5% -- rejected requests are effectively downtime for those consumers)

## Symptom

The alert fires when the concurrency limiter middleware has been rejecting requests for 5 minutes (`rate(cashdrugs_rejected_requests_total[5m]) > 0`). This means the maximum in-flight request limit (`MAX_CONCURRENT_REQUESTS`) is being sustained and incoming requests are receiving HTTP 503 Service Unavailable responses.

Current limits:
- **Staging:** 25 per instance (2 instances = 50 effective behind LB)
- **Production:** 150

## Likely Cause

1. **Traffic spike** -- a downstream consumer is issuing more concurrent requests than usual
2. **Slow upstream fetches holding slots** -- cache misses trigger upstream fetches that take seconds, occupying concurrency slots
3. **MongoDB slow responses** -- cache lookups taking longer than normal, causing request pile-up
4. **Scheduler competing with live traffic** -- scheduled bulk fetches consume concurrency slots, leaving fewer for live requests
5. **Single consumer flooding** -- one consumer sending rapid-fire requests without backoff
6. **Limit set too low for the workload** -- the configured limit does not match actual traffic patterns

## Diagnostic Steps

### 1. Check rejection rate

```
# PromQL: rejection rate
rate(cashdrugs_rejected_requests_total[5m])

# PromQL: current in-flight requests
cashdrugs_inflight_requests
```

### 2. Check both instances (staging)

```bash
# Leader
curl -s http://192.168.1.145:8085/health | jq .
# Replica
curl -s http://192.168.1.145:8086/health | jq .
```

If only one instance is rejecting, the load balancer may not be distributing evenly, or one instance is degraded.

### 3. Check what is consuming concurrency slots

```bash
# Look for slow requests in logs
docker logs cash-drugs-leader --tail 200 --since 5m 2>&1 | grep -i "duration\|slow\|timeout\|fetch"
docker logs cash-drugs-replica --tail 200 --since 5m 2>&1 | grep -i "duration\|slow\|timeout\|fetch"
```

### 4. Check if upstream fetches are the bottleneck

```
# PromQL: are cache misses high? (misses trigger upstream fetches)
rate(cashdrugs_cache_hits_total{outcome="miss"}[5m])

# PromQL: upstream fetch duration (if available)
rate(cashdrugs_upstream_fetch_errors_total[5m])
```

### 5. Check if scheduler is running during the spike

```
# PromQL: recent scheduler runs
increase(cashdrugs_scheduler_runs_total[10m])
```

If scheduler ran recently and the rejection spike followed, scheduled fetches are likely consuming concurrency slots.

### 6. Check request patterns

```bash
# Check nginx access logs for request patterns (staging)
docker logs cash-drugs --tail 200 --since 5m 2>&1 | grep -c "503"
docker logs cash-drugs --tail 200 --since 5m 2>&1 | grep "503"
```

## Remediation

### Quick Fix

**Increase the concurrency limit temporarily:**

```bash
# Staging: edit docker-compose.staging.yml
# Change MAX_CONCURRENT_REQUESTS from 25 to 50
docker compose -f docker-compose.staging.yml up -d drugs-leader drugs-replica
```

Note: increasing the limit trades rejection for higher latency. Only do this if the instances have CPU and memory headroom.

**If scheduler is the cause, temporarily disable it:**

```bash
# Edit docker-compose.staging.yml: ENABLE_SCHEDULER=false on leader
docker compose -f docker-compose.staging.yml up -d drugs-leader

# Re-enable once traffic normalizes
```

**If one instance is degraded, restart it:**

```bash
docker compose -f docker-compose.staging.yml restart drugs-leader
# or
docker compose -f docker-compose.staging.yml restart drugs-replica
```

### Root Cause Fix

**Slow upstream fetches occupying slots:**

The fetch lock deduplicates concurrent fetches for the same slug+query, but different queries still each occupy a slot. If many distinct cache misses arrive simultaneously, upstream fetches pile up. Solutions:

1. Ensure warmup queries cover the most common requests so they are served from LRU/MongoDB
2. Increase TTLs to reduce the frequency of cache misses
3. Consider a separate concurrency pool for upstream fetches vs. cache hits

**Limit too low for traffic volume:**

Review actual traffic patterns and adjust `MAX_CONCURRENT_REQUESTS`:

```bash
# Check peak concurrent requests over the last hour
# PromQL
max_over_time(cashdrugs_inflight_requests[1h])
```

Set the limit to ~2x the observed p99 concurrent request count to handle bursts.

**Scale horizontally:**

Add another replica in staging:

```yaml
# docker-compose.staging.yml -- add a third instance
drugs-replica-2:
  image: dockerhub.calebdunn.tech/finish06/cash-drugs:beta
  container_name: cash-drugs-replica-2
  environment:
    - ENABLE_SCHEDULER=false
    # ... same config as drugs-replica
```

Update `nginx-lb.conf` to include the new upstream.

## Escalation

- **P2** if requests are being rejected continuously for > 10 minutes
- **P3** if rejections are intermittent and self-resolving
- **P1** if all instances are rejecting -- service is effectively down for new requests
- Consumers should implement retry with backoff; if they are not, coordinate with them

## Prevention

- Size `MAX_CONCURRENT_REQUESTS` based on observed traffic, not arbitrary defaults
- Run warmup queries after deployment to minimize cold-cache misses
- Stagger scheduler refresh times to avoid scheduled fetches competing with live traffic
- Monitor `cashdrugs_inflight_requests` as a leading indicator -- if it frequently approaches the limit, increase the limit or scale out
- Consider implementing consumer-level rate limiting (per-API-key or per-source-IP) to prevent a single consumer from exhausting the pool

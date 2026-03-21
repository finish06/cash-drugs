# Runbook: Circuit Breaker Open

**Alert:** `CashDrugsCircuitBreakerOpen`
**Severity:** critical
**SLO Impact:** Upstream fetch success rate (> 95%), Stale-serve guarantee (100% -- stale data will be served, but freshness degrades)

## Symptom

The alert fires when `cashdrugs_circuit_state == 2` (open) for more than 1 minute for a specific upstream slug. The circuit breaker has tripped after 5 consecutive failures to that upstream API. All new requests for that slug are short-circuited -- the service returns cached/stale data if available, or errors if no cached data exists.

The alert label `{{ $labels.slug }}` identifies which upstream is affected (e.g., `drugnames`, `fda-enforcement`, `rxnorm-find-drug`).

## Likely Cause

1. **Upstream API is down** -- DailyMed, openFDA, or RxNorm is experiencing an outage
2. **Upstream API is rate-limiting** -- too many requests triggered HTTP 429 responses
3. **DNS resolution failure** -- the container cannot resolve the upstream hostname
4. **Network connectivity issue** -- firewall, proxy, or ISP blocking outbound HTTPS
5. **Upstream API changed** -- endpoint path, response format, or authentication requirements changed
6. **TLS certificate issue** -- upstream renewed their cert and something in the chain is rejected

## Diagnostic Steps

### 1. Identify the affected slug

```
# PromQL: which slugs have open circuits?
cashdrugs_circuit_state == 2
```

### 2. Check app logs for the upstream errors

```bash
# Staging
docker logs cash-drugs-leader --tail 200 --since 10m 2>&1 | grep -i "circuit\|upstream\|error\|fetch"

# Production
docker logs cash-drugs-drugs-1 --tail 200 --since 10m 2>&1 | grep -i "circuit\|upstream\|error\|fetch"
```

### 3. Test the upstream directly

```bash
# DailyMed
curl -sI https://dailymed.nlm.nih.gov/dailymed/services/v2/drugnames.json?pagesize=1

# openFDA
curl -sI https://api.fda.gov/drug/enforcement.json?limit=1

# RxNorm
curl -sI https://rxnav.nlm.nih.gov/REST/rxcui.json?name=aspirin
```

### 4. Test from inside the container

```bash
# Staging leader
docker exec cash-drugs-leader wget -qO- --spider https://dailymed.nlm.nih.gov/dailymed/services/v2/drugnames.json?pagesize=1

# Check DNS resolution
docker exec cash-drugs-leader nslookup dailymed.nlm.nih.gov
```

### 5. Check circuit breaker state and history

```
# PromQL: circuit state history (0=closed, 1=half-open, 2=open)
cashdrugs_circuit_state

# PromQL: upstream error rate
rate(cashdrugs_upstream_fetch_errors_total[5m])
```

### 6. Check if stale data is being served

```
# PromQL: stale cache serves should be increasing
rate(cashdrugs_cache_hits_total{outcome="stale"}[5m])
```

## Remediation

### Quick Fix

**If upstream is down (confirmed via direct curl):**

No action needed on our side. The circuit breaker is working as designed -- it protects the service from hammering a failing upstream. Stale cached data will be served to consumers. The circuit breaker will automatically transition to half-open after 30 seconds and attempt a probe request. If the upstream recovers, the circuit closes automatically.

Monitor with:
```
# Watch for circuit to close
cashdrugs_circuit_state{slug="<affected-slug>"}
```

**If it is a DNS/network issue on our side:**

```bash
# Restart the affected app container to reset network state
# Staging
docker compose -f docker-compose.staging.yml restart drugs-leader drugs-replica

# Production
docker compose -f docker-compose.prod.yml restart drugs
```

**If upstream is rate-limiting:**

The circuit breaker will handle backoff automatically. If the scheduler triggered a burst of paginated fetches that caused rate limiting, consider adjusting the `pagesize` or staggering refresh schedules in `config.yaml`.

### Root Cause Fix

**Upstream API changed:**

1. Verify the endpoint path and response format against upstream documentation
2. Update `config.yaml` if the path, query parameters, or data keys changed
3. Redeploy:
   ```bash
   docker compose -f docker-compose.staging.yml up -d drugs-leader drugs-replica
   ```

**Persistent rate limiting:**

Reduce fetch frequency by widening the cron schedule in `config.yaml`:

```yaml
# Example: reduce from every 6 hours to every 12 hours
refresh: "0 */12 * * *"
```

**DNS issues:**

Check `/etc/resolv.conf` inside the container and ensure the Docker network has proper DNS configuration.

## Escalation

- If a single slug circuit is open: **P3** -- partial degradation, stale data still served
- If all circuits are open: **P1** -- check if it is a host-level network issue
- If the upstream has been down for > 24 hours, consider whether the TTL on cached data is sufficient -- if data is expiring faster than the upstream recovers, consumers may start getting empty responses
- Check upstream status pages: [DailyMed](https://dailymed.nlm.nih.gov), [openFDA](https://open.fda.gov), [RxNorm](https://rxnav.nlm.nih.gov)

## Prevention

- Stagger refresh schedules across slugs to avoid bursting all upstream APIs at the same time
- Set generous TTLs on cached data so stale-serve covers extended upstream outages
- Monitor upstream APIs independently (synthetic checks) to distinguish our issues from theirs
- Consider adding per-upstream rate limit configuration if an API is known to be sensitive

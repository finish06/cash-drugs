# Runbook: Upstream Error Rate

**Alert:** `CashDrugsUpstreamErrorRate`
**Severity:** warning
**SLO Impact:** Upstream fetch success rate (> 95% target -- alert fires at > 5% error rate)

## Symptom

The alert fires when more than 5% of upstream fetch attempts are failing over a 5-minute window, sustained for 5 minutes. This directly threatens the 95% upstream fetch success rate SLO. Cached data is still being served, but new data is not flowing in. If this persists beyond TTLs, data freshness will degrade.

Note: this alert measures the aggregate error rate across all upstreams on an instance. Individual upstream failures may or may not trip this depending on request volume.

## Likely Cause

1. **One or more upstream APIs are returning errors** -- HTTP 5xx, timeouts, or connection refused
2. **Rate limiting from upstream** -- HTTP 429 responses from DailyMed, openFDA, or RxNorm
3. **Scheduler burst** -- a scheduled refresh triggered many parallel page fetches that overwhelmed an upstream
4. **Network instability** -- intermittent connectivity causing partial failures
5. **DNS resolution failures** -- transient DNS issues causing some requests to fail
6. **Upstream API format change** -- responses parse as errors due to unexpected structure

## Diagnostic Steps

### 1. Check the error rate by slug

```
# PromQL: error rate per slug
rate(cashdrugs_upstream_fetch_errors_total[5m])

# PromQL: compare errors to successes per slug
rate(cashdrugs_upstream_fetch_pages_total[5m])
```

### 2. Check app logs for error details

```bash
# Staging
docker logs cash-drugs-leader --tail 200 --since 10m 2>&1 | grep -i "upstream\|error\|fetch\|status"

# Production
docker logs cash-drugs-drugs-1 --tail 200 --since 10m 2>&1 | grep -i "upstream\|error\|fetch\|status"
```

Look for HTTP status codes (429, 500, 502, 503), timeout messages, and connection errors.

### 3. Test each upstream directly

```bash
# DailyMed
curl -sw "\n%{http_code} %{time_total}s" https://dailymed.nlm.nih.gov/dailymed/services/v2/drugnames.json?pagesize=1

# openFDA
curl -sw "\n%{http_code} %{time_total}s" https://api.fda.gov/drug/enforcement.json?limit=1

# RxNorm
curl -sw "\n%{http_code} %{time_total}s" https://rxnav.nlm.nih.gov/REST/rxcui.json?name=aspirin
```

### 4. Check circuit breaker states

```
# PromQL: are any circuits open or half-open?
cashdrugs_circuit_state
```

If circuits are opening, errors are concentrated on specific upstreams. If circuits are closed but error rate is high, errors may be spread across multiple upstreams at a low per-slug rate.

### 5. Check if errors correlate with scheduler runs

```
# PromQL: scheduler run timing
rate(cashdrugs_scheduler_runs_total[5m])
```

If error spikes align with scheduler runs, the scheduled bulk fetches may be triggering rate limits.

## Remediation

### Quick Fix

**If a specific upstream is down:**

No action needed -- the circuit breaker will handle it. Stale data is served. Monitor for recovery.

**If errors are caused by rate limiting (HTTP 429):**

Wait for the rate limit window to reset. The circuit breaker's 30-second open period acts as natural backoff. If the scheduler is the trigger, consider temporarily disabling it:

```bash
# Restart leader without scheduler to stop scheduled fetches
# Edit docker-compose.staging.yml: ENABLE_SCHEDULER=false on leader
docker compose -f docker-compose.staging.yml up -d drugs-leader

# Re-enable once the upstream recovers
```

**If network issues:**

```bash
# Restart app containers to reset connections
docker compose -f docker-compose.staging.yml restart drugs-leader drugs-replica
```

### Root Cause Fix

**Scheduler causing rate limit spikes:**

Stagger refresh schedules in `config.yaml` so different slugs refresh at different times:

```yaml
# Instead of all at "0 */6 * * *", offset them:
# drugnames at minute 0, spls at minute 15, drugclasses at minute 30
- slug: drugnames
  refresh: "0 */6 * * *"
- slug: spls
  refresh: "15 */6 * * *"
- slug: drugclasses
  refresh: "30 */6 * * *"
```

**Upstream API changed format:**

1. Check the upstream API documentation for changes
2. Update `config.yaml` (`data_key`, `path`, `query_params`) as needed
3. Redeploy the app containers

**Persistent network issues:**

Check host-level DNS configuration, firewall rules, and whether a proxy is required for outbound HTTPS.

## Escalation

- **P3** if error rate is 5-10% and only one upstream is affected -- cached data covers the gap
- **P2** if error rate exceeds 10% or multiple upstreams are failing
- **P1** if all upstreams are failing -- likely a host-level network issue
- Check upstream status pages before escalating: [DailyMed](https://dailymed.nlm.nih.gov), [openFDA](https://open.fda.gov), [RxNorm](https://rxnav.nlm.nih.gov)

## Prevention

- Stagger refresh schedules to avoid upstream request bursts
- Monitor per-slug error rates (not just aggregate) to catch individual upstream degradation early
- Set TTLs generously so cached data survives extended upstream outages (current TTLs range from 6h to 720h)
- Consider adding a per-upstream request rate limiter to stay below known API rate limits

# cash-drugs Service Level Agreement

**Version:** 1.0.0
**Effective:** 2026-03-20
**Review cadence:** Monthly (first Monday)
**Scope:** cash-drugs API cache/proxy running on self-hosted homelab infrastructure

---

## 1. Service Level Objectives (SLOs)

| SLO | Target | Measurement Window |
|-----|--------|--------------------|
| **Availability** | 99.5% | Rolling 30 days |
| **Cache hit latency (P95)** | < 50ms | Rolling 30 days |
| **Upstream fetch success rate** | > 95% | Rolling 30 days |
| **Stale-serve guarantee** | 100% | Per-incident |

### SLO Definitions

**Availability (99.5%)**
The `/health` endpoint returns HTTP 200. Measured by periodic probe (e.g., every 30 seconds). Allows ~3.6 hours of downtime per 30-day window. This target accounts for the realities of homelab infrastructure -- power events, network maintenance, host reboots.

**Cache hit latency P95 < 50ms**
Requests served from MongoDB or LRU cache (outcome = `hit` or `stale`) complete within 50ms at the 95th percentile. Upstream fetch latency is excluded -- this measures only the cache-serving path.

**Upstream fetch success rate > 95%**
Of all upstream API fetch attempts (scheduled and on-demand), at least 95% succeed without error. This accounts for expected transient failures from external APIs (DailyMed, openFDA, RxNorm) while flagging systemic problems.

**Stale-serve guarantee (100%)**
When upstream APIs are unavailable, cash-drugs always serves the last cached response rather than returning an error. If cached data exists for a slug, consumers never see a failure due to upstream outage.

---

## 2. Metrics and Measurement

Each SLO maps to specific Prometheus metrics already exported by the service.

### Availability

| Metric | Usage |
|--------|-------|
| `cashdrugs_mongodb_up` | MongoDB reachability (1 = healthy) |
| `cashdrugs_uptime_seconds` | Process uptime -- resets indicate restarts |
| External probe on `/health` | Synthetic uptime check (Uptime Kuma, Blackbox Exporter, or equivalent) |

**PromQL (availability over 30d):**
```promql
avg_over_time(up{job="cash-drugs"}[30d]) * 100
```

### Cache Hit Latency

| Metric | Usage |
|--------|-------|
| `cashdrugs_http_request_duration_seconds` | Histogram of request latency by slug and method |

**PromQL (P95 cache latency):**
```promql
histogram_quantile(0.95,
  sum(rate(cashdrugs_http_request_duration_seconds_bucket{method="GET"}[5m])) by (le)
)
```

### Upstream Fetch Success Rate

| Metric | Usage |
|--------|-------|
| `cashdrugs_upstream_fetch_errors_total` | Failed upstream fetches per slug |
| `cashdrugs_upstream_fetch_pages_total` | Successful upstream page fetches per slug |
| `cashdrugs_circuit_state` | Circuit breaker state (0=closed, 1=half-open, 2=open) |

**PromQL (success rate over 30d):**
```promql
1 - (
  sum(increase(cashdrugs_upstream_fetch_errors_total[30d]))
  /
  (sum(increase(cashdrugs_upstream_fetch_pages_total[30d])) + sum(increase(cashdrugs_upstream_fetch_errors_total[30d])))
) * 100
```

### Stale-Serve Guarantee

| Metric | Usage |
|--------|-------|
| `cashdrugs_cache_hits_total{outcome="stale"}` | Stale cache serves (should increase during upstream outages) |
| `cashdrugs_circuit_state` | Circuit breaker open indicates upstream failure -- verify stale serves are happening |

**Verification:** During any period where `cashdrugs_circuit_state > 0` for a slug, `cashdrugs_cache_hits_total{outcome="stale"}` should be increasing (not returning errors).

---

## 3. Incident Response

### Severity Levels

| Severity | Definition | Examples |
|----------|-----------|----------|
| **P1 -- Critical** | Service is down or returning errors to all consumers | MongoDB unreachable, process crash loop, all circuits open |
| **P2 -- Major** | Significant degradation affecting most consumers | Cache latency P95 > 200ms, scheduler completely stalled, memory exhaustion |
| **P3 -- Minor** | Partial degradation, workarounds exist | Single upstream circuit open, one slug stale beyond 2x TTL, elevated error rate on one endpoint |
| **P4 -- Low** | Cosmetic or informational | Log noise, non-critical metric gap, documentation inaccuracy |

### Response Targets

| Severity | Acknowledge | Investigate | Resolve/Mitigate |
|----------|-------------|-------------|-------------------|
| **P1** | 15 minutes | 30 minutes | 2 hours |
| **P2** | 1 hour | 4 hours | 8 hours |
| **P3** | 4 hours | Next business day | 3 business days |
| **P4** | Next business day | Best effort | Next release |

These targets assume homelab availability -- evenings and weekends are best-effort. "Acknowledge" means someone has seen the alert and is looking.

### Escalation Path

1. **Automated alerts** fire to notification channel (Alertmanager/Grafana -> Discord/email)
2. **On-call owner** acknowledges and triages (single person for homelab)
3. **If unresolved within response target:** post in project channel, consider rollback
4. **If rollback needed:** redeploy previous known-good tag (`docker-compose.prod.yml` with prior image tag)

### Key Alerting Rules (to be implemented in M14)

| Alert | Condition | Severity |
|-------|-----------|----------|
| CacheLatencyHigh | P95 > 50ms for 5 minutes | P2 |
| UpstreamErrorRateHigh | Error rate > 10% for 10 minutes | P3 |
| CircuitBreakerOpen | Any circuit open for > 15 minutes | P3 |
| MongoDBDown | `cashdrugs_mongodb_up == 0` for 2 minutes | P1 |
| SchedulerStalled | No scheduler runs for 2x cron interval | P3 |
| HighMemoryUsage | `cashdrugs_container_memory_usage_ratio > 0.85` for 5 minutes | P2 |
| ConcurrencyExhausted | `cashdrugs_rejected_requests_total` increasing for 5 minutes | P2 |

---

## 4. Exclusions

The following are **not** covered by this SLA:

**Planned maintenance**
- Host OS updates, Docker engine upgrades, MongoDB version bumps
- Scheduled during low-traffic windows (late night / early morning)
- Announced at least 1 hour in advance when possible
- Target: < 30 minutes downtime per maintenance event

**External upstream API failures**
- DailyMed, openFDA, and RxNorm outages are outside our control
- The stale-serve guarantee means consumers still get data, but freshness degrades
- Upstream SLA is their problem; our SLA covers serving cached data through their outages

**Infrastructure failures outside service scope**
- Power outages to the homelab
- ISP/network failures
- Host hardware failure
- Docker daemon crashes
- These are tracked separately as infrastructure incidents, not service incidents

**Force majeure**
- Natural disasters, extended power outages, hardware death
- Best effort recovery; no SLA targets apply

---

## 5. Error Budget

With a 99.5% availability target over 30 days:

| Window | Allowed Downtime |
|--------|-----------------|
| 30 days | ~3 hours 36 minutes |
| 7 days | ~50 minutes |
| 24 hours | ~7 minutes |

**Error budget policy:**
- If > 50% of monthly error budget is consumed in a single incident, conduct a brief post-incident review
- If monthly error budget is exhausted, pause non-critical changes until the next window resets
- Error budget does not roll over between months

---

## 6. Review Cadence

**Monthly review (first Monday of each month):**
- Check SLO compliance for the past 30 days using Grafana dashboards
- Review any P1/P2 incidents and their resolution
- Assess whether targets need adjustment based on actual performance
- Update this document if targets change

**Quarterly review:**
- Evaluate whether SLO targets are appropriate for current usage patterns
- Consider tightening targets if consistently exceeded (e.g., 99.5% -> 99.9%)
- Review exclusion list for relevance

---

## Revision History

| Date | Version | Changes |
|------|---------|---------|
| 2026-03-20 | 1.0.0 | Initial SLA document |

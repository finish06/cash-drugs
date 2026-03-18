# Service Level Agreement — cash-drugs

**Version:** 1.0.0
**Effective:** Upon GA promotion
**Last Updated:** 2026-03-18

## Service Description

cash-drugs is an internal API cache/proxy microservice. It fetches data from external REST APIs, caches responses in MongoDB, and serves cached data to internal consumers with resilience against upstream failures.

## Availability

| Target | Value | Measurement |
|--------|-------|-------------|
| Uptime | 99.5% | Monthly, measured at `/health` endpoint |
| Planned maintenance | < 1 hour/month | Announced 24h in advance |
| Unplanned downtime | < 2 hours/month | Tracked via monitoring alerts |

## Performance

| Metric | Target | Measurement |
|--------|--------|-------------|
| Cached response (LRU hit) | P95 < 5ms | Prometheus `cashdrugs_http_request_duration_seconds` |
| Cached response (MongoDB hit) | P95 < 50ms | Same metric, cache="hit" |
| Upstream fetch (cache miss) | P95 < 5s | `cashdrugs_upstream_fetch_duration_seconds` |
| Warmup completion | < 5 minutes | `/ready` endpoint returns 200 |

## Incident Response

| Severity | Response Time | Resolution Time |
|----------|---------------|-----------------|
| Critical (service down) | < 15 minutes | < 1 hour |
| High (degraded, serving stale) | < 30 minutes | < 4 hours |
| Medium (single endpoint failing) | < 2 hours | < 24 hours |
| Low (cosmetic, non-blocking) | Next business day | Next cycle |

## Monitoring

| System | Purpose | Alert Threshold |
|--------|---------|-----------------|
| Prometheus `/metrics` | Operational metrics | Scraped every 15s |
| `/health` endpoint | Liveness check | 3 consecutive failures |
| `/ready` endpoint | Readiness check | 503 for > 10 minutes after restart |
| `cashdrugs_upstream_fetch_errors_total` | Upstream failures | > 10/min per slug |
| `cashdrugs_circuit_state` | Circuit breaker | Any slug in state=2 (open) |

## Escalation

1. **Automated:** Circuit breaker opens, stale cache served, alert fires
2. **On-call:** Check Grafana dashboard, verify upstream API status
3. **Manual:** Restart service, check MongoDB connectivity, review logs
4. **Rollback:** Redeploy previous tag from `dockerhub.calebdunn.tech`

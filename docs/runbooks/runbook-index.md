# Operational Runbooks -- cash-drugs

Quick-reference index for all alerting runbooks. Each runbook corresponds to a Prometheus alert defined in `docs/grafana/alerts.yml`.

| Alert | Severity | Runbook | Summary |
|-------|----------|---------|---------|
| `CashDrugsMongoDBDown` | critical | [mongodb-down.md](mongodb-down.md) | MongoDB unreachable -- all cache lookups miss, upstream fetches cannot be stored |
| `CashDrugsCircuitBreakerOpen` | critical | [circuit-breaker-open.md](circuit-breaker-open.md) | Upstream circuit breaker open -- requests to that upstream are being rejected |
| `CashDrugsHighLatency` | warning | [high-latency.md](high-latency.md) | P95 request latency exceeds 100ms for 5 minutes |
| `CashDrugsUpstreamErrorRate` | warning | [upstream-error-rate.md](upstream-error-rate.md) | More than 5% of upstream fetch attempts are failing |
| `CashDrugsHighMemory` | warning | [high-memory.md](high-memory.md) | Container RSS exceeds 80% of memory limit |
| `CashDrugsConcurrencyExhaustion` | warning | [concurrency-exhaustion.md](concurrency-exhaustion.md) | Concurrency limiter is rejecting incoming requests |
| `CashDrugsSchedulerStalled` | warning | [scheduler-stalled.md](scheduler-stalled.md) | Leader scheduler has not run in over 1 hour -- cached data going stale |

## Environment Reference

| Environment | Address | Instances |
|-------------|---------|-----------|
| Staging | `192.168.1.145:8083` | 2 (leader + replica) behind nginx `least_conn` LB |
| Production | `192.168.1.86:8083` | Single instance |
| Local dev | `localhost:8080` | Single instance via docker-compose |

## Key Defaults

| Parameter | Default |
|-----------|---------|
| `MAX_CONCURRENT_REQUESTS` | 50 (staging: 25, production: 150) |
| `LRU_CACHE_SIZE_MB` | 256 (staging: 128) |
| Circuit breaker threshold | 5 consecutive failures |
| Circuit breaker open duration | 30 seconds |
| Force-refresh cooldown | 30 seconds |
| `GOMEMLIMIT` (staging) | 450MiB |
| Container memory limit (staging) | 512MB |

## SLO Targets

| SLO | Target |
|-----|--------|
| Availability | 99.5% (rolling 30 days) |
| Cache hit latency P95 | < 50ms |
| Upstream fetch success rate | > 95% |
| Stale-serve guarantee | 100% |

See `docs/sla.md` for full SLA details.

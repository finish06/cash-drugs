# M8 — Prometheus Metrics & Observability

**Goal:** Expose a `/metrics` Prometheus endpoint that provides full operational visibility — MongoDB health, cache performance, upstream API behavior, and request throughput.

**Appetite:** Medium — new package, instrumentation across handler/cache/upstream/scheduler layers

**Target Maturity:** Beta

**Status:** NOW

## Success Criteria

- [ ] `/metrics` endpoint serves Prometheus-compatible metrics
- [ ] MongoDB health and size metrics are exported
- [ ] Cache hit/miss/stale counters are tracked per slug
- [ ] Upstream fetch duration, count, and error rate are tracked per slug
- [ ] Request throughput and latency histograms are available per slug and status code
- [ ] Scheduler job execution metrics are tracked
- [ ] Go runtime metrics (goroutines, memory, GC) are included
- [ ] Existing functionality and tests remain unaffected

## Hill Chart

| Feature | Position |
|---------|----------|
| Prometheus endpoint & Go runtime | SHAPED |
| Request metrics (throughput, latency, status codes) | SHAPED |
| Cache performance metrics (hit/miss/stale) | SHAPED |
| Upstream fetch metrics (duration, errors, pages) | SHAPED |
| MongoDB health & size metrics | SHAPED |
| Scheduler job metrics | SHAPED |

## Features

### 1. Prometheus Endpoint (`/metrics`)

Serve OpenMetrics/Prometheus exposition format via `promhttp.Handler()`. Include default Go runtime metrics (goroutines, heap, GC pause, open FDs).

### 2. HTTP Request Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `cashdrugs_http_requests_total` | Counter | `slug`, `method`, `status_code` | Total HTTP requests |
| `cashdrugs_http_request_duration_seconds` | Histogram | `slug`, `method`, `status_code` | Request latency distribution |
| `cashdrugs_http_requests_inflight` | Gauge | — | Currently processing requests |

### 3. Cache Performance Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `cashdrugs_cache_hits_total` | Counter | `slug` | Cache hit count |
| `cashdrugs_cache_misses_total` | Counter | `slug` | Cache miss count |
| `cashdrugs_cache_stale_serves_total` | Counter | `slug` | Stale-while-revalidate serves |
| `cashdrugs_cache_hit_ratio` | Gauge | `slug` | Rolling hit ratio (hits / total) |

### 4. Upstream Fetch Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `cashdrugs_upstream_fetches_total` | Counter | `slug`, `status_code` | Total upstream API calls |
| `cashdrugs_upstream_fetch_duration_seconds` | Histogram | `slug` | Upstream fetch latency |
| `cashdrugs_upstream_errors_total` | Counter | `slug`, `error_type` | Upstream failures (timeout, 5xx, connection) |
| `cashdrugs_upstream_pages_fetched_total` | Counter | `slug` | Total pages fetched across all requests |

### 5. MongoDB Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `cashdrugs_mongodb_up` | Gauge | — | 1 if MongoDB is reachable, 0 otherwise |
| `cashdrugs_mongodb_ping_duration_seconds` | Gauge | — | Last MongoDB ping latency |
| `cashdrugs_mongodb_documents_total` | Gauge | `slug` | Cached document count per slug |
| `cashdrugs_mongodb_data_size_bytes` | Gauge | `slug` | Estimated data size per slug |
| `cashdrugs_mongodb_collections_total` | Gauge | — | Total collections in the database |

### 6. Scheduler Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `cashdrugs_scheduler_runs_total` | Counter | `slug`, `result` | Scheduled job executions (success/failure) |
| `cashdrugs_scheduler_run_duration_seconds` | Histogram | `slug` | Scheduled job duration |
| `cashdrugs_scheduler_active_jobs` | Gauge | — | Number of registered cron jobs |
| `cashdrugs_scheduler_last_run_timestamp` | Gauge | `slug` | Unix timestamp of last scheduled run |

### 7. Fetch Lock Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `cashdrugs_fetchlock_dedup_total` | Counter | `slug` | Times a concurrent fetch was deduplicated |
| `cashdrugs_fetchlock_active` | Gauge | — | Currently held fetch locks |

## Dependencies

- `github.com/prometheus/client_golang` — Prometheus client library
- No other new dependencies expected

## Risks

| Risk | Mitigation |
|------|------------|
| Metrics cardinality explosion from dynamic slugs | Slugs are config-defined, bounded set (~13 currently) |
| MongoDB size query performance | Use `collStats` or `estimatedDocumentCount`, run on interval not per-scrape |
| Instrumentation clutters handler code | Isolate metrics in `internal/metrics/` package, pass recorder interface |

## Cycles

**Cycle 1:** Prometheus endpoint + request metrics + cache metrics
**Cycle 2:** Upstream fetch metrics + MongoDB metrics + scheduler/fetchlock metrics

---
*Milestone for cash-drugs. Spec: `specs/prometheus-metrics.md`*

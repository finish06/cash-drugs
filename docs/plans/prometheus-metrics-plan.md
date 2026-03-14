# Implementation Plan: Prometheus Metrics

**Spec:** specs/prometheus-metrics.md
**Created:** 2026-03-14
**Status:** Approved

## 1. Task Breakdown

### Task 1: Add Prometheus dependency
- `go get github.com/prometheus/client_golang/prometheus`
- `go get github.com/prometheus/client_golang/prometheus/promhttp`
- **Maps to:** AC-001, AC-014

### Task 2: Create `internal/metrics` package — metric definitions
- Define all metric variables (counters, histograms, gauges) with `cashdrugs_` namespace
- Register all metrics with the default Prometheus registry
- Export accessor functions for each metric
- **Maps to:** AC-001, AC-002, AC-003, AC-004, AC-005, AC-006, AC-007, AC-008, AC-009, AC-010, AC-011, AC-012, AC-013, AC-018

### Task 3: Instrument HTTP handler — request counter and duration
- Add middleware or inline instrumentation to `CacheHandler.ServeHTTP`
- Record `cashdrugs_http_requests_total` with slug, method, status_code
- Record `cashdrugs_http_request_duration_seconds` with slug, method
- Wrap response writer to capture status code
- **Maps to:** AC-002, AC-003

### Task 4: Instrument cache layer — hit/miss/stale counters
- Record `cashdrugs_cache_hits_total` in `CacheHandler.ServeHTTP` at each cache decision point
- Labels: slug, outcome (hit, miss, stale)
- **Maps to:** AC-004

### Task 5: Instrument upstream fetcher — duration, errors, pages
- Record `cashdrugs_upstream_fetch_duration_seconds` in `CacheHandler.ServeHTTP` and `scheduler.fetchEndpoint`
- Record `cashdrugs_upstream_fetch_errors_total` on fetch failure
- Record `cashdrugs_upstream_fetch_pages_total` on successful fetch
- **Maps to:** AC-005, AC-006, AC-007

### Task 6: Instrument MongoDB — health gauge, ping latency, document counts
- Add `CollectionStats` or `CountDocuments` method to `MongoRepository`
- Create background goroutine (every 30s) that pings MongoDB and counts documents per slug
- Update `cashdrugs_mongodb_up`, `cashdrugs_mongodb_ping_duration_seconds`, `cashdrugs_mongodb_documents_total`
- **Maps to:** AC-008, AC-009, AC-010, AC-019

### Task 7: Instrument scheduler — run counter and duration
- Record `cashdrugs_scheduler_runs_total` with slug, result (success/error)
- Record `cashdrugs_scheduler_run_duration_seconds` with slug
- **Maps to:** AC-011, AC-012

### Task 8: Instrument fetch locks — dedup counter
- Record `cashdrugs_fetchlock_dedup_total` when TryLock returns false
- **Maps to:** AC-013

### Task 9: Register `/metrics` endpoint
- Add `mux.Handle("/metrics", promhttp.Handler())` in `main.go`
- **Maps to:** AC-001, AC-015

### Task 10: Create Grafana dashboard JSON
- Create `docs/grafana/cash-drugs-dashboard.json`
- Include panels for: request rate, latency, cache hit ratio, upstream errors, MongoDB health, scheduler runs
- Use `${DS_PROMETHEUS}` datasource variable
- **Maps to:** AC-017

### Task 11: Verify no regressions
- Run full test suite
- Verify all existing tests pass
- **Maps to:** AC-016

## 2. File Changes

| File | Action | Purpose |
|------|--------|---------|
| `go.mod` / `go.sum` | Modified | Add prometheus/client_golang dependency |
| `internal/metrics/metrics.go` | Created | Metric definitions and registration |
| `internal/metrics/metrics_test.go` | Created | Unit tests for metric registration |
| `internal/metrics/collector.go` | Created | MongoDB background collector |
| `internal/metrics/collector_test.go` | Created | Tests for MongoDB collector |
| `internal/handler/cache.go` | Modified | Add HTTP + cache + upstream instrumentation |
| `internal/handler/cache_test.go` | Modified | Tests for metric instrumentation |
| `internal/scheduler/scheduler.go` | Modified | Add scheduler metric instrumentation |
| `internal/scheduler/scheduler_test.go` | Modified | Tests for scheduler metrics |
| `internal/fetchlock/fetchlock.go` | Modified | Add dedup counter |
| `internal/fetchlock/fetchlock_test.go` | Modified | Test dedup counting |
| `internal/cache/mongo.go` | Modified | Add CountDocuments method |
| `cmd/server/main.go` | Modified | Register `/metrics` endpoint, start MongoDB collector |
| `docs/grafana/cash-drugs-dashboard.json` | Created | Grafana dashboard |

## 3. Test Strategy

- **Unit tests:** Verify metric registration, label correctness, counter increments using `testutil` from prometheus/client_golang
- **Integration:** Verify `/metrics` endpoint returns valid exposition format
- **Regression:** Full test suite must pass after changes

## 4. Dependencies

- Task 1 must complete before all others
- Task 2 must complete before Tasks 3-8
- Tasks 3-8 are independent and can be done in any order
- Task 9 depends on Task 2
- Task 10 depends on Task 2 (needs metric names)
- Task 11 is final

## 5. Risks

| Risk | Mitigation |
|------|------------|
| High cardinality from slug labels | Bounded by config — only configured slugs appear |
| Prometheus dependency conflicts | client_golang is well-maintained, minimal deps |
| MongoDB CountDocuments performance | Run in background goroutine, not on scrape |
| Breaking existing tests | Metrics are opt-in — existing code unaffected unless instrumented |

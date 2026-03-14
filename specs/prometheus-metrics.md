# Spec: Prometheus Metrics

**Version:** 0.1.0
**Created:** 2026-03-14
**PRD Reference:** docs/prd.md (M8)
**Status:** Complete

## 1. Overview

Expose a `/metrics` endpoint in Prometheus exposition format providing full operational observability. Instrument all layers — HTTP handlers, cache, upstream fetcher, MongoDB, scheduler, and fetch locks — with labeled counters, histograms, and gauges. Include example Grafana dashboard JSON with variable-level datasource configuration.

### User Story

As an **operator**, I want a Prometheus-compatible `/metrics` endpoint, so that I can monitor cache performance, upstream API health, MongoDB status, and scheduler behavior in Grafana without custom log parsing.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | `GET /metrics` returns valid Prometheus exposition format (text/plain; version=0.0.4) | Must |
| AC-002 | HTTP request counter `cashdrugs_http_requests_total` with labels `slug`, `method`, `status_code` | Must |
| AC-003 | HTTP request duration histogram `cashdrugs_http_request_duration_seconds` with labels `slug`, `method` | Must |
| AC-004 | Cache outcome counter `cashdrugs_cache_hits_total` with labels `slug`, `outcome` (hit, miss, stale) | Must |
| AC-005 | Upstream fetch duration histogram `cashdrugs_upstream_fetch_duration_seconds` with labels `slug` | Must |
| AC-006 | Upstream fetch error counter `cashdrugs_upstream_fetch_errors_total` with labels `slug` | Must |
| AC-007 | Upstream fetch page counter `cashdrugs_upstream_fetch_pages_total` with labels `slug` | Must |
| AC-008 | MongoDB ping latency gauge `cashdrugs_mongodb_ping_duration_seconds` | Must |
| AC-009 | MongoDB health gauge `cashdrugs_mongodb_up` (1 = healthy, 0 = unhealthy) | Must |
| AC-010 | MongoDB document count gauge `cashdrugs_mongodb_documents_total` with label `slug` | Should |
| AC-011 | Scheduler job execution counter `cashdrugs_scheduler_runs_total` with labels `slug`, `result` (success, error) | Must |
| AC-012 | Scheduler job duration histogram `cashdrugs_scheduler_run_duration_seconds` with labels `slug` | Must |
| AC-013 | Fetch lock deduplication counter `cashdrugs_fetchlock_dedup_total` with label `slug` | Should |
| AC-014 | Go runtime metrics (goroutines, memory, GC) are included via default Prometheus collectors | Must |
| AC-015 | The `/metrics` endpoint does not require authentication | Must |
| AC-016 | Adding the metrics package does not break any existing tests or functionality | Must |
| AC-017 | Example Grafana dashboard JSON file(s) provided in `docs/grafana/` with variable datasource (`${DS_PROMETHEUS}`) | Must |
| AC-018 | All metric names use the `cashdrugs_` prefix for namespace isolation | Must |
| AC-019 | MongoDB collector runs periodically (every 30s) via background goroutine, not on every `/metrics` scrape | Should |

## 3. User Test Cases

### TC-001: Metrics endpoint returns Prometheus format

**Precondition:** Service is running
**Steps:**
1. `curl http://localhost:8080/metrics`
2. Inspect response headers and body
**Expected Result:** Content-Type is `text/plain; version=0.0.4; charset=utf-8`. Body contains `# HELP` and `# TYPE` lines. Body contains `cashdrugs_http_requests_total` metric.
**Maps to:** AC-001, AC-018

### TC-002: Cache hit/miss/stale counters increment

**Precondition:** Service running with endpoints configured
**Steps:**
1. Request `GET /api/cache/drugnames` (cache miss on first call)
2. Request `GET /api/cache/drugnames` again (cache hit)
3. Check `/metrics` for `cashdrugs_cache_hits_total`
**Expected Result:** `cashdrugs_cache_hits_total{slug="drugnames",outcome="miss"}` >= 1 and `cashdrugs_cache_hits_total{slug="drugnames",outcome="hit"}` >= 1
**Maps to:** AC-004

### TC-003: HTTP request metrics populated

**Precondition:** Service running
**Steps:**
1. Request `GET /api/cache/drugnames`
2. Request `GET /api/cache/nonexistent` (404)
3. Check `/metrics`
**Expected Result:** `cashdrugs_http_requests_total{slug="drugnames",method="GET",status_code="200"}` >= 1 and `cashdrugs_http_requests_total{slug="nonexistent",method="GET",status_code="404"}` >= 1. Duration histogram has observations.
**Maps to:** AC-002, AC-003

### TC-004: Upstream fetch metrics recorded

**Precondition:** Service running, cache empty
**Steps:**
1. Request an endpoint that triggers upstream fetch
2. Check `/metrics`
**Expected Result:** `cashdrugs_upstream_fetch_duration_seconds` has observations. `cashdrugs_upstream_fetch_pages_total` incremented.
**Maps to:** AC-005, AC-007

### TC-005: MongoDB health metrics present

**Precondition:** Service running with MongoDB connected
**Steps:**
1. Check `/metrics`
**Expected Result:** `cashdrugs_mongodb_up` is 1. `cashdrugs_mongodb_ping_duration_seconds` has a value.
**Maps to:** AC-008, AC-009

### TC-006: Scheduler metrics after cron execution

**Precondition:** Service running with scheduled endpoints
**Steps:**
1. Wait for a scheduled fetch to complete (or trigger via short cron)
2. Check `/metrics`
**Expected Result:** `cashdrugs_scheduler_runs_total{slug="...",result="success"}` >= 1. Duration histogram has observations.
**Maps to:** AC-011, AC-012

### TC-007: Grafana dashboard imports successfully

**Precondition:** Grafana instance available
**Steps:**
1. Import `docs/grafana/cash-drugs-dashboard.json` into Grafana
2. Select Prometheus datasource
**Expected Result:** Dashboard loads with all panels populated. Datasource variable works.
**Maps to:** AC-017

## 4. Data Model

No new MongoDB collections or documents. Metrics are in-memory only (Prometheus client library state).

### Metric Registry

| Metric Name | Type | Labels | Description |
|-------------|------|--------|-------------|
| `cashdrugs_http_requests_total` | Counter | `slug`, `method`, `status_code` | Total HTTP requests |
| `cashdrugs_http_request_duration_seconds` | Histogram | `slug`, `method` | HTTP request latency |
| `cashdrugs_cache_hits_total` | Counter | `slug`, `outcome` | Cache outcomes (hit/miss/stale) |
| `cashdrugs_upstream_fetch_duration_seconds` | Histogram | `slug` | Upstream fetch latency |
| `cashdrugs_upstream_fetch_errors_total` | Counter | `slug` | Upstream fetch failures |
| `cashdrugs_upstream_fetch_pages_total` | Counter | `slug` | Pages fetched from upstream |
| `cashdrugs_mongodb_ping_duration_seconds` | Gauge | — | Last MongoDB ping latency |
| `cashdrugs_mongodb_up` | Gauge | — | MongoDB health (1/0) |
| `cashdrugs_mongodb_documents_total` | Gauge | `slug` | Document count per slug |
| `cashdrugs_scheduler_runs_total` | Counter | `slug`, `result` | Scheduler executions |
| `cashdrugs_scheduler_run_duration_seconds` | Histogram | `slug` | Scheduler job latency |
| `cashdrugs_fetchlock_dedup_total` | Counter | `slug` | Deduplicated fetches |

## 5. API Contract

### GET /metrics

**Response:** `200 OK`
**Content-Type:** `text/plain; version=0.0.4; charset=utf-8`
**Body:** Standard Prometheus exposition format

No changes to existing endpoints. The `/metrics` endpoint is additive.

## 6. Edge Cases

| Scenario | Expected Behavior |
|----------|-------------------|
| MongoDB down at startup | `cashdrugs_mongodb_up` = 0, ping latency not recorded |
| Upstream returns 5xx | `cashdrugs_upstream_fetch_errors_total` increments, duration still recorded |
| Slug with special characters | Labels are sanitized by Prometheus client library |
| Very high cardinality (many slugs) | Bounded by config — only configured slugs appear as label values |
| Concurrent metric writes | Prometheus client library is thread-safe |
| `/metrics` called before any requests | Metrics exist with zero values |

## 7. Dependencies

- `github.com/prometheus/client_golang` — Prometheus Go client library
- Existing `cache.MongoRepository` — needs methods for document counts and health checks
- Existing `fetchlock.Map` — needs dedup counting

## 8. Screenshot Checkpoints

N/A — backend metrics endpoint, no UI.

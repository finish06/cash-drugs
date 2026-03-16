# Spec: Connection Resilience

**Version:** 0.1.0
**Created:** 2026-03-14
**PRD Reference:** docs/prd.md (M9)
**Status:** Complete

## 1. Overview

Prevent service collapse under concurrent load by adding concurrency limits, HTTP server timeouts, and health check isolation. Currently the service refuses connections entirely once saturated (~50 concurrent requests), returning hard `Connection refused` errors. This spec adds graceful degradation: queued requests get `503 Service Unavailable` with `Retry-After` headers, server timeouts prevent goroutine leaks, and `/health` + `/metrics` remain responsive regardless of application traffic.

### User Story

As an **operator**, I want the service to degrade gracefully under load, so that health checks remain reliable, clients get actionable retry guidance, and the service recovers without manual intervention.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | A concurrency limiter middleware caps in-flight application requests to a configurable limit (default: 50) | Must |
| AC-002 | When the concurrency limit is reached, new requests receive `503 Service Unavailable` with a JSON error body | Must |
| AC-003 | The `503` response includes a `Retry-After` header with a value in seconds (default: 1) | Must |
| AC-004 | The 503 response body follows the existing `model.ErrorResponse` format: `{"error": "service overloaded", "retry_after": 1}` | Must |
| AC-005 | `/health` and `/metrics` are exempt from the concurrency limiter and always accept connections | Must |
| AC-006 | `http.Server` has `ReadTimeout: 10s`, `WriteTimeout: 30s`, `IdleTimeout: 60s` configured | Must |
| AC-007 | The concurrency limit is configurable via `max_concurrent_requests` field in `config.yaml` (under top-level `AppConfig`), with `MAX_CONCURRENT_REQUESTS` env var override taking precedence | Must |
| AC-008 | Prometheus gauge `cashdrugs_inflight_requests` tracks current in-flight request count | Should |
| AC-009 | Prometheus counter `cashdrugs_rejected_requests_total` counts requests rejected by the limiter | Should |
| AC-010 | Existing endpoints (`/api/cache/{slug}`, `/api/endpoints`, `/swagger/`, `/openapi.json`) continue to function identically when under the concurrency limit | Must |
| AC-011 | No regression in existing tests or functionality | Must |

## 3. User Test Cases

### TC-001: Requests succeed under the concurrency limit

**Precondition:** Service is running with default concurrency limit (50)
**Steps:**
1. Send 10 concurrent `GET /api/cache/drugnames` requests
2. Collect all responses
**Expected Result:** All 10 return `200 OK` with cached data
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-002: Requests rejected when over the concurrency limit

**Precondition:** Service is running with concurrency limit set to 2 (via `MAX_CONCURRENT_REQUESTS=2`)
**Steps:**
1. Send 5 concurrent requests to `/api/cache/drugnames` (with artificial delay or slow upstream)
2. Collect all responses
**Expected Result:** 2 requests return `200 OK`. Remaining requests return `503` with `{"error": "service overloaded", "retry_after": 1}` and `Retry-After: 1` header
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-003: Health check bypasses concurrency limit

**Precondition:** Service is running with concurrency limit set to 1. One slow request is in-flight.
**Steps:**
1. Start a slow request to `/api/cache/fda-enforcement` (force-refresh)
2. While it's in-flight, send `GET /health`
**Expected Result:** `/health` returns `200 OK` with `{"status": "ok", ...}` regardless of the in-flight limit being reached
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-004: Metrics endpoint bypasses concurrency limit

**Precondition:** Service is running with concurrency limit set to 1. One slow request is in-flight.
**Steps:**
1. Start a slow request
2. While it's in-flight, send `GET /metrics`
**Expected Result:** `/metrics` returns `200 OK` with Prometheus exposition format
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-005: Server timeouts prevent hung connections

**Precondition:** Service is running with configured timeouts
**Steps:**
1. Open a TCP connection and send a partial HTTP request
2. Wait 11 seconds without completing the request
**Expected Result:** Server closes the connection after `ReadTimeout` (10s)
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

## 4. Data Model

No new data entities. This spec adds middleware and server configuration only.

## 5. API Contract

### Existing endpoints — new error response

**503 Service Unavailable** (new response for all application endpoints):
```json
{
  "error": "service overloaded",
  "retry_after": 1
}
```

**Headers:**
- `Retry-After: 1`
- `Content-Type: application/json`

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| Concurrency limit set to 0 or negative | Use default (50), log a warning |
| Request finishes while others are queued | Next request is immediately admitted (no delay) |
| Slow client drains response past WriteTimeout | Connection closed, goroutine freed |
| `/swagger/` paths (sub-paths of swagger) | Subject to concurrency limit (not exempt) |
| Panic in handler | Concurrency semaphore is still released (defer) |

## 7. Dependencies

- `golang.org/x/sync/semaphore` or equivalent (or simple channel-based semaphore)
- Existing `internal/metrics` package for new gauges/counters

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-14 | 0.1.0 | calebdunn | Initial spec from /add:spec interview |

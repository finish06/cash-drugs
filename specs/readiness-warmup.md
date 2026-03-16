# Spec: Readiness Endpoint + Bulk Cache Warmup

**Version:** 0.1.0
**Created:** 2026-03-16
**PRD Reference:** docs/prd.md
**Status:** Draft

## 1. Overview

Add a readiness probe (`GET /ready`) that reports whether the service has finished warming its cache, and a manual warmup trigger (`POST /api/warmup`) that pre-fetches configured endpoints in the background. During startup, the service warms all scheduled endpoints before reporting ready. Downstream services (e.g. drug-gate) can poll `/ready` to wait intelligently instead of retrying on 502 for 60+ seconds while caches populate.

### User Story

As a **downstream service (drug-gate)**, I want to know when cash-drugs has finished warming its cache, so I can wait intelligently instead of retrying on 502 for 60+ seconds.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | `GET /ready` returns `503` with `{"status": "warming", "progress": "3/5"}` during startup cache warm-up | Must |
| AC-002 | `GET /ready` returns `200` with `{"status": "ready"}` after warm-up is complete | Must |
| AC-003 | `GET /ready` is exempt from the concurrency limiter middleware | Must |
| AC-004 | `POST /api/warmup` with no request body triggers warm-up for all scheduled endpoints and returns `202 Accepted` | Must |
| AC-005 | `POST /api/warmup` with `{"slugs": ["drugnames", "fda-enforcement"]}` warms only the specified slugs and returns `202 Accepted` | Must |
| AC-006 | `POST /api/warmup` with an invalid slug in the list returns `400 Bad Request` with `{"error": "unknown slug", "slug": "<invalid>"}` | Must |
| AC-007 | Warmup runs in the background — the `POST /api/warmup` endpoint returns immediately without blocking | Must |
| AC-008 | Warmup progress is trackable via `GET /ready` — progress updates as each endpoint completes | Must |
| AC-009 | Warmup deduplicates with the scheduler by acquiring fetchlock before fetching each endpoint | Must |
| AC-010 | Swag annotations are present on both `GET /ready` and `POST /api/warmup` endpoints | Should |
| AC-011 | On startup, all scheduled endpoints (those with a `refresh` field in config) are warmed before `/ready` returns 200 | Must |
| AC-012 | If a warmup fetch fails for one endpoint, remaining endpoints continue warming — partial failure does not block readiness | Must |

## 3. User Test Cases

### TC-001: Readiness during startup warm-up

**Precondition:** Service has just started, scheduled endpoints not yet cached
**Steps:**
1. Start the service
2. Immediately send `GET /ready`
3. Observe response
**Expected Result:** `503` with `{"status": "warming", "progress": "0/N"}` where N is the number of scheduled endpoints
**Maps to:** AC-001

### TC-002: Readiness after warm-up complete

**Precondition:** Service has completed startup warm-up
**Steps:**
1. Wait for startup warm-up to finish
2. Send `GET /ready`
**Expected Result:** `200` with `{"status": "ready"}`
**Maps to:** AC-002

### TC-003: Manual warmup — all endpoints

**Precondition:** Service is running and ready
**Steps:**
1. Send `POST /api/warmup` with no body
2. Observe response
3. Poll `GET /ready` until warming completes
**Expected Result:** `202 Accepted` returned immediately. `/ready` shows warming progress, then returns to ready.
**Maps to:** AC-004, AC-007, AC-008

### TC-004: Manual warmup — specific slugs

**Precondition:** Service is running and ready
**Steps:**
1. Send `POST /api/warmup` with body `{"slugs": ["drugnames", "fda-enforcement"]}`
2. Observe response
3. Poll `GET /ready`
**Expected Result:** `202 Accepted`. Only the two specified endpoints are warmed. Progress shows `0/2`, `1/2`, then ready.
**Maps to:** AC-005, AC-008

### TC-005: Manual warmup — invalid slug

**Precondition:** Service is running
**Steps:**
1. Send `POST /api/warmup` with body `{"slugs": ["nonexistent"]}`
**Expected Result:** `400 Bad Request` with `{"error": "unknown slug", "slug": "nonexistent"}`
**Maps to:** AC-006

### TC-006: Readiness exempt from concurrency limiter

**Precondition:** Service running with `MAX_CONCURRENT_REQUESTS=1`, one slow request in-flight
**Steps:**
1. Start a slow request to `/api/cache/fda-enforcement?_force=true`
2. While in-flight, send `GET /ready`
**Expected Result:** `/ready` returns normally (200 or 503 depending on warmup state), not blocked by limiter
**Maps to:** AC-003

### TC-007: Warmup deduplicates with scheduler

**Precondition:** Scheduler is actively fetching `drugnames`
**Steps:**
1. Trigger `POST /api/warmup` with `{"slugs": ["drugnames"]}`
2. Observe logs
**Expected Result:** Warmup skips `drugnames` fetch (fetchlock already held), does not duplicate the in-progress fetch
**Maps to:** AC-009

## 4. Data Model

No new persistent entities. Warmup state is held in-memory:

| Field | Type | Description |
|-------|------|-------------|
| `warming` | `bool` | Whether a warmup operation is in progress |
| `total` | `int` | Total number of endpoints to warm |
| `completed` | `int` (atomic) | Number of endpoints that have finished warming |

## 5. API Contract

### GET /ready

**Response (warming):**
```json
{
  "status": "warming",
  "progress": "3/5"
}
```
Status code: `503 Service Unavailable`

**Response (ready):**
```json
{
  "status": "ready"
}
```
Status code: `200 OK`

### POST /api/warmup

**Request body (all scheduled endpoints):**
Empty or omitted.

**Request body (specific slugs):**
```json
{
  "slugs": ["drugnames", "fda-enforcement"]
}
```

**Response (accepted):**
```json
{
  "status": "accepted",
  "warming": 5
}
```
Status code: `202 Accepted`
`warming` indicates how many endpoints will be warmed.

**Response (invalid slug):**
```json
{
  "error": "unknown slug",
  "slug": "nonexistent"
}
```
Status code: `400 Bad Request`

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| `POST /api/warmup` while already warming | Returns `202` and merges — already-completed slugs are skipped, new slugs added to the queue. Progress counter resets to reflect total. |
| `POST /api/warmup` for a non-existent slug | Returns `400` with the invalid slug identified. No warming starts. |
| Concurrent `POST /api/warmup` requests | Second request gets `202` but does not start a duplicate warmup — deduplication via fetchlock per slug. |
| Warmup fetch fails for one endpoint | Log the error, increment completed count, continue warming remaining endpoints. `/ready` eventually returns 200. |
| All warmup fetches fail | `/ready` returns `200` after all attempts complete — readiness means "warmup finished", not "all data cached". |
| Service restarted mid-warmup | Warmup restarts from scratch on the new startup. |

## 7. Dependencies

- `internal/handler` — new `ReadyHandler` and `WarmupHandler`
- `internal/upstream` — existing `Fetcher` for pre-fetching
- `internal/cache` — existing `Repository` for storing warmed data
- `internal/handler.FetchLocks` — deduplication with scheduler
- `internal/config` — reading scheduled endpoints from config
- Concurrency limiter middleware — must exempt `/ready`

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-16 | 0.1.0 | calebdunn | Initial spec |

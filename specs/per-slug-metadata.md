# Spec: Per-Slug Metadata Endpoint

**Version:** 0.1.0
**Created:** 2026-03-20
**PRD Reference:** docs/prd.md (M15)
**Status:** Draft

## 1. Overview

Consumers sometimes need to check cache state (freshness, record count, circuit breaker state) before fetching a potentially large payload. Currently the only way to get this information is `GET /api/cache/status` (returns all slugs) or fetching the full cached response. This feature adds a lightweight `GET /api/cache/{slug}/_meta` endpoint that returns cache metadata for a single slug without transferring the data payload.

### User Story

As an **internal microservice developer**, I want to check whether a slug's cache is fresh and how many records it contains before fetching the full dataset, so I can make intelligent decisions about when to fetch and whether to expect data.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | `GET /api/cache/{slug}/_meta` returns cache metadata as JSON without the data payload | Must |
| AC-002 | Response includes `last_refreshed` (ISO 8601 timestamp or null if never cached) | Must |
| AC-003 | Response includes `ttl_remaining` (duration string, "0s" if stale or no TTL) | Must |
| AC-004 | Response includes `is_stale` (boolean) | Must |
| AC-005 | Response includes `page_count` (int, from cached response metadata) | Must |
| AC-006 | Response includes `record_count` (int, total items across all pages) | Must |
| AC-007 | Response includes `circuit_state` (string: "closed", "open", "half-open") reflecting the upstream circuit breaker state for this slug | Must |
| AC-008 | Response includes `slug` (string, echo of the path parameter) | Must |
| AC-009 | Response includes `has_schedule` (bool) and `schedule` (cron expression or null) | Should |
| AC-010 | If slug is not configured, return 404 with standard error envelope | Must |
| AC-011 | If slug is configured but never cached, return 200 with null/zero values for cache fields and `circuit_state` from breaker | Must |
| AC-012 | Response time is under 10ms (metadata-only MongoDB query, no data field projection) | Should |
| AC-013 | Endpoint is subject to the concurrency limiter middleware | Must |
| AC-014 | Swagger/OpenAPI annotations are present | Should |
| AC-015 | Existing endpoints and tests are unaffected | Must |

## 3. User Test Cases

### TC-001: Metadata for cached slug

**Precondition:** `fda-ndc` has cached data fetched 2 hours ago, TTL is 24h, 3 pages, 150 records.
**Steps:**
1. `GET /api/cache/fda-ndc/_meta`
**Expected Result:** 200 OK with `last_refreshed` set, `is_stale: false`, `ttl_remaining` ~"22h", `page_count: 3`, `record_count: 150`, `circuit_state: "closed"`.
**Maps to:** AC-001, AC-002, AC-003, AC-004, AC-005, AC-006, AC-007

### TC-002: Metadata for never-cached slug

**Precondition:** `rxnorm-drugs` is configured but has never been fetched.
**Steps:**
1. `GET /api/cache/rxnorm-drugs/_meta`
**Expected Result:** 200 OK with `last_refreshed: null`, `is_stale: true`, `ttl_remaining: "0s"`, `page_count: 0`, `record_count: 0`.
**Maps to:** AC-011

### TC-003: Unknown slug

**Steps:**
1. `GET /api/cache/nonexistent/_meta`
**Expected Result:** 404 with `error_code: "CD-H001"`.
**Maps to:** AC-010

### TC-004: Circuit breaker open

**Precondition:** Circuit breaker for `fda-ndc` is in open state.
**Steps:**
1. `GET /api/cache/fda-ndc/_meta`
**Expected Result:** `circuit_state: "open"`.
**Maps to:** AC-007

### TC-005: Stale cache

**Precondition:** `fda-ndc` cached 48 hours ago, TTL is 24h.
**Steps:**
1. `GET /api/cache/fda-ndc/_meta`
**Expected Result:** `is_stale: true`, `ttl_remaining: "0s"`.
**Maps to:** AC-003, AC-004

## 4. Data Model

### Response Body

```go
// SlugMeta is the response for GET /api/cache/{slug}/_meta.
type SlugMeta struct {
    Slug          string  `json:"slug"`
    LastRefreshed *string `json:"last_refreshed"` // ISO 8601 or null
    TTLRemaining  string  `json:"ttl_remaining"`
    IsStale       bool    `json:"is_stale"`
    PageCount     int     `json:"page_count"`
    RecordCount   int     `json:"record_count"`
    CircuitState  string  `json:"circuit_state"` // "closed", "open", "half-open"
    HasSchedule   bool    `json:"has_schedule"`
    Schedule      *string `json:"schedule,omitempty"` // cron expression or null
}
```

### Example Response

```json
{
  "slug": "fda-ndc",
  "last_refreshed": "2026-03-20T08:30:00Z",
  "ttl_remaining": "21h30m0s",
  "is_stale": false,
  "page_count": 3,
  "record_count": 150,
  "circuit_state": "closed",
  "has_schedule": true,
  "schedule": "0 0 */6 * * *"
}
```

## 5. API Contract

### `GET /api/cache/{slug}/_meta`

| Field | Value |
|-------|-------|
| Method | GET |
| Path | `/api/cache/{slug}/_meta` |
| Success | 200 `SlugMeta` |
| Not Found | 404 `ErrorResponse` (slug not configured) |
| Overloaded | 503 `ErrorResponse` (concurrency limiter) |

### MongoDB Query Strategy

To keep this fast, the handler should:
1. Use `repo.FetchedAt(slug)` for the timestamp (existing method, metadata-only)
2. For `page_count` and `record_count`, query MongoDB with a projection that excludes the `data` field: `db.cache.find({base_key: slug}, {data: 0})`
3. Alternatively, add a `MetadataOnly(slug string) (*CacheMetadata, error)` method to the Repository interface that returns count/page info without data

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| Slug has parameterized cache entries only (no base key) | Return metadata from the most recent parameterized entry, or zeros if none |
| Circuit breaker registry has no entry for slug | Return `circuit_state: "closed"` (default state) |
| MongoDB connection error | Return 500 with error envelope; `circuit_state` still available from in-memory breaker |
| Concurrent `_meta` and data requests | No interference; `_meta` is read-only |
| `_meta` literal as a slug name | Handled by route matching — `_meta` suffix is stripped before slug lookup. If someone configures a slug literally named `_meta`, it won't be reachable via this endpoint (documented limitation) |

## 7. Dependencies

- `internal/handler` — new `MetaHandler` struct (or extend `CacheHandler` with route detection)
- `internal/cache` — may need `Repository` extension for metadata-only queries
- `internal/upstream` — `CircuitRegistry.State(slug)` (existing method)
- `internal/config` — `Endpoint` struct for TTL, schedule
- No new external dependencies

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-20 | 0.1.0 | calebdunn | Initial spec |

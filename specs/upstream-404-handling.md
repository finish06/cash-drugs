# Spec: Upstream 404 Handling

**Version:** 1.0.0
**Created:** 2026-03-07
**PRD Reference:** docs/prd.md
**Status:** Approved

## 1. Overview

When an upstream API returns HTTP 404, cash-drugs currently treats it as an upstream failure and returns 502 to consumers. This makes it impossible for consumers to distinguish "data not found" from "upstream is down." Cash-drugs should detect upstream 404 responses and return 404 to consumers with context about what was searched. Not-found results are cached for 10 minutes to avoid repeatedly hitting the upstream API for the same non-existent resource.

### User Story

As an internal microservice developer, I want cash-drugs to return 404 when the upstream API says a resource doesn't exist, so that my service can distinguish "not found" from "upstream down" and respond appropriately to end users.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | When upstream returns HTTP 404, cash-drugs returns 404 to the consumer (not 502) | Must |
| AC-002 | The 404 response body includes `error`, `slug`, and `params` fields for debuggability | Must |
| AC-003 | Only upstream HTTP 404 is treated as "not found". Other 4xx (401, 403, 429) continue to be treated as upstream failure (502) | Must |
| AC-004 | Upstream 5xx and network errors continue to return 502 with stale cache fallback (existing behavior unchanged) | Must |
| AC-005 | When upstream returns 404, cash-drugs returns 404 even if stale cache exists for that key (not-found overrides stale) | Must |
| AC-006 | Not-found results are cached with a 10-minute TTL to avoid re-hitting upstream for the same non-existent resource | Must |
| AC-007 | Subsequent requests for a negatively-cached key return 404 from cache without hitting upstream, until the 10-minute TTL expires | Must |
| AC-008 | After the negative cache TTL expires, the next request re-checks upstream (which may now return data or 404 again) | Must |
| AC-009 | Existing endpoints that never return 404 (e.g., DailyMed paginated fetches) are unaffected | Must |
| AC-010 | The negative cache TTL is not configurable per-endpoint in v1 (hardcoded 10 minutes) | Should |

## 3. User Test Cases

### TC-001: Upstream 404 returns 404 to consumer

**Precondition:** Server running, upstream API returns 404 for a non-existent NDC
**Steps:**
1. Request `GET /api/cache/fda-ndc?BRAND_NAME=NonexistentDrug12345`
2. Upstream FDA API returns HTTP 404
3. Cash-drugs processes the response
**Expected Result:** Consumer receives HTTP 404 with body `{"error": "not found", "slug": "fda-ndc", "params": {"BRAND_NAME": "NonexistentDrug12345"}}`
**Screenshot Checkpoint:** N/A (API only)
**Maps to:** TBD

### TC-002: Upstream 500 still returns 502

**Precondition:** Server running, upstream API returns 500
**Steps:**
1. Request `GET /api/cache/fda-ndc?BRAND_NAME=Tylenol`
2. Upstream API returns HTTP 500
**Expected Result:** Consumer receives HTTP 502 with body `{"error": "upstream unavailable", "slug": "fda-ndc"}`
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-003: Upstream 404 overrides stale cache

**Precondition:** Server running, cached data exists for a key, upstream now returns 404
**Steps:**
1. Seed cache with data for `fda-ndc:BRAND_NAME=OldDrug`
2. Request `GET /api/cache/fda-ndc?BRAND_NAME=OldDrug&_force=true`
3. Upstream returns 404
**Expected Result:** Consumer receives 404 (not stale cached data)
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-004: Negative cache prevents repeated upstream calls

**Precondition:** Server running, upstream returns 404 for a key
**Steps:**
1. Request `GET /api/cache/fda-ndc?BRAND_NAME=FakeDrug` — upstream returns 404
2. Immediately request `GET /api/cache/fda-ndc?BRAND_NAME=FakeDrug` again
**Expected Result:** Second request returns 404 from negative cache without hitting upstream
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-005: Negative cache expires after 10 minutes

**Precondition:** Server running, negative cache entry exists
**Steps:**
1. Request that results in upstream 404 (negatively cached)
2. Wait for negative cache TTL to expire (or simulate with time manipulation)
3. Request same key again — upstream now returns data
**Expected Result:** Third request hits upstream and returns fresh data (200)
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-006: Upstream 429 still treated as failure (502)

**Precondition:** Server running, upstream returns 429 (rate limited)
**Steps:**
1. Request endpoint where upstream returns HTTP 429
**Expected Result:** Consumer receives 502 (upstream unavailable), not 404
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

## 4. Data Model

### Negative Cache Entry

Not-found results are stored in the same MongoDB collection as regular cached responses, with a sentinel value indicating "not found."

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| cache_key | string | Yes | Same key format as regular cache entries |
| slug | string | Yes | Endpoint slug |
| params | map | No | Query parameters that produced the 404 |
| not_found | bool | Yes | `true` to mark this as a negative cache entry |
| fetched_at | time | Yes | When the 404 was received from upstream |
| created_at | time | Yes | Document creation time |
| updated_at | time | Yes | Last update time |

### Relationships

Negative cache entries share the `cache_key` namespace with regular entries. A negative cache entry for a key replaces any existing cached data for that key (since upstream says it no longer exists).

## 5. API Contract

### GET /api/cache/{slug} (modified behavior)

**Description:** Returns cached upstream API data. Now returns 404 when upstream says resource doesn't exist.

**Response (404) — new:**
```json
{
  "error": "not found",
  "slug": "fda-ndc",
  "params": {
    "BRAND_NAME": "NonexistentDrug"
  }
}
```

**Response (502) — unchanged:**
```json
{
  "error": "upstream unavailable",
  "slug": "fda-ndc"
}
```

**Error Response changes:**
- `404` — Upstream API confirmed the resource does not exist
- `502` — Upstream API is unreachable, returned 5xx, or returned non-404 4xx error

## 6. UI Behavior

N/A — no UI component.

## 7. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| Upstream returns 404 with no JSON body | Still treated as 404 — response body parsing is best-effort |
| Upstream returns 404 for a paginated fetch (mid-pagination) | Stop pagination, return whatever was fetched so far (existing partial-data behavior for offset pagination) |
| Negative cache exists but regular cache also exists for same key | Negative cache takes precedence (not-found overrides stale data). Existing cached data is NOT deleted — both entries coexist for safer rollback. |
| Upstream returns 404, then later returns 200 for same key | After 10-min negative TTL expires, next request gets fresh 200 data |
| Network timeout (no HTTP response at all) | Treated as upstream failure (502), not 404 — no HTTP status to inspect |
| Upstream returns 404 during scheduled refresh | Log warning + increment `upstream_404_total` Prometheus counter per slug. Do not store negative cache (scheduled refreshes are for bulk prefetch, not individual lookups). |
| Concurrent requests for same non-existent key | First request caches the 404, subsequent requests served from negative cache |

## 8. Design Decisions (from spec review)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| 404 vs stale cache precedence | Negative entry takes precedence, existing data NOT deleted | Safer rollback if upstream 404 was transient — data can be recovered after negative TTL expires |
| Observability | Log warning + `upstream_404_total` Prometheus counter per slug | Enables Grafana alerting on sudden upstream 404 spikes |
| Cache layers for negative entries | Both LRU + MongoDB | LRU serves repeated 404s at sub-ms latency; MongoDB persists across restarts |
| Negative cache TTL | Hardcoded 10 minutes (AC-010) | Simple for v1; can be made configurable per-endpoint later |

## 9. Dependencies

- `internal/upstream/fetcher.go` — must propagate upstream HTTP status code (currently discards 4xx as generic error)
- `internal/handler/cache.go` — must distinguish 404 from other errors
- `internal/cache/mongo.go` — must support storing/retrieving negative cache entries
- `internal/model/response.go` — may need `NotFound` field on `CachedResponse`

## 10. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-07 | 0.1.0 | calebdunn | Initial spec from /add:spec interview |
| 2026-03-17 | 1.0.0 | calebdunn + agent | Spec review: added design decisions (precedence, metrics, LRU+Mongo, edge case clarifications). Status → Approved. |

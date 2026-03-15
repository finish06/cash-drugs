# Spec: Empty Upstream Results

**Version:** 0.1.0
**Created:** 2026-03-15
**PRD Reference:** docs/prd.md (M10)
**Status:** Draft

## 1. Overview

Return HTTP 200 with an empty data array when an upstream API returns valid but empty results, instead of treating it as an error that produces a 502. Currently, when an upstream JSON endpoint returns a valid 200 response but the `data_key` array is empty (e.g., an NDC lookup for a non-existent drug code), the fetcher returns a `CachedResponse` with an empty `Data` slice. Downstream in the handler, this can flow through to an error path or confuse consumers who cannot distinguish "upstream failed" (502) from "no matching records" (which should be 200 with empty data). This was identified during v0.7.1 stress testing where `fda-ndc?NDC={random}` showed 0% success despite the upstream working correctly. This spec also adds a `results_count` field to the response meta and caches empty results with a short TTL to prevent hammering the upstream for known-empty queries.

### User Story

As a **consumer of the cash-drugs API**, I want to receive a clear 200 response with `{"data": [], "meta": {"results_count": 0}}` when a query has no matches, so that I can distinguish "no results found" from "upstream is broken" without inspecting HTTP status codes for ambiguity.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | When upstream returns HTTP 200 with an empty data array (the `data_key` array has 0 items), `Fetch()` returns a valid `CachedResponse` with `Data` set to an empty `[]interface{}{}` (not nil) | Must |
| AC-002 | The handler returns HTTP 200 with `{"data": [], "meta": {"slug": "...", "results_count": 0, "stale": false}}` for empty upstream results | Must |
| AC-003 | A new `ResultsCount` field is added to `model.ResponseMeta` (`json:"results_count"`) containing the number of items in the data array | Must |
| AC-004 | `ResultsCount` is populated for all responses (empty and non-empty) by counting the data array length | Must |
| AC-005 | Empty results are cached in MongoDB with the same upsert flow as non-empty results | Must |
| AC-006 | Empty results cached in the LRU use a short TTL of 2 minutes (configurable) to allow re-checking upstream without excessive hammering | Must |
| AC-007 | Upstream errors (network failures, HTTP 4xx/5xx) continue to return 502 with `model.ErrorResponse` — only valid 200 responses with empty data produce the new behavior | Must |
| AC-008 | Existing non-empty response behavior is completely unchanged — `ResultsCount` is the only addition to the meta envelope | Must |
| AC-009 | The `stale` field in meta is set correctly for empty cached results (follows same TTL staleness logic as non-empty) | Must |
| AC-010 | Raw/XML format endpoints are unaffected by this change (they don't use the JSON data array pattern) | Must |
| AC-011 | Prometheus metrics correctly count empty-result responses as HTTP 200 (not errors) | Must |
| AC-012 | Cache outcome metric labels empty-result cache hits as `"hit"` (not `"miss"` or `"error"`) | Must |
| AC-013 | All existing tests pass without modification | Must |

## 3. User Test Cases

### TC-001: Empty upstream results return 200

**Precondition:** Upstream API for `fda-ndc` returns `{"results": [], "meta": {"total": 0}}` for `NDC=99999999`
**Steps:**
1. Send `GET /api/cache/fda-ndc?NDC=99999999`
**Expected Result:** HTTP 200 with body:
```json
{
  "data": [],
  "meta": {
    "slug": "fda-ndc",
    "source_url": "...",
    "fetched_at": "...",
    "page_count": 1,
    "results_count": 0,
    "stale": false
  }
}
```
**Maps to:** AC-001, AC-002, AC-003

### TC-002: Non-empty results include results_count

**Precondition:** Upstream API for `drugnames` returns 150 items across 2 pages
**Steps:**
1. Send `GET /api/cache/drugnames`
**Expected Result:** HTTP 200 with `"results_count": 150` in meta alongside existing fields.
**Maps to:** AC-003, AC-004, AC-008

### TC-003: Upstream error still returns 502

**Precondition:** Upstream API for `fda-ndc` is unreachable (network timeout)
**Steps:**
1. Send `GET /api/cache/fda-ndc?NDC=12345` with no cached data
**Expected Result:** HTTP 502 with `{"error": "upstream unavailable", "slug": "fda-ndc"}`
**Maps to:** AC-007

### TC-004: Empty results are cached and served from cache

**Precondition:** First request for `fda-ndc?NDC=99999999` has been made and cached
**Steps:**
1. Send `GET /api/cache/fda-ndc?NDC=99999999` again within 2 minutes
**Expected Result:** HTTP 200 with empty data, served from cache (no upstream fetch). Cache outcome metric shows `"hit"`.
**Maps to:** AC-005, AC-012

### TC-005: Empty cached results become stale after short TTL

**Precondition:** Empty result for `fda-ndc?NDC=99999999` was cached 3 minutes ago. LRU TTL for empty results is 2 minutes.
**Steps:**
1. Send `GET /api/cache/fda-ndc?NDC=99999999`
**Expected Result:** LRU cache misses (expired). MongoDB cache hit may show stale. Background revalidation triggered if TTL-based staleness applies.
**Maps to:** AC-006, AC-009

## 4. Data Model

### Modified: `ResponseMeta` (`internal/model/response.go`)

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| `ResultsCount` | `int` | `results_count` | Number of items in the data array. 0 for empty results. |

No changes to `CachedResponse` — empty results are already representable as `Data: []interface{}{}`.

## 5. API Contract

### Modified response envelope

**200 OK** (all JSON responses now include `results_count`):
```json
{
  "data": [...],
  "meta": {
    "slug": "fda-ndc",
    "source_url": "https://api.fda.gov/...",
    "fetched_at": "2026-03-15T10:30:00Z",
    "page_count": 1,
    "results_count": 42,
    "stale": false
  }
}
```

**200 OK** (empty results):
```json
{
  "data": [],
  "meta": {
    "slug": "fda-ndc",
    "source_url": "https://api.fda.gov/...",
    "fetched_at": "2026-03-15T10:30:00Z",
    "page_count": 1,
    "results_count": 0,
    "stale": false
  }
}
```

**502 Bad Gateway** (upstream error — unchanged):
```json
{
  "error": "upstream unavailable",
  "slug": "fda-ndc"
}
```

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| Upstream returns 200 with `data_key` missing entirely | Existing behavior: fetcher wraps entire parsed response as single item in array. `results_count: 1`. |
| Upstream returns 200 with `data_key` set to `null` | Treat as empty: `Data = []interface{}{}`, `results_count: 0` |
| Upstream returns 200 with `data_key` as a non-array scalar | Existing behavior: wraps as `[]interface{}{scalar}`. `results_count: 1`. |
| Multi-page fetch where some pages are empty and some have data | Combine all pages normally. `results_count` = total items across all pages. |
| Empty result cached, then upstream starts returning data | Next fetch (on TTL expiry or force-refresh) replaces the empty cache with real data. |
| `results_count` for raw/XML responses | Not included — raw/XML responses bypass the JSON envelope entirely. |
| Force-refresh on an empty cached result | Fetches from upstream normally. If still empty, re-caches with updated timestamp. |

## 7. Dependencies

- `internal/model/response.go` — add `ResultsCount` to `ResponseMeta`
- `internal/handler/cache.go` — populate `ResultsCount` in `respondWithCached()`, handle empty data as valid 200
- `internal/upstream/fetcher.go` — ensure empty data arrays are returned as `[]interface{}{}` not nil
- `internal/cache/lru.go` — no changes (TTL is passed by caller)
- `internal/handler/cache.go` — use shorter LRU TTL for empty results

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-15 | 0.1.0 | calebdunn | Initial spec for M10 |

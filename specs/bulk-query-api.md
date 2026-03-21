# Spec: Bulk Query API

**Version:** 0.1.0
**Created:** 2026-03-20
**PRD Reference:** docs/prd.md (M15)
**Status:** Draft

## 1. Overview

Consumers often need data for multiple query parameter sets within the same slug (e.g., look up 50 NDCs in one call). Today each lookup requires a separate HTTP request, adding round-trip overhead and complicating client code. This feature adds a `POST /api/cache/{slug}/bulk` endpoint that accepts a batch of queries and returns per-query results with concurrent cache lookups.

### User Story

As an **internal microservice developer**, I want to send a batch of queries to a single endpoint in one HTTP call, so I can reduce round-trip latency and simplify my client integration code.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | `POST /api/cache/{slug}/bulk` accepts a JSON body with `{"queries": [{"params": {...}}, ...]}` | Must |
| AC-002 | Each query in the batch is looked up concurrently in the cache, capped at 10 concurrent goroutines via a semaphore | Must |
| AC-003 | The response contains a `results` array with one entry per query, preserving input order | Must |
| AC-004 | Each result entry includes `status` ("hit", "miss", "error"), `params` (echo of input), `data` (cached payload or null), and `meta` (ResponseMeta or null) | Must |
| AC-005 | Partial success is supported — a failed lookup for one query does not fail the entire batch | Must |
| AC-006 | Batch size is limited to 100 queries. Requests exceeding the limit receive 400 with a clear error message | Must |
| AC-007 | An empty `queries` array returns 200 with an empty `results` array | Must |
| AC-008 | Requests with missing or malformed JSON body return 400 | Must |
| AC-009 | If `slug` is not configured, return 404 (same as GET handler) | Must |
| AC-010 | Prometheus metrics are recorded: `cashdrugs_bulk_request_size` histogram (batch size), `cashdrugs_bulk_request_duration_seconds` histogram (total request time) | Must |
| AC-011 | Each individual cache lookup within the batch records standard `cashdrugs_cache_hits_total` / `cashdrugs_cache_misses_total` metrics | Should |
| AC-012 | Bulk endpoint is subject to the existing concurrency limiter middleware | Must |
| AC-013 | Swagger/OpenAPI annotations are present on the handler | Should |
| AC-014 | `X-Request-ID` from the request context is included in error responses | Must |
| AC-015 | Existing endpoints and tests are unaffected | Must |

## 3. User Test Cases

### TC-001: Successful bulk lookup with all cache hits

**Precondition:** `fda-ndc` slug is configured. Cache contains data for `BRAND_NAME=Tylenol` and `BRAND_NAME=Advil`.
**Steps:**
1. `POST /api/cache/fda-ndc/bulk` with body `{"queries": [{"params": {"BRAND_NAME": "Tylenol"}}, {"params": {"BRAND_NAME": "Advil"}}]}`
2. Inspect response
**Expected Result:** 200 OK. `results` has 2 entries, both with `status: "hit"`, non-null `data` and `meta`. Order matches input.
**Maps to:** AC-001, AC-003, AC-004

### TC-002: Mixed hit and miss

**Precondition:** `fda-ndc` slug configured. Cache has `BRAND_NAME=Tylenol` but not `BRAND_NAME=ZZZNotCached`.
**Steps:**
1. `POST /api/cache/fda-ndc/bulk` with both queries
**Expected Result:** 200 OK. First result `status: "hit"`, second `status: "miss"` with null `data`.
**Maps to:** AC-004, AC-005

### TC-003: Batch size exceeds limit

**Steps:**
1. `POST /api/cache/fda-ndc/bulk` with 101 queries
**Expected Result:** 400 Bad Request with error message indicating batch limit of 100.
**Maps to:** AC-006

### TC-004: Empty queries array

**Steps:**
1. `POST /api/cache/fda-ndc/bulk` with `{"queries": []}`
**Expected Result:** 200 OK with `{"results": [], "total": 0, "hits": 0, "misses": 0}`.
**Maps to:** AC-007

### TC-005: Malformed JSON body

**Steps:**
1. `POST /api/cache/fda-ndc/bulk` with `{invalid json`
**Expected Result:** 400 Bad Request.
**Maps to:** AC-008

### TC-006: Unknown slug

**Steps:**
1. `POST /api/cache/nonexistent/bulk` with valid body
**Expected Result:** 404 Not Found with `error_code: "CD-H001"`.
**Maps to:** AC-009

### TC-007: Concurrent goroutine cap

**Precondition:** 20 queries submitted.
**Steps:**
1. `POST /api/cache/fda-ndc/bulk` with 20 queries
2. Observe that no more than 10 goroutines execute concurrently (test via atomic counter or semaphore instrumentation)
**Expected Result:** Maximum 10 concurrent lookups.
**Maps to:** AC-002

## 4. Data Model

### Request Body

```json
{
  "queries": [
    {"params": {"BRAND_NAME": "Tylenol"}},
    {"params": {"NDC": "12345-6789"}},
    {"params": {"BRAND_NAME": "Advil", "GENERIC_NAME": "Ibuprofen"}}
  ]
}
```

### Response Body

```json
{
  "results": [
    {
      "index": 0,
      "status": "hit",
      "params": {"BRAND_NAME": "Tylenol"},
      "data": [...],
      "meta": {
        "slug": "fda-ndc",
        "source_url": "...",
        "fetched_at": "...",
        "page_count": 1,
        "results_count": 42,
        "stale": false
      }
    },
    {
      "index": 1,
      "status": "miss",
      "params": {"NDC": "12345-6789"},
      "data": null,
      "meta": null
    }
  ],
  "total": 2,
  "hits": 1,
  "misses": 1,
  "errors": 0,
  "duration_ms": 12
}
```

### New Types

```go
// BulkQueryRequest is the request body for POST /api/cache/{slug}/bulk.
type BulkQueryRequest struct {
    Queries []BulkQueryItem `json:"queries"`
}

type BulkQueryItem struct {
    Params map[string]string `json:"params"`
}

// BulkQueryResponse is the response envelope.
type BulkQueryResponse struct {
    Results    []BulkQueryResult `json:"results"`
    Total      int               `json:"total"`
    Hits       int               `json:"hits"`
    Misses     int               `json:"misses"`
    Errors     int               `json:"errors"`
    DurationMs int64             `json:"duration_ms"`
}

type BulkQueryResult struct {
    Index  int               `json:"index"`
    Status string            `json:"status"` // "hit", "miss", "error"
    Params map[string]string `json:"params"`
    Data   interface{}       `json:"data"`
    Meta   *ResponseMeta     `json:"meta"`
    Error  string            `json:"error,omitempty"`
}
```

## 5. API Contract

### `POST /api/cache/{slug}/bulk`

| Field | Value |
|-------|-------|
| Method | POST |
| Path | `/api/cache/{slug}/bulk` |
| Content-Type | `application/json` |
| Request Body | `BulkQueryRequest` |
| Success | 200 `BulkQueryResponse` |
| Bad Request | 400 `ErrorResponse` (malformed JSON or batch limit exceeded) |
| Not Found | 404 `ErrorResponse` (slug not configured) |
| Overloaded | 503 `ErrorResponse` (concurrency limiter) |

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| Query with empty params `{"params": {}}` | Looks up the base slug key (no params) — valid |
| Duplicate queries in batch | Each processed independently; both get results |
| All queries miss | 200 with all `status: "miss"`, `hits: 0` |
| MongoDB connection error during lookup | Affected queries get `status: "error"`, others unaffected |
| Request body exceeds reasonable size (>1MB) | HTTP server's default body size limit applies |
| Concurrent bulk requests | Each bulk request uses its own goroutine pool; concurrency limiter governs overall |
| Slug has path params (e.g., `{RXCUI}`) | Params from bulk query used as path params, same as GET handler logic |

## 7. Dependencies

- `internal/handler` — new `BulkHandler` struct
- `internal/cache` — existing `Repository.Get()` and `BuildCacheKey()`
- `internal/model` — new bulk request/response types
- `internal/metrics` — new histogram metrics
- `golang.org/x/sync/semaphore` — or channel-based semaphore for goroutine cap
- No new external dependencies beyond what's already in go.mod

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-20 | 0.1.0 | calebdunn | Initial spec |

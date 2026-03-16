# Spec: Parallel Page Fetches

**Version:** 0.1.0
**Created:** 2026-03-15
**PRD Reference:** docs/prd.md (M10)
**Status:** Complete

## 1. Overview

Fetch upstream API pages concurrently instead of sequentially to reduce multi-page fetch latency. Currently, `internal/upstream/fetcher.go` loops through pages one at a time in `fetchJSON()` — for an endpoint returning 20 pages, total fetch time is `20 * per-page-latency`. This spec modifies the fetch flow: the first page is always fetched sequentially (to discover `total_pages` from the response metadata), then remaining pages are fetched concurrently with a configurable concurrency cap (default 3). Results are combined in page order. This reduces multi-page fetch time from `N * latency` to approximately `latency + ceil((N-1) / concurrency) * latency`.

### User Story

As an **operator**, I want multi-page upstream fetches to run in parallel, so that endpoints with many pages (e.g., `drugnames` with 20+ pages) complete in seconds instead of minutes and scheduled refreshes finish within their cron window.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | The first page of a paginated fetch is always fetched sequentially to discover the total page count from the response metadata | Must |
| AC-002 | After the first page, remaining pages are fetched concurrently using a worker pool or semaphore pattern | Must |
| AC-003 | The maximum number of concurrent page fetches is configurable via a `FetchConcurrency` field on `HTTPFetcher` (default: 3) | Must |
| AC-004 | Results from all pages are combined into the `CachedResponse.Pages` slice in page-number order regardless of fetch completion order | Must |
| AC-005 | If any page fetch fails, the entire `Fetch()` call returns an error (no partial results for non-offset pagination) | Must |
| AC-006 | For offset-style pagination, partial results are still returned on error (existing behavior preserved) — only successfully fetched pages are included | Must |
| AC-007 | Single-page endpoints (`pagination: nil` or `pagination: 1`) are unaffected — no goroutines spawned | Must |
| AC-008 | Raw/XML format endpoints (`ep.Format == "xml"` or `"raw"`) are unaffected — `fetchRaw()` is not modified | Must |
| AC-009 | The concurrency cap is configurable via `FETCH_CONCURRENCY` env var (overrides default) and `fetch_concurrency` field in `config.yaml` | Should |
| AC-010 | Prometheus metric `cashdrugs_upstream_fetch_pages_total` continues to accurately count total pages fetched per slug | Must |
| AC-011 | No goroutine leaks — all spawned goroutines complete or are cleaned up even when errors occur | Must |
| AC-012 | Existing unit tests for `fetchJSON` and `Fetch` pass without modification | Must |
| AC-013 | A test demonstrates that fetching 6 pages with concurrency=3 completes in approximately 2x single-page latency (not 6x) | Should |

## 3. User Test Cases

### TC-001: Multi-page fetch runs concurrently

**Precondition:** Upstream API returns `total_pages: 6` in first-page response metadata. Each page takes ~100ms.
**Steps:**
1. Call `Fetch(ep, params)` where `ep.Pagination = "all"`
2. Measure total fetch duration
**Expected Result:** Total duration is ~300ms (1 sequential + 2 batches of 3 concurrent), not ~600ms (6 sequential). Response contains all 6 pages in order.
**Maps to:** AC-001, AC-002, AC-004

### TC-002: Error on any page fails the entire fetch

**Precondition:** Upstream API returns `total_pages: 4`. Page 3 returns HTTP 500.
**Steps:**
1. Call `Fetch(ep, params)` with page-style pagination
**Expected Result:** `Fetch` returns an error. No partial `CachedResponse` is returned.
**Maps to:** AC-005

### TC-003: Offset pagination returns partial on error

**Precondition:** Upstream API uses offset pagination. Pages 1-3 succeed, page 4 fails.
**Steps:**
1. Call `Fetch(ep, params)` with `ep.PaginationStyle = "offset"`
**Expected Result:** `Fetch` returns a `CachedResponse` containing data from pages 1-3. Log warns about partial data.
**Maps to:** AC-006

### TC-004: Single-page endpoint is unaffected

**Precondition:** Endpoint has `pagination: nil` (default: 1 page)
**Steps:**
1. Call `Fetch(ep, params)`
**Expected Result:** Fetches one page sequentially. No goroutines spawned. Returns normally.
**Maps to:** AC-007

### TC-005: Concurrency cap is respected

**Precondition:** Upstream returns `total_pages: 10`. Concurrency cap is 3.
**Steps:**
1. Call `Fetch(ep, params)` with the mock tracking concurrent in-flight requests
**Expected Result:** At no point are more than 3 page requests in-flight simultaneously (excluding the sequential first page).
**Maps to:** AC-003

## 4. Data Model

No data model changes. `CachedResponse.Pages` is already a `[]PageData` slice populated by the fetcher.

### Modified Types

```go
type HTTPFetcher struct {
    Client          *http.Client
    FetchConcurrency int  // max concurrent page fetches (default: 3)
}
```

## 5. API Contract

No API changes. This is an internal optimization to the upstream fetch layer.

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| `total_pages = 1` (only first page) | No concurrent fetching. Return single page result immediately. |
| `total_pages = 0` from metadata | Treat as single page (already fetched). Return first page data. |
| Concurrency cap > remaining pages (e.g., cap=3, 2 pages remaining) | Only 2 goroutines spawned. No issue. |
| Upstream rate-limits at concurrency=3 | Operator reduces cap via env var. Default of 3 is conservative. |
| Context cancellation during parallel fetch | All in-flight goroutines should respect HTTP client timeout (30s). Error propagated. |
| `fetchAll = true` with unknown total pages | First page determines total. If total cannot be determined (`hasMorePages` returns false), only first page is returned. |
| `maxPages` set to specific number (e.g., 5) but upstream has 20 pages | Fetch at most 5 pages (1 sequential + 4 concurrent). Existing cap logic unchanged. |

## 7. Dependencies

- `internal/upstream/fetcher.go` — modify `fetchJSON()` to use concurrent fetching after first page
- `sync` — for `WaitGroup` (Go stdlib)
- `golang.org/x/sync/semaphore` or channel-based semaphore — for concurrency limiting
- `internal/config/loader.go` — add `FetchConcurrency` to `AppConfig` (optional)
- `cmd/server/main.go` — wire `FetchConcurrency` config to `HTTPFetcher`

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-15 | 0.1.0 | calebdunn | Initial spec for M10 |

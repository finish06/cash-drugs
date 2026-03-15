# Implementation Plan: Parallel Page Fetches

**Spec Version:** 0.1.0
**Spec:** specs/parallel-page-fetches.md
**Created:** 2026-03-15
**Team Size:** Solo (1 agent)
**Estimated Duration:** 3.5 hours

## Overview

Refactor the upstream page-fetching loop to fetch page 1 sequentially (to discover `total_pages`), then fetch remaining pages concurrently via a goroutine pool with a channel-based semaphore (default cap: 3). Results are reassembled in page order. Per-page errors are handled differently based on pagination style: page-style fails the entire fetch, offset-style returns partial results.

## Objectives

- Fetch first page sequentially to discover total page count
- Fetch remaining pages concurrently with configurable concurrency cap
- Combine results in page order regardless of completion order
- Fail entire fetch on any page error (page-style pagination)
- Return partial results on error (offset-style pagination)
- No goroutine leaks on error or context cancellation
- Single-page and raw/XML endpoints unaffected

## Acceptance Criteria Analysis

### AC-001: Sequential first page (Must)
- **Complexity:** Simple
- **Effort:** 15min
- **Tasks:** TASK-001, TASK-005
- **Approach:** Existing behavior for page 1. Extract total_pages from response metadata before spawning parallel fetches.

### AC-002: Concurrent remaining pages (Must)
- **Complexity:** Medium
- **Effort:** 1h
- **Tasks:** TASK-001, TASK-005
- **Approach:** After page 1, spawn goroutines for pages 2..N. Use a buffered channel as semaphore to cap concurrency. Each goroutine acquires a semaphore slot, fetches its page, stores result in a pre-allocated slice at its page index, releases slot.

### AC-003: Configurable concurrency (Must)
- **Complexity:** Simple
- **Effort:** 15min
- **Tasks:** TASK-002, TASK-006
- **Approach:** Add `FetchConcurrency int` field to `HTTPFetcher`. Default to 3. Read from config/env.

### AC-004: Page-order reassembly (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-001, TASK-005
- **Approach:** Pre-allocate `results := make([]PageResult, totalPages)`. Each goroutine writes to `results[pageNum-1]`. No sorting needed.

### AC-005: Fail-all on page error (page-style) (Must)
- **Complexity:** Medium
- **Effort:** 20min
- **Tasks:** TASK-003, TASK-005
- **Approach:** Use `errgroup` or collect first error via atomic/channel. If any page fails and pagination style is page-based, return error immediately (or after WaitGroup completes to avoid goroutine leaks).

### AC-006: Partial results on error (offset-style) (Must)
- **Complexity:** Medium
- **Effort:** 20min
- **Tasks:** TASK-003, TASK-005
- **Approach:** For offset pagination, collect successful pages and log warning for failed ones. Return partial results.

### AC-007, AC-008: Single-page and raw/XML unaffected (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-004, TASK-005
- **Approach:** Guard clause: if `totalPages <= 1`, skip concurrent fetch path. Raw/XML use `fetchRaw()` which is not modified.

### AC-009: Config/env for concurrency (Should)
- **Complexity:** Simple
- **Effort:** 15min
- **Tasks:** TASK-006
- **Approach:** Add `FetchConcurrency` to `AppConfig`, read `FETCH_CONCURRENCY` env var override in `main.go`.

### AC-010: Prometheus pages metric (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-005
- **Approach:** Existing `cashdrugs_upstream_fetch_pages_total` counter incremented per page regardless of parallel/serial — no change needed if counter is in per-page fetch function.

### AC-011: No goroutine leaks (Must)
- **Complexity:** Medium
- **Effort:** 15min
- **Tasks:** TASK-003, TASK-005
- **Approach:** Use `sync.WaitGroup` to ensure all goroutines complete before returning, even on error. Semaphore channel is unbuffered-receive safe.

### AC-012: Existing tests pass (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-008
- **Approach:** Run full test suite after implementation.

### AC-013: Timing benchmark (Should)
- **Complexity:** Medium
- **Effort:** 20min
- **Tasks:** TASK-004
- **Approach:** Test with mock server adding artificial latency per page. Verify 6 pages with concurrency=3 completes in ~2x single-page time.

## Implementation Phases

### Phase 1: RED — Write Failing Tests (1h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-001 | Write unit tests for concurrent page fetching — mock server returns N pages with per-page latency, verify all pages returned in order, verify wall-clock time is reduced | 25min | AC-001, AC-002, AC-004 | — |
| TASK-002 | Write unit tests for concurrency cap — mock server tracks max concurrent in-flight requests, verify never exceeds configured cap | 15min | AC-003 | — |
| TASK-003 | Write unit tests for error handling — page-style: single page failure fails entire fetch; offset-style: partial results returned. Verify no goroutine leaks (all goroutines complete). | 15min | AC-005, AC-006, AC-011 | — |
| TASK-004 | Write unit tests for single-page and raw/XML bypass — verify no goroutines spawned for single-page endpoints, raw/XML unaffected | 5min | AC-007, AC-008, AC-013 | — |

**Phase Output:** All tests fail (RED). No implementation yet.

### Phase 2: GREEN — Minimal Implementation (1.5h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-005 | Refactor `fetchJSON()` in `internal/upstream/fetcher.go` — after first-page fetch, if `totalPages > 1`, spawn goroutines with channel-based semaphore for pages 2..N. Collect results in pre-allocated slice. Handle errors per pagination style. Use WaitGroup for cleanup. | 60min | AC-001, AC-002, AC-004, AC-005, AC-006, AC-007, AC-008, AC-010, AC-011 | — |
| TASK-006 | Add `FetchConcurrency` field to `HTTPFetcher`, add `FetchConcurrency` to `AppConfig` in `internal/config/loader.go`, wire in `cmd/server/main.go` with `FETCH_CONCURRENCY` env var override | 20min | AC-003, AC-009 | — |
| TASK-007 | Verify Prometheus `cashdrugs_upstream_fetch_pages_total` still counts correctly with concurrent fetches | 10min | AC-010 | TASK-005 |

**Phase Output:** All tests pass (GREEN).

### Phase 3: REFACTOR (30min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-008 | Run full existing test suite, `go vet ./...`, verify no regressions | 10min | AC-012 | Phase 2 |
| TASK-009 | Extract concurrent fetch logic into a helper function if `fetchJSON` is too long. Clean up error collection pattern. | 15min | — | Phase 2 |
| TASK-010 | Review goroutine lifecycle — ensure all paths (success, error, context cancel) clean up properly | 5min | AC-011 | Phase 2 |

**Phase Output:** Clean code, all tests pass.

### Phase 4: VERIFY (30min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-011 | Run `make test-coverage` — verify coverage meets 80% threshold | 10min | — | Phase 3 |
| TASK-012 | Run timing test — verify 6 pages with concurrency=3 completes in ~2x single-page latency | 10min | AC-013 | Phase 3 |
| TASK-013 | Spec compliance check — verify each AC has a passing test | 10min | — | Phase 3 |

**Phase Output:** All gates pass, spec compliance verified.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/upstream/fetcher.go` | Modify | Refactor `fetchJSON()` for concurrent page fetching after first page |
| `internal/upstream/fetcher_test.go` | Modify | Add tests for concurrency, error handling, timing, cap enforcement |
| `internal/config/loader.go` | Modify | Add `FetchConcurrency` field to `AppConfig` |
| `internal/config/loader_test.go` | Modify | Test new config field |
| `cmd/server/main.go` | Modify | Wire `FetchConcurrency` to `HTTPFetcher`, read env var |

## Effort Summary

| Phase | Estimated Hours |
|-------|-----------------|
| Phase 1: RED | 1h |
| Phase 2: GREEN | 1.5h |
| Phase 3: REFACTOR | 0.5h |
| Phase 4: VERIFY | 0.5h |
| **Total** | **3.5h** |

## Dependencies

### External
- `sync` — Go stdlib (WaitGroup)
- Channel-based semaphore (no external package needed)

### Internal
- `internal/upstream/fetcher.go` — primary modification target
- `internal/config/loader.go` — new config field
- `cmd/server/main.go` — wiring

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Goroutine leak on early error return | Medium | High | WaitGroup ensures all goroutines complete before return. Tested explicitly. |
| Upstream rate-limits concurrent requests | Medium | Medium | Default cap is 3 (conservative). Configurable via env var. |
| Race condition writing to results slice | Low | High | Each goroutine writes to its own index — no shared writes. |
| Context cancellation leaves goroutines dangling | Low | Medium | HTTP client timeout (30s) ensures goroutines eventually complete. |

## Testing Strategy

1. **Unit tests** (Phase 1) — concurrent fetch, page ordering, error handling, cap enforcement
2. **Timing tests** (Phase 1) — verify latency reduction with mock server delays
3. **Regression** (Phase 3) — full `make test-unit`
4. **Coverage** (Phase 4) — `make test-coverage` >= 80%

## Spec Traceability

| AC | Tasks | Test Coverage |
|----|-------|---------------|
| AC-001 | TASK-001, TASK-005 | fetcher_test.go (sequential first page) |
| AC-002 | TASK-001, TASK-005 | fetcher_test.go (concurrent remaining pages) |
| AC-003 | TASK-002, TASK-006 | fetcher_test.go (cap enforcement) |
| AC-004 | TASK-001, TASK-005 | fetcher_test.go (page-order reassembly) |
| AC-005 | TASK-003, TASK-005 | fetcher_test.go (fail-all on page error) |
| AC-006 | TASK-003, TASK-005 | fetcher_test.go (partial results offset) |
| AC-007 | TASK-004, TASK-005 | fetcher_test.go (single-page bypass) |
| AC-008 | TASK-004 | fetcher_test.go (raw/XML unaffected) |
| AC-009 | TASK-006 | loader_test.go (config field + env) |
| AC-010 | TASK-007 | fetcher_test.go (metric count) |
| AC-011 | TASK-003, TASK-010 | fetcher_test.go (no goroutine leaks) |
| AC-012 | TASK-008 | existing test suite |
| AC-013 | TASK-004, TASK-012 | fetcher_test.go (timing benchmark) |

## Next Steps

1. Review and approve this plan
2. Run `/add:tdd-cycle specs/parallel-page-fetches.md` to execute

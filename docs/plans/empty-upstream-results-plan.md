# Implementation Plan: Empty Upstream Results

**Spec Version:** 0.1.0
**Spec:** specs/empty-upstream-results.md
**Created:** 2026-03-15
**Team Size:** Solo (1 agent)
**Estimated Duration:** 2.5 hours

## Overview

Return HTTP 200 with an empty data array when an upstream API returns valid but empty results, instead of treating it as an error. Add a `results_count` field to the response meta for all responses. Cache empty results with a short LRU TTL (2 minutes) to avoid hammering upstream for known-empty queries.

## Objectives

- Distinguish empty results (valid 200) from upstream errors (502)
- Add `ResultsCount` field to `ResponseMeta` for all responses
- Cache empty results in MongoDB via normal upsert flow
- Use short LRU TTL for empty results (2 minutes, configurable)
- Correctly classify empty-result responses in Prometheus metrics

## Acceptance Criteria Analysis

### AC-001: Empty data as valid CachedResponse (Must)
- **Complexity:** Simple
- **Effort:** 15min
- **Tasks:** TASK-001, TASK-005
- **Approach:** In fetcher, ensure empty `data_key` arrays are returned as `[]interface{}{}` not nil. No change to `Fetch()` return signature.

### AC-002: Handler returns 200 with empty data (Must)
- **Complexity:** Simple
- **Effort:** 20min
- **Tasks:** TASK-001, TASK-006
- **Approach:** In `respondWithCached()`, check data length but always return 200 for valid responses. Build response envelope with `"data": []` and populated meta.

### AC-003, AC-004: ResultsCount in ResponseMeta (Must)
- **Complexity:** Simple
- **Effort:** 15min
- **Tasks:** TASK-002, TASK-004
- **Approach:** Add `ResultsCount int` with `json:"results_count"` to `ResponseMeta`. Populate by counting the data array length in the handler for all responses.

### AC-005: Empty results cached in MongoDB (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-003, TASK-006
- **Approach:** No change to upsert flow — empty `Data` slice is valid and persisted normally.

### AC-006: Short LRU TTL for empty results (Must)
- **Complexity:** Medium
- **Effort:** 20min
- **Tasks:** TASK-003, TASK-006
- **Approach:** In the handler, when storing empty results in LRU, use a shorter TTL (2 minutes vs default). Add a configurable `EmptyResultTTL` or pass TTL to the LRU Set call.

### AC-007: Upstream errors still return 502 (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-001, TASK-006
- **Approach:** Existing error paths unchanged. Only valid 200 responses with empty data get the new treatment.

### AC-008: Non-empty responses unchanged except ResultsCount (Must)
- **Complexity:** Simple
- **Effort:** 5min
- **Tasks:** TASK-002, TASK-004
- **Approach:** `ResultsCount` is populated for all responses. No other envelope changes.

### AC-009: Stale field correct for empty cached results (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-003, TASK-006
- **Approach:** Same staleness logic applies — TTL-based. Empty cached results age the same way.

### AC-010: Raw/XML unaffected (Must)
- **Complexity:** Simple
- **Effort:** 5min
- **Tasks:** TASK-006
- **Approach:** Raw/XML bypass the JSON envelope entirely. No changes needed.

### AC-011, AC-012: Prometheus metrics correct (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-003, TASK-006
- **Approach:** Empty results are valid 200s — Prometheus HTTP status counter labels them as 200. Cache outcome metric labels empty-result cache hits as `"hit"`.

### AC-013: Existing tests pass (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-008
- **Approach:** Run full test suite after implementation.

## Implementation Phases

### Phase 1: RED — Write Failing Tests (45min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-001 | Write unit tests for empty upstream results — fetcher returns valid `CachedResponse` with `Data: []interface{}{}`, handler returns 200 with `"data": []`. Verify upstream errors still produce 502. | 20min | AC-001, AC-002, AC-007 | — |
| TASK-002 | Write unit tests for `ResultsCount` in `ResponseMeta` — verify present in both empty and non-empty responses, correct count for multi-page aggregated results | 10min | AC-003, AC-004, AC-008 | — |
| TASK-003 | Write unit tests for empty result caching — verify cached in MongoDB, LRU uses short TTL, stale detection works, Prometheus labels correct | 15min | AC-005, AC-006, AC-009, AC-011, AC-012 | — |

**Phase Output:** All tests fail (RED). No implementation yet.

### Phase 2: GREEN — Minimal Implementation (1h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-004 | Add `ResultsCount int` with `json:"results_count"` to `ResponseMeta` in `internal/model/response.go` | 5min | AC-003, AC-004 | — |
| TASK-005 | Update fetcher in `internal/upstream/fetcher.go` — ensure empty `data_key` arrays are returned as `[]interface{}{}` not nil, handle `null` data_key as empty | 15min | AC-001 | — |
| TASK-006 | Update handler in `internal/handler/cache.go` — populate `ResultsCount` from `len(data)` in `respondWithCached()`, always return 200 for valid responses regardless of data length, use short LRU TTL for empty results, ensure Prometheus metrics label empty results as 200 and cache hits as `"hit"` | 30min | AC-002, AC-005, AC-006, AC-007, AC-008, AC-009, AC-010, AC-011, AC-012 | TASK-004, TASK-005 |
| TASK-007 | Verify raw/XML responses are unaffected — no ResultsCount in non-JSON responses | 5min | AC-010 | TASK-006 |

**Phase Output:** All tests pass (GREEN).

### Phase 3: REFACTOR (20min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-008 | Run full existing test suite, `go vet ./...`, verify no regressions | 10min | AC-013 | Phase 2 |
| TASK-009 | Review response building logic for clean separation of empty vs non-empty paths. Ensure no duplication. | 10min | — | Phase 2 |

**Phase Output:** Clean code, all tests pass.

### Phase 4: VERIFY (15min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-010 | Run `make test-coverage` — verify coverage meets 80% threshold | 5min | — | Phase 3 |
| TASK-011 | Spec compliance check — verify each AC has a passing test | 5min | — | Phase 3 |
| TASK-012 | Update swag annotations if response envelope changed, regenerate swagger docs | 5min | — | Phase 3 |

**Phase Output:** All gates pass, spec compliance verified.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/model/response.go` | Modify | Add `ResultsCount` to `ResponseMeta` |
| `internal/upstream/fetcher.go` | Modify | Ensure empty data arrays are `[]interface{}{}` not nil |
| `internal/handler/cache.go` | Modify | Populate `ResultsCount`, handle empty data as valid 200, short LRU TTL for empty results |
| `internal/handler/cache_test.go` | Modify | Add tests for empty result handling, ResultsCount population |
| `internal/upstream/fetcher_test.go` | Modify | Add tests for empty data array handling |
| `docs/swagger.json` | Regenerate | Updated response envelope with results_count |
| `docs/swagger.yaml` | Regenerate | Updated response envelope with results_count |

## Effort Summary

| Phase | Estimated Hours |
|-------|-----------------|
| Phase 1: RED | 0.75h |
| Phase 2: GREEN | 1h |
| Phase 3: REFACTOR | 0.33h |
| Phase 4: VERIFY | 0.25h |
| **Total** | **2.5h** |

## Dependencies

### External
- None — all changes are internal

### Internal
- `internal/model/response.go` — add 1 field
- `internal/upstream/fetcher.go` — empty array handling
- `internal/handler/cache.go` — response building and LRU TTL logic

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Existing consumers break on new `results_count` field | Very Low | Low | Additive change — new field, no existing fields removed or renamed |
| Empty results with short TTL cause upstream request spikes | Low | Medium | 2-minute TTL is a balance. Configurable for tuning. |
| Nil vs empty slice confusion in Go JSON marshaling | Medium | Medium | Explicitly set `Data = []interface{}{}` not nil. Test JSON output. |
| Cache hit metric miscounted for empty results | Low | Medium | Explicitly test metric labels in unit tests |

## Testing Strategy

1. **Unit tests** (Phase 1) — empty result handling, ResultsCount, caching, metrics
2. **Regression** (Phase 3) — full `make test-unit`
3. **Coverage** (Phase 4) — `make test-coverage` >= 80%

## Spec Traceability

| AC | Tasks | Test Coverage |
|----|-------|---------------|
| AC-001 | TASK-001, TASK-005 | fetcher_test.go (empty data handling) |
| AC-002 | TASK-001, TASK-006 | cache_test.go (200 for empty) |
| AC-003 | TASK-002, TASK-004 | cache_test.go (ResultsCount field) |
| AC-004 | TASK-002, TASK-004 | cache_test.go (populated for all responses) |
| AC-005 | TASK-003, TASK-006 | cache_test.go (MongoDB caching) |
| AC-006 | TASK-003, TASK-006 | cache_test.go (short LRU TTL) |
| AC-007 | TASK-001, TASK-006 | cache_test.go (502 on upstream error) |
| AC-008 | TASK-002, TASK-004 | cache_test.go (non-empty unchanged) |
| AC-009 | TASK-003, TASK-006 | cache_test.go (stale field) |
| AC-010 | TASK-007 | cache_test.go (raw/XML bypass) |
| AC-011 | TASK-003, TASK-006 | cache_test.go (Prometheus 200 count) |
| AC-012 | TASK-003, TASK-006 | cache_test.go (cache hit label) |
| AC-013 | TASK-008 | existing test suite |

## Next Steps

1. Review and approve this plan
2. Run `/add:tdd-cycle specs/empty-upstream-results.md` to execute

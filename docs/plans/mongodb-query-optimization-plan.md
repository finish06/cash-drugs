# Implementation Plan: MongoDB Query Optimization

**Spec Version:** 0.1.0
**Spec:** specs/mongodb-query-optimization.md
**Created:** 2026-03-15
**Team Size:** Solo (1 agent)
**Estimated Duration:** 4 hours

## Overview

Replace regex-based `cache_key` matching with indexed exact-match queries by adding a `base_key` field to every document, creating a compound index, and switching all lookups from `$regex` to `$eq`. Includes a startup migration to backfill existing documents.

## Objectives

- Add `BaseKey` field to `CachedResponse` model
- Create compound index `idx_base_key_page` on `{base_key: 1, page: 1}`
- Convert `Get()`, `FetchedAt()`, and stale-page deletion from regex to exact match
- Implement idempotent `backfillBaseKey()` migration at startup
- Remove `buildRegexFilter()` and `escapeRegex()` after migration is in place

## Acceptance Criteria Analysis

### AC-001: BaseKey field on model (Must)
- **Complexity:** Simple
- **Effort:** 15min
- **Tasks:** TASK-001, TASK-005
- **Approach:** Add `BaseKey string` with `bson:"base_key"` tag to `CachedResponse` in `internal/model/cache.go`.

### AC-002: Compound index creation (Must)
- **Complexity:** Simple
- **Effort:** 20min
- **Tasks:** TASK-002, TASK-006
- **Approach:** Add `idx_base_key_page` index model to `ensureIndexes()` in `internal/cache/mongo.go`.

### AC-003, AC-004: Get/FetchedAt use $eq on base_key (Must)
- **Complexity:** Medium
- **Effort:** 45min
- **Tasks:** TASK-003, TASK-007
- **Approach:** Replace `buildRegexFilter(cacheKey)` calls in `Get()` and `FetchedAt()` with `bson.M{"base_key": cacheKey}`. Add sort `{page: 1}` to `Get()` cursor.

### AC-005, AC-006, AC-007: Upsert writes base_key and uses exact match for stale cleanup (Must)
- **Complexity:** Medium
- **Effort:** 45min
- **Tasks:** TASK-003, TASK-007
- **Approach:** In `Upsert()`, set `base_key` on each page document to `resp.CacheKey` (the key without `:page:N`). For single-doc responses, `base_key = cache_key`. Replace stale-page regex deletion with `bson.M{"base_key": cacheKey, "page": bson.M{"$gt": pageCount}}`.

### AC-008, AC-009: Backfill migration (Must)
- **Complexity:** Medium
- **Effort:** 45min
- **Tasks:** TASK-004, TASK-008
- **Approach:** New `backfillBaseKey()` function: find documents where `base_key` is empty/missing, parse `cache_key` to extract base (split on last `:page:` occurrence), bulk-update. Called from `NewMongoRepository()`. Idempotent (skips docs with `base_key` set). Logs count.

### AC-010: Remove buildRegexFilter/escapeRegex (Should)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-010
- **Approach:** Delete functions after all callers are migrated.

### AC-011: Retain idx_cache_key (Must)
- **Complexity:** Simple
- **Effort:** 5min
- **Tasks:** TASK-006
- **Approach:** Do not remove existing index creation code.

### AC-012: Existing tests pass (Must)
- **Complexity:** Simple
- **Effort:** 15min
- **Tasks:** TASK-011
- **Approach:** Run full test suite after implementation.

### AC-013: Benchmark test (Should)
- **Complexity:** Medium
- **Effort:** 30min
- **Tasks:** TASK-004
- **Approach:** Write benchmark test comparing regex vs exact-match Get() with 500+ documents.

## Implementation Phases

### Phase 1: RED — Write Failing Tests (1.5h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-001 | Write unit tests for `CachedResponse.BaseKey` field — verify it marshals/unmarshals to `bson:"base_key"` | 15min | AC-001 | — |
| TASK-002 | Write unit tests for `ensureIndexes()` — verify `idx_base_key_page` compound index is created alongside existing `idx_cache_key` | 15min | AC-002, AC-011 | — |
| TASK-003 | Write unit tests for `Get()`, `FetchedAt()`, and `Upsert()` — verify they use `base_key` exact match, multi-page sort order, stale page cleanup by `base_key` filter, and that `base_key` is set correctly for both single and multi-page docs | 30min | AC-003, AC-004, AC-005, AC-006, AC-007 | — |
| TASK-004 | Write unit tests for `backfillBaseKey()` — test migration of docs with/without `:page:` suffix, idempotency on re-run, edge case of `cache_key` containing multiple `:page:` substrings, empty collection | 30min | AC-008, AC-009, AC-013 | — |

**Phase Output:** All tests fail (RED). No implementation yet.

### Phase 2: GREEN — Minimal Implementation (1.5h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-005 | Add `BaseKey string` field with `bson:"base_key"` to `CachedResponse` in `internal/model/cache.go` | 10min | AC-001 | — |
| TASK-006 | Add `idx_base_key_page` compound index to `ensureIndexes()` in `internal/cache/mongo.go`. Retain `idx_cache_key`. | 15min | AC-002, AC-011 | TASK-005 |
| TASK-007 | Modify `Get()`, `FetchedAt()`, and `Upsert()` — replace regex filters with `bson.M{"base_key": cacheKey}`, add page sort to `Get()`, set `base_key` in upsert documents, use `base_key` filter for stale page cleanup | 30min | AC-003, AC-004, AC-005, AC-006, AC-007 | TASK-005 |
| TASK-008 | Implement `backfillBaseKey()` — query docs with empty/missing `base_key`, parse `cache_key` (split on last `:page:`), bulk-update, log count. Call from `NewMongoRepository()`. | 30min | AC-008, AC-009 | TASK-005, TASK-006 |

**Phase Output:** All tests pass (GREEN).

### Phase 3: REFACTOR (30min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-009 | Review query construction for clean helper usage, ensure consistent filter building | 15min | — | Phase 2 |
| TASK-010 | Remove `buildRegexFilter()` and `escapeRegex()` functions. Verify no remaining callers. | 10min | AC-010 | Phase 2 |
| TASK-011 | Run full existing test suite, `go vet ./...`, verify no regressions | 5min | AC-012 | TASK-010 |

**Phase Output:** Clean code, all tests pass.

### Phase 4: VERIFY (30min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-012 | Run `make test-coverage` — verify coverage meets 80% threshold | 10min | — | Phase 3 |
| TASK-013 | Write benchmark test comparing regex vs exact-match Get() with 500+ seeded documents | 15min | AC-013 | Phase 3 |
| TASK-014 | Spec compliance check — verify each AC has a passing test | 5min | — | Phase 3 |

**Phase Output:** All gates pass, spec compliance verified.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/model/cache.go` | Modify | Add `BaseKey` field to `CachedResponse` |
| `internal/cache/mongo.go` | Modify | Update `ensureIndexes`, `Get`, `FetchedAt`, `Upsert`, add `backfillBaseKey`, remove `buildRegexFilter`/`escapeRegex` |
| `internal/cache/mongo_test.go` | Modify | Add tests for base_key queries, migration, benchmark |
| `internal/cache/collection.go` | Verify | Ensure no regex usage remains in collection helpers |

## Effort Summary

| Phase | Estimated Hours |
|-------|-----------------|
| Phase 1: RED | 1.5h |
| Phase 2: GREEN | 1.5h |
| Phase 3: REFACTOR | 0.5h |
| Phase 4: VERIFY | 0.5h |
| **Total** | **4h** |

## Dependencies

### External
- None — all work is internal to the storage layer

### Internal
- `internal/model` package (add 1 field)
- `internal/cache/mongo.go` (modify queries and indexes)

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Backfill migration slow on large collections | Low | Medium | Uses bulk update, runs once at startup. Log timing. |
| Existing tests rely on regex filter internals | Low | Medium | AC-012 requires no test modifications — if tests inspect filter, they test behavior not implementation |
| cache_key with multiple `:page:` substrings | Low | Low | Split on last occurrence per edge case spec |
| Index creation fails on existing data | Very Low | High | Compound index is non-unique, no conflict risk. Startup error propagated. |

## Testing Strategy

1. **Unit tests** (Phase 1) — test query filters, base_key population, migration logic
2. **Migration tests** (Phase 1) — test backfill with various cache_key patterns
3. **Regression** (Phase 3) — full `make test-unit`
4. **Benchmark** (Phase 4) — regex vs exact-match comparison
5. **Coverage** (Phase 4) — `make test-coverage` >= 80%

## Spec Traceability

| AC | Tasks | Test Coverage |
|----|-------|---------------|
| AC-001 | TASK-001, TASK-005 | mongo_test.go (model field) |
| AC-002 | TASK-002, TASK-006 | mongo_test.go (index creation) |
| AC-003 | TASK-003, TASK-007 | mongo_test.go (Get exact match) |
| AC-004 | TASK-003, TASK-007 | mongo_test.go (FetchedAt exact match) |
| AC-005 | TASK-003, TASK-007 | mongo_test.go (Upsert multi-page base_key) |
| AC-006 | TASK-003, TASK-007 | mongo_test.go (Upsert single-doc base_key) |
| AC-007 | TASK-003, TASK-007 | mongo_test.go (stale page cleanup) |
| AC-008 | TASK-004, TASK-008 | mongo_test.go (backfill migration) |
| AC-009 | TASK-004, TASK-008 | mongo_test.go (idempotent re-run) |
| AC-010 | TASK-010 | compilation check (removed functions) |
| AC-011 | TASK-006 | mongo_test.go (index retained) |
| AC-012 | TASK-011 | existing test suite |
| AC-013 | TASK-013 | mongo_test.go (benchmark) |

## Next Steps

1. Review and approve this plan
2. Run `/add:tdd-cycle specs/mongodb-query-optimization.md` to execute

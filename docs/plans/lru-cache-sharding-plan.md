# Implementation Plan: LRU Cache Sharding

**Spec Version:** 0.1.0
**Spec:** specs/lru-cache-sharding.md
**Created:** 2026-03-15
**Team Size:** Solo (1 agent)
**Estimated Duration:** 3 hours

## Overview

Split the single-mutex LRU cache into N sharded buckets (default 16) with independent mutexes to reduce lock contention under concurrent load. Each shard has its own map, doubly-linked list, and mutex. Keys are assigned to shards via FNV-1a hash. The total memory budget is divided evenly across shards.

## Objectives

- Create `shardedLRUCache` implementing the `LRUCache` interface
- Use FNV-1a hash for deterministic shard selection
- Per-shard mutex, map, and list for independent locking
- Split `maxBytes` evenly across shards
- Replace `NewLRUCache` to return sharded implementation by default
- Preserve `noopLRU` for `maxBytes <= 0`

## Acceptance Criteria Analysis

### AC-001: ShardedLRUCache implements LRUCache (Must)
- **Complexity:** Medium
- **Effort:** 45min
- **Tasks:** TASK-001, TASK-005
- **Approach:** New `shardedLRUCache` struct with `Get`, `Set`, `Invalidate`, `SizeBytes` methods delegating to the correct shard.

### AC-002, AC-003: Configurable shard count, per-shard internals (Must)
- **Complexity:** Medium
- **Effort:** 30min
- **Tasks:** TASK-001, TASK-005
- **Approach:** `NewShardedLRUCache(maxBytes, shardCount)` constructor creates N `lruShard` structs, each with its own `sync.Mutex`, `map[string]*list.Element`, and `*list.List`.

### AC-004: FNV-1a shard selection (Must)
- **Complexity:** Simple
- **Effort:** 15min
- **Tasks:** TASK-002, TASK-006
- **Approach:** `shardIndex(key, shardCount)` using `hash/fnv` New32a. Modulo for non-power-of-2 counts.

### AC-005: Even budget split (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-002, TASK-005
- **Approach:** Each shard's `maxBytes = totalMaxBytes / shardCount`.

### AC-006: SizeBytes aggregation (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-003, TASK-005
- **Approach:** `SizeBytes()` iterates all shards, sums `curBytes` under each shard's lock.

### AC-007: NewLRUCache returns sharded (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-005
- **Approach:** `NewLRUCache(maxBytes)` calls `NewShardedLRUCache(maxBytes, 16)`.

### AC-008: noopLRU preserved (Must)
- **Complexity:** Simple
- **Effort:** 5min
- **Tasks:** TASK-005
- **Approach:** Guard check at top of `NewLRUCache`: `if maxBytes <= 0 { return &noopLRU{} }`.

### AC-009: Shard count clamping (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-002, TASK-006
- **Approach:** Clamp `shardCount` to `[1, maxBytes]` in constructor.

### AC-010, AC-011: TTL and independent eviction (Must)
- **Complexity:** Medium
- **Effort:** 30min
- **Tasks:** TASK-003, TASK-005
- **Approach:** Each shard runs the same TTL check and LRU eviction logic as the old single-mutex implementation. No cross-shard effects.

### AC-012: Existing tests pass (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-008
- **Approach:** Run existing `lru_test.go` — tests use `NewLRUCache` which now returns sharded.

### AC-013: Concurrent benchmark (Should)
- **Complexity:** Medium
- **Effort:** 20min
- **Tasks:** TASK-004
- **Approach:** Benchmark N goroutines doing random Get/Set with 1 shard vs 16 shards.

### AC-014: estimateSize unchanged (Must)
- **Complexity:** Simple
- **Effort:** 0min
- **Tasks:** —
- **Approach:** `estimateSize` is a standalone function, not modified.

## Implementation Phases

### Phase 1: RED — Write Failing Tests (1h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-001 | Write unit tests for `ShardedLRUCache` — verify it implements `LRUCache` interface, basic Get/Set/Invalidate work across shards | 20min | AC-001, AC-002, AC-003 | — |
| TASK-002 | Write unit tests for shard selection — verify deterministic FNV-1a routing, shard count clamping, even budget split | 15min | AC-004, AC-005, AC-009 | — |
| TASK-003 | Write unit tests for SizeBytes aggregation, TTL expiration per shard, independent eviction across shards | 15min | AC-006, AC-010, AC-011 | — |
| TASK-004 | Write concurrent benchmark test — N goroutines with 1 shard vs 16 shards, measure wall-clock time | 10min | AC-013 | — |

**Phase Output:** All tests fail (RED). No implementation yet.

### Phase 2: GREEN — Minimal Implementation (1.5h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-005 | Implement `shardedLRUCache` and `lruShard` types in `internal/cache/lru.go` — constructor, Get, Set, Invalidate, SizeBytes. Each shard reuses the existing entry struct and eviction logic. Update `NewLRUCache` to return sharded with 16 shards. | 60min | AC-001, AC-002, AC-003, AC-005, AC-006, AC-007, AC-008, AC-010, AC-011, AC-014 | — |
| TASK-006 | Implement `shardIndex()` helper using `hash/fnv` New32a. Add shard count clamping logic. | 15min | AC-004, AC-009 | — |
| TASK-007 | Verify `NewLRUCache(0)` still returns `noopLRU` | 5min | AC-008 | TASK-005 |

**Phase Output:** All tests pass (GREEN).

### Phase 3: REFACTOR (15min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-008 | Run full existing test suite (`lru_test.go` and all others), `go vet ./...`, verify no regressions | 10min | AC-012 | Phase 2 |
| TASK-009 | Clean up shard struct methods, ensure consistent naming, remove dead code from old single-mutex path | 5min | — | Phase 2 |

**Phase Output:** Clean code, all tests pass.

### Phase 4: VERIFY (15min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-010 | Run `make test-coverage` — verify coverage meets 80% threshold | 5min | — | Phase 3 |
| TASK-011 | Run concurrent benchmark, verify 16-shard is measurably faster than 1-shard | 5min | AC-013 | Phase 3 |
| TASK-012 | Spec compliance check — verify each AC has a passing test | 5min | — | Phase 3 |

**Phase Output:** All gates pass, spec compliance verified.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/cache/lru.go` | Modify | Add `shardedLRUCache`, `lruShard`, `shardIndex`; update `NewLRUCache` to return sharded; remove old single-mutex `lruCache` struct |
| `internal/cache/lru_test.go` | Modify | Add sharding-specific tests (shard selection, budget split, aggregation, concurrency benchmark) |

## Effort Summary

| Phase | Estimated Hours |
|-------|-----------------|
| Phase 1: RED | 1h |
| Phase 2: GREEN | 1.5h |
| Phase 3: REFACTOR | 0.25h |
| Phase 4: VERIFY | 0.25h |
| **Total** | **3h** |

## Dependencies

### External
- `hash/fnv` — Go stdlib, no new external dependencies

### Internal
- `internal/cache/lru.go` — sole modified file
- Existing `LRUCache` interface must be preserved

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Uneven key distribution across shards | Low | Low | FNV-1a has good distribution. Pathological inputs are acknowledged in spec. |
| Old lruCache tests reference internals | Low | Medium | Tests use the `LRUCache` interface, not struct fields directly |
| Shard-per-shard eviction reduces effective cache utilization | Medium | Low | Accepted tradeoff — lock contention reduction outweighs slight utilization loss |
| Race conditions in shard logic | Low | High | Each shard has its own mutex. Tests include concurrent access patterns. |

## Testing Strategy

1. **Unit tests** (Phase 1) — interface compliance, shard routing, budget splitting, TTL, eviction
2. **Concurrency tests** (Phase 1) — multi-goroutine Get/Set stress test
3. **Regression** (Phase 3) — full `make test-unit`
4. **Benchmark** (Phase 4) — 1-shard vs 16-shard wall-clock comparison
5. **Coverage** (Phase 4) — `make test-coverage` >= 80%

## Spec Traceability

| AC | Tasks | Test Coverage |
|----|-------|---------------|
| AC-001 | TASK-001, TASK-005 | lru_test.go (interface compliance) |
| AC-002 | TASK-001, TASK-005 | lru_test.go (configurable shards) |
| AC-003 | TASK-001, TASK-005 | lru_test.go (per-shard internals) |
| AC-004 | TASK-002, TASK-006 | lru_test.go (FNV-1a routing) |
| AC-005 | TASK-002, TASK-005 | lru_test.go (budget split) |
| AC-006 | TASK-003, TASK-005 | lru_test.go (SizeBytes aggregation) |
| AC-007 | TASK-005 | lru_test.go (NewLRUCache returns sharded) |
| AC-008 | TASK-007 | lru_test.go (noopLRU for maxBytes<=0) |
| AC-009 | TASK-002, TASK-006 | lru_test.go (clamping) |
| AC-010 | TASK-003, TASK-005 | lru_test.go (TTL expiration) |
| AC-011 | TASK-003, TASK-005 | lru_test.go (independent eviction) |
| AC-012 | TASK-008 | existing test suite |
| AC-013 | TASK-004, TASK-011 | lru_test.go (benchmark) |
| AC-014 | — | estimateSize unchanged (no test needed) |

## Next Steps

1. Review and approve this plan
2. Run `/add:tdd-cycle specs/lru-cache-sharding.md` to execute

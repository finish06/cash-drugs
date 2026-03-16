# Spec: LRU Cache Sharding

**Version:** 0.1.0
**Created:** 2026-03-15
**PRD Reference:** docs/prd.md (M10)
**Status:** Complete

## 1. Overview

Split the single-mutex LRU cache into N sharded buckets with independent mutexes to reduce lock contention under concurrent load. Currently, `internal/cache/lru.go` uses a single `sync.Mutex` for all `Get`, `Set`, and `Invalidate` operations. At 150+ concurrent requests, this becomes a serialization bottleneck — every cache operation across all slugs contends for the same lock. This spec introduces a `ShardedLRUCache` that hashes cache keys to one of N shards (default 16), where each shard is an independent LRU with its own mutex, list, and map. The total memory budget is split evenly across shards.

### User Story

As an **operator**, I want the in-memory LRU cache to handle concurrent access without single-lock contention, so that cache hit latency stays low under sustained load and p99 tail latency is reduced.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | A new `ShardedLRUCache` type implements the existing `LRUCache` interface (`Get`, `Set`, `Invalidate`, `SizeBytes`) | Must |
| AC-002 | The shard count is configurable via `NewShardedLRUCache(maxBytes int64, shardCount int)`. Default shard count is 16 when called from `NewLRUCache`. | Must |
| AC-003 | Each shard has its own `sync.Mutex`, `map[string]*list.Element`, and `*list.List` | Must |
| AC-004 | Cache keys are assigned to shards using a deterministic hash function (FNV-1a) modulo shard count | Must |
| AC-005 | The total memory budget (`maxBytes`) is divided evenly across shards: each shard's limit is `maxBytes / shardCount` | Must |
| AC-006 | `SizeBytes()` returns the sum of all shards' current byte usage | Must |
| AC-007 | `NewLRUCache(maxBytes)` returns a `ShardedLRUCache` with 16 shards (replacing the current single-mutex implementation) | Must |
| AC-008 | When `maxBytes <= 0`, `NewLRUCache` still returns a `noopLRU` (no change) | Must |
| AC-009 | When `shardCount <= 0` or `shardCount > maxBytes`, it is clamped to a sensible value (minimum 1, maximum such that each shard gets at least 1 byte) | Must |
| AC-010 | TTL expiration behavior is unchanged — expired entries are evicted on `Get` within their shard | Must |
| AC-011 | LRU eviction within a shard is independent — evicting the least-recently-used entry in shard N does not affect shard M | Must |
| AC-012 | All existing LRU unit tests pass without modification | Must |
| AC-013 | A concurrent benchmark test demonstrates reduced lock contention: N goroutines performing random Get/Set operations complete faster with 16 shards than with 1 shard | Should |
| AC-014 | The `estimateSize` function remains unchanged and shared across all shards | Must |

## 3. User Test Cases

### TC-001: Concurrent access across shards does not block

**Precondition:** ShardedLRUCache with 4 shards, maxBytes = 4MB (1MB per shard)
**Steps:**
1. Launch 4 goroutines, each writing to keys that hash to different shards
2. Measure total wall-clock time
**Expected Result:** All 4 goroutines run truly concurrently (no serialization). Total time is approximately equal to single-goroutine time.
**Maps to:** AC-003, AC-004

### TC-002: Size budget enforced per shard

**Precondition:** ShardedLRUCache with 2 shards, maxBytes = 200KB (100KB per shard)
**Steps:**
1. Write entries to shard 0 until it exceeds 100KB
2. Verify shard 0 evicts LRU entries to stay within 100KB
3. Verify shard 1 still has full 100KB capacity available
**Expected Result:** Eviction in one shard does not affect the other. `SizeBytes()` never exceeds 200KB.
**Maps to:** AC-005, AC-011

### TC-003: Deterministic shard assignment

**Precondition:** ShardedLRUCache with 16 shards
**Steps:**
1. Call `Set("drugnames", ...)` twice
2. Call `Get("drugnames")`
**Expected Result:** Both Set and Get route to the same shard. Get returns the latest value.
**Maps to:** AC-004

### TC-004: TTL expiration works per shard

**Precondition:** ShardedLRUCache with 4 shards
**Steps:**
1. Set key "alpha" with TTL 50ms
2. Wait 60ms
3. Get key "alpha"
**Expected Result:** Get returns `(nil, false)` — entry expired within its shard.
**Maps to:** AC-010

### TC-005: SizeBytes aggregates all shards

**Precondition:** ShardedLRUCache with 4 shards, empty
**Steps:**
1. Set one entry in each of 3 different shards
2. Call `SizeBytes()`
**Expected Result:** Returns the sum of sizes across all 3 populated shards.
**Maps to:** AC-006

## 4. Data Model

No data model changes. This is an in-memory implementation change.

### New Types

```go
type shardedLRUCache struct {
    shards     []*lruShard
    shardCount int
    shardMask  int  // shardCount - 1, for fast modulo when shardCount is power of 2
}

type lruShard struct {
    mu       sync.Mutex
    maxBytes int64
    curBytes int64
    items    map[string]*list.Element
    order    *list.List
}
```

### Hash Function

```go
func shardIndex(key string, shardCount int) int {
    h := fnv.New32a()
    h.Write([]byte(key))
    return int(h.Sum32()) % shardCount
}
```

## 5. API Contract

No API changes. The `LRUCache` interface is unchanged. This is a drop-in replacement for the internal implementation.

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| `shardCount = 1` | Behaves identically to the current single-mutex LRU |
| `maxBytes = 100`, `shardCount = 16` | Each shard gets 6 bytes (100/16 = 6). Small but functional. |
| `maxBytes = 0` | Returns `noopLRU`, sharding is not involved |
| `Invalidate` on a key that doesn't exist | No-op within the target shard, no cross-shard effects |
| All keys hash to the same shard (pathological input) | That shard operates at full capacity; other shards are empty. Correct but suboptimal. |
| `shardCount` not a power of 2 | Use modulo operator (not bitmask). Works correctly, slightly slower. |

## 7. Dependencies

- `internal/cache/lru.go` — replace `lruCache` with `shardedLRUCache` + `lruShard`
- `hash/fnv` — Go stdlib, no new external dependencies
- `internal/cache/lru_test.go` — existing tests run against new implementation
- `cmd/server/main.go` — no changes needed (`NewLRUCache` signature unchanged)

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-15 | 0.1.0 | calebdunn | Initial spec for M10 |

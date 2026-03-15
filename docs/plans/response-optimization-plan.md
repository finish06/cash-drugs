# Implementation Plan: Response Optimization

**Spec Version:** 0.1.0
**Spec:** specs/response-optimization.md
**Created:** 2026-03-14
**Team Size:** Solo (1 agent)
**Estimated Duration:** 7-8 hours

## Overview

Add gzip compression middleware, singleflight request coalescing, and an in-memory LRU cache layer to reduce response latency, bandwidth, and memory pressure. This is three distinct subsystems that integrate into the existing handler pipeline.

## Objectives

- Compress JSON/XML responses 3-5x via gzip middleware
- Deduplicate concurrent requests for the same cache key via singleflight
- Serve hot responses from in-memory LRU cache (sub-ms) instead of MongoDB
- Expose LRU and singleflight metrics to Prometheus

## Acceptance Criteria Analysis

### AC-001–AC-005: Gzip compression (Must)
- **Complexity:** Medium
- **Effort:** 1.5h
- **Approach:** HTTP middleware wrapping `http.ResponseWriter` with a `gzip.Writer`. Sniff content type from response headers. Buffer initial bytes to check size threshold (1 KB). Set `Content-Encoding: gzip` and `Vary: Accept-Encoding`.

### AC-006–AC-008: Singleflight coalescing (Must)
- **Complexity:** Medium
- **Effort:** 1.5h
- **Approach:** Add `singleflight.Group` to `CacheHandler`. Wrap the cache-lookup + upstream-fetch path in `group.Do(cacheKey, fn)`. On error, use `group.Forget(cacheKey)` so errors aren't shared (AC-008).

### AC-009–AC-012, AC-016: LRU cache (Must)
- **Complexity:** Complex
- **Effort:** 2.5h
- **Approach:** New `internal/cache/lru.go` package. Size-bounded LRU using `github.com/hashicorp/golang-lru/v2` with a custom size tracking wrapper. Each entry stores `*model.CachedResponse` + `ExpiresAt`. On `Get()`: check TTL, evict if expired. On `Set()`: estimate entry size, evict LRU entries until under budget. Force-refresh invalidates key. Implements a new `LRUCache` interface consumed by `CacheHandler`.

### AC-013–AC-015: Prometheus metrics (Should)
- **Complexity:** Simple
- **Effort:** 30min
- **Approach:** Add 4 new collectors to `Metrics` struct: LRU size gauge, LRU hit/miss counters, singleflight dedup counter.

### AC-017: Scheduler LRU population (Should)
- **Complexity:** Simple
- **Effort:** 20min
- **Approach:** Pass `LRUCache` to `Scheduler` via option. After successful fetch + upsert, also populate LRU.

## Implementation Phases

### Phase 1: RED — Write Failing Tests (2h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-001 | Write unit tests for gzip middleware — compressed response for JSON, XML; uncompressed for no Accept-Encoding; skip for < 1 KB; Content-Encoding header | 30min | AC-001–AC-005 | — |
| TASK-002 | Write unit tests for LRU cache — Get/Set, TTL expiry, size eviction, size tracking, invalidation, disabled when size=0 | 30min | AC-009–AC-012, AC-016 | — |
| TASK-003 | Write unit tests for singleflight integration — concurrent requests produce 1 fetch, errors not cached | 30min | AC-006–AC-008 | — |
| TASK-004 | Write unit tests for Prometheus metrics — LRU hit/miss counters, size gauge, singleflight dedup counter | 15min | AC-013–AC-015 | — |
| TASK-005 | Write unit test for config field — `lru_cache_size_mb` in AppConfig, env var override, default | 15min | AC-010 | — |

**Phase Output:** All tests fail (RED).

### Phase 2: GREEN — Minimal Implementation (3.5h)

#### Sub-phase 2a: Infrastructure (45min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-006 | Add `LRUCacheSizeMB` field to `AppConfig` in `internal/config/loader.go` | 10min | AC-010 | — |
| TASK-007 | Add LRU + singleflight Prometheus collectors to `internal/metrics/metrics.go` — `lru_cache_size_bytes`, `lru_cache_hits_total`, `lru_cache_misses_total`, `singleflight_dedup_total` | 20min | AC-013–AC-015 | — |
| TASK-008 | `go get golang.org/x/sync` and `go get github.com/hashicorp/golang-lru/v2` | 15min | — | — |

#### Sub-phase 2b: LRU Cache (1h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-009 | Create `internal/cache/lru.go` — `LRUCache` interface (`Get`, `Set`, `Invalidate`, `SizeBytes`) + `NewLRUCache(maxBytes int64)` constructor | 15min | AC-009 | TASK-008 |
| TASK-010 | Implement size-bounded LRU with TTL expiry — wrap hashicorp LRU with size accounting, expiry-on-read, and eviction callback | 45min | AC-009, AC-011, AC-012, AC-016 | TASK-009 |

#### Sub-phase 2c: Gzip Middleware (45min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-011 | Create `internal/middleware/gzip.go` — wraps ResponseWriter, buffers initial bytes for size check, compresses JSON/XML, sets headers | 45min | AC-001–AC-005 | — |

#### Sub-phase 2d: Handler Integration (1h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-012 | Integrate singleflight into `CacheHandler` — add `singleflight.Group` field, wrap cache-lookup + fetch in `Do()`, `Forget()` on error | 30min | AC-006–AC-008 | TASK-008 |
| TASK-013 | Integrate LRU into `CacheHandler` — check LRU before MongoDB on Get, populate LRU after MongoDB read or upstream fetch, invalidate on force-refresh | 20min | AC-009, AC-012, AC-016 | TASK-010 |
| TASK-014 | Wire LRU metrics — increment hit/miss counters in handler, update size gauge after Set/Invalidate, increment singleflight dedup counter | 10min | AC-013–AC-015 | TASK-007, TASK-012, TASK-013 |

#### Sub-phase 2e: Wiring (15min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-015 | Update `cmd/server/main.go` — read LRU config, resolve env override, create LRU cache, pass to CacheHandler, wrap mux with gzip middleware | 10min | AC-010 | TASK-006, TASK-010, TASK-011 |
| TASK-016 | Pass LRU cache to `Scheduler` via option — populate LRU after successful scheduled fetch | 5min | AC-017 | TASK-010 |

**Phase Output:** All tests pass (GREEN).

### Phase 3: REFACTOR (30min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-017 | Review gzip middleware — ensure streaming works correctly, no double-close on writer, proper Vary header | 15min | — | Phase 2 |
| TASK-018 | Run full test suite, `go vet ./...`, verify no regressions | 15min | AC-018 | Phase 2 |

### Phase 4: VERIFY (30min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-019 | Run `make test-coverage` — verify ≥ 80% | 10min | — | Phase 3 |
| TASK-020 | Spec compliance check — verify each AC has a passing test | 10min | — | Phase 3 |
| TASK-021 | Regenerate swagger docs (503 response from limiter may be in same build) | 10min | — | Phase 3 |

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/config/loader.go` | Modify | Add `LRUCacheSizeMB` to `AppConfig` |
| `internal/config/loader_test.go` | Modify | Test new config field |
| `internal/metrics/metrics.go` | Modify | Add LRU + singleflight collectors |
| `internal/cache/lru.go` | Create | LRU cache interface + implementation |
| `internal/cache/lru_test.go` | Create | LRU unit tests |
| `internal/middleware/gzip.go` | Create | Gzip compression middleware |
| `internal/middleware/gzip_test.go` | Create | Gzip middleware unit tests |
| `internal/handler/cache.go` | Modify | Add singleflight + LRU integration |
| `internal/handler/cache_test.go` | Modify | Test singleflight + LRU behavior |
| `internal/scheduler/scheduler.go` | Modify | Add LRU population after fetch |
| `cmd/server/main.go` | Modify | Wire LRU, gzip middleware, config |
| `go.mod` / `go.sum` | Modify | Add singleflight + hashicorp-lru deps |

## Effort Summary

| Phase | Estimated Hours |
|-------|-----------------|
| Phase 1: RED | 2h |
| Phase 2: GREEN | 3.5h |
| Phase 3: REFACTOR | 0.5h |
| Phase 4: VERIFY | 0.5h |
| **Total** | **6.5h** |

## Dependencies

### External
- `golang.org/x/sync/singleflight` — request coalescing
- `github.com/hashicorp/golang-lru/v2` — LRU cache foundation

### Internal
- `internal/cache` — add LRU layer alongside MongoDB Repository
- `internal/handler` — integrate singleflight + LRU into CacheHandler
- `internal/middleware` — new package (also used by connection-resilience spec)
- `internal/metrics` — add 4 new collectors
- `internal/scheduler` — optional LRU population

### Cross-Spec
- `connection-resilience` also creates `internal/middleware/` — if implemented first, gzip middleware adds to existing package; if implemented second, create the package here

## Architecture: Request Flow After Implementation

```
Client Request
  → Gzip Middleware (wrap ResponseWriter)
    → Concurrency Limiter (from connection-resilience spec)
      → CacheHandler.ServeHTTP
        → Singleflight.Do(cacheKey)
          → LRU Cache.Get(cacheKey)
            → [hit] → respond with cached data
            → [miss] → MongoDB.Get(cacheKey)
              → [hit] → populate LRU → respond
              → [miss] → Upstream.Fetch → Upsert MongoDB → populate LRU → respond
```

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| LRU size estimation inaccurate for large responses | Medium | Low | Use `len(json.Marshal(data))` as rough estimate; exact size less important than order of magnitude |
| Gzip middleware interferes with `X-Cache-Stale` headers | Low | Medium | Gzip wraps the writer but passes through all header writes; test explicitly |
| Singleflight `Do()` holds waiters during slow upstream fetch | Medium | Medium | Expected behavior — waiters get result faster than independent fetches. Can add timeout if needed later |
| hashicorp-lru v2 API changes | Very Low | Low | Pin version in go.mod |
| LRU cache stale after scheduler updates MongoDB | Low | Medium | AC-017 addresses this — scheduler populates LRU directly |

## Testing Strategy

1. **Unit tests — Gzip middleware** (Phase 1)
   - Compressed JSON response with Accept-Encoding
   - Compressed XML response
   - Uncompressed when no Accept-Encoding
   - Skip compression for < 1 KB response
   - Content-Encoding header set correctly
   - Vary header set

2. **Unit tests — LRU cache** (Phase 1)
   - Get/Set basic operations
   - TTL expiry on read
   - Size eviction when full
   - Invalidation
   - Disabled when size = 0
   - Concurrent access safety

3. **Unit tests — Singleflight** (Phase 1)
   - Concurrent requests coalesced into 1 DB call
   - Error not shared (Forget on error)
   - Different keys are independent

4. **Regression** (Phase 3) — full `make test-unit`
5. **Coverage** (Phase 4) — `make test-coverage` ≥ 80%

## Spec Traceability

| AC | Tasks | Test Coverage |
|----|-------|---------------|
| AC-001 | TASK-001, TASK-011 | gzip_test.go |
| AC-002 | TASK-001, TASK-011 | gzip_test.go |
| AC-003 | TASK-001, TASK-011 | gzip_test.go |
| AC-004 | TASK-001, TASK-011 | gzip_test.go |
| AC-005 | TASK-001, TASK-011 | gzip_test.go |
| AC-006 | TASK-003, TASK-012 | cache_test.go |
| AC-007 | TASK-003, TASK-012 | cache_test.go |
| AC-008 | TASK-003, TASK-012 | cache_test.go |
| AC-009 | TASK-002, TASK-010, TASK-013 | lru_test.go |
| AC-010 | TASK-005, TASK-006, TASK-015 | loader_test.go |
| AC-011 | TASK-002, TASK-010 | lru_test.go |
| AC-012 | TASK-002, TASK-013 | lru_test.go, cache_test.go |
| AC-013 | TASK-004, TASK-007, TASK-014 | cache_test.go |
| AC-014 | TASK-004, TASK-007, TASK-014 | cache_test.go |
| AC-015 | TASK-004, TASK-007, TASK-014 | cache_test.go |
| AC-016 | TASK-002, TASK-013 | lru_test.go, cache_test.go |
| AC-017 | TASK-016 | scheduler_test.go |
| AC-018 | TASK-018 | existing test suite |

## Next Steps

1. Review and approve this plan
2. Implement `connection-resilience` first (creates `internal/middleware/`)
3. Run `/add:tdd-cycle specs/response-optimization.md` to execute

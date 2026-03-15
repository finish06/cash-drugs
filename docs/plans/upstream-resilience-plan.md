# Implementation Plan: Upstream Resilience

**Spec Version:** 0.1.0
**Spec:** specs/upstream-resilience.md
**Created:** 2026-03-14
**Team Size:** Solo (1 agent)
**Estimated Duration:** 6-7 hours

## Overview

Add per-endpoint circuit breakers to stop cascading failures from flaky upstreams, and force-refresh rate limiting (30s per-key cooldown) to prevent upstream abuse via `_force=true`. Integrates with the existing handler, scheduler, and fetchlock systems.

## Objectives

- Per-slug circuit breakers (5 failures → open 30s → half-open probe)
- Stale cache fallback when circuit is open
- Force-refresh cooldown (30s per cache key)
- Scheduler respects circuit state
- Circuit and cooldown metrics exported to Prometheus

## Acceptance Criteria Analysis

### AC-001–AC-006: Circuit breaker state machine (Must)
- **Complexity:** Medium
- **Effort:** 2h
- **Approach:** Use `github.com/sony/gobreaker` — it implements the full closed → open → half-open state machine. Create one `gobreaker.CircuitBreaker` per slug. Wrap upstream `Fetch()` calls in `cb.Execute()`. Configure `MaxRequests=1` (half-open probe), `Interval=0` (consecutive count, not windowed), `Timeout=30s` (open duration), `ReadyToTrip` fires after 5 consecutive failures.

### AC-007, AC-008: Circuit-open response behavior (Must)
- **Complexity:** Medium
- **Effort:** 1h
- **Approach:** In `CacheHandler`, when circuit is open (`gobreaker.ErrOpenState`), attempt stale cache fallback. If stale exists, serve with `stale_reason: "circuit_open"`. If no cache, return 503 with retry_after. Same pattern for scheduler — skip fetch and log.

### AC-009–AC-011: Force-refresh cooldown (Must)
- **Complexity:** Medium
- **Effort:** 1h
- **Approach:** New `internal/upstream/cooldown.go` — a `sync.Map` of cache key → `time.Time` (last refresh). On force-refresh, check if last refresh was within cooldown window. If so, serve from cache with `X-Force-Cooldown: true` header. After successful force-refresh, record timestamp.

### AC-012–AC-014: Prometheus metrics (Should)
- **Complexity:** Simple
- **Effort:** 30min
- **Approach:** Add `CircuitState` gauge, `CircuitRejectionsTotal` counter, `ForceRefreshCooldownTotal` counter to `Metrics` struct. Update in handler on circuit rejection and cooldown rejection.

### AC-015: Scheduler circuit awareness (Must)
- **Complexity:** Simple
- **Effort:** 30min
- **Approach:** Pass circuit breaker registry to `Scheduler` via option. In `fetchEndpoint()`, check circuit state before attempting fetch. If open, log warning and skip.

### AC-016, AC-017: Configurable thresholds (Should)
- **Complexity:** Simple
- **Effort:** 20min
- **Approach:** Env var resolution in `main.go` — `CIRCUIT_FAILURE_THRESHOLD`, `CIRCUIT_OPEN_DURATION`, `FORCE_REFRESH_COOLDOWN`. Pass to constructors.

## Implementation Phases

### Phase 1: RED — Write Failing Tests (2h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-001 | Write unit tests for circuit breaker registry — per-slug isolation, state transitions (closed → open after 5 failures → half-open after 30s → closed on success / re-open on failure) | 30min | AC-001–AC-006 | — |
| TASK-002 | Write unit tests for handler circuit-open behavior — stale fallback with `stale_reason: "circuit_open"`, 503 when no cache exists | 20min | AC-007, AC-008 | — |
| TASK-003 | Write unit tests for force-refresh cooldown — cooldown within window returns cached + header, expired cooldown allows refresh, key granularity (slug + params) | 25min | AC-009–AC-011 | — |
| TASK-004 | Write unit tests for scheduler circuit awareness — skips fetch when open, proceeds when closed | 15min | AC-015 | — |
| TASK-005 | Write unit tests for Prometheus metrics — circuit state gauge, rejection counter, cooldown counter | 15min | AC-012–AC-014 | — |
| TASK-006 | Write unit tests for configurable thresholds — custom failure count, open duration, cooldown duration | 15min | AC-016, AC-017 | — |

**Phase Output:** All tests fail (RED).

### Phase 2: GREEN — Minimal Implementation (3h)

#### Sub-phase 2a: Dependencies + Infrastructure (30min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-007 | `go get github.com/sony/gobreaker/v2` | 5min | — | — |
| TASK-008 | Add circuit + cooldown Prometheus collectors to `internal/metrics/metrics.go` — `circuit_state`, `circuit_rejections_total`, `force_refresh_cooldown_total` | 20min | AC-012–AC-014 | — |
| TASK-009 | Add env var resolution for thresholds in `cmd/server/main.go` | 5min | AC-016, AC-017 | — |

#### Sub-phase 2b: Circuit Breaker (1h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-010 | Create `internal/upstream/circuit.go` — `CircuitRegistry` that lazily creates per-slug `gobreaker.CircuitBreaker` instances with configurable settings | 30min | AC-001–AC-006 | TASK-007 |
| TASK-011 | Add `CircuitRegistry` method: `Execute(slug, fn)` wraps a function call in the slug's circuit breaker. Returns `ErrCircuitOpen` if circuit is open. | 15min | AC-001–AC-006 | TASK-010 |
| TASK-012 | Add `CircuitRegistry` method: `State(slug)` returns current circuit state for Prometheus reporting | 15min | AC-012 | TASK-010 |

#### Sub-phase 2c: Force-Refresh Cooldown (30min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-013 | Create `internal/upstream/cooldown.go` — `CooldownTracker` with `Check(key) bool` and `Record(key)` methods. Uses `sync.Map` internally. Configurable duration. | 30min | AC-009–AC-011 | — |

#### Sub-phase 2d: Handler + Scheduler Integration (1h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-014 | Integrate `CircuitRegistry` into `CacheHandler` — wrap upstream fetch in `circuit.Execute()`, handle `ErrCircuitOpen` with stale fallback or 503 | 25min | AC-003, AC-007, AC-008 | TASK-010 |
| TASK-015 | Integrate `CooldownTracker` into `CacheHandler` — check cooldown before force-refresh, record after successful refresh, set `X-Force-Cooldown` header | 15min | AC-009–AC-011 | TASK-013 |
| TASK-016 | Wire Prometheus metrics in handler — update circuit state gauge, increment rejection/cooldown counters | 10min | AC-012–AC-014 | TASK-008, TASK-014, TASK-015 |
| TASK-017 | Integrate `CircuitRegistry` into `Scheduler` — check circuit state before `fetchEndpoint()`, skip with warning log if open | 10min | AC-015 | TASK-010 |

#### Sub-phase 2e: Wiring (10min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-018 | Update `cmd/server/main.go` — create `CircuitRegistry` and `CooldownTracker`, pass to handler and scheduler via options | 10min | — | TASK-010, TASK-013 |

**Phase Output:** All tests pass (GREEN).

### Phase 3: REFACTOR (30min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-019 | Review circuit/cooldown integration — ensure FetchLock dedup still works correctly alongside circuit breaker | 15min | — | Phase 2 |
| TASK-020 | Run full test suite, `go vet ./...`, verify no regressions | 15min | AC-018 | Phase 2 |

### Phase 4: VERIFY (30min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-021 | Run `make test-coverage` — verify ≥ 80% | 10min | — | Phase 3 |
| TASK-022 | Spec compliance check — verify each AC has a passing test | 10min | — | Phase 3 |
| TASK-023 | Regenerate swagger docs (new 503 response for circuit open) | 10min | — | Phase 3 |

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/upstream/circuit.go` | Create | CircuitRegistry — per-slug gobreaker instances |
| `internal/upstream/circuit_test.go` | Create | Circuit breaker unit tests |
| `internal/upstream/cooldown.go` | Create | CooldownTracker — per-key force-refresh throttle |
| `internal/upstream/cooldown_test.go` | Create | Cooldown unit tests |
| `internal/metrics/metrics.go` | Modify | Add circuit + cooldown Prometheus collectors |
| `internal/handler/cache.go` | Modify | Integrate circuit breaker + cooldown |
| `internal/handler/cache_test.go` | Modify | Test circuit/cooldown handler behavior |
| `internal/scheduler/scheduler.go` | Modify | Check circuit state before fetch |
| `internal/scheduler/scheduler_test.go` | Modify | Test scheduler circuit awareness |
| `cmd/server/main.go` | Modify | Wire circuit + cooldown, env var resolution |
| `go.mod` / `go.sum` | Modify | Add gobreaker dependency |

## Effort Summary

| Phase | Estimated Hours |
|-------|-----------------|
| Phase 1: RED | 2h |
| Phase 2: GREEN | 3h |
| Phase 3: REFACTOR | 0.5h |
| Phase 4: VERIFY | 0.5h |
| **Total** | **6h** |

## Dependencies

### External
- `github.com/sony/gobreaker/v2` — circuit breaker implementation

### Internal
- `internal/upstream` — new files in existing package
- `internal/handler` — integrate into CacheHandler
- `internal/scheduler` — integrate into fetchEndpoint
- `internal/metrics` — add 3 new collectors
- `internal/fetchlock` — still operates alongside circuit breaker (dedup within, circuit wraps outside)

### Cross-Spec
- **connection-resilience:** Concurrency limiter runs _before_ circuit breaker in the middleware chain (limiter → handler → circuit → upstream)
- **response-optimization:** Singleflight coalesces requests _before_ circuit breaker check (singleflight → circuit → upstream). This is correct — one coalesced request hits the circuit, not N.

## Architecture: Integration with Existing Systems

```
Request arrives
  → Concurrency Limiter (connection-resilience)
    → CacheHandler.ServeHTTP
      → Singleflight.Do(cacheKey)  (response-optimization)
        → LRU Check  (response-optimization)
        → MongoDB Check
        → Force-refresh cooldown check  ← NEW
          → [cooldown active] → serve cache + X-Force-Cooldown
        → CircuitRegistry.Execute(slug)  ← NEW
          → [circuit open] → stale fallback or 503
          → [circuit closed/half-open] → upstream.Fetch()
            → FetchLock dedup (existing)
            → Actual HTTP call
```

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| gobreaker state transitions don't match spec exactly | Low | Medium | gobreaker is mature, well-tested; write explicit state transition tests |
| Circuit breaker + fetchlock interaction causes deadlock | Low | High | Circuit wraps outside fetchlock — if circuit is open, fetchlock is never acquired. Test both paths. |
| Cooldown `sync.Map` grows unbounded with unique cache keys | Medium | Low | Keys are finite (11 configured endpoints × limited param combos). Add optional cleanup if needed later. |
| Half-open probe fails due to unrelated network blip | Medium | Low | Expected behavior — circuit re-opens. Operator can monitor via `circuit_state` gauge. |
| Force-refresh cooldown conflicts with singleflight | Low | Low | Cooldown check happens before singleflight — if cooled down, request never enters singleflight group |

## Testing Strategy

1. **Unit tests — Circuit breaker** (Phase 1)
   - Per-slug isolation (different slugs have independent circuits)
   - State transitions: closed → open (5 failures) → half-open (30s) → closed (success)
   - Half-open failure re-opens circuit
   - Configurable thresholds

2. **Unit tests — Handler integration** (Phase 1)
   - Circuit open → stale cache served with `stale_reason: "circuit_open"`
   - Circuit open → no cache → 503 with `retry_after`
   - Circuit closed → normal flow

3. **Unit tests — Cooldown** (Phase 1)
   - Within cooldown → returns true (blocked)
   - After cooldown → returns false (allowed)
   - Key includes full cache key (slug + params)
   - `X-Force-Cooldown: true` header set

4. **Unit tests — Scheduler** (Phase 1)
   - Circuit open → skip fetch with warning log
   - Circuit closed → fetch proceeds normally

5. **Regression** (Phase 3) — full `make test-unit`
6. **Coverage** (Phase 4) — `make test-coverage` ≥ 80%

## Spec Traceability

| AC | Tasks | Test Coverage |
|----|-------|---------------|
| AC-001 | TASK-001, TASK-010 | circuit_test.go |
| AC-002 | TASK-001, TASK-010 | circuit_test.go |
| AC-003 | TASK-002, TASK-014 | cache_test.go |
| AC-004 | TASK-001, TASK-010 | circuit_test.go |
| AC-005 | TASK-001, TASK-010 | circuit_test.go |
| AC-006 | TASK-001, TASK-010 | circuit_test.go |
| AC-007 | TASK-002, TASK-014 | cache_test.go |
| AC-008 | TASK-002, TASK-014 | cache_test.go |
| AC-009 | TASK-003, TASK-013, TASK-015 | cooldown_test.go, cache_test.go |
| AC-010 | TASK-003, TASK-013 | cooldown_test.go |
| AC-011 | TASK-003, TASK-015 | cache_test.go |
| AC-012 | TASK-005, TASK-008, TASK-016 | cache_test.go |
| AC-013 | TASK-005, TASK-008, TASK-016 | cache_test.go |
| AC-014 | TASK-005, TASK-008, TASK-016 | cache_test.go |
| AC-015 | TASK-004, TASK-017 | scheduler_test.go |
| AC-016 | TASK-006, TASK-009 | circuit_test.go |
| AC-017 | TASK-006, TASK-009 | cooldown_test.go |
| AC-018 | TASK-020 | existing test suite |

## Next Steps

1. Review and approve this plan
2. Implement after `connection-resilience` and `response-optimization` (depends on middleware package and singleflight)
3. Run `/add:tdd-cycle specs/upstream-resilience.md` to execute

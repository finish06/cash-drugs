# Implementation Plan: Connection Resilience

**Spec Version:** 0.1.0
**Spec:** specs/connection-resilience.md
**Created:** 2026-03-14
**Team Size:** Solo (1 agent)
**Estimated Duration:** 4-5 hours

## Overview

Add concurrency limiting middleware, HTTP server timeouts, and health/metrics isolation to prevent service collapse under load. This is the highest-priority performance fix — the stress test showed 78% failure rate at 100 concurrent connections.

## Objectives

- Cap in-flight requests at a configurable limit (default: 50)
- Return 503 + Retry-After instead of connection refused
- Exempt `/health` and `/metrics` from concurrency limits
- Configure server-level read/write/idle timeouts
- Expose inflight and rejection metrics to Prometheus

## Acceptance Criteria Analysis

### AC-001: Concurrency limiter middleware (Must)
- **Complexity:** Medium
- **Effort:** 1.5h
- **Tasks:** TASK-001, TASK-002
- **Approach:** Channel-based semaphore (simpler than `x/sync/semaphore` for this use case — `make(chan struct{}, limit)`). Middleware wraps the mux, attempts non-blocking send to channel, defers receive.

### AC-002, AC-003, AC-004: 503 response with Retry-After (Must)
- **Complexity:** Simple
- **Effort:** 30min
- **Tasks:** TASK-002
- **Approach:** When semaphore is full, write JSON error response with `Retry-After` header. Reuse `model.ErrorResponse` — add `RetryAfter` field.

### AC-005: Health/metrics exemption (Must)
- **Complexity:** Simple
- **Effort:** 30min
- **Tasks:** TASK-003
- **Approach:** In `main.go`, register `/health` and `/metrics` directly on the outer mux without the limiter middleware. Only wrap application routes (`/api/cache/`, `/api/endpoints`, `/swagger/`, `/openapi.json`).

### AC-006: Server timeouts (Must)
- **Complexity:** Simple
- **Effort:** 15min
- **Tasks:** TASK-004
- **Approach:** Set `ReadTimeout`, `WriteTimeout`, `IdleTimeout` on the `http.Server` struct in `main.go`.

### AC-007: Config field + env override (Must)
- **Complexity:** Simple
- **Effort:** 30min
- **Tasks:** TASK-005
- **Approach:** Add `MaxConcurrentRequests int` to `AppConfig` struct. In `main.go`, read from config, then check `MAX_CONCURRENT_REQUESTS` env var override. Default to 50.

### AC-008, AC-009: Prometheus metrics (Should)
- **Complexity:** Simple
- **Effort:** 30min
- **Tasks:** TASK-006
- **Approach:** Add `InFlightRequests` gauge and `RejectedRequestsTotal` counter to `Metrics` struct. Middleware increments/decrements gauge and increments counter on rejection.

### AC-010, AC-011: No regression (Must)
- **Complexity:** Simple
- **Effort:** 30min
- **Tasks:** TASK-008
- **Approach:** Run full test suite after implementation.

## Implementation Phases

### Phase 1: RED — Write Failing Tests (1.5h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-001 | Write unit tests for concurrency limiter middleware — test admission, rejection, semaphore release on completion, panic safety (defer) | 45min | AC-001, AC-002, AC-003, AC-004 | — |
| TASK-002 | Write unit tests for health/metrics bypass — verify exempt paths skip the limiter | 20min | AC-005 | — |
| TASK-003 | Write unit tests for config loading — `max_concurrent_requests` field in AppConfig, env var override, default value, invalid values | 15min | AC-007 | — |
| TASK-004 | Write unit test verifying Prometheus inflight gauge and rejection counter are updated | 10min | AC-008, AC-009 | — |

**Phase Output:** All tests fail (RED). No implementation yet.

### Phase 2: GREEN — Minimal Implementation (2h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-005 | Add `MaxConcurrentRequests` field to `AppConfig` in `internal/config/loader.go` | 15min | AC-007 | — |
| TASK-006 | Add `InFlightRequests` gauge and `RejectedRequestsTotal` counter to `internal/metrics/metrics.go` | 15min | AC-008, AC-009 | — |
| TASK-007 | Create `internal/middleware/limiter.go` — channel-based concurrency limiter middleware with 503 + Retry-After response | 45min | AC-001, AC-002, AC-003, AC-004 | TASK-005, TASK-006 |
| TASK-008 | Update `cmd/server/main.go` — read config, resolve env override, wire limiter middleware around application routes only, set server timeouts | 30min | AC-005, AC-006, AC-007 | TASK-005, TASK-006, TASK-007 |
| TASK-009 | Add `RetryAfter` field to `model.ErrorResponse` (or use inline struct in middleware to avoid coupling) | 10min | AC-004 | — |

**Phase Output:** All tests pass (GREEN).

### Phase 3: REFACTOR (30min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-010 | Review middleware for clean separation, ensure defer safety, tidy imports | 15min | — | Phase 2 |
| TASK-011 | Run full existing test suite, `go vet ./...`, verify no regressions | 15min | AC-010, AC-011 | Phase 2 |

**Phase Output:** Clean code, all tests pass.

### Phase 4: VERIFY (30min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-012 | Run `make test-coverage` — verify coverage meets 80% threshold | 10min | — | Phase 3 |
| TASK-013 | Update swag annotations if new error response added, regenerate swagger docs | 10min | — | Phase 3 |
| TASK-014 | Spec compliance check — verify each AC has a passing test | 10min | — | Phase 3 |

**Phase Output:** All gates pass, spec compliance verified.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/config/loader.go` | Modify | Add `MaxConcurrentRequests` to `AppConfig` |
| `internal/config/loader_test.go` | Modify | Test new config field |
| `internal/middleware/limiter.go` | Create | Concurrency limiter middleware |
| `internal/middleware/limiter_test.go` | Create | Limiter unit tests |
| `internal/metrics/metrics.go` | Modify | Add inflight gauge + rejection counter |
| `internal/model/response.go` | Modify | Add `RetryAfter` field to `ErrorResponse` |
| `cmd/server/main.go` | Modify | Wire middleware, set server timeouts, read config |
| `docs/swagger.json` | Regenerate | Add 503 response to endpoint docs |
| `docs/swagger.yaml` | Regenerate | Add 503 response to endpoint docs |
| `docs/docs.go` | Regenerate | Swagger codegen |

## Effort Summary

| Phase | Estimated Hours |
|-------|-----------------|
| Phase 1: RED | 1.5h |
| Phase 2: GREEN | 2h |
| Phase 3: REFACTOR | 0.5h |
| Phase 4: VERIFY | 0.5h |
| **Total** | **4.5h** |

## Dependencies

### External
- None — all work is internal to the service

### Internal
- Existing `internal/metrics` package (add 2 new collectors)
- Existing `internal/config` package (add 1 field)
- Existing `internal/model` package (add 1 field)

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Middleware breaks existing handler tests | Low | Medium | Run full suite in Phase 3; middleware only wraps mux, doesn't change handlers |
| Channel semaphore leaks on panic | Low | High | Use `defer` to release semaphore; test panic scenario explicitly |
| WriteTimeout kills legitimate slow responses (bulk endpoints) | Medium | Medium | 30s is generous — stress test showed max 13s for slowest endpoint |

## Testing Strategy

1. **Unit tests** (Phase 1) — test limiter in isolation with `httptest.Server`
2. **Config tests** (Phase 1) — test field parsing, defaults, env override
3. **Regression** (Phase 3) — full `make test-unit`
4. **Coverage** (Phase 4) — `make test-coverage` ≥ 80%

## Spec Traceability

| AC | Tasks | Test Coverage |
|----|-------|---------------|
| AC-001 | TASK-001, TASK-007 | limiter_test.go |
| AC-002 | TASK-001, TASK-007 | limiter_test.go |
| AC-003 | TASK-001, TASK-007 | limiter_test.go |
| AC-004 | TASK-001, TASK-007, TASK-009 | limiter_test.go |
| AC-005 | TASK-002, TASK-008 | limiter_test.go |
| AC-006 | TASK-008 | main.go (config verification) |
| AC-007 | TASK-003, TASK-005, TASK-008 | loader_test.go |
| AC-008 | TASK-004, TASK-006 | limiter_test.go |
| AC-009 | TASK-004, TASK-006 | limiter_test.go |
| AC-010 | TASK-011 | existing test suite |
| AC-011 | TASK-011 | existing test suite |

## Next Steps

1. Review and approve this plan
2. Run `/add:tdd-cycle specs/connection-resilience.md` to execute

# Implementation Plan: Stack-Wide Health & Version Compliance

**Spec Version:** 0.1.0
**Spec:** specs/stack-health-version-spec.md
**Created:** 2026-04-11
**Team Size:** Solo
**Estimated Duration:** ~3 hours

## Overview

Rework `/health` to match the stack-wide contract (uptime, start_time, structured dependencies) and strip runtime fields from `/version`. Reference implementation is rx-dag. This is a breaking change to the `/health` response (removes flat `db` field).

## Objectives

- `/health` returns structured `dependencies` array with measured latency
- `/health` carries `uptime`, `start_time`, `cache_slug_count`, `leader`
- `/version` contains only build-time fields (no runtime)
- `build_date` → `build_time` rename
- k6 smoke test updated and passing

## Success Criteria

- All 17 acceptance criteria met
- All existing tests still pass (or updated to match new contract)
- k6 smoke test passes against staging
- Coverage >= 85%

## Acceptance Criteria Analysis

### AC-001 through AC-009: /health rework
- **Complexity**: Medium
- **Tasks**: Rewrite `HealthHandler.ServeHTTP`, define new response structs, extend pinger to return latency, pass start_time + slug count + leader flag into handler
- **Risk**: `cache.Pinger` interface change affects callers

### AC-010 through AC-013: /version cleanup
- **Complexity**: Simple
- **Tasks**: Remove runtime fields from `VersionInfo`, rename `BuildDate` → `BuildTime` (and JSON tag), update `NewVersionHandler` signature (drop endpointCount/leader options)

### AC-014: Remove flat `db` field
- **Complexity**: Trivial — just don't emit it
- **Risk**: Intentional breaking change

### AC-016: k6 smoke test update
- **Complexity**: Simple

### AC-017: Docker HEALTHCHECK compat
- **Complexity**: Simple — verify HEALTHCHECK (if any) still parses `status` field

## Implementation Phases

### Phase 1: Interface Changes (foundation)

| Task | Description | Effort | AC |
|------|-------------|--------|-----|
| TASK-01 | Extend `cache.Pinger` to `PingWithLatency() (time.Duration, error)` OR add new `LatencyPinger` interface. Adapt MongoRepo. | 30min | AC-005, AC-006 |

### Phase 2: /health Handler Rewrite

| Task | Description | Effort | AC |
|------|-------------|--------|-----|
| TASK-02 | Define `HealthResponse` and `Dependency` structs in `internal/handler/health.go` | 15min | AC-001–009 |
| TASK-03 | Rewrite `HealthHandler` — takes `startTime time.Time`, `slugCount int`, `leader bool`, builds full response with measured latency | 45min | AC-001–009 |
| TASK-04 | Wire new fields in `cmd/server/main.go` (`NewHealthHandler` call site) | 10min | AC-003, AC-004, AC-008, AC-009 |

### Phase 3: /version Handler Cleanup

| Task | Description | Effort | AC |
|------|-------------|--------|-----|
| TASK-05 | Remove runtime fields from `VersionInfo` struct (uptime_seconds, start_time, endpoint_count, leader, hostname, gomaxprocs) | 10min | AC-012 |
| TASK-06 | Rename `BuildDate` → `BuildTime` (struct field + JSON tag `build_time`) | 5min | AC-011 |
| TASK-07 | Simplify `NewVersionHandler` signature — drop `endpointCount` param and `WithLeader` option | 10min | AC-012 |
| TASK-08 | Update `cmd/server/main.go` to pass renamed `buildTime` var and new signature | 10min | AC-011, AC-013 |

### Phase 4: Tests

| Task | Description | Effort | AC |
|------|-------------|--------|-----|
| TASK-09 | Rewrite `health_test.go` — assert new struct shape, dependencies array, healthy/unhealthy cases | 40min | AC-001–009 |
| TASK-10 | Rewrite `version_test.go` — assert only build-time fields present, `build_time` key, no runtime fields | 30min | AC-010–013 |
| TASK-11 | Update `tests/k6/smoke-test.js` — new field assertions for /health and /version | 15min | AC-016 |

### Phase 5: Docs & Polish

| Task | Description | Effort | AC |
|------|-------------|--------|-----|
| TASK-12 | Update swagger annotations on both handlers | 10min | AC-015 |
| TASK-13 | Verify Dockerfile HEALTHCHECK still works (if it uses /health) | 5min | AC-017 |
| TASK-14 | Update CHANGELOG with breaking change note | 5min | — |

## Effort Summary

| Phase | Effort |
|-------|--------|
| Phase 1 | 30min |
| Phase 2 | 70min |
| Phase 3 | 35min |
| Phase 4 | 85min |
| Phase 5 | 20min |
| **Total** | **~4h** |

## Risk Assessment

| Risk | Prob | Impact | Mitigation |
|------|------|--------|------------|
| Breaking `db` field affects consumers | Medium | Medium | Intentional per stack spec. Flag in CHANGELOG. No known cash-drugs consumers read `db` today. |
| `cache.Pinger` interface change cascades | Low | Low | Only `MongoRepo` implements it. Rebuild narrow. |
| Docker HEALTHCHECK uses old field | Low | High | Dockerfile uses `curl /health` with grep on `status` — preserved. |
| Removing /version runtime fields breaks dashboards | Low | Low | The new `/health` carries `uptime`, `start_time`, `leader` — migration is trivial. |

## File Change Summary

| File | Action |
|------|--------|
| `internal/cache/repository.go` (or pinger interface file) | Extend interface |
| `internal/handler/health.go` | Rewrite response shape |
| `internal/handler/health_test.go` | Rewrite tests |
| `internal/handler/version.go` | Remove runtime fields, rename build_time |
| `internal/handler/version_test.go` | Rewrite tests |
| `cmd/server/main.go` | Update handler wiring |
| `tests/k6/smoke-test.js` | Update field assertions |
| `CHANGELOG.md` | Breaking change note |
| `Dockerfile` | Verify HEALTHCHECK compat |

## Spec Traceability

| AC | Tasks |
|----|-------|
| AC-001 | TASK-02, TASK-03, TASK-09 |
| AC-002 | TASK-03, TASK-09 |
| AC-003 | TASK-03, TASK-04, TASK-09 |
| AC-004 | TASK-03, TASK-04, TASK-09 |
| AC-005 | TASK-01, TASK-02, TASK-03, TASK-09 |
| AC-006 | TASK-01, TASK-03, TASK-09 |
| AC-007 | TASK-03, TASK-09 |
| AC-008 | TASK-03, TASK-04, TASK-09 |
| AC-009 | TASK-03, TASK-04, TASK-09 |
| AC-010 | TASK-06, TASK-10 |
| AC-011 | TASK-06, TASK-08, TASK-10 |
| AC-012 | TASK-05, TASK-07, TASK-10 |
| AC-013 | TASK-08 |
| AC-014 | TASK-03 (just don't emit) |
| AC-015 | TASK-12 |
| AC-016 | TASK-11 |
| AC-017 | TASK-13 |

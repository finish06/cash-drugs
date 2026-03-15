# Implementation Plan: Version Endpoint

**Spec Version:** 0.1.0
**Spec:** specs/version-endpoint.md
**Created:** 2026-03-15
**Team Size:** Solo (1 agent)
**Estimated Duration:** 2.5 hours

## Overview

Add a `GET /version` endpoint that returns build and runtime metadata as JSON. Export build info and uptime as Prometheus metrics. Embed `version`, `gitCommit`, `gitBranch`, and `buildDate` via `-ldflags` at compile time. Exempt from the concurrency limiter.

## Objectives

- Create `VersionHandler` returning all build/runtime fields as JSON
- Embed build vars via `-ldflags` in Dockerfile
- Register `cashdrugs_build_info` and `cashdrugs_uptime_seconds` Prometheus gauges
- Register `/version` on outer mux (exempt from concurrency limiter)
- Add Swagger annotations

## Acceptance Criteria Analysis

### AC-001: Version endpoint returns all fields (Must)
- **Complexity:** Medium
- **Effort:** 30min
- **Tasks:** TASK-001, TASK-005
- **Approach:** `VersionHandler` struct with `startTime`, `endpointCount`, build vars. `ServeHTTP` builds `VersionInfo` struct with runtime values and JSON-encodes it.

### AC-002: Build vars via ldflags (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-005, TASK-009
- **Approach:** Package-level `var` declarations in `cmd/server/main.go`: `version`, `gitCommit`, `gitBranch`, `buildDate`. Default `version` to `"dev"`.

### AC-003, AC-004, AC-005, AC-006: Runtime fields (Must)
- **Complexity:** Simple
- **Effort:** 15min
- **Tasks:** TASK-005
- **Approach:** `runtime.Version()`, `runtime.GOOS`, `runtime.GOARCH`, `os.Hostname()`, `runtime.GOMAXPROCS(0)`. Hostname captured at startup, GOMAXPROCS read per request.

### AC-007: Uptime and start_time (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-001, TASK-005
- **Approach:** `startTime` recorded at handler creation. `uptime_seconds = time.Since(startTime).Seconds()` computed per request. `start_time` formatted as ISO 8601.

### AC-008: Endpoint count (Must)
- **Complexity:** Simple
- **Effort:** 5min
- **Tasks:** TASK-005
- **Approach:** Pass `len(endpoints)` to handler constructor.

### AC-009: Prometheus build_info gauge (Must)
- **Complexity:** Simple
- **Effort:** 15min
- **Tasks:** TASK-002, TASK-006
- **Approach:** `prometheus.NewGaugeVec` with labels `version`, `git_commit`, `go_version`, `build_date`. Set to 1 at registration time.

### AC-010: Prometheus uptime_seconds gauge (Must)
- **Complexity:** Simple
- **Effort:** 15min
- **Tasks:** TASK-002, TASK-006
- **Approach:** `prometheus.NewGauge` updated by `SystemCollector` interval loop (or standalone goroutine). Set to `time.Since(startTime).Seconds()`.

### AC-011: Exempt from concurrency limiter (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-003, TASK-007
- **Approach:** Register `/version` on the outer mux alongside `/health` and `/metrics`, outside the limiter-wrapped inner mux.

### AC-012: Dockerfile ldflags (Must)
- **Complexity:** Simple
- **Effort:** 15min
- **Tasks:** TASK-009
- **Approach:** Update `go build` command in Dockerfile to pass `-ldflags "-X main.version=$VERSION -X main.gitCommit=$(git rev-parse HEAD) -X main.gitBranch=$(git symbolic-ref --short HEAD) -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"`.

### AC-013: Swagger annotations (Should)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-010
- **Approach:** Add `// @Summary`, `// @Tags`, `// @Success` swag annotations to handler.

### AC-014: Existing tests pass (Must)
- **Complexity:** Simple
- **Effort:** 10min
- **Tasks:** TASK-008
- **Approach:** Run full test suite after implementation.

### AC-015: Content-Type application/json (Must)
- **Complexity:** Simple
- **Effort:** 5min
- **Tasks:** TASK-001, TASK-005
- **Approach:** Set `Content-Type: application/json` header in handler.

## Implementation Phases

### Phase 1: RED — Write Failing Tests (45min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-001 | Write unit tests for `VersionHandler` — verify all fields present in JSON response, `uptime_seconds` increases over time, `Content-Type` is `application/json`, dev build shows default values | 20min | AC-001, AC-002, AC-003, AC-004, AC-005, AC-006, AC-007, AC-008, AC-015 | — |
| TASK-002 | Write unit tests for Prometheus metrics — verify `cashdrugs_build_info` gauge exists with correct labels and value 1, `cashdrugs_uptime_seconds` gauge exists and updates | 15min | AC-009, AC-010 | — |
| TASK-003 | Write unit test for limiter exemption — verify `/version` responds 200 even when concurrency limit is saturated | 10min | AC-011 | — |

**Phase Output:** All tests fail (RED). No implementation yet.

### Phase 2: GREEN — Minimal Implementation (1h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-004 | Define `VersionInfo` response struct in `internal/handler/version.go` (or `internal/model/`) | 5min | AC-001 | — |
| TASK-005 | Implement `VersionHandler` in `internal/handler/version.go` — constructor takes `startTime`, `endpointCount`, build vars; `ServeHTTP` builds response with runtime values | 20min | AC-001, AC-002, AC-003, AC-004, AC-005, AC-006, AC-007, AC-008, AC-015 | TASK-004 |
| TASK-006 | Add `BuildInfo` gauge vec and `UptimeSeconds` gauge to `internal/metrics/metrics.go`. Set `BuildInfo` to 1 with labels at init. Update `UptimeSeconds` in `SystemCollector` loop or standalone goroutine. | 15min | AC-009, AC-010 | — |
| TASK-007 | Wire in `cmd/server/main.go` — declare build vars (`version`, `gitCommit`, `gitBranch`, `buildDate` with `"dev"` default), record `startTime`, create `VersionHandler`, register on outer mux exempt from limiter | 15min | AC-002, AC-011 | TASK-005, TASK-006 |

**Phase Output:** All tests pass (GREEN).

### Phase 3: REFACTOR (20min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-008 | Run full existing test suite, `go vet ./...`, verify no regressions | 10min | AC-014 | Phase 2 |
| TASK-009 | Update Dockerfile `-ldflags` to pass `gitCommit`, `gitBranch`, `buildDate` from build context | 10min | AC-012 | Phase 2 |

**Phase Output:** Clean code, all tests pass.

### Phase 4: VERIFY (15min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-010 | Add swag annotations to `VersionHandler`, regenerate swagger docs | 5min | AC-013 | Phase 3 |
| TASK-011 | Run `make test-coverage` — verify coverage meets 80% threshold | 5min | — | Phase 3 |
| TASK-012 | Spec compliance check — verify each AC has a passing test | 5min | — | Phase 3 |

**Phase Output:** All gates pass, spec compliance verified.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/handler/version.go` | Create | `VersionHandler` with `ServeHTTP`, `VersionInfo` struct |
| `internal/handler/version_test.go` | Create | Unit tests for version handler |
| `internal/metrics/metrics.go` | Modify | Add `BuildInfo` gauge vec and `UptimeSeconds` gauge |
| `internal/metrics/system_collector.go` | Modify | Update `UptimeSeconds` gauge in collection loop |
| `cmd/server/main.go` | Modify | Declare build vars, record startTime, wire handler on outer mux |
| `Dockerfile` | Modify | Update `-ldflags` to include gitCommit, gitBranch, buildDate |
| `docs/swagger.json` | Regenerate | Add /version endpoint docs |
| `docs/swagger.yaml` | Regenerate | Add /version endpoint docs |
| `docs/docs.go` | Regenerate | Swagger codegen |

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
- None — uses Go stdlib (`runtime`, `os`, `time`)

### Internal
- `internal/metrics/metrics.go` — add 2 new gauges
- `internal/metrics/system_collector.go` — update uptime gauge
- `cmd/server/main.go` — build vars and handler wiring

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| `os.Hostname()` fails in container | Low | Low | Fallback to `"unknown"`, log warning |
| Build vars not set in CI | Low | Medium | Default `version` to `"dev"`. CI pipeline must pass ldflags. |
| Existing `/health` tests affected by outer mux changes | Low | Medium | `/health` registration location unchanged — `/version` added alongside it |
| Swagger regeneration conflicts | Low | Low | Regenerate after all handler changes are complete |

## Testing Strategy

1. **Unit tests** (Phase 1) — handler response fields, Prometheus metrics, limiter exemption
2. **Regression** (Phase 3) — full `make test-unit`
3. **Coverage** (Phase 4) — `make test-coverage` >= 80%

## Spec Traceability

| AC | Tasks | Test Coverage |
|----|-------|---------------|
| AC-001 | TASK-001, TASK-005 | version_test.go (all fields present) |
| AC-002 | TASK-001, TASK-005, TASK-007 | version_test.go (build vars) |
| AC-003 | TASK-001, TASK-005 | version_test.go (go_version) |
| AC-004 | TASK-001, TASK-005 | version_test.go (os/arch) |
| AC-005 | TASK-001, TASK-005 | version_test.go (hostname) |
| AC-006 | TASK-001, TASK-005 | version_test.go (gomaxprocs) |
| AC-007 | TASK-001, TASK-005 | version_test.go (uptime increases) |
| AC-008 | TASK-001, TASK-005 | version_test.go (endpoint_count) |
| AC-009 | TASK-002, TASK-006 | metrics_test.go (build_info gauge) |
| AC-010 | TASK-002, TASK-006 | metrics_test.go (uptime gauge) |
| AC-011 | TASK-003, TASK-007 | version_test.go (limiter exempt) |
| AC-012 | TASK-009 | Dockerfile inspection |
| AC-013 | TASK-010 | swagger docs inspection |
| AC-014 | TASK-008 | existing test suite |
| AC-015 | TASK-001, TASK-005 | version_test.go (Content-Type) |

## Next Steps

1. Review and approve this plan
2. Run `/add:tdd-cycle specs/version-endpoint.md` to execute

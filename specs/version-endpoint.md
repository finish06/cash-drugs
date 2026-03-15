# Spec: Version Endpoint

**Version:** 0.1.0
**Created:** 2026-03-15
**PRD Reference:** docs/prd.md (M10)
**Status:** Draft

## 1. Overview

Add a `GET /version` endpoint that returns build and runtime metadata as JSON, and export the same information as Prometheus metrics. Currently, the service embeds a `version` string via `-ldflags` and exposes it only through the `/health` endpoint's response. Operators need a single endpoint to verify what build is running, when it was compiled, what Go version compiled it, how long it has been up, and how many endpoints are configured — all without parsing health check output or SSH-ing into the container. Additionally, Prometheus gauges (`cashdrugs_build_info` and `cashdrugs_uptime_seconds`) enable Grafana annotations on version changes and uptime monitoring dashboards.

### User Story

As an **operator**, I want a dedicated `/version` endpoint that shows build metadata, runtime info, and uptime, so that I can verify deployments at a glance and track version changes in Grafana via Prometheus metrics.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | `GET /version` returns HTTP 200 with a JSON body containing: `version`, `git_commit`, `git_branch`, `build_date`, `go_version`, `os`, `arch`, `hostname`, `gomaxprocs`, `uptime_seconds`, `endpoint_count`, `start_time` | Must |
| AC-002 | `version`, `git_commit`, `git_branch`, and `build_date` are embedded at compile time via `-ldflags "-X main.version=... -X main.gitCommit=... -X main.gitBranch=... -X main.buildDate=..."` | Must |
| AC-003 | `go_version` is populated from `runtime.Version()` at runtime | Must |
| AC-004 | `os` and `arch` are populated from `runtime.GOOS` and `runtime.GOARCH` | Must |
| AC-005 | `hostname` is populated from `os.Hostname()` at startup | Must |
| AC-006 | `gomaxprocs` is populated from `runtime.GOMAXPROCS(0)` | Must |
| AC-007 | `start_time` is an ISO 8601 timestamp recorded at server startup. `uptime_seconds` is calculated as `time.Since(startTime).Seconds()` at request time. | Must |
| AC-008 | `endpoint_count` is the number of configured endpoints from `config.yaml` | Must |
| AC-009 | A Prometheus gauge `cashdrugs_build_info` with value 1 and labels `version`, `git_commit`, `go_version`, `build_date` is registered and exported on `/metrics` | Must |
| AC-010 | A Prometheus gauge `cashdrugs_uptime_seconds` is updated by the existing `SystemCollector` interval (or a dedicated goroutine if SystemCollector is not running on the platform) to track process uptime | Must |
| AC-011 | `/version` is exempt from the concurrency limiter (registered on the outer mux alongside `/health` and `/metrics`) | Must |
| AC-012 | The `Dockerfile` is updated to pass `git_commit`, `git_branch`, and `build_date` via `-ldflags` using build args or shell commands | Must |
| AC-013 | Swagger/swag annotations are added to the `/version` handler for OpenAPI documentation | Should |
| AC-014 | All existing tests pass without modification | Must |
| AC-015 | The `/version` response has `Content-Type: application/json` | Must |

## 3. User Test Cases

### TC-001: Version endpoint returns all fields

**Precondition:** Service is running with build flags set (e.g., `version=v0.8.0`, `gitCommit=abc123`)
**Steps:**
1. Send `GET /version`
**Expected Result:** HTTP 200 with JSON body:
```json
{
  "version": "v0.8.0",
  "git_commit": "abc123",
  "git_branch": "main",
  "build_date": "2026-03-15T12:00:00Z",
  "go_version": "go1.24",
  "os": "linux",
  "arch": "amd64",
  "hostname": "cash-drugs-pod-1",
  "gomaxprocs": 4,
  "uptime_seconds": 3621.5,
  "endpoint_count": 8,
  "start_time": "2026-03-15T11:00:00Z"
}
```
**Maps to:** AC-001, AC-002, AC-003, AC-004, AC-005, AC-006, AC-007, AC-008

### TC-002: Version endpoint exempt from concurrency limiter

**Precondition:** Service is running with `MAX_CONCURRENT_REQUESTS=1`. One slow request is in-flight.
**Steps:**
1. Start a slow request to `/api/cache/drugnames`
2. While it's in-flight, send `GET /version`
**Expected Result:** `/version` returns 200 immediately, not blocked by the limiter.
**Maps to:** AC-011

### TC-003: Prometheus build_info metric exported

**Precondition:** Service is running
**Steps:**
1. Send `GET /metrics`
2. Search for `cashdrugs_build_info`
**Expected Result:** Output contains:
```
cashdrugs_build_info{version="v0.8.0",git_commit="abc123",go_version="go1.24",build_date="2026-03-15T12:00:00Z"} 1
```
**Maps to:** AC-009

### TC-004: Uptime seconds increases over time

**Precondition:** Service has been running for at least 10 seconds
**Steps:**
1. Send `GET /version`, note `uptime_seconds`
2. Wait 5 seconds
3. Send `GET /version` again, note `uptime_seconds`
**Expected Result:** Second `uptime_seconds` is approximately 5 greater than the first.
**Maps to:** AC-007

### TC-005: Dev build shows default values

**Precondition:** Service is running without `-ldflags` (local development: `go run ./cmd/server`)
**Steps:**
1. Send `GET /version`
**Expected Result:** `version: "dev"`, `git_commit: ""`, `git_branch: ""`, `build_date: ""`. Runtime fields (`go_version`, `hostname`, etc.) are still populated.
**Maps to:** AC-001, AC-002

## 4. Data Model

### New: `VersionInfo` (response struct, in `internal/handler/` or `internal/model/`)

| Field | Type | JSON Tag | Source |
|-------|------|----------|--------|
| `Version` | `string` | `version` | `-ldflags -X main.version` |
| `GitCommit` | `string` | `git_commit` | `-ldflags -X main.gitCommit` |
| `GitBranch` | `string` | `git_branch` | `-ldflags -X main.gitBranch` |
| `BuildDate` | `string` | `build_date` | `-ldflags -X main.buildDate` |
| `GoVersion` | `string` | `go_version` | `runtime.Version()` |
| `OS` | `string` | `os` | `runtime.GOOS` |
| `Arch` | `string` | `arch` | `runtime.GOARCH` |
| `Hostname` | `string` | `hostname` | `os.Hostname()` |
| `GOMAXPROCS` | `int` | `gomaxprocs` | `runtime.GOMAXPROCS(0)` |
| `UptimeSeconds` | `float64` | `uptime_seconds` | `time.Since(startTime).Seconds()` |
| `EndpointCount` | `int` | `endpoint_count` | `len(endpoints)` |
| `StartTime` | `string` | `start_time` | ISO 8601, recorded at startup |

### New Prometheus Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `cashdrugs_build_info` | Gauge (value=1) | `version`, `git_commit`, `go_version`, `build_date` | Build identity for Grafana annotations |
| `cashdrugs_uptime_seconds` | Gauge | (none) | Process uptime in seconds, updated periodically |

## 5. API Contract

### `GET /version`

**Response: 200 OK**
```json
{
  "version": "v0.8.0",
  "git_commit": "abc123def",
  "git_branch": "main",
  "build_date": "2026-03-15T12:00:00Z",
  "go_version": "go1.24",
  "os": "linux",
  "arch": "amd64",
  "hostname": "cash-drugs-7f8b9c-xk2p4",
  "gomaxprocs": 4,
  "uptime_seconds": 86421.3,
  "endpoint_count": 8,
  "start_time": "2026-03-14T12:00:00Z"
}
```

**Headers:**
- `Content-Type: application/json`

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| Binary built without `-ldflags` (local dev) | `version: "dev"`, commit/branch/build_date are empty strings |
| `os.Hostname()` fails | Set `hostname` to `"unknown"`, log warning |
| Extremely long uptime (months) | `uptime_seconds` is a float64 — handles large values without overflow |
| Multiple `/version` requests at the same instant | Each computes its own `uptime_seconds` — no shared mutable state |
| Container hostname changes (unlikely at runtime) | `hostname` is captured once at startup, not re-read per request |
| `GOMAXPROCS` changed at runtime via `runtime.GOMAXPROCS()` | `/version` reads current value per request (reflects runtime changes) |
| No endpoints configured (empty config) | `endpoint_count: 0` |

## 7. Dependencies

- `cmd/server/main.go` — add `gitCommit`, `gitBranch`, `buildDate` package-level vars, record `startTime`, wire `VersionHandler`
- `internal/handler/version.go` — new file: `VersionHandler` with `ServeHTTP`
- `internal/metrics/metrics.go` — add `BuildInfo` gauge vec and `UptimeSeconds` gauge
- `internal/metrics/system_collector.go` — update `UptimeSeconds` gauge in collection loop (or add standalone goroutine for non-Linux)
- `Dockerfile` — update `-ldflags` to include `gitCommit`, `gitBranch`, `buildDate` from build context
- `docs/swagger.go` or swag annotations — document `/version` endpoint

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-15 | 0.1.0 | calebdunn | Initial spec for M10 |

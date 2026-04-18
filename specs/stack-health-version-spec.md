# Spec: Stack-Wide Health & Version Endpoint Compliance

**Version:** 0.1.0
**Created:** 2026-04-11
**PRD Reference:** docs/prd.md
**Status:** Complete

## 1. Overview

Align cash-drugs' `/health` and `/version` endpoints with the stack-wide specification so that all services (rx-dag, cash-drugs, drug-gate, drugs-quiz BFF) expose a consistent, structured contract. The reference implementation is rx-dag.

Current gaps:
- `/health` is missing `uptime`, `start_time`, and a structured `dependencies` array (currently flat `db: connected`)
- `/version` names its build timestamp `build_date` instead of the required `build_time`
- `/version` carries runtime fields (`uptime_seconds`, `start_time`, `endpoint_count`, `leader`, `hostname`, `gomaxprocs`) that belong on `/health` per the stack spec

### User Story

As an operator running the cash-drugs + rx-dag + drug-gate + drugs-quiz stack, I want every service to return health and version responses in the same shape, so that I can write dashboards, alerts, and smoke tests once and reuse them across all services.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | `GET /health` returns `status` field with values `ok`, `degraded`, or `error` | Must |
| AC-002 | `GET /health` returns `version` (running version string) | Must |
| AC-003 | `GET /health` returns `uptime` as a human-readable duration string (e.g. `4h32m10s`) | Must |
| AC-004 | `GET /health` returns `start_time` as a UTC RFC3339 timestamp | Must |
| AC-005 | `GET /health` returns `dependencies` as an array of `{name, status, latency_ms, error}` objects | Must |
| AC-006 | `/health` `dependencies` array includes an entry for MongoDB with `name: "mongodb"`, `status: "connected"` or `"disconnected"`, and `latency_ms` from the ping | Must |
| AC-007 | When MongoDB ping fails, `/health` `status` is `error`, the MongoDB dependency has `status: "disconnected"` and an `error` field, and the HTTP response is 503 | Must |
| AC-008 | `/health` returns domain-specific `cache_slug_count` (total configured slugs) as an additional field | Should |
| AC-009 | `/health` returns domain-specific `leader` boolean (scheduler leader flag) as an additional field | Should |
| AC-010 | `GET /version` returns `version`, `git_commit`, `git_branch`, `go_version`, `os`, `arch`, and `build_time` | Must |
| AC-011 | `/version` field is named `build_time` (not `build_date`) | Must |
| AC-012 | `/version` does NOT contain runtime-varying fields (`uptime_seconds`, `start_time`, `endpoint_count`, `leader`, `hostname`, `gomaxprocs`) — these belong on `/health` | Must |
| AC-013 | `/version` build-time fields are injected via ldflags at compile time (existing mechanism, unchanged) | Must |
| AC-014 | Consumers querying `/health` for `db` (old flat field) can migrate by reading `dependencies[?name==mongodb].status` instead — old `db` field is REMOVED | Must |
| AC-015 | Existing Swagger annotations on both handlers updated to reflect new response shape | Should |
| AC-016 | Smoke tests (tests/k6/smoke-test.js) updated to assert the new field contract on both endpoints | Must |
| AC-017 | Healthcheck in Dockerfile (if any) still works against new `/health` — the `status: "ok"` check is preserved | Must |

## 3. User Test Cases

### TC-001: /health happy path with new contract

**Precondition:** Server running, MongoDB healthy
**Steps:**
1. `GET /health`
**Expected Result:**
```json
{
  "status": "ok",
  "version": "beta-c72bdb3",
  "uptime": "2h15m3s",
  "start_time": "2026-04-11T14:00:00Z",
  "dependencies": [
    { "name": "mongodb", "status": "connected", "latency_ms": 1.23 }
  ],
  "cache_slug_count": 20,
  "leader": true
}
```
HTTP 200.
**Maps to:** TBD

### TC-002: /health when MongoDB is down

**Precondition:** MongoDB unreachable
**Steps:**
1. `GET /health`
**Expected Result:** HTTP 503 with
```json
{
  "status": "error",
  "version": "beta-c72bdb3",
  "uptime": "...",
  "start_time": "...",
  "dependencies": [
    {
      "name": "mongodb",
      "status": "disconnected",
      "latency_ms": 0,
      "error": "server selection error: context deadline exceeded"
    }
  ],
  "cache_slug_count": 20,
  "leader": true
}
```
**Maps to:** TBD

### TC-003: /version returns build metadata only

**Precondition:** Server running with ldflags-injected build info
**Steps:**
1. `GET /version`
**Expected Result:**
```json
{
  "version": "beta-c72bdb3",
  "git_commit": "c72bdb3",
  "git_branch": "main",
  "go_version": "go1.24.13",
  "os": "linux",
  "arch": "amd64",
  "build_time": "2026-04-05T04:34:51Z"
}
```
No runtime fields. HTTP 200.
**Maps to:** TBD

### TC-004: /version rejects runtime fields

**Precondition:** Server running
**Steps:**
1. `GET /version`
2. Parse JSON
**Expected Result:** Response does NOT contain `uptime_seconds`, `start_time`, `endpoint_count`, `leader`, `hostname`, or `gomaxprocs`
**Maps to:** TBD

### TC-005: uptime format is human-readable

**Precondition:** Server running for at least 10 seconds
**Steps:**
1. `GET /health`
2. Inspect `uptime` field
**Expected Result:** Value matches Go's `time.Duration.String()` format (e.g. `10s`, `4h32m10s`, `7d2h`) — not a numeric seconds value
**Maps to:** TBD

### TC-006: /health dependency latency is measured

**Precondition:** Server running, MongoDB healthy
**Steps:**
1. `GET /health`
2. Inspect `dependencies[0].latency_ms`
**Expected Result:** Value is a non-negative float representing actual ping latency in milliseconds (not hardcoded or zero when connected)
**Maps to:** TBD

### TC-007: Smoke test passes against updated contract

**Precondition:** Staging running new build
**Steps:**
1. Run `BASE_URL=http://192.168.1.145:8083 k6 run tests/k6/smoke-test.js`
**Expected Result:** All `/health` and `/version` checks pass with the new field contract
**Maps to:** TBD

## 4. Data Model

### HealthResponse (new shape)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| status | string | Yes | `ok`, `degraded`, `error` |
| version | string | Yes | Running version (same value as `/version`.`version`) |
| uptime | string | Yes | Human-readable duration since process start |
| start_time | string | Yes | UTC RFC3339 timestamp of process start |
| dependencies | []Dependency | Yes | Downstream dependency checks |
| cache_slug_count | int | No | Total configured endpoint slugs (domain-specific) |
| leader | bool | No | Scheduler leader flag (domain-specific) |

### Dependency

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Dependency identifier (e.g. `mongodb`) |
| status | string | Yes | `connected` or `disconnected` |
| latency_ms | float64 | Yes | Measured latency in ms |
| error | string | No | Error message when `status != connected` |

### VersionResponse (new shape)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| version | string | Yes | Semver or `dev` |
| git_commit | string | Yes | Short SHA |
| git_branch | string | Yes | Branch the binary was built from |
| go_version | string | Yes | Go runtime version |
| os | string | Yes | Target OS |
| arch | string | Yes | Target arch |
| build_time | string | Yes | UTC RFC3339 build timestamp |

## 5. API Contract

### GET /health

**Response (200)** — all dependencies healthy. See TC-001.

**Response (503)** — critical dependency down. See TC-002.

### GET /version

**Response (200)** — See TC-003. No error path (static data).

## 6. UI Behavior

N/A — API only

## 7. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| MongoDB ping takes > 1s | Measured latency still returned; status may shift to `degraded` if > threshold (future) |
| Empty/missing ldflags (local `go run`) | `version = "dev"`, empty git_commit/git_branch/build_time are allowed (not required to be non-empty) |
| Container restart mid-request | New `start_time` and reset `uptime` on next request — acceptable |
| Old consumer reading `db` field | Breaks — `db` removed. Stack spec breaks backward compat intentionally; consumers must migrate |
| `/health` under concurrency limiter | `/health` is exempt from the limiter (existing behavior, unchanged) |

## 8. Dependencies

- Reference implementation: rx-dag (`dag-rx` repo) — matches the target shape
- Existing `cache.Pinger` interface in `internal/cache` (provides ping method)
- Need to extend `cache.Pinger` or add a new interface to return `(latency time.Duration, err error)` for measured latency
- Existing ldflags build injection in `cmd/server/main.go` — no changes
- Existing Dockerfile `HEALTHCHECK` (if any) depends on `status: "ok"` being parseable

## 9. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-04-11 | 0.1.0 | calebdunn | Initial spec — stack-wide contract alignment |

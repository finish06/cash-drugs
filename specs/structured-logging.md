# Spec: Structured Logging

**Version:** 0.1.0
**Created:** 2026-03-07
**PRD Reference:** docs/prd.md
**Status:** Draft

## 1. Overview

Add structured logging throughout the drugs service using Go's `log/slog` stdlib. Log level defaults to `warn` and is configurable via the `LOG_LEVEL` environment variable (with `config.yaml` fallback). Output is JSON by default, with an optional plain-text mode via `LOG_FORMAT=text`. INFO-level logs answer "what code is running" while DEBUG-level logs capture each step in detail.

### User Story

As an **operator**, I want configurable structured logging, so that I can observe service behavior in production and debug issues without redeploying code.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | Service uses `log/slog` for all log output — no `fmt.Println` or `log.Printf` in production code paths | Must |
| AC-002 | Default log level is `warn` when no configuration is provided | Must |
| AC-003 | `LOG_LEVEL` environment variable sets the log level (valid values: `debug`, `info`, `warn`, `error`) | Must |
| AC-004 | `log_level` field in `config.yaml` sets the log level as a fallback when `LOG_LEVEL` env var is not set | Must |
| AC-005 | `LOG_LEVEL` env var takes precedence over `config.yaml` `log_level` field | Must |
| AC-006 | Invalid log level values are rejected at startup with a clear error message | Must |
| AC-007 | Default output format is JSON (one JSON object per log line) | Must |
| AC-008 | `LOG_FORMAT=text` switches output to human-readable plain text | Must |
| AC-009 | INFO-level logs are emitted for: server start/stop, endpoint registered, fetch started, fetch completed, scheduler tick, cache warm started | Must |
| AC-010 | DEBUG-level logs are emitted for: HTTP request method/path/status/duration, upstream response status/duration/pages, cache hit/miss, MongoDB connect/query, scheduler skip reasons | Must |
| AC-011 | WARN-level logs are emitted for: stale cache served, upstream non-200 response, slow operations (>5s) | Should |
| AC-012 | ERROR-level logs are emitted for: upstream fetch failure, MongoDB errors, unrecoverable handler errors | Must |
| AC-013 | All log entries include a `component` field identifying the source (e.g. `handler`, `fetcher`, `scheduler`, `cache`, `server`) | Should |
| AC-014 | Changing the log level requires a service restart — no runtime log level changes | Must |

## 3. User Test Cases

### TC-001: Default log level filters debug and info

**Precondition:** Service started with no `LOG_LEVEL` env var and no `log_level` in config.yaml
**Steps:**
1. Start the service
2. Make a `GET /api/cache/drugnames` request
3. Inspect log output
**Expected Result:** Only WARN and ERROR lines appear. No INFO or DEBUG lines are emitted.
**Screenshot Checkpoint:** N/A (backend only)
**Maps to:** TBD

### TC-002: Environment variable sets log level to debug

**Precondition:** Service started with `LOG_LEVEL=debug`
**Steps:**
1. Start the service with `LOG_LEVEL=debug`
2. Make a `GET /api/cache/drugnames` request
3. Inspect log output
**Expected Result:** DEBUG, INFO, WARN, and ERROR lines all appear. Log lines include details like cache hit/miss, upstream response status, request duration.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-003: Config file fallback

**Precondition:** `config.yaml` contains `log_level: info`, no `LOG_LEVEL` env var set
**Steps:**
1. Start the service
2. Inspect log output during startup
**Expected Result:** INFO-level lines appear (e.g. "server starting", "endpoint registered"). DEBUG-level lines do not appear.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-004: Env var overrides config file

**Precondition:** `config.yaml` contains `log_level: debug`, `LOG_LEVEL=error` is set
**Steps:**
1. Start the service
2. Make requests to trigger logging
3. Inspect log output
**Expected Result:** Only ERROR-level lines appear. DEBUG and INFO are filtered despite config.yaml setting.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-005: Invalid log level rejected at startup

**Precondition:** `LOG_LEVEL=verbose` (invalid value)
**Steps:**
1. Attempt to start the service
**Expected Result:** Service exits with a clear error: `invalid log level "verbose": must be one of debug, info, warn, error`
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-006: JSON output format by default

**Precondition:** Service started with `LOG_LEVEL=info`, no `LOG_FORMAT` set
**Steps:**
1. Start the service
2. Capture a log line
**Expected Result:** Each log line is valid JSON with keys like `time`, `level`, `msg`, `component`.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-007: Text output format

**Precondition:** Service started with `LOG_LEVEL=info LOG_FORMAT=text`
**Steps:**
1. Start the service
2. Capture a log line
**Expected Result:** Log lines are human-readable plain text (not JSON).
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

## 4. Data Model

### Log Configuration

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| log_level | string | No | Log level in config.yaml: `debug`, `info`, `warn`, `error`. Defaults to `warn`. |

This is a top-level field in `config.yaml`, not nested under `endpoints`.

### Environment Variables

| Variable | Values | Default | Description |
|----------|--------|---------|-------------|
| LOG_LEVEL | `debug`, `info`, `warn`, `error` | `warn` | Overrides config.yaml `log_level` |
| LOG_FORMAT | `json`, `text` | `json` | Output format |

## 5. API Contract

No new API endpoints. This feature affects log output only.

## 6. UI Behavior

N/A — backend-only feature.

## 7. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| `LOG_LEVEL` set to empty string | Treated as unset, falls back to config.yaml then default (`warn`) |
| `log_level` missing from config.yaml and no env var | Defaults to `warn` |
| `LOG_FORMAT` set to invalid value | Defaults to `json`, logs a warning |
| Very high log volume at debug level | No throttling — operator responsibility to set appropriate level |
| Logger initialization fails | Service should still start, falling back to stderr with default Go logger |

## 8. Dependencies

- Go 1.21+ (for `log/slog` stdlib) — already using Go 1.22+
- No external dependencies

## 9. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-07 | 0.1.0 | calebdunn | Initial spec from /add:spec interview |

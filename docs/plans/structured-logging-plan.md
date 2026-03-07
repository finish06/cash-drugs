# Plan: Structured Logging

**Spec:** specs/structured-logging.md
**Created:** 2026-03-07

## Task Breakdown

### 1. Create `internal/logging` package
- `internal/logging/logging.go` — `Setup(level, format string) (*slog.Logger, error)`, `ParseLevel`, `ParseFormat`
- Validates level/format, returns configured `*slog.Logger`
- AC-001, AC-002, AC-003, AC-006, AC-007, AC-008, AC-014

### 2. Add `log_level` to config loader
- Add `LogLevel` field to `configFile` struct
- Return it from `Load` or provide `LoadFull` that returns both endpoints and config-level settings
- AC-004, AC-005

### 3. Wire logger into `cmd/server/main.go`
- Read `LOG_LEVEL` env var, fall back to config `log_level`, default to `warn`
- Read `LOG_FORMAT` env var, default to `json`
- Call `logging.Setup()`, set as default `slog.SetDefault()`
- Replace all `log.*` calls in main.go
- AC-003, AC-004, AC-005, AC-007, AC-008

### 4. Replace `log.*` calls in all packages
- `handler/cache.go` — component: `handler`
- `scheduler/scheduler.go` — component: `scheduler`
- `cache/mongo.go` — component: `cache` (if any)
- AC-009, AC-010, AC-011, AC-012, AC-013

### 5. Add contextual log points per spec
- INFO: server start/stop, endpoint registered, fetch started/completed, scheduler tick, cache warm
- DEBUG: HTTP request details, upstream response, cache hit/miss, MongoDB ops, scheduler skip
- WARN: stale cache served, upstream non-200, slow ops
- ERROR: fetch failure, MongoDB errors, unrecoverable handler errors
- AC-009, AC-010, AC-011, AC-012

## File Changes

| File | Action | Notes |
|------|--------|-------|
| `internal/logging/logging.go` | Create | Core setup, ParseLevel, ParseFormat |
| `internal/logging/logging_test.go` | Create | Tests for AC-001 through AC-008, AC-014 |
| `internal/config/loader.go` | Modify | Add LogLevel to configFile, expose via LoadConfig |
| `internal/config/loader_test.go` | Modify | Test log_level parsing |
| `cmd/server/main.go` | Modify | Wire logger, replace log.* calls |
| `internal/handler/cache.go` | Modify | Replace log.* with slog |
| `internal/scheduler/scheduler.go` | Modify | Replace log.* with slog |

## Test Strategy

- Unit tests for `internal/logging` package (level parsing, format selection, defaults)
- Unit tests for config `log_level` field loading
- Existing tests continue to pass (no behavioral changes)

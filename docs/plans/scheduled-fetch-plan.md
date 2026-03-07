# Implementation Plan: Scheduled Fetch

**Spec:** specs/scheduled-fetch.md
**Created:** 2026-03-07
**Status:** Active

## Task Breakdown

### Phase 1: Config Extension (AC-001, AC-002, AC-006, AC-009)

1. **Add `refresh` field to Endpoint struct**
   - Add `Refresh string` field with `yaml:"refresh"` tag
   - Validate cron expression at load time (fail startup if invalid)
   - Warn if endpoint has path params AND refresh field
   - Files: `internal/config/loader.go`

### Phase 2: Scheduler Core (AC-003, AC-004, AC-005, AC-007, AC-008, AC-010)

2. **Implement Scheduler**
   - `NewScheduler(endpoints, fetcher, repo, logger)` constructor
   - `Start(ctx)` — warm cache immediately, then start cron jobs
   - `Stop()` — graceful shutdown, cancel all cron jobs
   - Per-endpoint goroutine with overlap protection (skip if fetch in progress)
   - Log each fetch: slug, outcome, duration
   - Files: `internal/scheduler/scheduler.go`

### Phase 3: Server Integration (AC-003, AC-008)

3. **Wire scheduler into server entrypoint**
   - Create scheduler after loading config and connecting to MongoDB
   - Start scheduler in background goroutine
   - Stop scheduler on SIGINT/SIGTERM before server shutdown
   - Files: `cmd/server/main.go`

### Phase 4: Config Update (AC-001)

4. **Update seed config with refresh example**
   - Add `refresh: "0 */6 * * *"` to drugnames endpoint
   - Files: `config.yaml`

## Spec Traceability

| Task | ACs Covered |
|------|-------------|
| 1. Config extension | AC-001, AC-002, AC-006, AC-009 |
| 2. Scheduler core | AC-003, AC-004, AC-005, AC-007, AC-008, AC-010 |
| 3. Server integration | AC-003, AC-008 |
| 4. Config update | AC-001 |

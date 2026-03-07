# Spec: Scheduled Fetch

**Version:** 0.1.0
**Created:** 2026-03-07
**PRD Reference:** docs/prd.md (M2: Scheduling + Staleness)
**Status:** Draft

## 1. Overview

Background cron-based refresh of cached upstream API data. Endpoints in config.yaml can declare a `refresh` field with a cron expression. On startup the scheduler immediately fetches all scheduled endpoints (cache warming), then continues on the cron schedule. Only endpoints without path parameters are eligible for scheduling. If a scheduled fetch fails, the existing cache is preserved and the next cron tick retries.

### User Story

As an internal microservice developer, I want cached data to refresh automatically on a schedule, so that consumers always get reasonably fresh data without needing to trigger fetches themselves.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | Endpoints in config.yaml support an optional `refresh` field containing a cron expression (e.g., `"0 */6 * * *"`) | Must |
| AC-002 | Endpoints without a `refresh` field remain on-demand only — no background fetching | Must |
| AC-003 | A `Scheduler` component starts background goroutines for each scheduled endpoint | Must |
| AC-004 | On startup, the scheduler immediately fetches all scheduled endpoints once (cache warming) before starting the cron timers | Must |
| AC-005 | Each scheduled tick triggers a full upstream fetch and cache upsert, regardless of existing cache age | Must |
| AC-006 | Endpoints with path parameters (e.g., `/v2/spls/{SETID}`) are excluded from scheduling, even if they have a `refresh` field. A startup warning is logged. | Must |
| AC-007 | If a scheduled fetch fails, the existing cache is preserved and a warning is logged. The next cron tick retries normally. | Must |
| AC-008 | The scheduler respects graceful shutdown — stops all cron jobs when the service receives SIGINT/SIGTERM | Must |
| AC-009 | Invalid cron expressions in config prevent startup with a clear error identifying the endpoint | Must |
| AC-010 | Scheduler logs each fetch attempt with endpoint slug, outcome (success/failure), and duration | Should |

## 3. User Test Cases

### TC-001: Scheduled endpoint refreshes on cron

**Precondition:** Config defines `drugnames` with `refresh: "* * * * *"` (every minute). Service is running.
**Steps:**
1. Wait for at least one cron tick
2. Check MongoDB for `drugnames` cache entry
**Expected Result:** Cache entry exists with `fetched_at` within the last minute. Logs show scheduled fetch.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-002: Cache warming on startup

**Precondition:** Config defines `drugnames` with `refresh: "0 */6 * * *"`. MongoDB has no cached data.
**Steps:**
1. Start the service
2. Immediately check MongoDB
**Expected Result:** Cache entry exists — the scheduler fetched it at startup before the first cron tick.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-003: On-demand-only endpoint not scheduled

**Precondition:** Config defines `spl-detail` without `refresh` field.
**Steps:**
1. Start the service
2. Wait 2 minutes
3. Check MongoDB for `spl-detail` cache entry
**Expected Result:** No cache entry — the endpoint was not fetched in the background.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-004: Parameterized endpoint excluded from scheduling

**Precondition:** Config defines `spl-detail` with `path: /v2/spls/{SETID}` and `refresh: "* * * * *"`.
**Steps:**
1. Start the service
2. Check startup logs
**Expected Result:** Warning log: "endpoint 'spl-detail' has path parameters and cannot be scheduled". No background fetch occurs for this endpoint.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-005: Scheduled fetch failure preserves cache

**Precondition:** Cache exists for `drugnames`. Upstream becomes unreachable.
**Steps:**
1. Scheduled tick fires
2. Fetch fails
3. Check MongoDB
**Expected Result:** Cache entry unchanged. Warning logged. Next tick retries.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-006: Invalid cron expression prevents startup

**Precondition:** Config defines `drugnames` with `refresh: "not-a-cron"`.
**Steps:**
1. Attempt to start the service
**Expected Result:** Service fails to start with error: "endpoint 'drugnames': invalid cron expression 'not-a-cron'"
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-007: Graceful shutdown stops scheduler

**Precondition:** Service is running with scheduled endpoints.
**Steps:**
1. Send SIGTERM to the service
2. Observe shutdown logs
**Expected Result:** Logs show "Scheduler stopped". No fetch attempts after shutdown signal.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

## 4. Data Model

### Config Extension

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| refresh | string | No | Cron expression for background refresh (e.g., `"0 */6 * * *"`). If omitted, endpoint is on-demand only. |

Added to the existing `Endpoint` struct in config.yaml.

### No new MongoDB collections or fields.

The scheduler reuses the existing `upstream.Fetcher` and `cache.Repository.Upsert()` — same data model.

## 5. API Contract

No new API endpoints. The scheduler is an internal background component.

Existing `GET /api/cache/{slug}` continues to work — consumers benefit from warmer caches.

## 6. UI Behavior

N/A — no UI.

## 7. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| All endpoints are on-demand (no `refresh` fields) | Scheduler starts but has nothing to do. No error. |
| Cron expression fires while previous fetch is still running | Skip the overlapping tick. Log a warning. Don't pile up concurrent fetches for the same endpoint. |
| Service restarts frequently (crash loop) | Each startup triggers cache warming. Acceptable — idempotent upserts prevent data issues. |
| Cron expression with seconds (6 fields) | Depends on cron library. Document supported format. |

## 8. Dependencies

- Cron library: `github.com/robfig/cron/v3` (standard Go cron library)
- Existing `upstream.Fetcher` interface
- Existing `cache.Repository` interface
- Existing `config.Endpoint` struct (extended with `refresh` field)

## 9. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-07 | 0.1.0 | calebdunn | Initial spec from /add:spec interview |

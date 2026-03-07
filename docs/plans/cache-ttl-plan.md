# Implementation Plan: Cache TTL & Stale-While-Revalidate

**Spec:** specs/cache-ttl.md
**Created:** 2026-03-07

## Task Breakdown

### 1. Add TTL field to config and validate at load time
- Add `TTL string` field to `config.Endpoint` struct
- Add `TTLDuration time.Duration` (computed, not YAML) for runtime use
- Parse with `time.ParseDuration` during `Load()`, fail-fast on invalid
- AC: AC-001, AC-002, AC-003

### 2. Add TTL config tests
- Test valid TTL strings parse correctly
- Test invalid TTL prevents load
- Test missing TTL defaults to zero (no expiry)
- AC: AC-001, AC-002, AC-003

### 3. Add IsStale helper to config or handler
- `IsStale(ep config.Endpoint, fetchedAt time.Time) bool`
- Returns false if TTLDuration is zero (no TTL configured)
- Returns true if time.Since(fetchedAt) > TTLDuration
- AC: AC-003, AC-004, AC-005, AC-010

### 4. Modify cache handler for TTL-aware serving
- After cache hit, check IsStale
- If fresh: serve with stale=false (existing behavior)
- If stale: serve with stale=true, stale_reason="ttl_expired", trigger background revalidation
- AC: AC-004, AC-005, AC-006, AC-007

### 5. Implement background revalidation in handler
- Spawn goroutine that acquires per-slug mutex (shared with scheduler)
- On lock acquired: fetch upstream, upsert cache
- On lock busy: skip (scheduler already fetching)
- On fetch failure: log warning, preserve stale cache
- AC: AC-006, AC-008, AC-009

### 6. Share scheduler's per-slug mutex with handler
- Extract mutex map from scheduler or create a shared `FetchLock` component
- Both scheduler and handler use the same mutex per slug
- AC: AC-008

## File Changes

| File | Action | Description |
|------|--------|-------------|
| internal/config/loader.go | Modify | Add TTL + TTLDuration fields, validate in Load() |
| internal/config/loader_test.go or ttl_test.go | Create | TTL config validation tests |
| internal/config/staleness.go | Create | IsStale helper function |
| internal/config/staleness_test.go | Create | IsStale tests |
| internal/handler/cache.go | Modify | TTL-aware cache serving, background revalidation |
| internal/handler/cache_test.go | Modify | Add TTL-related handler tests |
| internal/scheduler/scheduler.go | Modify | Export or share per-slug mutex map |
| config.yaml | Modify | Add ttl example to seed config |

## Test Strategy

- Unit tests: config parsing, IsStale logic, handler TTL behavior (mocks)
- Integration tests: not needed (TTL logic is in-process, no new DB operations)

## Dependencies

- Task 6 (shared mutex) must be designed before task 5 (background revalidation)
- Tasks 1-3 can be done in parallel with task design for 4-6

## Spec Traceability

| AC | Test(s) | Implementation |
|----|---------|----------------|
| AC-001 | config TTL parse test | config/loader.go |
| AC-002 | config invalid TTL test | config/loader.go |
| AC-003 | IsStale with no TTL | config/staleness.go |
| AC-004 | handler fresh cache test | handler/cache.go |
| AC-005 | handler stale cache test | handler/cache.go |
| AC-006 | handler background revalidation test | handler/cache.go |
| AC-007 | handler stale_reason test | handler/cache.go |
| AC-008 | handler dedup with scheduler test | handler/cache.go + scheduler |
| AC-009 | handler revalidation failure test | handler/cache.go |
| AC-010 | IsStale uses fetched_at test | config/staleness.go |

# Spec: Cache TTL & Stale-While-Revalidate

**Version:** 0.1.0
**Created:** 2026-03-07
**PRD Reference:** docs/prd.md (M2: Scheduling + Staleness)
**Status:** Draft

## 1. Overview

Per-endpoint cache TTL with stale-while-revalidate semantics. Endpoints in config.yaml can declare an optional `ttl` field (Go duration string). When a consumer requests cached data past its TTL, the handler serves the stale cache immediately and triggers a background upstream refresh. The response metadata includes a `stale` flag so consumers can react if they care.

### User Story

As an internal microservice developer, I want cached data to indicate when it's past its freshness window and automatically refresh in the background, so that consumers always get fast responses while data stays reasonably fresh.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | Endpoints in config.yaml support an optional `ttl` field containing a Go duration string (e.g., `"6h"`, `"30m"`, `"24h"`) | Must |
| AC-002 | Invalid TTL values in config prevent startup with a clear error identifying the endpoint | Must |
| AC-003 | Endpoints without a `ttl` field have no expiry — cache is always considered fresh | Must |
| AC-004 | When a consumer requests data within the TTL window, the response has `"stale": false` in metadata | Must |
| AC-005 | When a consumer requests data past its TTL, the handler returns the stale cache immediately with `"stale": true` in metadata | Must |
| AC-006 | When stale data is served due to TTL expiry, a background goroutine fetches fresh data from upstream and upserts the cache | Must |
| AC-007 | The stale reason in metadata is `"ttl_expired"` when serving TTL-expired data (distinct from `"upstream unavailable"`) | Must |
| AC-008 | Background revalidation reuses the scheduler's per-slug mutex to prevent redundant concurrent fetches | Must |
| AC-009 | If the background revalidation fetch fails, the stale cache is preserved (no data loss) | Should |
| AC-010 | The TTL check uses the `fetched_at` timestamp from the cached response, not `created_at` or `updated_at` | Should |

## 3. User Test Cases

### TC-001: Fresh cache served within TTL

**Precondition:** Config defines `drugnames` with `ttl: "6h"`. Cache exists with `fetched_at` 2 hours ago.
**Steps:**
1. Request `GET /api/cache/drugnames`
**Expected Result:** Response returned with `"stale": false` in metadata. No background fetch triggered.
**Screenshot Checkpoint:** N/A
**Maps to:** AC-003, AC-004

### TC-002: Stale cache served past TTL with background refresh

**Precondition:** Config defines `drugnames` with `ttl: "6h"`. Cache exists with `fetched_at` 8 hours ago.
**Steps:**
1. Request `GET /api/cache/drugnames`
2. Wait briefly for background refresh
3. Request `GET /api/cache/drugnames` again
**Expected Result:** First response has `"stale": true`, `"stale_reason": "ttl_expired"`. Second response has `"stale": false` with updated `fetched_at`.
**Screenshot Checkpoint:** N/A
**Maps to:** AC-005, AC-006, AC-007

### TC-003: No TTL means never stale

**Precondition:** Config defines `drugnames` with no `ttl` field. Cache exists with `fetched_at` 30 days ago.
**Steps:**
1. Request `GET /api/cache/drugnames`
**Expected Result:** Response returned with `"stale": false`. Cache is never considered expired.
**Screenshot Checkpoint:** N/A
**Maps to:** AC-003

### TC-004: Invalid TTL prevents startup

**Precondition:** Config defines `drugnames` with `ttl: "not-a-duration"`.
**Steps:**
1. Attempt to start the service
**Expected Result:** Service fails to start with error: "endpoint 'drugnames': invalid ttl 'not-a-duration'"
**Screenshot Checkpoint:** N/A
**Maps to:** AC-002

### TC-005: Background revalidation deduplicates with scheduler

**Precondition:** Config defines `drugnames` with `ttl: "1m"` and `refresh: "* * * * *"`. Cache is stale.
**Steps:**
1. Scheduler is already fetching `drugnames`
2. Consumer requests stale data
**Expected Result:** Stale data served immediately. Background revalidation skipped (scheduler already holds the lock).
**Screenshot Checkpoint:** N/A
**Maps to:** AC-008

### TC-006: Background revalidation failure preserves cache

**Precondition:** Cache exists for `drugnames` but upstream is down.
**Steps:**
1. Consumer requests stale data
2. Background revalidation fails
**Expected Result:** Stale cache preserved. No data loss. Consumer got the stale response.
**Screenshot Checkpoint:** N/A
**Maps to:** AC-009

## 4. Data Model

### Config Extension

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| ttl | string | No | Go duration string (e.g., `"6h"`). Parsed by `time.ParseDuration`. If omitted, cache never expires. |

Added to the existing `Endpoint` struct in config.yaml.

### No new MongoDB collections or fields.

TTL check uses existing `fetched_at` timestamp on `CachedResponse`.

## 5. API Contract

No new API endpoints. Existing `GET /api/cache/{slug}` gains TTL-aware staleness:

**Response metadata changes:**
- `stale: true` when cache is past TTL (already exists, currently only set for upstream failures)
- `stale_reason: "ttl_expired"` (new reason value, alongside existing `"upstream unavailable"`)

## 6. UI Behavior

N/A — no UI.

## 7. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| TTL of "0s" | Cache is always stale — every request triggers background refresh. Stale data still served immediately. |
| TTL set but no cache exists yet | Normal on-demand fetch (no stale-while-revalidate — nothing to serve stale). |
| Multiple concurrent requests for same stale endpoint | First triggers background revalidation, subsequent ones serve stale without triggering duplicate fetches (mutex). |
| Background revalidation completes while another request is in-flight | Next request sees fresh data. No race condition — upsert is atomic. |

## 8. Dependencies

- Existing `config.Endpoint` struct (extended with `ttl` field)
- Existing `cache.Repository` interface (Get + Upsert)
- Existing `upstream.Fetcher` interface
- Scheduler's per-slug mutex (for deduplication)
- Go stdlib `time.ParseDuration` for TTL parsing

## 9. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-07 | 0.1.0 | calebdunn | Initial spec from /add:spec interview |

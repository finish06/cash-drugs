# Spec: Upstream Resilience

**Version:** 0.1.0
**Created:** 2026-03-14
**PRD Reference:** docs/prd.md (M9)
**Status:** Complete

## 1. Overview

Protect the service from upstream API instability through two mechanisms: per-endpoint circuit breakers that stop hammering failing upstreams, and force-refresh rate limiting that prevents abuse of the `_force=true` cache bypass. The stress test showed that slow or failing upstreams cascade into goroutine exhaustion and connection refusal, and concurrent force-refresh requests independently call the same upstream API without deduplication.

### User Story

As an **operator**, I want the service to automatically stop calling failing upstream APIs and recover gracefully, so that one flaky upstream doesn't cascade into service-wide degradation.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | Each upstream endpoint has its own independent circuit breaker | Must |
| AC-002 | Circuit breaker opens after 5 consecutive failures | Must |
| AC-003 | Open circuit breaker rejects upstream calls immediately and returns cached data (stale fallback) | Must |
| AC-004 | Circuit breaker transitions to half-open after 30 seconds, allowing 1 probe request | Must |
| AC-005 | Successful probe in half-open state closes the circuit (normal operation resumes) | Must |
| AC-006 | Failed probe in half-open state re-opens the circuit for another 30 seconds | Must |
| AC-007 | When circuit is open, handler serves stale cache with `stale_reason: "circuit_open"` | Must |
| AC-008 | When circuit is open and no cache exists, return `503` with `{"error": "upstream circuit open", "slug": "{slug}", "retry_after": 30}` | Must |
| AC-009 | Force-refresh (`_force=true`) has a 30-second per-key cooldown — requests within the cooldown window are served from cache instead | Must |
| AC-010 | Force-refresh cooldown key includes slug + sorted params (same granularity as cache key) | Must |
| AC-011 | Force-refresh during cooldown returns cached data with a response header `X-Force-Cooldown: true` | Must |
| AC-012 | Prometheus gauge `cashdrugs_circuit_state` with label `slug` reports circuit state (0=closed, 1=half-open, 2=open) | Should |
| AC-013 | Prometheus counter `cashdrugs_circuit_rejections_total` with label `slug` counts requests rejected by open circuit | Should |
| AC-014 | Prometheus counter `cashdrugs_force_refresh_cooldown_total` with label `slug` counts force-refresh requests rejected by cooldown | Should |
| AC-015 | Scheduler respects circuit breaker state — skips fetch when circuit is open, logs warning | Must |
| AC-016 | Circuit breaker thresholds configurable via environment variables: `CIRCUIT_FAILURE_THRESHOLD` (default: 5), `CIRCUIT_OPEN_DURATION` (default: 30s) | Should |
| AC-017 | Force-refresh cooldown duration configurable via `FORCE_REFRESH_COOLDOWN` environment variable (default: 30s) | Should |
| AC-018 | No regression in existing tests or functionality | Must |

## 3. User Test Cases

### TC-001: Circuit opens after consecutive failures

**Precondition:** Service is running. Upstream API is unreachable.
**Steps:**
1. Send 5 requests to `/api/cache/{slug}?_force=true` with upstream down
2. Send a 6th request
**Expected Result:** First 5 requests attempt upstream and fail (returning stale cache or 502). 6th request does NOT attempt upstream — circuit is open. Returns stale cache with `stale_reason: "circuit_open"` or `503` if no cache.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-002: Circuit half-opens and recovers

**Precondition:** Circuit is open for a slug. Upstream has recovered.
**Steps:**
1. Wait 30 seconds for circuit to transition to half-open
2. Send a request to `/api/cache/{slug}?_force=true`
**Expected Result:** One probe request reaches upstream. Upstream responds successfully. Circuit closes. Subsequent requests flow normally.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-003: Circuit is per-endpoint

**Precondition:** Upstream for `fda-enforcement` is down. Upstream for `drugnames` is healthy.
**Steps:**
1. Trigger circuit open for `fda-enforcement` (5 failures)
2. Send request to `/api/cache/drugnames`
**Expected Result:** `drugnames` request succeeds normally — its circuit is independent and closed.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-004: Force-refresh cooldown prevents rapid upstream calls

**Precondition:** Service is running. Cache is populated for `fda-ndc`.
**Steps:**
1. `GET /api/cache/fda-ndc?BRAND_NAME=ASPIRIN&_force=true` (succeeds, fetches upstream)
2. Within 30 seconds, send same request again with `_force=true`
**Expected Result:** Second request returns cached data (not from upstream). Response includes `X-Force-Cooldown: true` header.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-005: Force-refresh cooldown expires and allows refresh

**Precondition:** Service is running. Force-refresh was issued 31 seconds ago.
**Steps:**
1. `GET /api/cache/fda-ndc?BRAND_NAME=ASPIRIN&_force=true`
**Expected Result:** Request fetches from upstream (cooldown expired). No `X-Force-Cooldown` header.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-006: Scheduler skips fetch when circuit is open

**Precondition:** Upstream for `drugnames` is down. Circuit is open. Scheduler tick fires.
**Steps:**
1. Wait for scheduled refresh tick
2. Check logs
**Expected Result:** Log shows `"skipping scheduled fetch — circuit open"` for `drugnames`. No upstream call attempted. Existing cache preserved.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

## 4. Data Model

### Circuit Breaker State (in-memory only)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| Slug | string | Yes | Endpoint slug this breaker protects |
| State | enum | Yes | closed, half-open, open |
| ConsecutiveFailures | int | Yes | Rolling failure count (resets on success) |
| LastFailureAt | time.Time | No | Timestamp of most recent failure |
| OpenUntil | time.Time | No | When the circuit transitions to half-open |

### Force-Refresh Cooldown (in-memory only)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| Key | string | Yes | Cache key (slug + sorted params) |
| LastRefreshAt | time.Time | Yes | Timestamp of last force-refresh |

## 5. API Contract

### Existing endpoints — new error response

**503 Service Unavailable** (circuit open, no cache):
```json
{
  "error": "upstream circuit open",
  "slug": "fda-enforcement",
  "retry_after": 30
}
```

### New response headers

| Header | When | Value |
|--------|------|-------|
| `X-Force-Cooldown` | Force-refresh rejected by cooldown | `true` |
| `X-Cache-Stale-Reason` | Circuit open, serving stale | `circuit_open` |

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| Circuit open + no stale cache + no LRU | Return 503 with `retry_after` matching circuit open duration |
| Force-refresh cooldown + circuit open | Cooldown takes precedence (don't attempt upstream at all) |
| Multiple slugs hitting same upstream host | Independent circuits per slug — one slug's failures don't affect another |
| Service restart | All circuits reset to closed, all cooldowns cleared (in-memory state) |
| Half-open probe request times out | Counts as failure, circuit re-opens |
| Scheduler and handler both try upstream for same slug | FetchLock dedup still applies; circuit breaker wraps the fetch attempt |

## 7. Dependencies

- `github.com/sony/gobreaker` or equivalent circuit breaker library
- Existing `internal/fetchlock` for dedup coordination
- Existing `internal/metrics` package for new counters/gauges

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-14 | 0.1.0 | calebdunn | Initial spec from /add:spec interview |

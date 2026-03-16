# Spec: Response Optimization

**Version:** 0.1.0
**Created:** 2026-03-14
**PRD Reference:** docs/prd.md (M9)
**Status:** Complete

## 1. Overview

Reduce response latency and memory pressure through three optimizations: gzip compression for large JSON/XML responses, singleflight request coalescing to deduplicate concurrent identical requests, and an in-memory LRU cache layer in front of MongoDB. The stress test showed 8-9 MB payloads on bulk endpoints, heap spikes from 15 MiB to 185 MiB under load, and redundant MongoDB queries for the same cache key from concurrent clients.

### User Story

As an **internal service consumer**, I want cached responses served faster and with less bandwidth, so that my service handles bulk data endpoints without timeouts or excessive memory consumption.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | Gzip compression middleware compresses responses when the client sends `Accept-Encoding: gzip` | Must |
| AC-002 | Compression applies to `application/json` and `application/xml` content types | Must |
| AC-003 | Responses smaller than 1 KB are not compressed (overhead exceeds benefit) | Should |
| AC-004 | The `Content-Encoding: gzip` header is set on compressed responses | Must |
| AC-005 | Clients without `Accept-Encoding: gzip` receive uncompressed responses (backward compatible) | Must |
| AC-006 | Concurrent requests for the same cache key are coalesced via `singleflight` — only one MongoDB query (or upstream fetch) executes, and all waiters receive the same result | Must |
| AC-007 | Singleflight groups by the full cache key (slug + sorted params), not just slug | Must |
| AC-008 | Singleflight errors are not shared — if the single flight fails, each caller gets the error independently (no stale error caching) | Must |
| AC-009 | An in-memory LRU cache sits between the handler and MongoDB, checked before MongoDB on every `GET /api/cache/{slug}` request | Must |
| AC-010 | LRU cache has a configurable max size (default: 256 MB) via `lru_cache_size_mb` field in `config.yaml` (under top-level `AppConfig`), with `LRU_CACHE_SIZE_MB` env var override taking precedence | Must |
| AC-011 | LRU entries expire based on the endpoint's configured TTL — stale entries are evicted on access | Must |
| AC-012 | Cache writes (from upstream fetch or MongoDB read) populate the LRU cache | Must |
| AC-013 | Prometheus gauge `cashdrugs_lru_cache_size_bytes` tracks current LRU memory usage | Should |
| AC-014 | Prometheus counter `cashdrugs_lru_cache_hits_total` and `cashdrugs_lru_cache_misses_total` with label `slug` | Should |
| AC-015 | Prometheus counter `cashdrugs_singleflight_dedup_total` with label `slug` counts deduplicated requests | Should |
| AC-016 | Force-refresh (`_force=true`) bypasses the LRU cache (reads and writes go directly to upstream + MongoDB), but the LRU entry is invalidated | Must |
| AC-017 | Scheduler cache updates also update the LRU cache for scheduled endpoints | Should |
| AC-018 | No regression in existing tests or functionality | Must |

## 3. User Test Cases

### TC-001: Gzip compression reduces response size

**Precondition:** Service is running. `fda-enforcement` data is cached in MongoDB.
**Steps:**
1. `curl -H "Accept-Encoding: gzip" http://localhost:8080/api/cache/fda-enforcement -o /dev/null -w '%{size_download}'`
2. Compare to uncompressed: `curl http://localhost:8080/api/cache/fda-enforcement -o /dev/null -w '%{size_download}'`
**Expected Result:** Compressed response is 3-5x smaller. Response has `Content-Encoding: gzip` header.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-002: Clients without gzip get uncompressed response

**Precondition:** Service is running with compression middleware enabled
**Steps:**
1. `curl http://localhost:8080/api/cache/drugnames` (no Accept-Encoding header)
**Expected Result:** Response has no `Content-Encoding` header. JSON body is readable as-is.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-003: Singleflight deduplicates concurrent requests

**Precondition:** Service is running. Cache is empty for a slug.
**Steps:**
1. Send 10 concurrent `GET /api/cache/drugnames` requests simultaneously
2. Check upstream fetch count via `/metrics`
**Expected Result:** Only 1 upstream fetch occurs (`cashdrugs_upstream_fetch_duration_seconds_count` increments by 1, not 10). All 10 requests return the same data.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-004: LRU cache serves sub-millisecond responses

**Precondition:** Service is running. First request to `drugnames` populates LRU.
**Steps:**
1. `GET /api/cache/drugnames` (first request — populates LRU from MongoDB)
2. `GET /api/cache/drugnames` (second request — should hit LRU)
3. Check `/metrics` for `cashdrugs_lru_cache_hits_total`
**Expected Result:** Second request returns in < 5ms. LRU hit counter incremented.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-005: Force-refresh bypasses and invalidates LRU

**Precondition:** Service is running. `drugnames` is in LRU cache.
**Steps:**
1. `GET /api/cache/drugnames?_force=true`
2. Check that upstream is fetched
3. `GET /api/cache/drugnames` (next normal request)
**Expected Result:** Force-refresh fetches from upstream, updates MongoDB, and invalidates the LRU entry. Subsequent normal request repopulates LRU from the fresh MongoDB data.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-006: LRU respects TTL expiry

**Precondition:** Service is running. Endpoint configured with short TTL.
**Steps:**
1. `GET /api/cache/{slug}` — populates LRU
2. Wait until TTL expires
3. `GET /api/cache/{slug}` — should miss LRU
**Expected Result:** Expired LRU entry is not served. Request falls through to MongoDB.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

## 4. Data Model

### LRU Cache Entry (in-memory only)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| Key | string | Yes | Cache key (slug + sorted params) |
| Response | *model.CachedResponse | Yes | Pointer to cached response |
| Size | int64 | Yes | Approximate memory size in bytes |
| ExpiresAt | time.Time | Yes | TTL-based expiry timestamp |

## 5. API Contract

No new endpoints. Existing responses gain optional `Content-Encoding: gzip` header when client requests it.

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| LRU cache full, new entry arrives | Least recently used entry evicted |
| LRU size set to 0 | LRU disabled, all requests go to MongoDB |
| Singleflight upstream fetch fails | Error returned to all waiting callers; no error cached in singleflight group |
| XML response (`spl-xml` endpoint) with gzip | Compressed normally — XML is highly compressible |
| Very small response (< 1 KB) with gzip requested | Served uncompressed — overhead exceeds benefit |
| Concurrent force-refresh for same key | Singleflight still applies to force-refresh (dedup upstream calls) |

## 7. Dependencies

- `golang.org/x/sync/singleflight`
- LRU implementation: `github.com/hashicorp/golang-lru/v2` or equivalent
- `compress/flate` / `compress/gzip` (stdlib)
- Existing `internal/metrics` package for new counters/gauges

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-14 | 0.1.0 | calebdunn | Initial spec from /add:spec interview |

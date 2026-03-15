# M10 — Performance Optimization

**Goal:** Address remaining performance bottlenecks, API usability gaps, and operational visibility: MongoDB query efficiency, LRU cache contention, upstream fetch parallelism, empty upstream result handling, and build/deployment info endpoint.

**Appetite:** Small-Medium — 5 targeted improvements

**Target Maturity:** Beta

**Status:** LATER

## Success Criteria

- [ ] MongoDB cache lookups use indexed exact-match instead of regex
- [ ] LRU cache supports concurrent access without single-mutex contention
- [ ] Upstream paginated fetches run in parallel (3-5 concurrent page fetches)
- [ ] p99 latency reduced by 15-25% under sustained load
- [ ] Empty upstream results return 200 with empty data array (not 502)
- [ ] `/version` endpoint returns build version, build date, deploy date, uptime, Go version
- [ ] Prometheus gauges export version info and uptime for scraping
- [ ] No regressions in existing tests

## Features

### 1. MongoDB Query Restructure
Replace regex-based `cache_key` matching with an indexed `cache_key_prefix` field for exact-match lookups. Current regex queries can't use the index effectively, causing O(n) collection scans for multi-page responses.

### 2. LRU Cache Sharding
Split the single-mutex LRU cache into 8-16 sharded buckets with independent mutexes. Reduces lock contention at 150 concurrent requests.

### 3. Parallel Page Fetches
Fetch upstream API pages concurrently (3-5 at a time) instead of sequentially. Reduces multi-page fetch time from N*latency to latency+(N/concurrency)*latency.

### 4. Empty Upstream Result Handling
When an upstream API returns valid but empty results (e.g., NDC lookup for a non-existent NDC), return `200` with `{"data": [], "meta": {"results_count": 0, ...}}` instead of treating it as an error. Consumers currently can't distinguish "upstream failed" from "no matching records" — this causes confusion and false error alerts. Identified during v0.7.1 stress testing where `fda-ndc?NDC={random}` showed 0% success despite working correctly.

### 5. Version & Deployment Info Endpoint
Add `GET /version` returning build and runtime metadata: version string, build date (embedded via `-ldflags`), deployment start time, uptime duration, Go version, GOMAXPROCS, and configured endpoint count. Also export as Prometheus metrics: `cashdrugs_build_info` gauge with labels `version`, `go_version`, `build_date` (set to 1), and `cashdrugs_uptime_seconds` gauge tracking process uptime. Gives operators a single endpoint to verify what's running and when it was deployed, and enables Grafana annotations on version changes.

## Dependencies

- Requires M9 features to be stable in production

## Risks

| Risk | Mitigation |
|------|------------|
| MongoDB schema change requires migration | Add new field alongside existing, backfill, then switch queries |
| LRU sharding adds complexity | Use consistent hashing; benchmark before/after |
| Parallel fetches may trigger upstream rate limits | Add configurable concurrency cap per endpoint |

---
*Milestone for cash-drugs. Driven by performance profiling (2026-03-14).*

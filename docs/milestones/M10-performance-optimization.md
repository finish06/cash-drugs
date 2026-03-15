# M10 — Performance Optimization

**Goal:** Address remaining performance bottlenecks and API usability gaps: MongoDB query efficiency, LRU cache contention, upstream fetch parallelism, and empty upstream result handling.

**Appetite:** Small-Medium — 4 targeted improvements

**Target Maturity:** Beta

**Status:** LATER

## Success Criteria

- [ ] MongoDB cache lookups use indexed exact-match instead of regex
- [ ] LRU cache supports concurrent access without single-mutex contention
- [ ] Upstream paginated fetches run in parallel (3-5 concurrent page fetches)
- [ ] p99 latency reduced by 15-25% under sustained load
- [ ] Empty upstream results return 200 with empty data array (not 502)
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

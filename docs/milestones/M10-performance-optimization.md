# M10 — Performance Optimization

**Goal:** Address remaining performance bottlenecks identified by profiling after M9: MongoDB query efficiency, LRU cache contention, and upstream fetch parallelism.

**Appetite:** Small-Medium — 3 targeted optimizations

**Target Maturity:** Beta

**Status:** LATER

## Success Criteria

- [ ] MongoDB cache lookups use indexed exact-match instead of regex
- [ ] LRU cache supports concurrent access without single-mutex contention
- [ ] Upstream paginated fetches run in parallel (3-5 concurrent page fetches)
- [ ] p99 latency reduced by 15-25% under sustained load
- [ ] No regressions in existing tests

## Features

### 1. MongoDB Query Restructure
Replace regex-based `cache_key` matching with an indexed `cache_key_prefix` field for exact-match lookups. Current regex queries can't use the index effectively, causing O(n) collection scans for multi-page responses.

### 2. LRU Cache Sharding
Split the single-mutex LRU cache into 8-16 sharded buckets with independent mutexes. Reduces lock contention at 150 concurrent requests.

### 3. Parallel Page Fetches
Fetch upstream API pages concurrently (3-5 at a time) instead of sequentially. Reduces multi-page fetch time from N*latency to latency+(N/concurrency)*latency.

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

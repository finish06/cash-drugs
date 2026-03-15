# Cycle 2 — M10 Query & Fetch Optimization

**Milestone:** M10 — Query & Fetch Optimization
**Maturity:** Beta
**Status:** PLANNED
**Started:** TBD
**Completed:** TBD
**Duration Budget:** 2 days

## Work Items

| Feature | Current Pos | Target Pos | Track | Est. Effort | Validation |
|---------|-------------|-----------|-------|-------------|------------|
| MongoDB Query Optimization | PLANNED | VERIFIED | Track 1 | ~4h | 13 ACs passing, regex eliminated, backfill migration works, benchmark shows improvement |
| LRU Cache Sharding | PLANNED | VERIFIED | Track 1 | ~3h | 14 ACs passing, 16-shard default, concurrent benchmark faster than 1-shard |
| Parallel Page Fetches | PLANNED | VERIFIED | Track 2 | ~3.5h | 13 ACs passing, 6-page fetch in ~2x single-page time, no goroutine leaks |
| Empty Upstream Results | PLANNED | VERIFIED | Track 2 | ~2.5h | 13 ACs passing, empty results return 200, results_count in meta, short LRU TTL |
| Version Endpoint | PLANNED | VERIFIED | Track 3 | ~2.5h | 15 ACs passing, /version returns all fields, build_info + uptime metrics exported |

## Dependencies & Serialization

```
Track 1 (serial chain — cache layer):
  MongoDB Query Optimization (adds base_key, changes mongo queries)
      ↓
  LRU Cache Sharding (replaces single-mutex LRU with sharded)

Track 2 (serial chain — upstream/handler layer):
  Parallel Page Fetches (refactors fetchJSON for concurrency)
      ↓
  Empty Upstream Results (modifies fetcher empty handling + handler response)

Track 3 (independent):
  Version Endpoint (new handler, new metrics, Dockerfile update)
```

**Serial rationale:**
- Track 1: LRU sharding depends on MongoDB query optimization being stable — both touch the cache layer, and sharding tests rely on consistent cache behavior.
- Track 2: Both features modify `internal/upstream/fetcher.go`. Parallel page fetches restructures the fetch loop; empty upstream results modifies how empty data arrays flow through the same code path. Serial avoids merge conflicts.
- Track 3: Version endpoint is fully independent — new handler file, additive metrics, Dockerfile change. No overlap with Track 1 or 2.

## Parallel Strategy

### File Reservations

**Track 1 (Agent 1):**
- `internal/model/cache.go` (add BaseKey field)
- `internal/cache/mongo.go` (query optimization, backfill migration)
- `internal/cache/mongo_test.go` (new tests)
- `internal/cache/lru.go` (sharded LRU replacement)
- `internal/cache/lru_test.go` (sharding tests)
- `internal/cache/collection.go` (verify no regex usage)

**Track 2 (Agent 2):**
- `internal/upstream/fetcher.go` (parallel fetches, empty result handling)
- `internal/upstream/fetcher_test.go` (concurrency tests, empty result tests)
- `internal/handler/cache.go` (empty result 200, ResultsCount, short LRU TTL)
- `internal/handler/cache_test.go` (empty result handler tests)
- `internal/model/response.go` (add ResultsCount field)
- `internal/config/loader.go` (add FetchConcurrency field)
- `internal/config/loader_test.go` (config tests)

**Track 3 (Agent 3):**
- `internal/handler/version.go` (new file)
- `internal/handler/version_test.go` (new file)
- `internal/metrics/metrics.go` (add BuildInfo + UptimeSeconds gauges)
- `internal/metrics/system_collector.go` (update UptimeSeconds in loop)
- `cmd/server/main.go` (build vars, startTime, wire VersionHandler)
- `Dockerfile` (ldflags update)

### Shared Files (Merge Sequence)

| File | Tracks | Who Merges First | Reason |
|------|--------|-------------------|--------|
| `internal/metrics/metrics.go` | Track 3 | Track 3 first | Additive gauges, smallest change surface |
| `cmd/server/main.go` | Track 2, Track 3 | Track 3 first | Track 3 adds build vars + version handler wiring; Track 2 adds FetchConcurrency wiring |
| `internal/model/response.go` | Track 2 | Track 2 only | Only Track 2 modifies this file (ResultsCount) |
| `internal/config/loader.go` | Track 2 | Track 2 only | Only Track 2 modifies this file (FetchConcurrency) |

### Merge Sequence

1. **Track 3: Version Endpoint** — merges first (fully independent, smallest surface, touches shared files first)
2. **Track 2: Parallel Page Fetches + Empty Upstream Results** — merges second (rebases on Track 3's changes to `cmd/server/main.go`)
3. **Track 1: MongoDB Query Optimization + LRU Cache Sharding** — merges last (no shared files with Track 2 or 3, cleanest merge)

Integration tests run after each merge.

## Execution Plan

### Day 1

**Track 1 (Agent 1):** MongoDB Query Optimization
- Phase 1: RED — write failing tests for base_key queries, migration (1.5h)
- Phase 2: GREEN — implement base_key field, query changes, backfill (1.5h)
- Phase 3: REFACTOR — remove regex functions, clean up (0.5h)
- Phase 4: VERIFY — coverage, benchmark, spec compliance (0.5h)
- **Output:** Feature complete, tests passing

**Track 2 (Agent 2):** Parallel Page Fetches
- Phase 1: RED — write failing tests for concurrent fetch, cap, error handling (1h)
- Phase 2: GREEN — refactor fetchJSON for goroutine pool, config field (1.5h)
- Phase 3: REFACTOR — extract helper, review goroutine lifecycle (0.5h)
- Phase 4: VERIFY — coverage, timing test, spec compliance (0.5h)
- **Output:** Feature complete, tests passing

**Track 3 (Agent 3):** Version Endpoint
- Phase 1: RED — write failing tests for handler, metrics, limiter exemption (0.75h)
- Phase 2: GREEN — implement handler, metrics, wiring, Dockerfile (1h)
- Phase 3: REFACTOR — regression tests, Dockerfile update (0.33h)
- Phase 4: VERIFY — swagger, coverage, spec compliance (0.25h)
- **Output:** Feature complete, PR ready. **Merge first.**

### Day 2

**Track 1 (Agent 1):** LRU Cache Sharding
- Phase 1: RED — write failing tests for sharding, routing, budget split (1h)
- Phase 2: GREEN — implement shardedLRUCache, shardIndex, constructor (1.5h)
- Phase 3: REFACTOR — clean up old single-mutex code, regression (0.25h)
- Phase 4: VERIFY — benchmark, coverage, spec compliance (0.25h)
- **Output:** Feature complete, tests passing

**Track 2 (Agent 2):** Empty Upstream Results
- Phase 1: RED — write failing tests for empty results, ResultsCount, caching (0.75h)
- Phase 2: GREEN — implement model field, fetcher fix, handler changes (1h)
- Phase 3: REFACTOR — clean up response building, regression (0.33h)
- Phase 4: VERIFY — swagger, coverage, spec compliance (0.25h)
- **Output:** Feature complete, tests passing

**End of Day 2:** Merge Track 2 (rebase on Track 3), then merge Track 1. Full regression test. Update milestone hill chart.

## Validation Criteria

### Per-Item Validation

- **MongoDB Query Optimization:** 13 ACs passing. `Get()` uses `$eq` on `base_key` not `$regex`. `backfillBaseKey()` migrates existing docs. Benchmark shows exact-match faster than regex at 500+ docs. `buildRegexFilter()` removed.
- **LRU Cache Sharding:** 14 ACs passing. `NewLRUCache(N)` returns 16-shard cache. Concurrent benchmark shows reduced contention vs 1-shard. `SizeBytes()` aggregates all shards. `noopLRU` preserved for `maxBytes <= 0`.
- **Parallel Page Fetches:** 13 ACs passing. 6-page fetch at concurrency=3 completes in ~2x single-page latency. Error fails entire fetch (page-style). Partial results returned (offset-style). No goroutine leaks.
- **Empty Upstream Results:** 13 ACs passing. Empty upstream returns 200 with `"data": []`. `results_count` in all response metas. Short LRU TTL for empty results. Prometheus counts empty as 200.
- **Version Endpoint:** 15 ACs passing. `/version` returns all 12 fields. `cashdrugs_build_info{version="...",git_commit="..."}` exported. `/version` exempt from limiter. Dockerfile passes ldflags.

### Cycle Success Criteria

- [ ] All 5 features reach VERIFIED position
- [ ] All 68 acceptance criteria have passing tests
- [ ] Code coverage >= 80%
- [ ] `go vet ./...` clean
- [ ] All existing tests still pass (no regressions)
- [ ] Code review completed on all PRs
- [ ] Swagger docs regenerated
- [ ] Merge sequence followed (Track 3 -> Track 2 -> Track 1)
- [ ] Integration tests pass after each merge

## Agent Autonomy & Checkpoints

**Beta maturity, human available:** Balanced autonomy.
- Human approves cycle plan at start (this document)
- Agents execute TDD cycles autonomously
- Agents create PRs for human review
- Human reviews and approves merges
- Human available for questions via quick check

## Notes

- Track 1 features are purely storage/cache layer — no API contract changes
- Track 2 features touch the upstream fetch and handler response — `results_count` is an additive API change
- Track 3 is the cleanest — new endpoint, no modifications to existing behavior
- All 5 specs and plans are complete and approved
- Total estimated effort: ~15.5h across 3 parallel tracks over 2 days

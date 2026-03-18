# Cycle 4 — M12: Upstream 404 Handling

**Milestone:** M12 — Client Enhancements
**Maturity:** Beta
**Status:** COMPLETE
**Started:** 2026-03-17
**Completed:** 2026-03-18
**Duration Budget:** 1 day

## Work Items

| Feature | Current Pos | Target Pos | Est. Effort | Validation |
|---------|-------------|-----------|-------------|------------|
| Upstream 404 handling | SPECCED | VERIFIED | ~4h | 10 ACs passing, all existing tests pass, coverage ≥ 80% |

## Dependencies & Serialization

```
Update spec with review decisions
    ↓
RED: Write failing tests (10 ACs)
    ↓
GREEN: Implement in fetcher.go, cache.go, mongo.go, model
    ↓
REFACTOR: Clean up
    ↓
VERIFY: Full test suite + lint + coverage
```

Single-threaded execution. No parallel agents needed.

## Implementation Plan

### Files to modify:
- `internal/model/response.go` — Add `NotFound bool` field to `CachedResponse`
- `internal/upstream/fetcher.go` — Propagate upstream HTTP 404 as distinct error type (not generic error)
- `internal/handler/cache.go` — Detect 404 error from fetcher, return 404 to consumer, store negative cache
- `internal/cache/mongo.go` — Support storing/retrieving negative cache entries with 10-min TTL
- `internal/metrics/metrics.go` — Add `upstream_404_total` counter per slug

### Key design decisions (from spec review):
1. **Negative entry takes precedence** — don't delete existing cached data. `not_found=true` checked first. Safer rollback if 404 was transient.
2. **Prometheus counter** — `upstream_404_total` per slug for Grafana visibility.
3. **Both LRU + MongoDB** — negative entries stored in both cache layers for sub-ms repeated 404 lookups.

## Validation Criteria

### Per-AC Validation
- AC-001: Upstream 404 → consumer gets 404 (not 502)
- AC-002: 404 body includes `error`, `slug`, `params`
- AC-003: Other 4xx (401, 403, 429) → still 502
- AC-004: 5xx + network errors → still 502 with stale fallback
- AC-005: 404 overrides stale cache (negative entry takes precedence)
- AC-006: Negative cache stored with 10-min TTL
- AC-007: Repeated requests served from negative cache
- AC-008: After TTL expires, re-checks upstream
- AC-009: Existing endpoints unaffected
- AC-010: TTL hardcoded 10 minutes (not configurable)

### Cycle Success Criteria
- [x] All 10 ACs have passing tests (12 tests)
- [x] golangci-lint passes (0 issues)
- [x] go vet passes
- [x] Coverage ≥ 80% (80.7%)
- [x] All existing tests pass (no regressions)
- [x] Spec updated to v1.0.0 with review decisions
- [x] PR #20 merged to main

## Agent Autonomy & Checkpoints

Beta + away mode: Agent executes full TDD cycle autonomously.
- Update spec with review decisions
- Write failing tests (RED)
- Implement minimal code (GREEN)
- Refactor
- Run /add:verify
- Commit to feature branch
- Create PR for human review on return

## Notes

- Spec edge case: scheduled refresh 404 → log warning + increment `upstream_404_total`, do NOT store negative cache
- Mid-pagination 404 → stop pagination, return partial data
- Concurrent requests for same non-existent key → singleflight deduplicates, first caches 404

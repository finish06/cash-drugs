# Cycle 3 — M11 RxNorm Integration + Parameterized Warmup

**Milestone:** M11 — RxNorm API Integration
**Maturity:** Beta
**Status:** IN_PROGRESS
**Started:** 2026-03-16
**Completed:** TBD
**Duration Budget:** 1 day

## Work Items

| Feature | Current Pos | Target Pos | Est. Effort | Validation |
|---------|-------------|-----------|-------------|------------|
| RxNorm config endpoints (6) | DONE | DONE | Config-only | E2E tests pass (17/17) |
| warmup-queries.yaml (top 100 drugs) | DONE | DONE | Config-only | File created with 196 queries |
| Parameterized warmup handler | IN_PROGRESS | VERIFIED | ~3h | 15 ACs, handler tests pass |
| Parameterized warmup wiring | PLANNED | VERIFIED | ~1h | main.go integration, runtime tests |

## Dependencies & Serialization

```
RxNorm config (DONE)
    ↓
warmup-queries.yaml (DONE)
    ↓
Parameterized warmup handler (PR #16 merged)
    ↓
Wiring in main.go (remaining)
```

## Validation Criteria

- [ ] All 6 RxNorm endpoints return cached data
- [ ] POST /api/warmup loads warmup-queries.yaml and executes parameterized queries
- [ ] GET /ready shows progress through parameterized queries
- [ ] Failed queries don't block readiness
- [ ] Concurrency cap (5) respected during warmup
- [ ] Circuit breaker skips open slugs during warmup

## Notes

- RxNorm endpoints and warmup-queries.yaml are config-only (no code changes)
- Parameterized warmup handler (PR #16) merged — interfaces ready, needs main.go wiring
- 3 deferred ACs (concurrency cap, env var path, circuit breaker) need integration wiring

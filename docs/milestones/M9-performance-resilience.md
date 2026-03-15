# M9 — Performance & Resilience

**Goal:** Prevent service collapse under concurrent load, reduce response latency and bandwidth, protect against upstream API instability, and export container-level system metrics — addressing all findings from the 2026-03-14 stress test.

**Appetite:** Medium-Large — 4 features across middleware, cache, upstream, and metrics layers

**Target Maturity:** Beta

**Status:** NOW

## Success Criteria

- [ ] Service handles 100 concurrent connections without connection refused errors
- [ ] Overloaded requests receive 503 + Retry-After instead of hard failures
- [ ] `/health` and `/metrics` respond under any load level
- [ ] Bulk endpoint responses compressed 3-5x via gzip
- [ ] Concurrent identical requests deduplicated via singleflight
- [ ] Hot responses served from in-memory LRU cache (sub-5ms)
- [ ] Failing upstreams trigger circuit breaker (no cascading failures)
- [ ] Force-refresh rate-limited (30s per-key cooldown)
- [ ] Container CPU, memory, disk, network metrics exported to Prometheus
- [ ] All existing tests pass, coverage ≥ 80%

## Hill Chart

| Feature | Position |
|---------|----------|
| Connection Resilience | PLANNED |
| Response Optimization | PLANNED |
| Upstream Resilience | PLANNED |
| Container System Metrics | PLANNED |

## Features

### 1. Connection Resilience (P0 — Service Stability)

Concurrency limiter middleware (50 max in-flight), HTTP server timeouts (10s/30s/60s), health/metrics exemption from limits. Returns 503 + Retry-After instead of connection refused.

- **Spec:** `specs/connection-resilience.md`
- **Plan:** `docs/plans/connection-resilience-plan.md`
- **Effort:** ~4.5h

### 2. Response Optimization (P1 — Performance)

Gzip compression middleware, singleflight request coalescing, in-memory LRU cache (256 MB default) in front of MongoDB.

- **Spec:** `specs/response-optimization.md`
- **Plan:** `docs/plans/response-optimization-plan.md`
- **Effort:** ~6.5h
- **Depends on:** Connection Resilience (shared `internal/middleware/` package)

### 3. Upstream Resilience (P2 — Resilience)

Per-endpoint circuit breakers (gobreaker), force-refresh 30s per-key cooldown. Scheduler respects circuit state.

- **Spec:** `specs/upstream-resilience.md`
- **Plan:** `docs/plans/upstream-resilience-plan.md`
- **Effort:** ~6h
- **Depends on:** Response Optimization (singleflight runs before circuit breaker)

### 4. Container System Metrics (Independent)

CPU, memory, disk, network metrics from procfs/cgroup exported via `/metrics`. Linux-only (amd64/arm64). Follows MongoCollector pattern.

- **Spec:** `specs/container-system-metrics.md`
- **Plan:** `docs/plans/container-system-metrics-plan.md`
- **Effort:** ~5h
- **Depends on:** Nothing (independent)

## Dependencies

```
Connection Resilience
    ↓
Response Optimization
    ↓
Upstream Resilience

Container System Metrics (parallel, independent)
```

## Risks

| Risk | Mitigation |
|------|------------|
| Middleware chain ordering bugs | Test each middleware independently, then integration test the full chain |
| LRU size estimation inaccuracy | Use rough byte estimates; exact sizing less critical than order of magnitude |
| gobreaker state transitions don't match spec | gobreaker is mature; write explicit state transition tests |
| procfs parsing breaks on unusual kernel versions | Parse defensively, skip unknown lines, test with fixtures |
| Cross-feature merge conflicts | 2-track parallel strategy with file reservations |

## Cycles

**Cycle 1:** All 4 features (2 parallel tracks, ~22h, ~3 days)

---
*Milestone for cash-drugs. Driven by stress test results (2026-03-14).*

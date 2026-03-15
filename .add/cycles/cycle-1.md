# Cycle 1 — M9 Performance & Resilience

**Milestone:** M9 — Performance & Resilience
**Maturity:** Beta
**Status:** PLANNED
**Started:** TBD
**Completed:** TBD
**Duration Budget:** 3 days

## Work Items

| Feature | Current Pos | Target Pos | Track | Est. Effort | Validation |
|---------|-------------|-----------|-------|-------------|------------|
| Connection Resilience | PLANNED | VERIFIED | Track 1 | ~4.5h | 11 ACs passing, 503 + Retry-After under load, health/metrics exempt |
| Response Optimization | PLANNED | VERIFIED | Track 1 | ~6.5h | 18 ACs passing, gzip 3-5x compression, LRU sub-5ms, singleflight dedup |
| Upstream Resilience | PLANNED | VERIFIED | Track 1 | ~6h | 18 ACs passing, circuit opens after 5 failures, 30s cooldown works |
| Container System Metrics | PLANNED | VERIFIED | Track 2 | ~5h | 15 ACs passing, CPU/mem/disk/net in /metrics output |

## Dependencies & Serialization

```
Track 1 (serial chain):
  Connection Resilience (creates internal/middleware/)
      ↓
  Response Optimization (adds gzip to middleware/, singleflight + LRU to handler)
      ↓
  Upstream Resilience (adds circuit breaker + cooldown, integrates with singleflight)

Track 2 (parallel, independent):
  Container System Metrics (new SystemCollector in internal/metrics/)
```

## Parallel Strategy

### File Reservations

**Track 1 (Agent 1):**
- `internal/middleware/**` (creates package, owns limiter + gzip)
- `internal/handler/cache.go` (singleflight, LRU, circuit, cooldown integration)
- `internal/cache/lru.go` (new LRU cache)
- `internal/upstream/circuit.go` (new circuit breaker registry)
- `internal/upstream/cooldown.go` (new cooldown tracker)
- `internal/config/loader.go` (new AppConfig fields)
- `cmd/server/main.go` (wiring)
- `internal/metrics/metrics.go` (new collectors — coordinate with Track 2)

**Track 2 (Agent 2):**
- `internal/metrics/system.go` (new SystemSource interface)
- `internal/metrics/system_linux.go` (new procfs implementation)
- `internal/metrics/system_collector.go` (new SystemCollector)
- `internal/metrics/system_collector_test.go` (new tests)
- `internal/metrics/system_test.go` (new tests)
- `internal/metrics/testdata/**` (new fixtures)

**Shared files (serialize access):**
- `internal/metrics/metrics.go` — Track 1 adds limiter/LRU/circuit collectors, Track 2 adds container collectors. **Merge sequence:** Track 2 merges first (smaller change), Track 1 rebases.
- `internal/config/loader.go` — Track 1 adds `MaxConcurrentRequests`, `LRUCacheSizeMB`. Track 2 adds `SystemMetricsInterval`. **Merge sequence:** Track 2 merges first.
- `cmd/server/main.go` — Both tracks wire their collectors. **Merge sequence:** Track 2 merges first.

### Merge Sequence

1. **Container System Metrics** (Track 2) — merges first (independent, smaller surface area)
2. **Connection Resilience** (Track 1, step 1) — merges second
3. **Response Optimization** (Track 1, step 2) — merges third
4. **Upstream Resilience** (Track 1, step 3) — merges last

Integration tests run after each merge.

## Execution Plan

### Day 1

**Track 1 (Agent 1):** Connection Resilience
- Phase 1: RED — write failing tests for limiter middleware (1.5h)
- Phase 2: GREEN — implement limiter, config field, server timeouts (2h)
- Phase 3: REFACTOR + VERIFY (1h)
- **Output:** PR ready for review

**Track 2 (Agent 2):** Container System Metrics
- Phase 1: RED — write failing tests with fixtures (1.5h)
- Phase 2: GREEN — implement SystemSource, procfs parsing, SystemCollector (2.5h)
- Phase 3: REFACTOR + VERIFY (1h)
- **Output:** PR ready for review

### Day 2

**Track 1 (Agent 1):** Response Optimization
- Phase 1: RED — write failing tests for gzip, LRU, singleflight (2h)
- Phase 2: GREEN — implement gzip middleware, LRU cache, singleflight integration (3.5h)
- Phase 3: REFACTOR + VERIFY (1h)
- **Output:** PR ready for review (depends on connection-resilience merge)

**Track 2:** Merge container-system-metrics PR. Run integration tests.

### Day 3

**Track 1 (Agent 1):** Upstream Resilience
- Phase 1: RED — write failing tests for circuit breaker, cooldown (2h)
- Phase 2: GREEN — implement circuit registry, cooldown tracker, handler/scheduler integration (3h)
- Phase 3: REFACTOR + VERIFY (1h)
- **Output:** PR ready for review (depends on response-optimization merge)

**Final:** Merge all PRs in sequence. Full regression test. Update milestone hill chart.

## Validation Criteria

### Per-Item Validation

- **Connection Resilience:** 11 ACs passing. `make test-unit` green. 50 concurrent → all succeed, 51st → 503. `/health` responds under full load.
- **Response Optimization:** 18 ACs passing. Gzip reduces `fda-enforcement` response 3-5x. 10 concurrent identical requests → 1 upstream fetch. LRU second-request < 5ms.
- **Upstream Resilience:** 18 ACs passing. 5 upstream failures → circuit opens. 30s later half-open probe succeeds → circuit closes. Force-refresh cooldown blocks within 30s.
- **Container System Metrics:** 15 ACs passing. `/metrics` output contains `cashdrugs_container_cpu_usage_seconds_total`, `cashdrugs_container_memory_rss_bytes`, `cashdrugs_container_disk_total_bytes`, `cashdrugs_container_network_receive_bytes_total`.

### Cycle Success Criteria

- [ ] All 4 features reach VERIFIED position
- [ ] All 62 acceptance criteria have passing tests
- [ ] Code coverage ≥ 80%
- [ ] `go vet ./...` clean
- [ ] All existing tests still pass (no regressions)
- [ ] Code review completed on all PRs
- [ ] Swagger docs regenerated

## Agent Autonomy & Checkpoints

**Beta maturity, human available:** Balanced autonomy.
- Human approves cycle plan at start (this document)
- Agents execute TDD cycles autonomously
- Agents create PRs for human review
- Human reviews and approves merges
- Human available for questions via quick check

## Notes

- Stress test results driving this work: `reports/stress-test-results.md`
- Track 1 features must merge in order due to `internal/middleware/` package creation and handler integration
- Track 2 is fully independent — can start and finish without waiting for Track 1
- All 4 specs and plans are complete and approved

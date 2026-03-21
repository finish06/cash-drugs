# Cycle 11 — M17: Intelligent Data Layer (Quick Wins)

**Milestone:** M17 — Intelligent Data Layer
**Maturity:** Beta
**Status:** IN_PROGRESS
**Started:** 2026-03-21
**Completed:** TBD
**Duration Budget:** 2-3 hours

## Work Items

| Feature | Current Pos | Target Pos | Assigned | Est. Effort | Validation |
|---------|-------------|-----------|----------|-------------|------------|
| Go pprof endpoints | SHAPED | DONE | Agent-1 | ~30 min | pprof on :6060, not exposed on :8080 |
| MongoDB TTL indexes | SHAPED | DONE | Agent-2 | ~1 hour | TTL index on updated_at, stale docs auto-expire |
| Performance benchmarks | SHAPED | DONE | Agent-3 | ~1 hour | go test -bench suite with committed baselines |

## Dependencies & Serialization

All independent.

```
pprof endpoints ──────┐
MongoDB TTL indexes ──┤── all parallel
Performance benchmarks┘
```

## Parallel Strategy

3 agents in parallel. No file conflicts.

### File Reservations
- **Agent-1 (pprof):** cmd/server/main.go (add pprof listener)
- **Agent-2 (TTL):** internal/cache/mongo.go (add TTL index creation)
- **Agent-3 (benchmarks):** internal/cache/benchmark_test.go, internal/handler/benchmark_test.go

## Validation Criteria

### Per-Item Validation
- **pprof:** GET localhost:6060/debug/pprof/ returns profile index. Not accessible on :8080.
- **TTL indexes:** MongoDB documents older than 2x endpoint TTL are automatically deleted. ensureIndexes creates TTL index on updated_at.
- **Benchmarks:** go test -bench suite covering LRU get/set, MongoDB get, handler ServeHTTP. Results committed as baseline.

### Cycle Success Criteria
- [ ] All 3 features reach DONE
- [ ] All existing tests pass
- [ ] Coverage remains >= 95%
- [ ] go vet clean
- [ ] Benchmarks produce reproducible results

## Agent Autonomy & Checkpoints

**Mode:** Active collaboration. Implement, verify, check in per feature.

## Notes

- pprof should be on a separate net/http server (not the main mux) for security
- TTL index: use 2x the endpoint's TTL duration, default 24h if no TTL configured
- Benchmarks: use testing.B, table-driven with different cache sizes
- Remaining M17 features (search, autocomplete, field filtering) deferred to cycle 12

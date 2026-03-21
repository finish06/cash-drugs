# Cycle 8 — M15: Consumer Value (API Features)

**Milestone:** M15 — Consumer Value & API Ergonomics
**Maturity:** Beta
**Status:** COMPLETE
**Started:** 2026-03-21
**Completed:** 2026-03-21
**Duration Budget:** 1 day

## Work Items

| Feature | Current Pos | Target Pos | Assigned | Est. Effort | Validation |
|---------|-------------|-----------|----------|-------------|------------|
| Per-slug metadata | SHAPED | DONE | Agent-1 | ~3 hours | GET /api/cache/{slug}/_meta returns cache state |
| Bulk query API | SHAPED | DONE | Agent-2 | ~5 hours | POST /api/cache/{slug}/bulk with concurrent lookups |
| Rich endpoint discovery | SHAPED | DONE | Agent-3 | ~4 hours | GET /api/endpoints returns param docs + schemas |

## Dependencies & Serialization

All 3 features are independent — no serialization required.

```
Per-slug metadata ────┐
Bulk query API ───────┤── all parallel, no dependencies
Rich discovery ───────┘
```

Go client SDK deferred to cycle 9 (depends on these 3 stabilizing the API surface).

## Parallel Strategy

3 agents in parallel with file reservations.

### File Reservations
- **Agent-1 (metadata):** internal/handler/meta.go, internal/handler/meta_test.go
- **Agent-2 (bulk):** internal/handler/bulk.go, internal/handler/bulk_test.go
- **Agent-3 (discovery):** internal/handler/endpoints.go, internal/handler/endpoints_test.go
- **Shared (serialize):** cmd/server/main.go (route registration — merge sequentially)

### Merge Sequence
1. Per-slug metadata (smallest, cleanest)
2. Bulk query API (new endpoint pattern)
3. Rich endpoint discovery (modifies existing handler)

## Validation Criteria

### Per-Item Validation
- **Per-slug metadata:** GET /api/cache/{slug}/_meta returns last_refreshed, ttl_remaining, is_stale, page_count, record_count, circuit_state. 15 ACs from spec.
- **Bulk query API:** POST /api/cache/{slug}/bulk handles 100 queries with concurrent lookups, partial success, Prometheus metrics. 15 ACs from spec.
- **Rich endpoint discovery:** GET /api/endpoints returns param metadata, example URLs, response schemas, cache status. 10 ACs from spec.

### Cycle Success Criteria
- [ ] All 3 features reach DONE position
- [ ] All acceptance criteria verified
- [ ] All existing tests pass + new tests for each feature
- [ ] Coverage remains >= 95%
- [ ] go vet clean
- [ ] No regressions
- [ ] k6 smoke test passes on staging

## Agent Autonomy & Checkpoints

**Mode:** Autonomous execution. 3 agents implement in parallel, orchestrator verifies and merges.

**Checkpoint:** Single check-in after all features implemented and tested.

## Notes

- Each feature gets its own handler file (no shared mutable files except main.go)
- Bulk query needs new Prometheus metrics (bulk_request_size, bulk_request_duration)
- Rich discovery enhances existing EndpointsHandler — Agent-3 must read current implementation first
- AllowMethods middleware: metadata is GET, bulk is POST — update middleware accordingly

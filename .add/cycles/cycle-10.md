# Cycle 10 — M16: Operational Resilience (Hot Reload + Chaos + Config Validate)

**Milestone:** M16 — Operational Resilience & Runtime Management
**Maturity:** Beta
**Status:** IN_PROGRESS
**Started:** 2026-03-21
**Completed:** TBD
**Duration Budget:** 4 hours (away mode)

## Work Items

| Feature | Current Pos | Target Pos | Assigned | Est. Effort | Validation |
|---------|-------------|-----------|----------|-------------|------------|
| Hot config reload | SHAPED | DONE | Agent-1 | ~3 hours | fsnotify + SIGHUP, new slugs route immediately |
| Chaos test suite | SHAPED | DONE | Agent-2 | ~2 hours | 4+ Go tests against Docker stack |
| Config validation endpoint | SHAPED | DONE | Agent-3 | ~1 hour | POST /api/config/validate |

## Dependencies & Serialization

All independent — no serialization required.

```
Hot config reload ────┐
Chaos test suite ─────┤── all parallel
Config validate ──────┘
```

## Parallel Strategy

3 agents in parallel with file reservations.

### File Reservations
- **Agent-1 (hot reload):** internal/config/watcher.go, cmd/server/main.go (reload logic)
- **Agent-2 (chaos):** tests/chaos/*.go (new directory, no conflicts)
- **Agent-3 (config validate):** internal/handler/configvalidate.go, internal/handler/configvalidate_test.go

### Merge Sequence
1. Config validate (smallest, cleanest)
2. Hot reload (touches main.go)
3. Chaos tests (independent, tests/ directory)

## Validation Criteria

### Per-Item Validation
- **Hot reload:** config.yaml changes detected via fsnotify within 5s. SIGHUP triggers reload. New slugs added to router. Removed slugs stop scheduling. Zero downtime.
- **Chaos tests:** 4+ tests: kill MongoDB (stale-serve), block upstream (circuit breaker), exhaust concurrency (503+Retry-After), SIGTERM (graceful shutdown).
- **Config validate:** POST /api/config/validate accepts YAML, validates format and required fields, returns pass/fail with details.

### Cycle Success Criteria
- [ ] All 3 features reach DONE position
- [ ] All existing tests pass + new tests
- [ ] Coverage remains >= 95%
- [ ] go vet clean
- [ ] No regressions

## Agent Autonomy & Checkpoints

**Mode:** Away mode — autonomous execution for ~4 hours. Agent works through all 3 features, commits, pushes, checks in with results.

## Notes

- Hot reload: use fsnotify for file watching, signal.Notify for SIGHUP
- Hot reload must be safe: validate new config before swapping, rollback on error
- Chaos tests run against local Docker only (not CI — Docker required)
- Config validate is similar to test-fetch but only validates structure, doesn't fetch
- Add go.sum entry for fsnotify if not already present
- Update AllowMethods for /api/config/validate POST

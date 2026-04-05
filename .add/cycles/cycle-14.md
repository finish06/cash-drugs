# Cycle 14 — M19: rx-dag NDC Integration

**Milestone:** M19 — rx-dag NDC Integration
**Maturity:** Beta
**Status:** IN_PROGRESS
**Started:** 2026-04-04
**Completed:** TBD
**Duration Budget:** 1 day (away mode, ~6-8 hours)

## Work Items

| Feature | Current Pos | Target Pos | Assigned | Est. Effort | Validation |
|---------|-------------|-----------|----------|-------------|------------|
| Generic headers config | PLANNED | VERIFIED | Agent-1 | ~2h | Headers field parsed, env vars resolved, warning on missing vars |
| doRequest refactor | PLANNED | VERIFIED | Agent-1 | ~1h | All 3 Client.Get() calls replaced, headers applied, backward compat |
| fda-ndc upstream swap | PLANNED | VERIFIED | Agent-1 | ~30min | Config updated, same consumer contract, tests pass |
| 3 new rx-dag slugs | PLANNED | VERIFIED | Agent-1 | ~30min | Config entries added, params work, tests pass |
| Unit + integration tests | PLANNED | VERIFIED | Agent-1 | ~3h | ResolveHeaders, doRequest, config parsing, backward compat |
| Docs & polish | PLANNED | VERIFIED | Agent-1 | ~1h | Swagger updated, .env.example updated |

## Dependencies & Serialization

```
Generic headers config (Endpoint struct + ResolveHeaders)
    ↓
doRequest refactor (replace Client.Get calls)
    ↓
Config changes (fda-ndc swap + 3 new slugs)  ← can parallel with tests
    ↓
Tests (unit + integration)
    ↓
Docs & polish
    ↓
Feature branch commit + PR
```

Single-threaded execution (solo agent, away mode).

## Validation Criteria

### Per-Item Validation
- Generic headers: config parses with/without headers, env vars interpolated, missing vars logged
- doRequest: headers sent on upstream requests, no headers = no crash, existing tests pass
- fda-ndc swap: config points to rx-dag, search_params unchanged
- New slugs: config entries valid, params extracted correctly
- Tests: all new + existing tests pass, coverage >= 85%

### Cycle Success Criteria
- [ ] All work items reach VERIFIED
- [ ] All existing tests pass (no regressions)
- [ ] Coverage >= 85%
- [ ] go vet clean
- [ ] Feature branch committed
- [ ] PR created

## Agent Autonomy (Away Mode)

**Autonomous (proceed without asking):**
- Commit to feature branch
- Create PR
- Fix lint/type/test failures
- Read specs and plans

**Queued for human return:**
- E2E tests against live rx-dag
- Merge to main
- Deploy

## Notes

- Plan: docs/plans/rxdag-ndc-migration-plan.md
- Spec: specs/rxdag-ndc-migration.md
- Key files: internal/config/loader.go, internal/upstream/fetcher.go, config.yaml

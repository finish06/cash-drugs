# Cycle 9 — M16: Operational Resilience (Runbooks + Test-Fetch)

**Milestone:** M16 — Operational Resilience & Runtime Management
**Maturity:** Beta
**Status:** IN_PROGRESS
**Started:** 2026-03-21
**Completed:** TBD
**Duration Budget:** 1 day

## Work Items

| Feature | Current Pos | Target Pos | Assigned | Est. Effort | Validation |
|---------|-------------|-----------|----------|-------------|------------|
| Operational runbooks | SHAPED | DONE | Agent-1 | ~3 hours | 7 runbooks (one per alerting rule) + index |
| Test-fetch endpoint | SHAPED | DONE | Agent-2 | ~3 hours | POST /api/test-fetch validates configs dry-run |

## Dependencies & Serialization

Independent — no dependencies between runbooks and test-fetch.

```
Operational runbooks (Agent-1) ── docs only, no code
Test-fetch endpoint (Agent-2)  ── new handler + tests
```

## Parallel Strategy

2 agents in parallel. No file conflicts (runbooks are docs, test-fetch is code).

### File Reservations
- **Agent-1:** docs/runbooks/*, docs/runbooks/runbook-index.md
- **Agent-2:** internal/handler/testfetch.go, internal/handler/testfetch_test.go, cmd/server/main.go

## Validation Criteria

### Per-Item Validation
- **Runbooks:** 7 runbooks in docs/runbooks/ (one per alert in docs/grafana/alerts.yml). Each has: symptom, likely cause, diagnostic steps, remediation, escalation. Index at runbook-index.md.
- **Test-fetch:** POST /api/test-fetch accepts endpoint config JSON, fetches one page from upstream without caching, returns parsed result or detailed error. 400 for invalid config, 502 for upstream failure.

### Cycle Success Criteria
- [ ] All features reach DONE position
- [ ] 7 runbooks with consistent format
- [ ] Test-fetch endpoint works with real upstream
- [ ] All existing tests pass + new tests for test-fetch
- [ ] Coverage remains >= 95%
- [ ] go vet clean
- [ ] No regressions

## Agent Autonomy & Checkpoints

**Mode:** Autonomous execution. 2 agents implement in parallel, check in with results.

## Notes

- Remaining M16 features (hot reload, chaos tests, config validate) deferred to cycle 10
- Runbooks should reference the actual Prometheus metric names and PromQL from alerts.yml
- Test-fetch should use the existing upstream.Fetcher but skip cache storage
- Test-fetch needs POST method allowance in AllowMethods middleware

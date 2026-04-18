# M20 — Stack-Wide Health & Version Compliance

## Goal

Align cash-drugs' `/health` and `/version` endpoints with the stack-wide specification so that all services (rx-dag, cash-drugs, drug-gate, drugs-quiz BFF) return identical response shapes. Enables reusable dashboards, alerts, and smoke tests across the stack.

## Status: IN_PROGRESS

## Appetite: 1 day

## Success Criteria

- [ ] `/health` returns structured dependencies array with measured latency
- [ ] `/health` carries `uptime`, `start_time`, `cache_slug_count`, `leader`
- [ ] `/version` contains only build-time fields
- [ ] `build_date` → `build_time` field rename
- [ ] Test coverage >= 85%
- [ ] k6 smoke test updated and passing on staging
- [ ] PR created and reviewed

## Hill Chart

| Feature | Position | PR |
|---------|----------|----|
| /health stack-compliant shape | DONE | branch (d13f35b) |
| /version cleanup | DONE | branch (d13f35b) |
| k6 smoke updates | DONE | branch (d13f35b) |

## Dependencies

- Reference implementation: rx-dag (`dag-rx` repo) — matches target contract
- Breaking change to `/health` response — no known cash-drugs consumers read legacy `db` field
- No Dockerfile HEALTHCHECK (verified) — no container-orchestration breakage

## Risks

| Risk | Mitigation |
|------|-----------|
| Breaking `db` field affects unknown consumers | Documented in CHANGELOG; matches stack spec intentionally |
| Pinger interface change cascades | Narrow — only `MongoRepository` implements it |

## Retrospective

_To be filled at milestone completion._

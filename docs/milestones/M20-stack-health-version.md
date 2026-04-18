# M20 — Stack-Wide Health & Version Compliance

## Goal

Align cash-drugs' `/health` and `/version` endpoints with the stack-wide specification so that all services (rx-dag, cash-drugs, drug-gate, drugs-quiz BFF) return identical response shapes. Enables reusable dashboards, alerts, and smoke tests across the stack.

## Status: IN_PROGRESS

## Appetite: 1 day

## Success Criteria

- [x] `/health` returns structured dependencies array with measured latency
- [x] `/health` carries `uptime`, `start_time`, `cache_slug_count`, `leader`
- [x] `/version` contains only build-time fields
- [x] `build_date` → `build_time` field rename
- [x] Test coverage >= 85% — internal packages at 91.4% (`go test ./internal/...`, measured 2026-04-18). The `./...` figure of 83.5% is pulled down by `cmd/server/main.go` (untestable, per M13 precedent) and the `tests/e2e/` package (no production statements).
- [ ] k6 smoke test updated and passing on staging (script updated; staging run unverified)
- [x] PR created and reviewed (PR #24 merged 2026-04-11)

## Hill Chart

| Feature | Position | PR |
|---------|----------|----|
| /health stack-compliant shape | DONE | #24 |
| /version cleanup | DONE | #24 |
| k6 smoke updates | DONE | #24 |

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

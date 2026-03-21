# M13 — GA Readiness

## Goal

Address all remaining gaps from the Beta → GA promotion check. Prepare the project for General Availability maturity promotion on 2026-04-04 (30-day stability date).

## Status: IN_PROGRESS

## Appetite: 1 day

## Success Criteria

- [x] LICENSE file present (MIT)
- [x] PR template at `.github/pull_request_template.md`
- [x] SLA document with uptime, latency, and incident response targets
- [ ] Production monitoring verified (Prometheus scraping, alerting)
- [x] Test coverage >= 85% (93.6% excluding untestable cmd/server main)
- [x] Documentation audit complete (README, CHANGELOG, CLAUDE.md current)

## Hill Chart

| Feature | Position | PR |
|---------|----------|----|
| LICENSE file | DONE | — |
| PR template | DONE | — |
| SLA documentation | DONE | — |
| Production monitoring | PLANNED | — |
| Coverage to 85% | DONE | — |
| Documentation audit | DONE | — |

## Dependencies

- M8-M12 complete (all done)
- 30-day stability: Beta since 2026-03-07, eligible 2026-04-04

## Risks

| Risk | Mitigation |
|------|-----------|
| Coverage push may require testing hard-to-reach code paths | Focus on highest-impact gaps first |
| Production monitoring may need infra changes | Verify existing setup before adding new |

## Retrospective

_To be filled at milestone completion._

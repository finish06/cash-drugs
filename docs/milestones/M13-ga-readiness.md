# M13 — GA Readiness

## Goal

Address all remaining gaps from the Beta → GA promotion check. Prepare the project for General Availability maturity promotion on 2026-04-04 (30-day stability date).

## Status: COMPLETE

## Appetite: 1 day

## Success Criteria

- [x] LICENSE file present (MIT)
- [x] PR template at `.github/pull_request_template.md`
- [x] SLA document with uptime, latency, and incident response targets
- [x] Production monitoring verified — Prometheus scraping prod at 192.168.1.86:8083; dashboard live at grafana.calebdunn.tech/d/cashdrugs-api-v2 (env switcher covers staging + prod). Alertmanager wiring deferred — see PRD Deferred Items.
- [x] Test coverage >= 85% (93.6% excluding untestable cmd/server main)
- [x] Documentation audit complete (README, CHANGELOG, CLAUDE.md current)

## Hill Chart

| Feature | Position | PR |
|---------|----------|----|
| LICENSE file | DONE | — |
| PR template | DONE | — |
| SLA documentation | DONE | — |
| Production monitoring | DONE | — |
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

Milestone completed 2026-04-18. 30-day Beta stability window elapsed 2026-04-04; all GA readiness gaps closed. Production Prometheus scrape verified via `grafana.calebdunn.tech/d/cashdrugs-api-v2` with env switcher covering staging + prod. **Scope change:** the original "alerting" half of the production-monitoring criterion was descoped — Alertmanager routing is not yet wired for the 7 rules in `docs/grafana/alerts.yml`. That work moved to PRD Deferred Items for a future milestone. **Maturity decision:** although M13 closing makes the GA gate technically open, the project is intentionally staying at Beta. The current release cadence satisfies business needs; no `/add:promote` run is planned.

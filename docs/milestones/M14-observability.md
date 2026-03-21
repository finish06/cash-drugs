# M14 — Observability & Operational Foundation

## Goal

Production-grade observability — SLAs defined, alerts firing, requests traceable, errors classified. Sleep-at-night confidence.

## Status: DONE

## Appetite: 2 weeks

## Priority: P0 — GA promotion blocker

## Success Criteria

- [x] SLA document with measurable targets
- [x] `X-Request-ID` on every log line and response header
- [x] All error paths classified with stable codes (CD-H001, CD-U001-U003, CD-S001)
- [x] 7 alerting rules with Alertmanager template (docs/grafana/alerts.yml)
- [x] `/api/cache/status` returns per-slug health
- [ ] Alert rules tested (manually trigger each condition)

## Hill Chart

| Feature | Position | PR |
|---------|----------|----|
| SLA definition | DONE | — |
| Request correlation IDs | DONE | — |
| Error taxonomy | DONE | — |
| Prometheus alerting rules | DONE | — |
| Cache status endpoint | DONE | — |

## Dependencies

- M13 GA Readiness (5/6 items complete)
- SLA doc already created during cycle 6

## Risks

| Risk | Mitigation |
|------|-----------|
| Error taxonomy requires touching many handler files | Use middleware for consistent error envelope |
| Alerting rules need real Prometheus to validate | Test with docker-compose Prometheus instance |
| X-Request-ID propagation to upstream may need upstream fetcher changes | Scope to internal tracing first, upstream propagation as stretch |

## Retrospective

_To be filled at milestone completion._

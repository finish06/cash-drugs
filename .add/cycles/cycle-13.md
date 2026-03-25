# Cycle 13 — M18: Landing Page

**Milestone:** M18 — Landing Page
**Maturity:** Beta
**Status:** COMPLETE
**Started:** 2026-03-24
**Completed:** 2026-03-25
**Duration Budget:** 1 day (away mode)
**Actual Duration:** ~3 hours

## Work Items

| Feature | Current Pos | Target Pos | Final Pos | Assigned | Validation |
|---------|-------------|-----------|-----------|----------|------------|
| Static landing page | SHAPED | DONE | DONE | Agent-1 | `landing/index.html` 25KB, 5 sections, ocean branding, responsive, scroll animations |
| GitHub Pages config | SHAPED | DONE | DONE | Agent-1 | `landing/CNAME`, `.github/workflows/pages.yml`, site live at drug-cash.calebdunn.tech |
| LANDING_URL redirect | SHAPED | DONE | DONE | Agent-1 | Handler redirects exact `GET /` when env var set, no-op when unset |
| Tests | SHAPED | DONE | DONE | Agent-1 | 8 tests passing: redirect, route isolation, content, file size |

## Cycle Success Criteria

- [x] All 4 work items reach DONE
- [x] All existing tests pass + 8 new tests
- [x] Coverage remains >= 85% (91.5%)
- [x] go vet clean
- [x] No regressions
- [x] PR #22 merged to main
- [x] Site live at https://drug-cash.calebdunn.tech/

## Notes

- Initial design was too minimal — iterated to match drug-gate quality bar (nav, metrics strip, syntax-highlighted JSON, scroll-reveal, CTA section)
- Custom domain required setting CNAME via GitHub API after initial deploy — first deploy 404'd until domain was configured and workflow re-triggered
- Added `workflow_dispatch` trigger to pages.yml for manual re-deploys
- Branding is "drug-cash" (not "cash-drugs") per user direction
- TLS cert provisioning caused brief 404 window after custom domain setup

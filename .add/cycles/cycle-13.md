# Cycle 13 — M18: Landing Page

**Milestone:** M18 — Landing Page
**Maturity:** Beta
**Status:** PLANNED
**Started:** TBD
**Completed:** TBD
**Duration Budget:** 1 day (away mode)

## Work Items

| Feature | Current Pos | Target Pos | Assigned | Est. Effort | Validation |
|---------|-------------|-----------|----------|-------------|------------|
| Static landing page | SHAPED | DONE | Agent-1 | ~1 hour | `landing/index.html` exists, < 50KB, all 5 sections, ocean branding, responsive |
| GitHub Pages config | SHAPED | DONE | Agent-1 | ~30 min | `landing/CNAME`, `.github/workflows/pages.yml` created |
| LANDING_URL redirect | SHAPED | DONE | Agent-1 | ~1 hour | Handler redirects exact `GET /` when env var set, no-op when unset |
| Tests | SHAPED | DONE | Agent-1 | ~30 min | 8 tests covering redirect behavior, route isolation, content, file size |

## Dependencies & Serialization

All items are sequential (single agent):

```
Static landing page (Task 1)
    ↓
GitHub Pages config (Task 2)
    ↓
LANDING_URL redirect handler (Task 3)
    ↓
Tests (Task 4)
```

## Parallel Strategy

Single-threaded execution. All tasks assigned to Agent-1 sequentially.

## Validation Criteria

### Per-Item Validation
- **Static landing page:** `landing/index.html` contains hero ("drug-cash"), API overview with JSON examples, quick-start, GitHub link, tech stack. Ocean palette. Responsive. < 50KB.
- **GitHub Pages config:** `landing/CNAME` contains `drug-cash.calebdunn.tech`. Workflow deploys `landing/` on push to main.
- **LANDING_URL redirect:** `GET /` returns 302 when `LANDING_URL` is set. No redirect when unset. Other routes (`/api/*`, `/health`) unaffected.
- **Tests:** 8 unit tests passing. `go vet` clean. Existing tests unbroken.

### Cycle Success Criteria
- [ ] All 4 work items reach DONE
- [ ] All existing tests pass + 8 new tests
- [ ] Coverage remains >= 85%
- [ ] go vet clean
- [ ] No regressions
- [ ] Committed to feature branch with PR ready for review

## Agent Autonomy & Checkpoints

**Mode:** Away mode — autonomous execution.

**Autonomous (proceed without asking):**
- Commit to `feature/landing-page` branch
- Create PR against main
- Run and fix quality gates
- Push to remote

**Queue for human return:**
- DNS setup for `drug-cash.calebdunn.tech` (requires manual config)
- GitHub Pages enable on repo settings (requires manual config)
- Merge to main

## Notes

- Branding is "drug-cash" (not "cash-drugs")
- No Swagger links — service is internal, landing page is for discovery/self-hosting
- No staging link — keep it simple
- JSON examples should be representative but not from live data
- DNS and GitHub Pages repo settings are manual prerequisites — document in PR description

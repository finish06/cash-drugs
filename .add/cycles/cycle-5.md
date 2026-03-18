# Cycle 5 — CI Hardening: golangci-lint in Pipeline

**Milestone:** N/A (infrastructure improvement from retro)
**Maturity:** Beta
**Status:** PLANNED
**Started:** TBD
**Completed:** TBD
**Duration Budget:** 1 hour

## Work Items

| Feature | Current Pos | Target Pos | Est. Effort | Validation |
|---------|-------------|-----------|-------------|------------|
| Add golangci-lint to CI | N/A | DONE | ~30min | CI runs lint, fails on errors |

## Implementation

Add `golangci-lint` step to `.github/workflows/ci.yml` between Vet and Unit tests.
Use `golangci/golangci-lint-action@v6` for caching and speed.

## Validation Criteria

- [ ] CI pipeline includes golangci-lint step
- [ ] Lint step runs on PRs and pushes to main
- [ ] Current codebase passes (0 issues confirmed locally)
- [ ] PR created and CI passes

## Notes

- From retro 2026-03-18 agreed action: "Add golangci-lint to CI pipeline to prevent lint debt accumulation"
- 41 errcheck issues accumulated undetected over M10-M11; this prevents recurrence

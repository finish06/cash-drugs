# Implementation Plan: Landing Page

**Spec:** `specs/landing-page.md`
**Milestone:** M18
**Estimated effort:** 2–3 hours

## Task Breakdown

### Task 1: Create static HTML landing page
**AC:** AC-001, AC-002, AC-003, AC-004, AC-005, AC-006, AC-010, AC-011

Create `landing/index.html` — a single self-contained HTML file with inline CSS, branded as "drug-cash".

**Sections:**
1. **Hero** — "drug-cash" name, tagline, brief description of DailyMed/FDA/RxNorm caching
2. **API Overview** — 6 key endpoints with inline example JSON response snippets
3. **Quick Start** — `git clone` + `docker compose up` code block
4. **GitHub Link** — `github.com/finish06/cash-drugs`
5. **Tech Stack** — Go, net/http, MongoDB, Prometheus, Docker

**Design:**
- Ocean palette (`#0891b2` accent, dark background gradient)
- Responsive: flexbox/grid, mobile-first, breakpoints at 768px and 1024px
- Inline CSS only — no external stylesheets, no JS
- Size budget: < 50KB

### Task 2: GitHub Pages configuration
**AC:** AC-001, AC-008

**Files created:**
- `landing/CNAME` — contains `drug-cash.calebdunn.tech`
- `.github/workflows/pages.yml` — GitHub Actions workflow to deploy `landing/` to GitHub Pages on push to main

**Workflow:**
```yaml
on:
  push:
    branches: [main]
    paths: [landing/**]
jobs:
  deploy:
    # Deploy landing/ directory to GitHub Pages
```

### Task 3: LANDING_URL redirect handler
**AC:** AC-007, AC-009

**Files created:**
- `internal/handler/landing.go` — redirect handler

**Files modified:**
- `cmd/server/main.go` — register redirect handler

**Implementation:**
1. Create `internal/handler/landing.go`:
   - Read `LANDING_URL` env var at startup
   - If set and non-empty: on exact `GET /` (path == `/`), respond with `302` + `Location` header
   - If unset or empty: handler is a no-op, falls through to existing routing
   - Only redirects GET method — POST/PUT/etc. unaffected
2. In `cmd/server/main.go`:
   - Wrap the existing catch-all `mux.Handle("/", ...)` so that exact `GET /` checks for redirect before delegating to appMux

### Task 4: Write tests
**AC:** AC-007, AC-009

**Files created:**
- `internal/handler/landing_test.go`

**Tests:**
| Test | Covers |
|------|--------|
| `TestAC007_RedirectWhenLandingURLSet` | `GET /` with `LANDING_URL` set returns 302 to the URL |
| `TestAC007_NoRedirectWhenLandingURLUnset` | `GET /` without `LANDING_URL` falls through normally |
| `TestAC007_EmptyLandingURLNoRedirect` | `GET /` with `LANDING_URL=""` falls through normally |
| `TestAC009_APIRoutesUnaffectedWithRedirect` | `GET /api/endpoints` with `LANDING_URL` set returns JSON, not redirect |
| `TestAC009_HealthUnaffectedWithRedirect` | `GET /health` with `LANDING_URL` set returns JSON |
| `TestAC007_PostMethodNoRedirect` | `POST /` with `LANDING_URL` set does not redirect |
| `TestAC010_LandingPageFileSize` | `landing/index.html` is < 50KB |
| `TestAC002_LandingPageContent` | `landing/index.html` contains key strings ("drug-cash", "DailyMed", "FDA", "RxNorm") |

## File Changes Summary

| File | Action |
|------|--------|
| `landing/index.html` | Create |
| `landing/CNAME` | Create |
| `.github/workflows/pages.yml` | Create |
| `internal/handler/landing.go` | Create |
| `internal/handler/landing_test.go` | Create |
| `cmd/server/main.go` | Modify (register redirect handler) |

## Test Strategy

- **Unit tests:** Redirect handler behavior with/without env var, route isolation
- **Static checks:** File size, content verification of landing page HTML
- **Manual verification:** Visual check at `drug-cash.calebdunn.tech` after deploy (TC-001, TC-002)
- **Integration:** Existing E2E tests continue to pass (no redirect without env var)

## Dependencies

- DNS: `drug-cash.calebdunn.tech` CNAME → GitHub Pages (manual setup)
- GitHub Pages enabled on `finish06/cash-drugs` repo (manual setup)

## Spec Traceability

| Task | Acceptance Criteria |
|------|-------------------|
| Task 1 | AC-001, AC-002, AC-003, AC-004, AC-005, AC-006, AC-010, AC-011 |
| Task 2 | AC-001, AC-008 |
| Task 3 | AC-007, AC-009 |
| Task 4 | AC-002, AC-007, AC-009, AC-010 |

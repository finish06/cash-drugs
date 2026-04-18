# Landing Page

**Version:** 1.0.0
**Created:** 2026-03-25
**PRD Reference:** docs/prd.md (M18)
**Status:** Complete

## Feature Description

A public landing page for drug-cash hosted on GitHub Pages at `drug-cash.calebdunn.tech`. Explains what the service does, shows key API endpoints with example JSON responses, and links to the GitHub repository for self-hosting. The Go service optionally redirects `GET /` to the landing page via a `LANDING_URL` env var.

## User Story

As a **developer** discovering drug-cash for the first time, I want a public landing page that explains what the service does and shows real API examples, so that I can understand the value and self-host it without reading source code.

## Acceptance Criteria

| ID | Criterion |
|----|-----------|
| AC-001 | Landing page is accessible at `drug-cash.calebdunn.tech` via GitHub Pages |
| AC-002 | Page has a hero section explaining what drug-cash does (API cache/proxy for drug data — DailyMed, FDA, RxNorm) |
| AC-003 | Page shows key API endpoints with example JSON responses inline |
| AC-004 | Page links to GitHub repo (`github.com/finish06/cash-drugs`) for source code and self-hosting |
| AC-005 | Page includes quick-start instructions (`docker compose up`) |
| AC-006 | Page is responsive (mobile + desktop) |
| AC-007 | `LANDING_URL` env var controls 302 redirect from `GET /` — when set, redirects; when unset, no redirect (default for self-hosters) |
| AC-008 | GitHub Pages auto-deploys from `landing/` directory on push to main |
| AC-009 | Landing page does not conflict with existing API routes (`/api/*`, `/swagger/*`, `/health`, `/ready`, `/metrics`, `/version`) |
| AC-010 | Page is a single static `index.html` with inline CSS — no external dependencies, no JS framework |
| AC-011 | Page follows project branding (ocean palette: `#0891b2`) |

## User Test Cases

| ID | Scenario | Steps | Expected Result |
|----|----------|-------|-----------------|
| TC-001 | First visit | Navigate to `drug-cash.calebdunn.tech` | See landing page with hero, API overview with JSON examples, quick-start, GitHub link, tech stack |
| TC-002 | Mobile view | Open on a 375px-wide viewport | All sections stack vertically, text is readable, no horizontal scroll |
| TC-003 | Redirect when set | Set `LANDING_URL=https://drug-cash.calebdunn.tech`, `GET /` | 302 redirect to `https://drug-cash.calebdunn.tech` |
| TC-004 | No redirect when unset | Unset `LANDING_URL`, `GET /` | No redirect, falls through to normal app routing |
| TC-005 | API routes unaffected | `GET /api/endpoints` with `LANDING_URL` set | Returns JSON endpoint list, not redirect |
| TC-006 | Health unaffected | `GET /health` with `LANDING_URL` set | Returns JSON health response |
| TC-007 | GitHub link | Click GitHub link on landing page | Opens `github.com/finish06/cash-drugs` |

## Data Model

No new data models. The landing page is a static HTML file served by GitHub Pages.

## API Contract

### `GET /` (when `LANDING_URL` is set)

**Response:** `302 Found`
- `Location: {LANDING_URL}`

### `GET /` (when `LANDING_URL` is unset)

No change — falls through to existing app routing (returns 404 or catch-all behavior).

**Routing:** The redirect handler only activates on exact `GET /` (path == `/`). All other paths (`/api/*`, `/health`, etc.) are unaffected.

## Page Sections

### 1. Hero
- Project name: **drug-cash**
- Tagline: "Drug data API cache — fast, reliable, always available"
- Brief description: Fetches from DailyMed, FDA openFDA, and RxNorm APIs. Caches in MongoDB. Serves to internal consumers in < 50ms.

### 2. API Overview (with inline JSON examples)
| Endpoint | Description |
|----------|-------------|
| `GET /api/cache/{slug}` | Retrieve cached data by endpoint slug |
| `GET /api/search?q=` | Cross-slug full-text search |
| `GET /api/autocomplete?q=` | Fast prefix-match autocomplete |
| `GET /api/endpoints` | Discover all configured endpoints |
| `GET /api/cache/{slug}/_meta` | Per-slug metadata and freshness |
| `POST /api/cache/{slug}/bulk` | Bulk cache lookups |

Each endpoint includes a representative JSON response snippet inline.

### 3. Quick Start
```
git clone https://github.com/finish06/cash-drugs.git
cd cash-drugs
docker compose up
# API available at http://localhost:8080
```

### 4. Links
- GitHub: `github.com/finish06/cash-drugs`

### 5. Tech Stack
Go · net/http · MongoDB · Prometheus · Docker

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| `LANDING_URL` set, `GET /api/endpoints` | No redirect — only exact `GET /` redirects |
| `LANDING_URL` set, `POST /` | No redirect — only GET method triggers redirect |
| `LANDING_URL` unset (default) | No redirect at all — self-hosters see normal behavior |
| `LANDING_URL` set to empty string | Treated as unset — no redirect |

## Technical Considerations

- **Hosting:** Static `index.html` in `landing/` directory, deployed via GitHub Pages
- **Custom domain:** `drug-cash.calebdunn.tech` — requires `CNAME` file in `landing/` and DNS configuration
- **GitHub Actions:** Workflow to deploy `landing/` to GitHub Pages on push to main
- **Redirect handler:** Simple middleware in Go that checks `LANDING_URL` env var, redirects exact `GET /` with 302
- **No framework:** Single `index.html` with inline CSS. No React, no templating engine
- **Design:** Ocean palette (`#0891b2` accent, dark background gradient), responsive flexbox/grid layout, "drug-cash" branding
- **Size budget:** < 50KB total (HTML + inline CSS)

## Dependencies

- DNS: `drug-cash.calebdunn.tech` CNAME pointing to GitHub Pages
- GitHub Pages enabled on the repository

## Milestone

M18: Landing Page

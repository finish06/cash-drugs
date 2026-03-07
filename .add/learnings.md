# Project Learnings — drugs

> **Tier 3: Project-Specific Knowledge**
> Generated from `.add/learnings.json` — do not edit directly.
> Agents read JSON for filtering; this file is for human review.

## Architecture
- **[high] Cache warm-up should check staleness before re-fetching** (L-002, 2026-03-07)
  Scheduler was re-fetching all paginated endpoints on every restart, wasting bandwidth. Added FetchedAt() to Repository interface for lightweight TTL check. Warm-up now skips endpoints with fresh data in MongoDB.

- **[medium] Config-only endpoint expansion works well for DailyMed** (L-003, 2026-03-07)
  New search endpoints (spls-by-name, spls-by-class, drugclasses) added purely through config.yaml with {PARAM} substitution. No code changes needed. This validates the core architecture of config-driven API caching.

## Technical
- **[medium] DailyMed API param names use underscores, not camelCase** (L-001, 2026-03-07)
  DailyMed /v2/spls.json uses drug_name (underscore) not drugname. Discovered via live curl testing — API docs were ambiguous. Always test actual API params before committing config.

- **[high] Integration tests require Docker MongoDB for 80%+ coverage** (L-004, 2026-03-07)
  Unit tests alone reach 70.6% due to cache/mongo.go at 25.7%. With Docker MongoDB (port 27018, tmpfs), coverage reaches 81.0%. CI workflow includes MongoDB service container. Always run make test-integration for accurate coverage.

- **[medium] log/slog logging philosophy: INFO=what runs, DEBUG=each step** (L-005, 2026-03-07)
  INFO logs should indicate what code path is executing (fetch started, server starting). DEBUG logs should show each step within that path (request method/path/status/duration, cache hit/miss). This keeps INFO useful in production and DEBUG useful for troubleshooting.

---
*5 entries. Last updated: 2026-03-07. Source: .add/learnings.json*

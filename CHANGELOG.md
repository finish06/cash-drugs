# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Conventional Commits](https://www.conventionalcommits.org/).

## [Unreleased]

## [0.3.0] — 2026-03-07 — M3: Documentation + Onboarding

### Added
- OpenAPI spec generated from swaggo/swag annotations, served at `GET /openapi.json`
- Swagger UI at `GET /swagger/` for interactive API exploration
- Endpoint discovery API: `GET /api/endpoints` lists all configured slugs with params, pagination, and schedule info
- Comprehensive README with usage examples, Go client snippet, and configuration reference
- Onboarding guide for adding new upstream APIs (config.yaml examples for every pattern)
- Dockerfile updated to generate OpenAPI spec during build

## [0.2.0] — 2026-03-07 — M2: Scheduling + Staleness

### Added
- Background scheduler with cron-based periodic refresh of configured endpoints
- Cache TTL with stale-while-revalidate pattern — stale data served immediately while background revalidation runs
- Per-endpoint TTL configuration via Go duration strings (e.g., `ttl: "6h"`)
- `format: raw` support — stores upstream response body as-is with original content type
- `{PARAM}` placeholder substitution in `query_params` values (e.g., `setid: "{SETID}"`)
- Per-page MongoDB storage for multi-page responses — avoids 16MB document limit
- Transparent page reassembly on read — consumers see a single combined response
- Non-blocking cache warming on startup via background goroutines
- Shared fetch lock deduplication between scheduler and handler (prevents concurrent fetches for the same endpoint)
- `ExtractAllParams` — extracts parameter names from both path and query_params
- New endpoints: `spls` (paginated SPL listing) and `spl-xml` (raw XML SPL documents)
- Tests for AC-016 through AC-021

### Changed
- `spl-detail` endpoint now uses query param approach (`/v2/spls.json?setid=X`) instead of path-based (`/v2/spls/{SETID}`) to avoid DailyMed 415 errors
- Scheduler uses `fetchlock.Map` from dedicated package (was internal mutex map)
- MongoDB `Get` uses regex matching to find and reassemble per-page documents

### Fixed
- DailyMed `/v2/spls/{SETID}` returning 415 (endpoint only supports XML, not JSON)
- MongoDB 16MB document size limit exceeded when storing large paginated responses (100k+ items)
- Cache warming blocking server startup — now runs in background

## [0.1.0] — 2026-03-05 — M1: Config + Fetch + Store

### Added
- YAML-driven endpoint configuration with validation
- On-demand upstream API fetching with auto-pagination
- MongoDB cache storage with upsert and metadata (fetched_at, source_url, page_count)
- Consumer REST API: `GET /api/cache/{slug}` with path parameter support
- Stale cache fallback when upstream is unavailable
- Health check endpoint: `GET /health`
- Docker Compose local development environment
- DailyMed seed endpoints: `drugnames`, `spl-detail`

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Conventional Commits](https://www.conventionalcommits.org/).

## [Unreleased]

## [0.5.0] — 2026-03-07 — FDA API Integration

### Added
- FDA openFDA drug API support via 6 new config.yaml endpoints
- Offset pagination (`pagination_style: offset`) — sends `skip=N&limit=N` for FDA-style APIs
- Configurable JSON response parsing: `data_key` (default: `data`) and `total_key` (default: `metadata.total_pages`)
- Dot-notation path traversal for nested total keys (e.g., `meta.results.total`)
- Graceful handling of FDA skip/limit cap — returns partial data instead of failing
- FDA Enforcement recalls endpoint (daily prefetch)
- FDA Drug Shortages endpoint (daily prefetch)
- FDA NDC lookup by brand name (on-demand)
- FDA Drugs@FDA lookup by brand name (on-demand)
- FDA Drug Labels search (on-demand)
- FDA Adverse Events search by drug name (on-demand)
- E2E test suite validating all 13 config.yaml endpoints against live APIs
- Swagger/OpenAPI docs updated with FDA query parameters

### Changed
- Fetcher now branches on `pagination_style` for URL building and page detection
- `hasMorePages` uses `resolveByDotPath` for configurable total key extraction

## [0.4.0] — 2026-03-07 — Structured Logging

### Added
- Structured logging via `log/slog` (Go stdlib) replacing all `log.*` calls
- Configurable log level: `LOG_LEVEL` env var or `log_level` in config.yaml (default: `warn`)
- Configurable output format: `LOG_FORMAT=json` (default) or `LOG_FORMAT=text`
- `component` field on all log entries (`server`, `handler`, `scheduler`, `cache`)
- INFO-level: server start/stop, endpoint registered, fetch started/completed, cache warm
- DEBUG-level: skip reasons, background revalidation, request details
- ERROR-level: fetch failures, MongoDB errors, upsert failures
- WARN-level: unschedulable endpoints
- `config.LoadConfig()` for reading top-level app settings beyond endpoints

### Changed
- All packages now use `log/slog` — zero `log.Printf`/`log.Fatalf` in production code
- `cmd/server/main.go` initializes logger before any other work; exits with clear error on invalid log level

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

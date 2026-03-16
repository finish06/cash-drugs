# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Conventional Commits](https://www.conventionalcommits.org/).

## [Unreleased]

### Added
- 6 RxNorm API endpoints (NLM): `rxnorm-find-drug`, `rxnorm-approximate-match`, `rxnorm-spelling-suggestions`, `rxnorm-ndcs`, `rxnorm-generic-product`, `rxnorm-all-related` — config-only, no code changes
- Fuzzy drug name search, spelling suggestions, brand→generic mapping, RxCUI→NDC mapping via RxNorm
- **Readiness endpoint:** `GET /ready` returns 503 with progress (`{"status": "warming", "progress": "5/17"}`) during cache warmup, 200 when ready — suitable for Kubernetes readiness probes and load balancer health checks
- **Cache warmup endpoint:** `POST /api/warmup` triggers background pre-fetch of cached endpoints. Accepts optional `{"slugs": [...]}` to warm specific slugs, or warms all scheduled endpoints when called with no body. Returns 202 immediately.
- **Response normalization:** `flatten` config flag per endpoint — when enabled, flattens nested upstream response arrays into a single top-level array for consistent client consumption
- Swagger docs: documented `drugclasses` response shape, added `search` query parameter guidance for drug lookup endpoints

### Fixed
- Dot-path `data_key` resolution in `fetchJSONPage` — nested keys like `rxnormdata.idGroup.rxnormId` now resolve correctly through intermediate map layers

## [0.8.0] — 2026-03-15 — M10: Performance Optimization

### Added
- **MongoDB query optimization:** `base_key` exact-match field replaces regex-based `cache_key` queries for all Get, FetchedAt, and stale-page-deletion operations. Compound index on `base_key + page` created at startup. Startup migration (`backfillBaseKey`) populates `base_key` on existing documents.
- **LRU cache sharding:** 16-shard FNV-1a hashed LRU cache with per-shard mutexes, replacing single-mutex design. Reduces lock contention under concurrent load. Shard count auto-adjusts for small budgets (minimum 32KB per shard).
- **Parallel page fetches:** Remaining pages (2..N) fetched concurrently after sequential first page, capped at 3 concurrent goroutines via semaphore (`FetchConcurrency` field on `HTTPFetcher`). Applies to both page-style and offset-style pagination.
- **Empty upstream results:** Upstream APIs returning 200 with empty data now return 200 with `{"data": [], "meta": {results_count: 0}}` instead of 502 error. Correctly distinguishes "no results" from "upstream failure".
- **Version endpoint:** `GET /version` returns build metadata (version, git_commit, git_branch, build_date, go_version, os, arch, hostname, GOMAXPROCS, uptime_seconds, endpoint_count, start_time). Exempt from concurrency limiter. Prometheus `cashdrugs_build_info` gauge and `cashdrugs_uptime_seconds` gauge added.
- New Prometheus metrics: `cashdrugs_build_info` (labels: version, git_commit, go_version, build_date), `cashdrugs_uptime_seconds`
- `results_count` field added to API response `meta` envelope

### Changed
- MongoDB `Get()` and `FetchedAt()` use `base_key` exact match instead of `cache_key` regex
- `Upsert()` writes `base_key` field on both single-document and multi-page responses
- LRU cache constructor (`NewLRUCache`) now returns a 16-shard implementation by default
- Paginated fetcher fetches page 1 sequentially, then remaining pages concurrently (was fully sequential)

## [0.7.1] — 2026-03-14 — Performance Quick Wins

### Changed
- LRU cache `estimateSize()` uses PageCount heuristic instead of `json.Marshal` — saves 50-200ms per Set on large responses
- Handler reuses first `repo.Get()` result in upstream-failure fallback — eliminates duplicate MongoDB query on cache miss
- Gzip middleware uses `sync.Pool` for `gzip.Writer` instances — reduces GC pressure at 150 concurrent requests

## [0.7.0] — 2026-03-14 — M9: Performance & Resilience

### Added
- **Connection resilience:** concurrency limiter middleware caps in-flight requests (default: 50, configurable via `max_concurrent_requests` / `MAX_CONCURRENT_REQUESTS`). Returns 503 + `Retry-After` instead of connection refused. `/health` and `/metrics` exempt from limits.
- **Response optimization:** gzip compression middleware for JSON/XML responses (1KB threshold, `Accept-Encoding: gzip`). Singleflight request coalescing deduplicates concurrent identical requests. In-memory LRU cache (256MB default, configurable via `lru_cache_size_mb` / `LRU_CACHE_SIZE_MB`) sits between handler and MongoDB.
- **Upstream resilience:** per-endpoint circuit breakers via gobreaker (5 consecutive failures → open 30s → half-open probe). Force-refresh 30s per-key cooldown prevents upstream abuse via `_force=true`. Scheduler respects circuit state.
- **Container system metrics:** CPU, memory, disk, and network metrics from procfs/cgroup exported via `/metrics` endpoint. `SystemCollector` follows `MongoCollector` pattern (background goroutine, configurable interval via `system_metrics_interval` / `SYSTEM_METRICS_INTERVAL`).
- HTTP server timeouts: ReadTimeout=10s, WriteTimeout=30s, IdleTimeout=60s
- New Prometheus metrics: `cashdrugs_inflight_requests`, `cashdrugs_rejected_requests_total`, `cashdrugs_lru_cache_hits_total`, `cashdrugs_lru_cache_misses_total`, `cashdrugs_lru_cache_size_bytes`, `cashdrugs_singleflight_dedup_total`, `cashdrugs_circuit_state`, `cashdrugs_circuit_rejections_total`, `cashdrugs_force_refresh_cooldown_total`, `cashdrugs_container_cpu_*`, `cashdrugs_container_memory_*`, `cashdrugs_container_disk_*`, `cashdrugs_container_network_*`
- New internal packages: `internal/middleware/` (limiter + gzip), `internal/cache/lru.go`, `internal/upstream/circuit.go`, `internal/upstream/cooldown.go`, `internal/metrics/system*.go`
- Sequence diagrams for all new flows (concurrency limiter, circuit breaker, cooldown, container metrics, LRU/singleflight)

### Changed
- `MAX_CONCURRENT_REQUESTS=150` in production docker-compose
- Handler request flow: gzip → limiter → LRU check → singleflight → MongoDB → circuit breaker → upstream fetch

## [0.6.1] — 2026-03-14 — M8 Polish

### Changed
- MongoCollector: `sync.Once` + done channel for safe shutdown
- MongoCollector: reset gauge before setting to clear stale slug label values
- Scheduler: removed duplicate `UpstreamFetchDuration` recording
- Handler: added metrics to `backgroundRevalidate` goroutine
- Grafana dashboard: added `$slug` template variable
- `go mod tidy`: fixed direct/indirect dependency annotations

### Fixed
- Metrics collector coverage increased to 85.7%

## [0.6.0] — 2026-03-14 — M8: Prometheus Metrics

### Added
- `/metrics` endpoint serving Prometheus exposition format via `promhttp.Handler()`
- HTTP request counter `cashdrugs_http_requests_total` with labels `slug`, `method`, `status_code`
- HTTP request duration histogram `cashdrugs_http_request_duration_seconds` with labels `slug`, `method`
- Cache outcome counter `cashdrugs_cache_hits_total` with labels `slug`, `outcome` (hit, miss, stale)
- Upstream fetch duration histogram, error counter, and page counter per slug
- MongoDB ping latency gauge, health gauge, and document count gauge per slug
- Scheduler job execution counter and duration histogram per slug
- Fetch lock deduplication counter per slug
- Go runtime metrics (goroutines, memory, GC) via default Prometheus collectors
- Background `MongoCollector` collecting MongoDB stats every 30s
- Example Grafana dashboard JSON with variable datasource (`${DS_PROMETHEUS}`)
- Prometheus setup guide (`docs/prometheus-setup.md`)
- `internal/metrics/` package with `Metrics` struct and `MongoCollector`

### Changed
- All metric names use `cashdrugs_` namespace prefix
- Version embedded via `-ldflags` at build time, reported in `/health`

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

# cash-drugs — Product Requirements Document

**Version:** 0.7.0
**Created:** 2026-03-05
**Author:** calebdunn
**Status:** Active

## 1. Problem Statement

Internal microservices frequently need data from external REST APIs. Each service making its own API calls leads to redundant requests, higher latency, rate limit issues, and service failures when upstream APIs go down. The cash-drugs microservice acts as an API cache/proxy — it fetches data from configurable REST API endpoints (on-demand and on schedule), stores responses in MongoDB, and serves cached data to internal consumers. When upstream APIs are unavailable, it continues serving stale cached data to maintain reliability.

## 2. Target Users

- **Internal microservices:** Backend services that need data from external REST APIs. They query cash-drugs instead of calling upstream APIs directly, getting lower latency, cached responses, and resilience against upstream failures.

## 3. Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Reduced API calls | 80%+ reduction in upstream API calls | Compare upstream call count before/after adoption |
| Lower latency | < 50ms for cached responses | P95 response time from MongoDB cache vs upstream |
| Reliability | Serve stale when upstream is down | Successful responses during upstream outage windows |

## 4. Scope

### In Scope

- Configurable API endpoints (URL + query params)
- On-demand fetch (internal service requests data, cash-drugs fetches if not cached)
- Scheduled fetch (cron-style periodic refresh of configured endpoints)
- MongoDB storage of API responses
- REST API for internal consumers to retrieve cached data
- Serve stale data when upstream is unavailable
- Docker Compose local development environment
- Health check endpoint with build version
- FDA openFDA drug API endpoints (enforcement, shortages, NDC, approvals, labels, adverse events)
- Offset pagination (skip/limit) for APIs that don't use page-based pagination
- Configurable JSON response parsing (data_key, total_key with dot-notation)
- Docker image publishing to private registry (dockerhub.calebdunn.tech)
- Versioned image tags via CI/CD (`:beta`, `:vX.Y.Z`, `:latest`)

### Out of Scope (v1)

- Upstream API authentication (OAuth, API keys, etc.)
- Response transformation / field mapping
- Rate limiting
- Web UI or dashboard
- Cache invalidation API
- Response diffing or change detection
- Webhook notifications on data changes
- Multi-tenancy / API keys (until 3+ consumer teams)
- Response diffing / change detection
- Pre-materialized responses
- Consistent hash routing (until 4+ instances)
- Dynamic config API (MongoDB-backed)
- Built-in HTML dashboard (Grafana covers this)

## 5. Architecture

### Tech Stack

| Layer | Technology | Version | Notes |
|-------|-----------|---------|-------|
| Language | Go | 1.22+ | Standard library net/http |
| Database | MongoDB | latest | Response cache storage |
| Containers | Docker Compose | — | Local dev and deployment |
| Registry | dockerhub.calebdunn.tech | registry:2 | Private self-hosted |
| CI/CD | GitHub Actions | — | Test + publish pipeline |

### Infrastructure

| Component | Choice | Notes |
|-----------|--------|-------|
| Git Host | GitHub | github.com/finish06/cash-drugs |
| Cloud Provider | Self-hosted | Homelab deployment |
| CI/CD | GitHub Actions | `.github/workflows/ci.yml` |
| Container Registry | dockerhub.calebdunn.tech | Self-hosted registry:2 |
| Containers | Docker Compose | Local dev + prod deployment |
| IaC | None | — |

### Environment Strategy

| Environment | Purpose | URL | Deploy Trigger |
|-------------|---------|-----|----------------|
| Local | Development & unit tests | http://localhost:8080 | Manual |
| Production | Live service | Homelab | Push git tag `v*` → `:latest` |

**CI/CD Pipeline:**
- Push to `main` → run tests → publish `:beta` image
- Push git tag `v*` → run tests → publish `:vX.Y.Z` + `:latest`

## 6. Milestones & Roadmap

### Current Maturity: Beta

### Roadmap

| Milestone | Goal | Target Maturity | Status | Success Criteria |
|-----------|------|-----------------|--------|------------------|
| M1: Config + Fetch + Store | Fetch from configured APIs and store in MongoDB | alpha | DONE | Configure endpoints, fetch responses, store in MongoDB, serve to consumers |
| M2: Scheduling + Staleness | Scheduled refresh and stale-while-revalidate | beta | DONE | Cron-based refresh, serve stale on upstream failure, TTL policies |
| M3: Documentation + Onboarding | Make the service easy to discover, understand, and consume | beta | DONE | OpenAPI spec, interactive docs, usage examples, onboarding guide |
| M4: Structured Logging | Structured logging via log/slog | beta | DONE | Configurable log level/format, component fields, structured output |
| M5: FDA API Integration | FDA openFDA drug endpoints via config-driven enhancements | beta | DONE | Offset pagination, configurable response parsing, 6 FDA endpoints |
| M6: Docker Build & Publish | Automated Docker image publishing to private registry | beta | DONE | CI publishes :beta on push, versioned tags on git tag, version in /health |
| M7: Auth + Transforms | Upstream auth and response transforms | ga | LATER | API key/OAuth support, field mapping, response filtering |
| M8: Prometheus Metrics | Prometheus endpoint with full operational observability | beta | DONE | `/metrics` endpoint, cache hit/miss, upstream latency, MongoDB health/size, scheduler stats |
| M9: Performance & Resilience | Prevent service collapse under load, optimize response delivery, protect against upstream instability | beta | DONE | Concurrency limiter (503+Retry-After), gzip compression, singleflight, LRU cache, circuit breakers, force-refresh cooldown, container system metrics |
| M10: Performance Optimization | MongoDB query restructure, LRU sharding, parallel page fetches, empty upstream handling, version endpoint | beta | DONE | Indexed exact-match queries, sharded LRU mutex, concurrent upstream page fetches, empty result 200s, /version endpoint with build info |
| M11: RxNorm + Warmup | RxNorm API integration, parameterized warmup, multi-instance support | beta | DONE | 6 RxNorm endpoints, warmup-queries.yaml (top 100 drugs), ENABLE_SCHEDULER leader/replica, nginx LB |
| M12: Client Enhancements | Readiness endpoint, response normalization, API doc fixes, upstream 404 handling | beta | DONE | /ready, /api/warmup, flatten config, upstream-404-handling |
| M13: GA Readiness | LICENSE, PR template, SLA doc, CI hardening, coverage, docs audit | ga | IN_PROGRESS | 30-day stability (eligible 2026-04-04), SLA targets, 85%+ coverage |
| M14: Observability & Operational Foundation | SLAs, alerting rules, request tracing, error taxonomy, cache status API | ga | NEXT | SLA doc, X-Request-ID tracing, error codes, 7+ alert rules, /api/cache/status |
| M15: Consumer Value & API Ergonomics | Bulk queries, rich discovery, Go SDK, per-slug metadata | ga | NEXT | Bulk endpoint, parameter docs, Go client pkg, /_meta endpoint |
| M16: Operational Resilience & Runtime Management | Runbooks, chaos tests, hot config reload, test-fetch dry-run | ga | LATER | Runbook per alert, 4+ chaos tests, fsnotify reload, test-fetch endpoint |
| M17: Intelligent Data Layer | Cross-slug search, autocomplete, field filtering, pprof, TTL indexes | ga | LATER | Cross-slug search <100ms, autocomplete <20ms, field filtering, pprof, TTL expiry |

### Milestone Detail

#### M1: Config + Fetch + Store [DONE]
**Goal:** End-to-end flow — configure an API endpoint, fetch its data, store in MongoDB, serve to internal consumers

**Appetite:** Small — focused core functionality

**Target maturity:** alpha

**Features:**
- API configuration — define upstream endpoints with URL and query params
- On-demand fetch — consumer requests trigger upstream fetch if not cached
- MongoDB storage — store raw API responses with metadata
- Consumer API — REST endpoints for internal services to retrieve cached data
- Health check — basic liveness/readiness endpoints

**Success criteria:**
- [x] Configure at least one upstream API endpoint
- [x] Fetch and store response in MongoDB
- [x] Serve cached response to consumer
- [x] Serve stale data when upstream is unavailable
- [x] Health check endpoint returns status

#### M3: Documentation + Onboarding [DONE]
**Goal:** Make the service easy for internal teams to discover, understand, and integrate with — no Slack messages or tribal knowledge needed

**Appetite:** Small — documentation and API discoverability

**Target maturity:** beta

**Features:**
- OpenAPI specification — auto-generated from swaggo annotations, served at `/openapi.json`
- Interactive API explorer — Swagger UI at `/swagger/`
- Usage examples — curl commands, Go client snippets, common workflows
- Endpoint discovery — `GET /api/endpoints` listing all configured slugs with metadata
- Onboarding guide — README section explaining how to add a new upstream API

**Success criteria:**
- [x] OpenAPI spec available at `/openapi.json`
- [x] Interactive docs at `/swagger/`
- [x] New team members can add an endpoint without reading source code
- [x] All configured endpoints discoverable via API

#### M5: FDA API Integration [DONE]
**Goal:** Add FDA openFDA drug API endpoints via generic fetcher enhancements

**Appetite:** Small — config-driven, no new services

**Target maturity:** beta

**Features:**
- Offset pagination (skip/limit) support via `pagination_style: offset`
- Configurable JSON response parsing via `data_key` and `total_key`
- Dot-notation path traversal for nested keys (e.g., `meta.results.total`)
- 6 FDA endpoints: enforcement, shortages, NDC, Drugs@FDA, labels, adverse events

**Success criteria:**
- [x] Offset pagination sends skip/limit params
- [x] Configurable data_key and total_key with dot-path traversal
- [x] Existing DailyMed endpoints unchanged (backward compatible)
- [x] All 13 config.yaml endpoints pass E2E tests against live APIs

#### M6: Docker Build & Publish [DONE]
**Goal:** Automated Docker image publishing to private registry with versioned tags

**Appetite:** Small — CI/CD enhancement

**Target maturity:** beta

**Features:**
- Version embedding via `-ldflags` at build time
- `/health` returns build version
- CI publishes `:beta` on push to main
- CI publishes `:vX.Y.Z` + `:latest` on git tag
- Registry: `dockerhub.calebdunn.tech/finish06/cash-drugs`

**Success criteria:**
- [x] CI builds and pushes images on push to main and git tags
- [x] `/health` returns embedded version
- [x] Production compose pulls from registry

#### M8: Prometheus Metrics [DONE]
**Goal:** Expose a `/metrics` Prometheus endpoint providing full operational observability — MongoDB health/size, cache performance, upstream API behavior, request throughput, and scheduler stats

**Appetite:** Medium — new metrics package, instrumentation across all layers

**Target maturity:** beta

**Features:**
- Prometheus `/metrics` endpoint via `promhttp`
- HTTP request counters and latency histograms per slug/status code
- Cache hit/miss/stale counters per slug
- Upstream fetch duration, error rate, and page counts per slug
- MongoDB connection health, ping latency, document count, data size per slug
- Scheduler job execution counters and duration per slug
- Fetch lock deduplication counters
- Go runtime metrics (goroutines, memory, GC) included by default

**Success criteria:**
- [x] `/metrics` returns Prometheus exposition format
- [x] Cache hit ratio visible per slug
- [x] Upstream error rate trackable per slug
- [x] MongoDB health and document counts exported
- [x] Scheduler run history and duration available
- [x] No regression in existing tests or functionality
- [x] Example Grafana dashboard available with JSON files with variable level datasource configuration

#### M9: Performance & Resilience [DONE]
**Goal:** Prevent service collapse under concurrent load, optimize response delivery, protect against upstream API instability, and export container-level system metrics

**Appetite:** Medium-Large — 4 features across middleware, cache, upstream, and metrics layers

**Target maturity:** beta

**Features:**
- Connection resilience — concurrency limiter middleware (503+Retry-After), HTTP server timeouts, health/metrics exemption
- Response optimization — gzip compression, singleflight request coalescing, in-memory LRU cache (256MB)
- Upstream resilience — per-endpoint circuit breakers (gobreaker), force-refresh 30s cooldown
- Container system metrics — CPU/memory/disk/network from procfs/cgroup via SystemCollector

**Success criteria:**
- [x] Service handles 150 concurrent connections without connection refused
- [x] Overloaded requests receive 503 + Retry-After
- [x] `/health` and `/metrics` respond under any load level
- [x] Bulk responses compressed 3-5x via gzip
- [x] Concurrent identical requests deduplicated via singleflight
- [x] Hot responses served from LRU cache (sub-5ms)
- [x] Failing upstreams trigger circuit breaker
- [x] Force-refresh rate-limited (30s cooldown)
- [x] Container CPU/memory/disk/network metrics exported to Prometheus
- [x] All existing tests pass, 62 new ACs verified

#### M10: Performance Optimization [DONE]
**Goal:** Optimize MongoDB query patterns, reduce LRU lock contention, parallelize upstream page fetches, handle empty upstream results gracefully, and add a version/build info endpoint

**Appetite:** Medium — performance improvements across cache, upstream, and handler layers

**Target maturity:** beta

**Features:**
- MongoDB query optimization — `base_key` exact-match replaces regex, compound index on `base_key + page`, startup migration for existing documents
- LRU cache sharding — 16-shard FNV-1a hashed cache with per-shard mutexes
- Parallel page fetches — concurrent upstream pages (cap 3 goroutines) after sequential first page
- Empty upstream results — 200 with empty data + `results_count: 0` instead of 502
- Version endpoint — `GET /version` with build info, Prometheus `build_info` and `uptime_seconds` gauges

**Success criteria:**
- [x] MongoDB queries use `base_key` exact match with compound index
- [x] Startup migration backfills `base_key` on existing documents
- [x] LRU cache uses 16 shards with independent per-shard locks
- [x] Pages 2..N fetched concurrently (semaphore cap 3)
- [x] Empty upstream results return 200 with `results_count: 0`
- [x] `GET /version` returns build metadata and uptime
- [x] `cashdrugs_build_info` and `cashdrugs_uptime_seconds` Prometheus gauges exported
- [x] All existing tests pass

#### M14: Observability & Operational Foundation [NEXT]
**Goal:** Production-grade observability — SLAs defined, alerts firing, requests traceable, errors classified. Sleep-at-night confidence.

**Appetite:** 2 weeks

**Target maturity:** ga

**Priority:** P0 — GA promotion blocker

**Features:**
- SLA definition (`docs/sla.md`) — P95 cache latency < 50ms, upstream success > 95%, availability > 99.5%, stale-serve guarantee
- Request correlation IDs — generate `X-Request-ID` on ingress, thread through slog, return in response headers, propagate to upstream
- Error taxonomy — stable error codes (`CD-U001`, `CD-M001`, etc.) in logs, metrics (`cashdrugs_errors_total{code,category,slug}`), and API responses. Consistent JSON error envelope with `error_code`, `message`, `request_id`, `retry_after`
- Prometheus alerting rules — 7 rules: cache latency, upstream errors, circuit breaker open, MongoDB down, scheduler stalled, high memory, concurrency exhaustion. Ship as `docs/grafana/alerts.yml` with Alertmanager templates
- Cache status endpoint — `GET /api/cache/status` returns per-slug freshness summary (doc count, last refresh, staleness, TTL remaining, warmup coverage, health score 0-100)

**Success criteria:**
- [ ] SLA document with measurable targets
- [ ] `X-Request-ID` on every log line and response header
- [ ] All error paths classified with stable codes
- [ ] 7+ alerting rules with Alertmanager template
- [ ] `/api/cache/status` returns per-slug health
- [ ] Alert rules tested (manually trigger each condition)

#### M15: Consumer Value & API Ergonomics [NEXT]
**Goal:** Eliminate consumer friction — bulk queries, richer discovery, Go SDK. Make adoption effortless.

**Appetite:** 2 weeks

**Target maturity:** ga

**Priority:** P1 — growth accelerator

**Features:**
- Bulk query API — `POST /api/cache/{slug}/bulk` with `{"queries": [...]}`. Concurrent cache lookups (capped 10 goroutines), partial success with per-query status, batch limit of 100
- Rich endpoint discovery — enhance `GET /api/endpoints` to include: required/optional parameters with types, example request URLs, response schema from last cached response, cache status per slug
- Go client SDK — `pkg/client` with `NewClient(baseURL, opts...)`, typed methods for all public endpoints, typed errors (`ErrUpstreamDown`, `ErrCacheMiss`), auto-retry with Retry-After, bulk query support
- Per-slug metadata — `GET /api/cache/{slug}/_meta` returns lightweight cache state (last_refreshed, ttl_remaining, is_stale, page_count, record_count, circuit_state) without full payload

**Success criteria:**
- [ ] Bulk endpoint handles 100 queries with concurrent lookups
- [ ] `/api/endpoints` returns parameter docs and response schemas
- [ ] Go client package with 80%+ test coverage
- [ ] `/_meta` returns cache state for any slug
- [ ] Prometheus metrics for bulk request size and latency

#### M16: Operational Resilience & Runtime Management [LATER]
**Goal:** Runbooks for every alert, chaos tests that prove resilience, hot config reload that eliminates restart-for-config friction.

**Appetite:** 2 weeks

**Target maturity:** ga

**Priority:** P1 — operational maturity

**Features:**
- Operational runbooks (`docs/runbooks/`) — one per M14 alerting rule. Each contains: symptom, likely cause, diagnostic steps (specific commands), remediation, escalation. Quick-reference index at `runbook-index.md`
- Chaos test suite (`tests/chaos/`) — Go tests against local Docker stack: kill MongoDB (verify stale-serve), block upstream (verify circuit breaker), exhaust concurrency (verify 503 + Retry-After), SIGTERM during requests (verify graceful shutdown)
- Hot config reload — `fsnotify` watcher on `config.yaml` + SIGHUP handler. New slugs added to router immediately, removed slugs stop scheduling, zero downtime during reload
- Test-fetch endpoint — `POST /api/test-fetch` accepts endpoint config, executes single-page fetch without caching. Returns parsed response or detailed error. Validates new upstream configs before committing
- Config validation — `POST /api/config/validate` accepts YAML snippet, validates reachability, query params, data_key/total_key paths. No restart required

**Success criteria:**
- [ ] Runbook exists for every M14 alerting rule
- [ ] 4+ chaos tests covering major failure modes, all passing
- [ ] Config file changes reload within 5 seconds, no restart
- [ ] Test-fetch validates new upstream configs dry-run
- [ ] Graceful shutdown verified: 0 dropped in-flight requests on SIGTERM

#### M17: Intelligent Data Layer [LATER]
**Goal:** Transform cash-drugs from a cache into a queryable drug data layer — cross-slug search, field filtering, performance profiling. The service consumers *prefer* over hitting upstreams directly.

**Appetite:** 2–3 weeks

**Target maturity:** ga

**Priority:** P2 — differentiation and long-term value

**Features:**
- Cross-slug search — `GET /api/search?q=metformin` searches across all cached data with MongoDB text indexes. Results grouped by slug with match count and preview. Exact > prefix > contains ranking
- Autocomplete — `GET /api/autocomplete?q=met&limit=10` returns fast prefix matches from drugnames/fda-ndc caches for typeahead UIs
- Field filtering — `GET /api/cache/{slug}?...&fields=product_ndc,brand_name` returns only requested fields. Reduces payload for consumers that need subsets
- Go pprof endpoints — `net/http/pprof` on internal port (:6060). CPU, heap, goroutine profiles on-demand. Not exposed on :8080
- Performance baselines — `go test -bench` suite covering cache hit, LRU vs MongoDB latency, upstream fetch with varying page counts. Committed results serve as regression reference
- MongoDB TTL indexes — TTL index on `updated_at` (2x endpoint TTL) for automatic stale document cleanup. Prevents unbounded collection growth

**Success criteria:**
- [ ] Cross-slug search returns grouped results in < 100ms
- [ ] Autocomplete returns in < 20ms
- [ ] Field filtering reduces response payload to requested fields
- [ ] pprof accessible on internal port
- [ ] Benchmark suite with committed P50/P95/P99 baselines
- [ ] Stale documents automatically expire via TTL index

### Milestone Sequencing

```
M13 (GA Readiness) ── in progress, eligible 2026-04-04
    │
    ▼
M14: Observability & Operational Foundation (2 weeks)
    │
    ├──▶ M15: Consumer Value & API Ergonomics (2 weeks, can overlap M14's second week)
    │
    └──▶ M16: Operational Resilience & Runtime Management (2 weeks, starts after M14)
              │
              ▼
         M17: Intelligent Data Layer (2-3 weeks, starts after M15 + M16 core)
```

M15 and M16 can run partially in parallel once M14's correlation IDs and alerting rules land. M17 starts after M15 (bulk API) and M16 (hot reload) are complete.

**GA promotion gate:** After M14 + M16, the service has SLAs, alerts, runbooks, chaos tests, and tracing. M15 and M17 are growth milestones that can ship post-GA.

### Deferred Items (Future Milestones)

| Item | Rationale |
|------|-----------|
| Multi-tenancy / API keys | Platform play — premature until 3+ consumer teams |
| Response diffing / change detection | High value but high complexity. Revisit post-GA |
| Pre-materialized responses | Architectural risk. Only if page reassembly proves bottleneck |
| Consistent hash routing | Only matters at 4+ instances |
| Dynamic config API (MongoDB-backed) | Hot reload (file-based) covers 90% of the pain |
| Built-in HTML dashboard | Grafana exists. Spike only if operator feedback demands it |

### Maturity Promotion Path

| From | To | Requirements |
|------|-----|-------------|
| alpha → beta | Feature specs for scheduling, >50% test coverage, PR workflow, 2+ environments |
| beta → ga | >80% coverage, full CI/CD, branch protection, release tags, monitoring, API documentation |

## 7. Key Features

### Feature 1: API Configuration
Define upstream REST API endpoints with URL, query parameters, pagination style, and response structure. Configuration stored in `config.yaml`. Supports page-based and offset-based pagination, configurable data/total keys.

### Feature 2: On-Demand Fetch
When an internal consumer requests data for a configured endpoint, cash-drugs checks MongoDB for a cached response. If no cache exists, it fetches from the upstream API, stores the response, and returns it. If upstream is down, returns the last cached response.

### Feature 3: Scheduled Fetch
Periodically refresh cached data on a configurable cron schedule. Ensures cached data stays reasonably fresh without waiting for consumer requests. TTL-based staleness detection with background revalidation.

### Feature 4: Consumer REST API
Simple REST API for internal microservices to query cached data. `GET /api/cache/{slug}` with query parameter support. Returns cached MongoDB data with metadata (cached timestamp, upstream status, page count, staleness).

### Feature 5: FDA Drug Data
Six FDA openFDA endpoints for drug enforcement actions, shortages, NDC codes, approvals, labels, and adverse events. Two prefetched daily, four on-demand with search parameters.

### Feature 6: Docker Publishing
Automated CI/CD pipeline builds and publishes Docker images to a private registry. Version-tagged releases, beta channel for testing, build version embedded in binary and exposed via `/health`.

## 8. Non-Functional Requirements

- **Performance:** Cached responses served in < 50ms (P95)
- **Reliability:** Serve stale cached data when upstream is unavailable — never return an error if cache exists
- **Security:** Internal network only — no public exposure required in v1
- **Data:** Store raw API responses as-is in MongoDB — no transformation in v1
- **Observability:** Structured logging via log/slog with configurable level and format; Prometheus metrics for all layers; container system metrics
- **Resilience:** Circuit breakers on upstream APIs, force-refresh cooldown, concurrency limiting with graceful 503 degradation

## 9. Open Questions

- ~~What upstream APIs will be configured first?~~ DailyMed + FDA openFDA (resolved)
- What is the expected query volume from internal consumers?
- ~~Should configuration be file-based (static) or API-based (dynamic)?~~ File-based via config.yaml (resolved)
- ~~What MongoDB deployment will be used?~~ Docker container with bind mount to /mnt/mongo (resolved)

## 10. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-05 | 0.1.0 | calebdunn | Initial draft from /add:init interview |
| 2026-03-07 | 0.2.0 | calebdunn | Added M4-M6, FDA integration, Docker publishing, renamed to cash-drugs, updated maturity to beta |
| 2026-03-14 | 0.3.0 | calebdunn | Added M9 (Performance & Resilience) as DONE, M10 (Performance Optimization) as LATER, updated NFRs with resilience |
| 2026-03-15 | 0.4.0 | calebdunn | M10 (Performance Optimization) marked DONE — MongoDB query optimization, LRU sharding, parallel page fetches, empty upstream handling, version endpoint |
| 2026-03-16 | 0.5.0 | calebdunn | M11 DONE (RxNorm, parameterized warmup, multi-instance, nginx LB), M12 IN_PROGRESS, added NFRs for scalability |
| 2026-03-20 | 0.7.0 | calebdunn | Added M14–M17 roadmap (observability, consumer value, operational resilience, intelligent data layer), deferred items, milestone sequencing, updated Out of Scope |

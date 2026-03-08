# cash-drugs — Product Requirements Document

**Version:** 0.2.0
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
- **Observability:** Structured logging via log/slog with configurable level and format

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

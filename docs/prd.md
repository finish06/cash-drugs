# drugs — Product Requirements Document

**Version:** 0.1.0
**Created:** 2026-03-05
**Author:** calebdunn
**Status:** Draft

## 1. Problem Statement

Internal microservices frequently need data from external REST APIs. Each service making its own API calls leads to redundant requests, higher latency, rate limit issues, and service failures when upstream APIs go down. The drugs microservice acts as an API cache/proxy — it fetches data from configurable REST API endpoints (on-demand and on schedule), stores responses in MongoDB, and serves cached data to internal consumers. When upstream APIs are unavailable, it continues serving stale cached data to maintain reliability.

## 2. Target Users

- **Internal microservices:** Backend services that need data from external REST APIs. They query drugs instead of calling upstream APIs directly, getting lower latency, cached responses, and resilience against upstream failures.

## 3. Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Reduced API calls | 80%+ reduction in upstream API calls | Compare upstream call count before/after adoption |
| Lower latency | < 50ms for cached responses | P95 response time from MongoDB cache vs upstream |
| Reliability | Serve stale when upstream is down | Successful responses during upstream outage windows |

## 4. Scope

### In Scope (MVP)

- Configurable API endpoints (URL + query params)
- On-demand fetch (internal service requests data, drugs fetches if not cached)
- Scheduled fetch (cron-style periodic refresh of configured endpoints)
- MongoDB storage of API responses
- REST API for internal consumers to retrieve cached data
- Serve stale data when upstream is unavailable
- Docker Compose local development environment
- Health check endpoint

### Out of Scope (v1)

- TTL / cache expiry policies
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

### Infrastructure

| Component | Choice | Notes |
|-----------|--------|-------|
| Git Host | GitHub | — |
| Cloud Provider | Self-hosted | Homelab deployment |
| CI/CD | GitHub Actions | `.github/workflows/ci.yml` |
| Containers | Docker Compose | Full local + deploy strategy |
| IaC | None | — |

### Environment Strategy

| Environment | Purpose | URL | Deploy Trigger |
|-------------|---------|-----|----------------|
| Local | Development & unit tests | http://localhost:8080 | Manual |
| Dev | Integration testing | TBD | Push to dev branch |
| Staging | Pre-production validation | TBD | Push to staging branch |
| Production | Live service | TBD | Merge to main |

**Environment Tier:** 3 (dev/staging/prod — all homelab)

## 6. Milestones & Roadmap

### Current Maturity: Alpha

### Roadmap

| Milestone | Goal | Target Maturity | Status | Success Criteria |
|-----------|------|-----------------|--------|------------------|
| M1: Config + Fetch + Store | Fetch from configured APIs and store in MongoDB | alpha | NOW | Configure endpoints, fetch responses, store in MongoDB, serve to consumers |
| M2: Scheduling + Staleness | Scheduled refresh and stale-while-revalidate | beta | NEXT | Cron-based refresh, serve stale on upstream failure, TTL policies |
| M3: Auth + Transforms | Upstream auth and response transforms | ga | LATER | API key/OAuth support, field mapping, response filtering |

### Milestone Detail

#### M1: Config + Fetch + Store [NOW]
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
- [ ] Configure at least one upstream API endpoint
- [ ] Fetch and store response in MongoDB
- [ ] Serve cached response to consumer
- [ ] Serve stale data when upstream is unavailable
- [ ] Health check endpoint returns status

### Maturity Promotion Path

| From | To | Requirements |
|------|-----|-------------|
| alpha → beta | Feature specs for scheduling, >50% test coverage, PR workflow, 2+ environments |
| beta → ga | >80% coverage, full CI/CD, branch protection, release tags, monitoring |

## 7. Key Features

### Feature 1: API Configuration
Define upstream REST API endpoints with URL, query parameters, and optional headers. Configuration stored in a config file or MongoDB collection. Supports multiple independent endpoints.

### Feature 2: On-Demand Fetch
When an internal consumer requests data for a configured endpoint, drugs checks MongoDB for a cached response. If no cache exists, it fetches from the upstream API, stores the response, and returns it. If upstream is down, returns the last cached response.

### Feature 3: Scheduled Fetch
Periodically refresh cached data on a configurable schedule (cron-style). Ensures cached data stays reasonably fresh without waiting for consumer requests.

### Feature 4: Consumer REST API
Simple REST API for internal microservices to query cached data. Endpoints map to configured upstream APIs. Returns cached MongoDB data with metadata (cached timestamp, upstream status).

## 8. Non-Functional Requirements

- **Performance:** Cached responses served in < 50ms (P95)
- **Reliability:** Serve stale cached data when upstream is unavailable — never return an error if cache exists
- **Security:** Internal network only — no public exposure required in v1
- **Data:** Store raw API responses as-is in MongoDB — no transformation in v1

## 9. Open Questions

- What upstream APIs will be configured first?
- What is the expected query volume from internal consumers?
- Should configuration be file-based (static) or API-based (dynamic)?
- What MongoDB deployment will be used (existing cluster or new)?

## 10. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-05 | 0.1.0 | calebdunn | Initial draft from /add:init interview |

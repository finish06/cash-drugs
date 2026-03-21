# M15 — Consumer Value & API Ergonomics

## Goal

Eliminate consumer friction — bulk queries, richer discovery, Go SDK, per-slug metadata. Make adoption effortless for internal microservice teams.

## Status: NOT_STARTED

## Appetite: 2 weeks

## Priority: P1 — growth accelerator

## Success Criteria

- [ ] Bulk endpoint handles 100 queries with concurrent lookups (10-goroutine cap)
- [ ] `/api/endpoints` returns parameter docs, example URLs, and response schemas
- [ ] Go client package in `pkg/client/` with 80%+ test coverage
- [ ] `GET /api/cache/{slug}/_meta` returns cache state for any slug
- [ ] Prometheus metrics for bulk request size and latency

## Hill Chart

| Feature | Position | PR |
|---------|----------|----|
| Bulk Query API (`specs/bulk-query-api.md`) | SHAPED | — |
| Rich Endpoint Discovery (`specs/rich-endpoint-discovery.md`) | SHAPED | — |
| Go Client SDK (`specs/go-client-sdk.md`) | SHAPED | — |
| Per-Slug Metadata (`specs/per-slug-metadata.md`) | SHAPED | — |

## Dependencies

- M14 Observability & Operational Foundation (DONE) — error codes, X-Request-ID, /api/cache/status patterns
- Bulk Query API should be implemented before Go Client SDK (SDK wraps bulk endpoint)
- Per-Slug Metadata can proceed independently
- Rich Endpoint Discovery can proceed independently

## Risks

| Risk | Mitigation |
|------|-----------|
| SDK API surface changes after initial release | Keep SDK in `pkg/client/` (internal module path); version as v0 until M15 complete |
| Bulk query concurrency causes MongoDB connection pool exhaustion | Semaphore cap at 10; test with 100-query batches against real MongoDB |
| Response schema inference is brittle for varied upstream data shapes | Limit to first-item field names + types; null for non-JSON or empty caches |
| `_meta` route conflicts with slugs containing `_meta` | Document as known limitation; unlikely given current slug naming convention |
| Rich endpoint discovery adds latency from cache lookups | Cache status computed per-endpoint; keep under 200ms with 20+ endpoints |

## Retrospective

_To be filled at milestone completion._

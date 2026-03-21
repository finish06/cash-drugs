# Implementation Plan: M15 — Consumer Value & API Ergonomics

**Spec Version:** 0.1.0
**Specs:** specs/bulk-query-api.md, specs/rich-endpoint-discovery.md, specs/go-client-sdk.md, specs/per-slug-metadata.md
**Created:** 2026-03-20
**Team Size:** Solo (1 agent)
**Estimated Duration:** 2 weeks

## Overview

Four features that reduce consumer integration friction: a bulk query endpoint for batch lookups, enriched endpoint discovery metadata, a typed Go client SDK, and a lightweight per-slug metadata endpoint. Implementation order ensures each feature builds on the previous where dependencies exist.

## Objectives

1. Ship `POST /api/cache/{slug}/bulk` with concurrent lookups and partial success
2. Enrich `GET /api/endpoints` with parameter metadata, example URLs, response schemas, and cache status
3. Create `pkg/client/` Go SDK with typed methods, errors, and auto-retry
4. Ship `GET /api/cache/{slug}/_meta` for lightweight cache state checks

## Recommended Implementation Order

```
Phase 1: Per-Slug Metadata (independent, small)
Phase 2: Bulk Query API (independent, medium)
Phase 3: Rich Endpoint Discovery (independent, medium)
Phase 4: Go Client SDK (depends on Phases 1-3 for full API surface)
```

Phases 1-3 can overlap or run in parallel. Phase 4 must wait for at least Phases 1 and 2 to complete so the SDK can wrap those endpoints.

---

## Phase 1: Per-Slug Metadata Endpoint

**Spec:** specs/per-slug-metadata.md
**Estimated Duration:** 3 hours

### Task Breakdown

| Task ID | Description | Phase | Effort | AC |
|---------|-------------|-------|--------|----|
| T1-001 | RED: Write unit tests for `MetaHandler` — 200 with all fields, 404 for unknown slug, null fields for never-cached, circuit state mapping | RED | 30min | AC-001..AC-011 |
| T1-002 | GREEN: Define `SlugMeta` response struct in `internal/handler/meta.go` | GREEN | 10min | AC-001, AC-008 |
| T1-003 | GREEN: Implement `MetaHandler.ServeHTTP` — endpoint lookup, FetchedAt query, TTL/staleness calculation, circuit state lookup | GREEN | 40min | AC-001..AC-009 |
| T1-004 | GREEN: Add `MetadataOnly(baseKey string) (*CacheMetadata, error)` to Repository interface for page_count/record_count without data | GREEN | 30min | AC-005, AC-006, AC-012 |
| T1-005 | GREEN: Wire handler in `cmd/server/main.go` — register on inner mux (under limiter) | GREEN | 15min | AC-013 |
| T1-006 | REFACTOR: Add Swagger annotations, run `go vet`, verify no regressions | REFACTOR | 15min | AC-014, AC-015 |

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/handler/meta.go` | Create | `MetaHandler` with `ServeHTTP`, `SlugMeta` struct |
| `internal/handler/meta_test.go` | Create | Unit tests |
| `internal/cache/repository.go` | Modify | Add `MetadataOnly` to `Repository` interface |
| `internal/cache/mongo.go` | Modify | Implement `MetadataOnly` with data-excluded projection |
| `internal/cache/mock_test.go` (or equivalent) | Modify | Add mock for new interface method |
| `cmd/server/main.go` | Modify | Wire and register `MetaHandler` |

---

## Phase 2: Bulk Query API

**Spec:** specs/bulk-query-api.md
**Estimated Duration:** 5 hours

### Task Breakdown

| Task ID | Description | Phase | Effort | AC |
|---------|-------------|-------|--------|----|
| T2-001 | RED: Write unit tests — successful batch, partial success, batch limit, empty batch, malformed body, unknown slug, concurrent cap verification | RED | 45min | AC-001..AC-009 |
| T2-002 | RED: Write unit tests for Prometheus metrics — histogram recorded for size and duration | RED | 15min | AC-010, AC-011 |
| T2-003 | GREEN: Define bulk request/response types in `internal/model/bulk.go` | GREEN | 15min | AC-001, AC-003, AC-004 |
| T2-004 | GREEN: Implement `BulkHandler` in `internal/handler/bulk.go` — parse body, validate batch size, validate slug, dispatch concurrent lookups via semaphore, collect results preserving order | GREEN | 60min | AC-001..AC-009, AC-012, AC-014 |
| T2-005 | GREEN: Add `cashdrugs_bulk_request_size` and `cashdrugs_bulk_request_duration_seconds` histograms to `internal/metrics/metrics.go` | GREEN | 15min | AC-010 |
| T2-006 | GREEN: Wire handler in `cmd/server/main.go` — register `POST /api/cache/{slug}/bulk` on inner mux | GREEN | 15min | AC-012 |
| T2-007 | REFACTOR: Add Swagger annotations, refactor concurrent lookup into helper function | REFACTOR | 20min | AC-013 |
| T2-008 | VERIFY: Run full test suite, coverage check | VERIFY | 15min | AC-015 |

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/model/bulk.go` | Create | `BulkQueryRequest`, `BulkQueryResponse`, `BulkQueryResult` types |
| `internal/handler/bulk.go` | Create | `BulkHandler` with `ServeHTTP` |
| `internal/handler/bulk_test.go` | Create | Unit tests |
| `internal/metrics/metrics.go` | Modify | Add 2 histogram metrics |
| `cmd/server/main.go` | Modify | Wire and register `BulkHandler` |

---

## Phase 3: Rich Endpoint Discovery

**Spec:** specs/rich-endpoint-discovery.md
**Estimated Duration:** 4 hours

### Task Breakdown

| Task ID | Description | Phase | Effort | AC |
|---------|-------------|-------|--------|----|
| T3-001 | RED: Update existing endpoint tests — verify new fields present, verify backward compat | RED | 30min | AC-001..AC-007, AC-010 |
| T3-002 | GREEN: Extend `EndpointInfo` struct with `Parameters`, `ExampleURL`, `ResponseSchema`, `CacheStatus` | GREEN | 15min | AC-001, AC-007 |
| T3-003 | GREEN: Implement parameter extraction — parse path params from URL template, mark required; include search_params as optional query | GREEN | 30min | AC-001, AC-002, AC-003 |
| T3-004 | GREEN: Implement example URL generation from slug and first parameter | GREEN | 20min | AC-004 |
| T3-005 | GREEN: Implement response schema inference — lookup base cache key, inspect first data item, map field types | GREEN | 40min | AC-005 |
| T3-006 | GREEN: Implement cache status per endpoint — reuse `FetchedAt` + staleness logic from StatusHandler | GREEN | 25min | AC-006 |
| T3-007 | GREEN: Update `NewEndpointsHandler` constructor to accept `cache.Repository` and `*upstream.CircuitRegistry` | GREEN | 15min | AC-005, AC-006 |
| T3-008 | REFACTOR: Update Swagger annotations, run vet, verify performance under 200ms | REFACTOR | 20min | AC-008, AC-009 |

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/handler/endpoints.go` | Modify | Extend `EndpointInfo`, `NewEndpointsHandler`, `ServeHTTP` |
| `internal/handler/endpoints_test.go` | Modify | Update and extend tests |
| `cmd/server/main.go` | Modify | Pass repo + circuit registry to `NewEndpointsHandler` |

---

## Phase 4: Go Client SDK

**Spec:** specs/go-client-sdk.md
**Estimated Duration:** 6 hours

### Task Breakdown

| Task ID | Description | Phase | Effort | AC |
|---------|-------------|-------|--------|----|
| T4-001 | RED: Write tests using `httptest.Server` — all methods, error types, retry behavior, context cancellation | RED | 60min | AC-001..AC-016 |
| T4-002 | GREEN: Create `pkg/client/client.go` — `Client` struct, `NewClient`, `ClientOption` funcs | GREEN | 30min | AC-001, AC-002, AC-012 |
| T4-003 | GREEN: Create `pkg/client/types.go` — all response/request types (no internal imports) | GREEN | 30min | AC-014 |
| T4-004 | GREEN: Create `pkg/client/errors.go` — `APIError`, sentinel errors, error wrapping from HTTP status | GREEN | 20min | AC-009, AC-010 |
| T4-005 | GREEN: Implement `GetCache`, `GetEndpoints`, `GetStatus`, `GetVersion`, `Health`, `GetMeta` | GREEN | 45min | AC-003..AC-006, AC-008, AC-016 |
| T4-006 | GREEN: Implement `BulkQuery` | GREEN | 20min | AC-007 |
| T4-007 | GREEN: Implement auto-retry with Retry-After header and exponential backoff | GREEN | 30min | AC-011 |
| T4-008 | REFACTOR: Add `doc.go` with package docs and usage example | REFACTOR | 15min | AC-015 |
| T4-009 | VERIFY: Run `go test ./pkg/client/...` — verify 80%+ coverage | VERIFY | 15min | AC-013 |

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `pkg/client/client.go` | Create | `Client`, `NewClient`, `ClientOption`, HTTP helper methods |
| `pkg/client/types.go` | Create | All public types (response structs, request structs) |
| `pkg/client/errors.go` | Create | `APIError`, sentinel errors |
| `pkg/client/doc.go` | Create | Package documentation |
| `pkg/client/client_test.go` | Create | Comprehensive tests with httptest |

---

## Cross-Feature Dependencies

```
Per-Slug Metadata ──────────────────────────┐
                                             │
Bulk Query API ──────────────────────────────┼──▶ Go Client SDK
                                             │
Rich Endpoint Discovery ────────────────────┘
```

- **Go Client SDK** depends on all three endpoints existing so it can provide typed methods
- **Per-Slug Metadata**, **Bulk Query API**, and **Rich Endpoint Discovery** are independent of each other
- All four features share the existing `cache.Repository` interface and `config.Endpoint` struct

## Test Strategy

| Feature | Unit Tests | Integration Tests | Coverage Target |
|---------|-----------|-------------------|-----------------|
| Per-Slug Metadata | `meta_test.go` — handler + mock repo | Optional (MongoDB) | 80%+ |
| Bulk Query API | `bulk_test.go` — handler + mock repo | Optional (MongoDB) | 80%+ |
| Rich Endpoint Discovery | `endpoints_test.go` — extended | None needed | 80%+ |
| Go Client SDK | `client_test.go` — httptest server | None needed | 80%+ |

All features: run `make test-unit` and `go vet ./...` after each phase.

## Effort Summary

| Phase | Feature | Estimated Hours |
|-------|---------|-----------------|
| Phase 1 | Per-Slug Metadata | 3h |
| Phase 2 | Bulk Query API | 5h |
| Phase 3 | Rich Endpoint Discovery | 4h |
| Phase 4 | Go Client SDK | 6h |
| **Total** | | **18h** |

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Repository interface change breaks existing tests | Medium | Medium | Add method to interface with backward-compatible mock |
| Bulk query goroutine leak on context cancel | Low | High | Use `errgroup` or explicit goroutine cleanup with context |
| Response schema inference too slow for endpoints with large cached data | Low | Medium | Query only first document with projection; cap at 20 fields |
| SDK type drift from server types | Medium | Medium | SDK types are standalone copies; integration tests catch drift |
| Route matching for `/_meta` conflicts with data queries | Low | Low | Match `_meta` suffix explicitly before falling through to cache handler |

## Spec Traceability

### Per-Slug Metadata
| AC | Tasks |
|----|-------|
| AC-001..AC-009 | T1-001, T1-002, T1-003 |
| AC-005, AC-006 | T1-004 |
| AC-010..AC-011 | T1-001, T1-003 |
| AC-012 | T1-004 |
| AC-013 | T1-005 |
| AC-014, AC-015 | T1-006 |

### Bulk Query API
| AC | Tasks |
|----|-------|
| AC-001..AC-009 | T2-001, T2-003, T2-004 |
| AC-010, AC-011 | T2-002, T2-005 |
| AC-012 | T2-006 |
| AC-013 | T2-007 |
| AC-014 | T2-004 |
| AC-015 | T2-008 |

### Rich Endpoint Discovery
| AC | Tasks |
|----|-------|
| AC-001..AC-003 | T3-001, T3-002, T3-003 |
| AC-004 | T3-004 |
| AC-005 | T3-005 |
| AC-006 | T3-006 |
| AC-007 | T3-001, T3-002 |
| AC-008, AC-009 | T3-008 |
| AC-010 | T3-001 |

### Go Client SDK
| AC | Tasks |
|----|-------|
| AC-001, AC-002 | T4-001, T4-002 |
| AC-003..AC-006, AC-008, AC-016 | T4-001, T4-005 |
| AC-007 | T4-001, T4-006 |
| AC-009, AC-010 | T4-001, T4-004 |
| AC-011 | T4-001, T4-007 |
| AC-012 | T4-002 |
| AC-013 | T4-009 |
| AC-014 | T4-003 |
| AC-015 | T4-008 |

## Next Steps

1. Review and approve this plan
2. Execute Phase 1: `specs/per-slug-metadata.md`
3. Execute Phases 2-3 (can overlap): `specs/bulk-query-api.md`, `specs/rich-endpoint-discovery.md`
4. Execute Phase 4: `specs/go-client-sdk.md`

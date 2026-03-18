# M12 — Client Enhancements

## Goal

Address client feedback from drug-gate integration: readiness signaling, cache warmup control, response normalization for consistent consumption, and API documentation accuracy.

## Status: DONE

## Appetite: 1 week

## Success Criteria

- [x] Clients can poll `/ready` to gate traffic until cache is warm
- [x] Clients can trigger selective or full cache warmup via `/api/warmup`
- [x] `flatten` flag normalizes nested upstream arrays into flat response shape
- [x] Swagger docs accurately describe all response shapes and query parameters
- [x] Upstream 404 responses handled gracefully (not cached as errors)

## Hill Chart

| Feature | Position | PR |
|---------|----------|----|
| Readiness endpoint (`/ready`) | DONE | #15 |
| Response normalization (`flatten` flag) | DONE | #14 |
| API documentation fixes | DONE | (swagger updated) |
| Upstream 404 handling | DONE | #20 |

## Features

### 1. Readiness Endpoint — DONE (PR #15)

**Spec:** `specs/readiness-warmup.md` (12 ACs)

`GET /ready` returns service readiness based on cache warmup state:
- **503** during warmup with `{"status": "warming", "progress": "5/17"}`
- **200** when ready with `{"status": "ready"}`

Includes `POST /api/warmup` for on-demand cache pre-fetch:
- No body: warms all scheduled endpoints
- `{"slugs": ["drugnames", "fda-enforcement"]}`: warms specific slugs
- Returns 202 immediately; fetch runs in background
- Validates slugs against config; returns 400 for unknown slugs

### 2. Response Normalization — DONE (PR #14)

**Spec:** `specs/response-normalization.md` (8 ACs)

`flatten` config flag per endpoint. When enabled, nested arrays in upstream responses are flattened into a single top-level array, giving clients a consistent shape regardless of upstream pagination or nesting structure.

### 3. API Documentation Fixes — DONE

- Documented `drugclasses` response shape in Swagger annotations
- Added `search` query parameter guidance for drug lookup endpoints
- Fixed dot-path `data_key` resolution in `fetchJSONPage`

### 4. Upstream 404 Handling — DONE (PR #20)

**Spec:** `specs/upstream-404-handling.md` (10 ACs)

When an upstream API returns HTTP 404, cash-drugs returns 404 to consumers (not 502) with `error`, `slug`, and `params` fields. Negative cache entries stored in LRU + MongoDB with 10-minute TTL prevent repeated upstream calls for non-existent resources. Other 4xx/5xx/network errors unchanged (502 with stale fallback). `upstream_404_total` Prometheus counter per slug.

## Dependencies

- M10 (Performance Optimization) — complete
- M11 (RxNorm Integration) — complete

## Risks

| Risk | Mitigation |
|------|-----------|
| Warmup delays server availability | Readiness probe separates liveness from readiness; traffic is gated |
| Flatten flag changes response shape | Opt-in per endpoint; existing endpoints unaffected |

## Cycle History

| Cycle | Features | Status | Notes |
|-------|----------|--------|-------|
| cycle-4 | Upstream 404 handling (SPECCED→DONE) | COMPLETE | 10 ACs, PR #20 merged. TDD cycle in away mode. |

## Retrospective

_To be filled at `/add:retro`._

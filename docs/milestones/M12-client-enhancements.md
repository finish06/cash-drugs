# M12 — Client Enhancements

## Goal

Address client feedback from drug-gate integration: readiness signaling, cache warmup control, response normalization for consistent consumption, and API documentation accuracy.

## Status: NOW

## Appetite: 1 week

## Success Criteria

- [ ] Clients can poll `/ready` to gate traffic until cache is warm
- [ ] Clients can trigger selective or full cache warmup via `/api/warmup`
- [ ] `flatten` flag normalizes nested upstream arrays into flat response shape
- [ ] Swagger docs accurately describe all response shapes and query parameters
- [ ] Upstream 404 responses handled gracefully (not cached as errors)

## Hill Chart

| Feature | Position | PR |
|---------|----------|----|
| Readiness endpoint (`/ready`) | DONE | #15 |
| Response normalization (`flatten` flag) | DONE | #14 |
| API documentation fixes | DONE | (swagger updated) |
| Upstream 404 handling | SPECCED | — |

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

### 4. Upstream 404 Handling — SPECCED (unimplemented)

**Spec:** `specs/upstream-404-handling.md`

Draft spec exists. When an upstream API returns 404, the system should handle it gracefully rather than treating it as an upstream error. Not yet planned for a cycle.

## Dependencies

- M10 (Performance Optimization) — complete
- M11 (RxNorm Integration) — complete

## Risks

| Risk | Mitigation |
|------|-----------|
| Warmup delays server availability | Readiness probe separates liveness from readiness; traffic is gated |
| Flatten flag changes response shape | Opt-in per endpoint; existing endpoints unaffected |

## Retrospective

_To be filled at milestone completion._

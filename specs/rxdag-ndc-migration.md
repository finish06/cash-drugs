# Spec: rx-dag NDC Migration

**Version:** 0.1.0
**Created:** 2026-04-04
**PRD Reference:** docs/prd.md
**Status:** Complete

## 1. Overview

Migrate the `fda-ndc` slug from the public openFDA API (`api.fda.gov/drug/ndc.json`) to the internal rx-dag ndc-loader service (`192.168.1.145:8081/api/openfda/ndc.json`), which is a drop-in replacement with identical response format. Add three new slugs for rx-dag's richer query endpoints: full-text search, direct NDC lookup, and package listing. Introduce a generic `headers` config field for upstream auth, used here to pass `X-API-Key` via an environment variable.

### User Story

As an internal microservice developer, I want NDC queries served from the internal rx-dag ndc-loader instead of the public FDA API, so that I get faster responses, richer query options, and independence from FDA rate limits and availability.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | `fda-ndc` slug's `base_url` changed to `http://192.168.1.145:8081` and `path` changed to `/api/openfda/ndc.json` — consumers see no change in slug name, query params, or response shape | Must |
| AC-002 | Config supports a `headers` map field on any endpoint — key/value pairs sent as HTTP headers on every upstream request | Must |
| AC-003 | Header values support environment variable interpolation via `${ENV_VAR}` syntax (e.g., `X-API-Key: "${RXDAG_API_KEY}"`) | Must |
| AC-004 | `fda-ndc` endpoint config includes `headers: { "X-API-Key": "${RXDAG_API_KEY}" }` | Must |
| AC-005 | New slug `rx-dag-ndc-search` proxies `GET /api/ndc/search` with query param `q` (required), `limit` (optional), `offset` (optional) | Must |
| AC-006 | New slug `rx-dag-ndc-lookup` proxies `GET /api/ndc/{NDC}` with path param `NDC` (any format: hyphenated, unhyphenated, 2-segment, 3-segment) | Must |
| AC-007 | New slug `rx-dag-ndc-packages` proxies `GET /api/ndc/{NDC}/packages` with path param `NDC` | Must |
| AC-008 | All four rx-dag slugs pass `X-API-Key` header from `RXDAG_API_KEY` env var on every upstream request | Must |
| AC-009 | When rx-dag is unreachable, stale cached data is served from MongoDB (consistent with all other slugs) | Must |
| AC-010 | When rx-dag is unreachable and no cache exists, return 502 with standard error envelope (`CD-U001`) | Must |
| AC-011 | All four rx-dag slugs are on-demand only (no scheduled refresh) with default TTL | Must |
| AC-012 | Existing `fda-enforcement`, `fda-shortages`, and `fda-label` slugs remain unchanged (backward compatible) | Must |
| AC-013 | Missing or empty `RXDAG_API_KEY` env var logged as warning at startup; upstream requests fail with 502 (not crash) | Must |
| AC-014 | `headers` config field is optional — endpoints without it behave unchanged | Must |
| AC-015 | OpenAPI/Swagger docs updated with new rx-dag slugs and their parameters | Should |
| AC-016 | `GET /api/endpoints` discovery includes new rx-dag slugs with parameter documentation | Should |

## 3. User Test Cases

### TC-001: Transparent fda-ndc swap — existing consumer query works

**Precondition:** Server running with `fda-ndc` pointing at rx-dag, `RXDAG_API_KEY` set
**Steps:**
1. Request `GET /api/cache/fda-ndc?BRAND_NAME=metformin`
2. Upstream request goes to `192.168.1.145:8081/api/openfda/ndc.json?search=brand_name:"metformin"` with `X-API-Key` header
3. Response cached in MongoDB
**Expected Result:** Returns NDC records in same format as before (openFDA response shape with `results` array and `meta`)
**Screenshot Checkpoint:** N/A (API only)
**Maps to:** TBD

### TC-002: rx-dag full-text search

**Precondition:** Server running with `rx-dag-ndc-search` slug configured, `RXDAG_API_KEY` set
**Steps:**
1. Request `GET /api/cache/rx-dag-ndc-search?q=metformin`
2. Upstream request goes to `192.168.1.145:8081/api/ndc/search?q=metformin` with `X-API-Key` header
3. Response cached in MongoDB
**Expected Result:** Returns search results with brand/generic name matches
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-003: rx-dag direct NDC lookup

**Precondition:** Server running with `rx-dag-ndc-lookup` slug configured, `RXDAG_API_KEY` set
**Steps:**
1. Request `GET /api/cache/rx-dag-ndc-lookup?NDC=0002-1433`
2. Upstream request goes to `192.168.1.145:8081/api/ndc/0002-1433` with `X-API-Key` header
**Expected Result:** Returns product details with packages for NDC 0002-1433
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-004: rx-dag package listing

**Precondition:** Server running with `rx-dag-ndc-packages` slug configured, `RXDAG_API_KEY` set
**Steps:**
1. Request `GET /api/cache/rx-dag-ndc-packages?NDC=0002-1433`
2. Upstream request goes to `192.168.1.145:8081/api/ndc/0002-1433/packages` with `X-API-Key` header
**Expected Result:** Returns package configurations for the product NDC
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-005: Stale-serve when rx-dag is down

**Precondition:** `fda-ndc` has cached data in MongoDB, rx-dag is unreachable
**Steps:**
1. Request `GET /api/cache/fda-ndc?BRAND_NAME=aspirin`
2. Upstream request to rx-dag fails (connection refused)
**Expected Result:** Returns cached response with `stale: true` in meta
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-006: Missing API key returns 502, not crash

**Precondition:** Server running with `RXDAG_API_KEY` unset or empty, no cache exists
**Steps:**
1. Request `GET /api/cache/fda-ndc?BRAND_NAME=aspirin`
2. Upstream request sent without valid API key
3. rx-dag returns 401
**Expected Result:** cash-drugs returns 502 with error envelope (`CD-U001`), logs warning. Server does not crash.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-007: Headers config field works generically

**Precondition:** An endpoint configured with `headers: { "X-Custom": "test-value" }`
**Steps:**
1. Request the endpoint
2. Upstream HTTP request includes `X-Custom: test-value` header
**Expected Result:** Custom header sent to upstream
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-008: Existing FDA slugs unaffected

**Precondition:** Server running with all FDA + rx-dag slugs
**Steps:**
1. Request `GET /api/cache/fda-enforcement`
2. Request `GET /api/cache/fda-shortages`
3. Request `GET /api/cache/fda-label?BRAND_NAME=aspirin`
**Expected Result:** All return data from api.fda.gov as before — no regression
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

## 4. Data Model

### Endpoint Config Extension

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| headers | map[string]string | No | Key/value pairs sent as HTTP headers on upstream requests. Values support `${ENV_VAR}` interpolation. |

### No MongoDB schema changes

Responses from rx-dag stored using the same `CachedResponse` structure. The openFDA-compatible endpoint returns identical shape to api.fda.gov. The native endpoints (`/api/ndc/*`) store their response as-is.

## 5. API Contract

### GET /api/cache/fda-ndc?BRAND_NAME={name}

**Description:** NDC search — transparently swapped from api.fda.gov to rx-dag. Same consumer contract.

**Upstream:** `GET http://192.168.1.145:8081/api/openfda/ndc.json?search=brand_name:"{name}"&limit=100`

**Response (200):**
```json
{
  "data": [...],
  "meta": {
    "slug": "fda-ndc",
    "source_url": "http://192.168.1.145:8081/api/openfda/ndc.json",
    "fetched_at": "2026-04-04T12:00:00Z",
    "page_count": 1,
    "stale": false
  }
}
```

### GET /api/cache/rx-dag-ndc-search?q={query}

**Description:** Full-text drug search via rx-dag ndc-loader

**Upstream:** `GET http://192.168.1.145:8081/api/ndc/search?q={query}`

**Query params:** `q` (required), `limit` (optional, default 50), `offset` (optional, default 0)

**Response (200):**
```json
{
  "data": [...],
  "meta": {
    "slug": "rx-dag-ndc-search",
    "source_url": "http://192.168.1.145:8081/api/ndc/search",
    "fetched_at": "2026-04-04T12:00:00Z",
    "page_count": 1,
    "stale": false
  }
}
```

### GET /api/cache/rx-dag-ndc-lookup?NDC={ndc}

**Description:** Direct NDC code lookup via rx-dag ndc-loader

**Upstream:** `GET http://192.168.1.145:8081/api/ndc/{ndc}`

**Response (200):**
```json
{
  "data": { "product_ndc": "0002-1433", "brand_name": "...", "packages": [...], ... },
  "meta": {
    "slug": "rx-dag-ndc-lookup",
    "source_url": "http://192.168.1.145:8081/api/ndc/0002-1433",
    "fetched_at": "2026-04-04T12:00:00Z",
    "page_count": 1,
    "stale": false
  }
}
```

### GET /api/cache/rx-dag-ndc-packages?NDC={ndc}

**Description:** Package listing for a product NDC via rx-dag ndc-loader

**Upstream:** `GET http://192.168.1.145:8081/api/ndc/{ndc}/packages`

**Response (200):**
```json
{
  "data": [...],
  "meta": {
    "slug": "rx-dag-ndc-packages",
    "source_url": "http://192.168.1.145:8081/api/ndc/0002-1433/packages",
    "fetched_at": "2026-04-04T12:00:00Z",
    "page_count": 1,
    "stale": false
  }
}
```

**Error Responses (all endpoints):**
- `404` — Slug not configured
- `502` — rx-dag unavailable, no cache available (`CD-U001`)

## 6. UI Behavior

N/A — API only

## 7. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| rx-dag returns 401 (bad API key) | Treat as upstream error, serve stale if available, 502 if not |
| rx-dag returns 404 for unknown NDC | Pass through as empty result (existing 404 handling) |
| `RXDAG_API_KEY` env var not set | Log warning at startup, upstream requests fail with 401 → 502 to consumer |
| `headers` field with empty map `{}` | No extra headers sent — same as omitting the field |
| `headers` value references undefined env var `${MISSING}` | Resolve to empty string, log warning |
| NDC in different formats (0002-1433 vs 00021433 vs 0002-1433-61) | Pass through to rx-dag as-is — rx-dag handles normalization |
| Consumer passes both `BRAND_NAME` and `GENERIC_NAME` on `fda-ndc` | Same behavior as before — rx-dag openFDA endpoint supports same search syntax |

## 8. Dependencies

- **rx-dag ndc-loader** running at `192.168.1.145:8081` with valid API key
- **Config extension:** `headers` map field on endpoint config (new — implemented as part of this spec)
- **Env var interpolation:** `${ENV_VAR}` syntax in header values (new — implemented as part of this spec)
- No new Go dependencies required
- Existing upstream fetcher, circuit breaker, and stale-serve mechanisms apply unchanged

## 9. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-04-04 | 0.1.0 | calebdunn | Initial spec from /add:spec interview |

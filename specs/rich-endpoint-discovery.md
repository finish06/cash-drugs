# Spec: Rich Endpoint Discovery

**Version:** 0.1.0
**Created:** 2026-03-20
**PRD Reference:** docs/prd.md (M15)
**Status:** Draft

## 1. Overview

The current `GET /api/endpoints` returns basic metadata (slug, path, format, params list, pagination flag, scheduled flag). Consumers still need to read config files or Swagger docs to understand parameter requirements, expected response shapes, and cache freshness. This feature enriches the endpoint discovery response with parameter metadata, example URLs, response schema samples, and per-slug cache status.

### User Story

As an **internal microservice developer**, I want `GET /api/endpoints` to tell me everything I need to call an endpoint correctly — required params, example URLs, what the response looks like, and whether the cache is fresh — so I can integrate without reading source code or config files.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | Each endpoint in the response includes a `parameters` array with objects containing `name`, `required` (bool), `type` (string), and `in` ("path" or "query") | Must |
| AC-002 | Path parameters (extracted from `{PARAM}` patterns in `path`) are marked `required: true, in: "path"` | Must |
| AC-003 | Search parameters (from `search_params` config) are marked `required: false, in: "query"` | Must |
| AC-004 | Each endpoint includes an `example_url` field with a fully-formed URL using placeholder values | Must |
| AC-005 | Each endpoint includes a `response_schema` object containing field names and inferred types from the last cached response (or null if no cache exists) | Should |
| AC-006 | Each endpoint includes `cache_status` with `last_refreshed` (ISO 8601 or null), `is_stale` (bool), and `ttl_remaining` (duration string) | Must |
| AC-007 | The enriched response is backward-compatible — existing fields (`slug`, `path`, `format`, `params`, `pagination`, `scheduled`) are preserved | Must |
| AC-008 | Swagger/OpenAPI annotations are updated for the enriched response type | Should |
| AC-009 | Response time for `GET /api/endpoints` remains under 200ms even with 20+ configured endpoints | Should |
| AC-010 | Existing tests for `GET /api/endpoints` are updated to cover new fields | Must |

## 3. User Test Cases

### TC-001: Parameter metadata for path-param endpoint

**Precondition:** `spl-detail` endpoint configured with path `/REST/document/{SETID}/details.json`
**Steps:**
1. `GET /api/endpoints`
2. Find the `spl-detail` entry
**Expected Result:** `parameters` includes `{"name": "SETID", "required": true, "type": "string", "in": "path"}`.
**Maps to:** AC-001, AC-002

### TC-002: Search params as optional query params

**Precondition:** `fda-ndc` endpoint configured with `search_params: [BRAND_NAME, GENERIC_NAME, NDC, PHARM_CLASS]`
**Steps:**
1. `GET /api/endpoints`
2. Find the `fda-ndc` entry
**Expected Result:** `parameters` includes 4 entries, all with `required: false, in: "query"`.
**Maps to:** AC-001, AC-003

### TC-003: Example URL generation

**Precondition:** `fda-ndc` endpoint configured with `base_url: https://api.fda.gov`, `path: /drug/ndc.json`
**Steps:**
1. `GET /api/endpoints`
2. Check `example_url` for `fda-ndc`
**Expected Result:** `example_url` is something like `http://localhost:8080/api/cache/fda-ndc?BRAND_NAME={BRAND_NAME}`.
**Maps to:** AC-004

### TC-004: Response schema from cached data

**Precondition:** `fda-ndc` has cached data. First item in cache has fields `product_ndc`, `brand_name`, `generic_name`.
**Steps:**
1. `GET /api/endpoints`
2. Check `response_schema` for `fda-ndc`
**Expected Result:** `response_schema` contains `{"product_ndc": "string", "brand_name": "string", "generic_name": "string", ...}`.
**Maps to:** AC-005

### TC-005: Cache status included

**Precondition:** `fda-ndc` was last refreshed 2 hours ago with a 24h TTL.
**Steps:**
1. `GET /api/endpoints`
2. Check `cache_status` for `fda-ndc`
**Expected Result:** `cache_status.last_refreshed` is a valid ISO 8601 timestamp, `is_stale` is false, `ttl_remaining` is approximately "22h0m0s".
**Maps to:** AC-006

### TC-006: No cache exists for endpoint

**Precondition:** `rxnorm-drugs` is configured but has never been fetched.
**Steps:**
1. `GET /api/endpoints`
2. Check `response_schema` and `cache_status`
**Expected Result:** `response_schema` is null. `cache_status.last_refreshed` is null, `is_stale` is true, `ttl_remaining` is "0s".
**Maps to:** AC-005, AC-006

### TC-007: Backward compatibility

**Steps:**
1. `GET /api/endpoints`
2. Parse response with a client that only reads `slug`, `path`, `format`, `params`, `pagination`, `scheduled`
**Expected Result:** All original fields present and unchanged.
**Maps to:** AC-007

## 4. Data Model

### Enhanced EndpointInfo

```go
type EndpointInfo struct {
    Slug           string            `json:"slug"`
    Path           string            `json:"path"`
    Format         string            `json:"format"`
    Params         []string          `json:"params,omitempty"`
    Pagination     bool              `json:"pagination"`
    Scheduled      bool              `json:"scheduled"`
    Parameters     []ParamInfo       `json:"parameters,omitempty"`
    ExampleURL     string            `json:"example_url"`
    ResponseSchema map[string]string `json:"response_schema,omitempty"`
    CacheStatus    *CacheStatusInfo  `json:"cache_status,omitempty"`
}

type ParamInfo struct {
    Name     string `json:"name"`
    Required bool   `json:"required"`
    Type     string `json:"type"`
    In       string `json:"in"` // "path" or "query"
}

type CacheStatusInfo struct {
    LastRefreshed *string `json:"last_refreshed"` // ISO 8601 or null
    IsStale       bool    `json:"is_stale"`
    TTLRemaining  string  `json:"ttl_remaining"`
}
```

### Response Schema Inference

To build `response_schema`:
1. Look up the base slug cache key (no params) via `repo.Get(slug)`
2. If data exists and is a JSON array, inspect the first element
3. For each field in the first element, map Go type to JSON type string: `string`, `number`, `boolean`, `object`, `array`
4. Cap at 20 fields to keep response concise
5. If no cached data exists, set `response_schema` to null

## 5. API Contract

### `GET /api/endpoints` (enhanced)

No change to method, path, or status codes. Response body gains new fields per endpoint.

**Example response entry:**

```json
{
  "slug": "fda-ndc",
  "path": "https://api.fda.gov/drug/ndc.json",
  "format": "json",
  "params": ["BRAND_NAME", "GENERIC_NAME", "NDC", "PHARM_CLASS"],
  "pagination": true,
  "scheduled": true,
  "parameters": [
    {"name": "BRAND_NAME", "required": false, "type": "string", "in": "query"},
    {"name": "GENERIC_NAME", "required": false, "type": "string", "in": "query"},
    {"name": "NDC", "required": false, "type": "string", "in": "query"},
    {"name": "PHARM_CLASS", "required": false, "type": "string", "in": "query"}
  ],
  "example_url": "/api/cache/fda-ndc?BRAND_NAME={BRAND_NAME}",
  "response_schema": {
    "product_ndc": "string",
    "brand_name": "string",
    "generic_name": "string",
    "labeler_name": "string",
    "dosage_form": "string"
  },
  "cache_status": {
    "last_refreshed": "2026-03-20T10:30:00Z",
    "is_stale": false,
    "ttl_remaining": "22h0m0s"
  }
}
```

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| Endpoint with both path params and search params | Both appear in `parameters` with correct `in` and `required` values |
| Endpoint with no configurable params | `parameters` is an empty array |
| Cached data is raw (non-JSON format) | `response_schema` is null |
| Cached data is a single object (not array) | Infer schema from that object directly |
| First cached item has nested objects | Nested fields show as `"field": "object"` — no deep recursion |
| MongoDB unavailable during endpoint listing | `response_schema` and `cache_status` degrade to null; core fields still returned |
| Endpoint with `format: raw` | `response_schema` is null (not JSON) |

## 7. Dependencies

- `internal/handler/endpoints.go` — modify `EndpointsHandler` and `EndpointInfo` struct
- `internal/cache` — `Repository` interface for cache lookups (read-only)
- `internal/config` — existing `Endpoint` struct, `ExtractAllParams`, `ExtractPathParams`, `IsStale`
- No new external dependencies

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-20 | 0.1.0 | calebdunn | Initial spec |

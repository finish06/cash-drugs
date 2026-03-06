# Spec: Upstream API Configuration

**Version:** 0.1.0
**Created:** 2026-03-05
**PRD Reference:** docs/prd.md
**Status:** Draft

## 1. Overview

Configuration-driven upstream API fetching and caching. The service reads a YAML config file at startup that defines upstream REST API endpoints (URLs, paths, query params, pagination rules). When an internal consumer requests data, drugs fetches from the upstream API (auto-paginating per config), stores the raw aggregated response in MongoDB, and returns it. Subsequent requests are served from cache. If the upstream is unreachable, stale cached data is returned. This provides a single unified access point for upstream data, protecting against rate limits and enabling downstream ETL services to iterate freely against cached raw data.

### User Story

As an internal microservice developer, I want a single cached endpoint for upstream API data, so that multiple services don't each call rate-limited external APIs and can freely iterate on the raw data without worrying about upstream availability.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | Service loads upstream API definitions from a YAML config file at startup | Must |
| AC-002 | Each config entry defines: name/slug, base URL, endpoint path, query parameters, response format (JSON/XML), and pagination settings | Must |
| AC-003 | Config entries support path parameters (e.g., `/v2/spls/{SETID}`) where the value is provided by the consumer at request time | Must |
| AC-004 | On consumer request, if no cached response exists, fetch from the upstream API | Must |
| AC-005 | Auto-paginate upstream responses: walk all pages and aggregate into a single cached document | Must |
| AC-006 | Pagination limit is configurable per endpoint: numeric cap (e.g., `10`) or `all` | Must |
| AC-007 | Store the full aggregated upstream response in MongoDB with metadata (fetched_at timestamp, source endpoint, HTTP status, page count) | Must |
| AC-008 | Return cached response to consumer if cache exists (skip upstream call) | Must |
| AC-009 | If upstream is unreachable or returns error, return last cached response with metadata indicating staleness | Must |
| AC-010 | If upstream fails and no cache exists, return error to consumer with upstream status details | Must |
| AC-011 | If pagination fails midway, discard partial fetch and serve stale cache (do not store incomplete data) | Must |
| AC-012 | Invalid config file prevents startup with clear error message identifying the invalid entries | Must |
| AC-013 | Consumer requests for endpoints not defined in config return 404 | Must |
| AC-014 | Consumer-facing URL pattern mirrors the configured slug (e.g., `GET /api/cache/{slug}`) | Should |
| AC-015 | Config file ships with seed entries for DailyMed endpoints (`/v2/drugnames`, `/v2/spls`) for validation | Should |

## 3. User Test Cases

### TC-001: First request triggers upstream fetch and cache

**Precondition:** Service is running with DailyMed `/v2/drugnames` configured. MongoDB has no cached data for this endpoint.
**Steps:**
1. Send `GET /api/cache/drugnames` to drugs
2. Observe drugs fetches from `https://dailymed.nlm.nih.gov/dailymed/services/v2/drugnames.json`
3. Observe drugs auto-paginates through all pages (or up to configured limit)
4. Observe response is stored in MongoDB
5. Receive aggregated drug names data in response
**Expected Result:** 200 response with full aggregated data. MongoDB contains the cached document with metadata (fetched_at, source, page_count).
**Screenshot Checkpoint:** N/A (no UI)
**Maps to:** TBD

### TC-002: Subsequent request served from cache

**Precondition:** TC-001 has run — cached data exists in MongoDB for `drugnames`.
**Steps:**
1. Send `GET /api/cache/drugnames` to drugs
2. Observe no upstream API call is made
**Expected Result:** 200 response with cached data. Response includes metadata indicating it was served from cache.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-003: Upstream failure returns stale cache

**Precondition:** Cached data exists for `drugnames`. Upstream API is unreachable (simulated).
**Steps:**
1. Send `GET /api/cache/drugnames` to drugs
2. Observe drugs attempts upstream fetch and fails
3. Observe drugs falls back to cached data
**Expected Result:** 200 response with stale cached data. Response metadata indicates staleness (e.g., `stale: true`, original `fetched_at`).
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-004: Upstream failure with no cache returns error

**Precondition:** No cached data exists. Upstream API is unreachable.
**Steps:**
1. Send `GET /api/cache/drugnames` to drugs
**Expected Result:** 502 response with error body containing upstream failure details (status, error message, endpoint attempted).
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-005: Path parameter substitution

**Precondition:** Config defines `/v2/spls/{SETID}` endpoint with slug `spl-detail`.
**Steps:**
1. Send `GET /api/cache/spl-detail?SETID=some-known-set-id` to drugs
2. Observe drugs fetches from `https://dailymed.nlm.nih.gov/dailymed/services/v2/spls/some-known-set-id.json`
3. Receive SPL detail data
**Expected Result:** 200 response with SPL detail for the given SETID. Cached in MongoDB keyed by slug + parameter values.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-006: Pagination with numeric limit

**Precondition:** Config defines `drugnames` with `pagination: 3` (fetch max 3 pages).
**Steps:**
1. Send `GET /api/cache/drugnames` to drugs
2. Observe drugs fetches pages 1, 2, 3 from upstream
3. Observe drugs does NOT fetch page 4+
**Expected Result:** 200 response with aggregated data from exactly 3 pages. Metadata shows `page_count: 3`.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-007: Pagination failure midway discards partial data

**Precondition:** Config defines `drugnames` with `pagination: all`. Upstream fails on page 3. Stale cache exists from previous successful fetch.
**Steps:**
1. Send `GET /api/cache/drugnames` to drugs
2. Observe drugs fetches page 1 (success), page 2 (success), page 3 (failure)
3. Observe drugs discards pages 1-2 partial data
4. Observe drugs returns stale cached data
**Expected Result:** 200 response with previous complete cached data. Partial fetch is not stored. Log entry indicates pagination failure.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-008: Unknown endpoint returns 404

**Precondition:** Service is running. No config entry for slug `nonexistent`.
**Steps:**
1. Send `GET /api/cache/nonexistent` to drugs
**Expected Result:** 404 response with error body: `{"error": "endpoint not configured", "slug": "nonexistent"}`.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-009: Invalid config prevents startup

**Precondition:** Config file has an entry missing required fields (e.g., no `base_url`).
**Steps:**
1. Attempt to start the service
**Expected Result:** Service fails to start. Log output identifies the invalid entry and which field is missing.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

## 4. Data Model

### EndpointConfig (YAML config file — in-memory after load)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| slug | string | Yes | Unique identifier used in consumer-facing URL (e.g., `drugnames`) |
| base_url | string | Yes | Upstream API base URL (e.g., `https://dailymed.nlm.nih.gov/dailymed/services`) |
| path | string | Yes | Endpoint path, may contain `{param}` placeholders (e.g., `/v2/drugnames` or `/v2/spls/{SETID}`) |
| format | string | Yes | Response format: `json` or `xml` |
| query_params | map[string]string | No | Default query parameters to include on every request |
| pagination | string or int | No | Pagination strategy: `"all"` to fetch every page, integer for max pages (default: `1` — no pagination) |
| page_param | string | No | Query parameter name for page number (default: `page`) |
| pagesize_param | string | No | Query parameter name for page size (default: `pagesize`) |
| pagesize | int | No | Number of items per page to request (default: `100`) |

### CachedResponse (MongoDB collection: `cached_responses`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| _id | ObjectID | Yes | MongoDB auto-generated ID |
| slug | string | Yes | Endpoint slug from config |
| params | map[string]string | No | Path parameter values used for this cached entry (e.g., `{"SETID": "abc-123"}`) |
| cache_key | string | Yes | Composite key: `slug` or `slug:param1=val1:param2=val2` for uniqueness |
| data | interface{} | Yes | Raw aggregated upstream response body |
| content_type | string | Yes | MIME type of the response (`application/json` or `application/xml`) |
| fetched_at | time.Time | Yes | Timestamp of last successful upstream fetch |
| source_url | string | Yes | Full upstream URL that was fetched |
| http_status | int | Yes | HTTP status code from upstream |
| page_count | int | Yes | Number of pages aggregated |
| created_at | time.Time | Yes | First cache creation time |
| updated_at | time.Time | Yes | Last update time |

### Relationships

- Each `EndpointConfig` maps to zero or more `CachedResponse` documents (one per unique set of path parameters)
- `cache_key` uniquely identifies a cached response for a given endpoint + parameter combination

## 5. API Contract

### GET /api/cache/{slug}

**Description:** Retrieve cached data for a configured upstream endpoint. If no cache exists, triggers an upstream fetch.

**Path Parameters:**
- `slug` (required) — The endpoint slug from config

**Query Parameters:**
- Any path parameters defined in the endpoint config (e.g., `SETID=abc-123`)

**Response (200 — cached or freshly fetched):**
```json
{
  "data": { ... },
  "meta": {
    "slug": "drugnames",
    "source_url": "https://dailymed.nlm.nih.gov/dailymed/services/v2/drugnames.json",
    "fetched_at": "2026-03-05T12:00:00Z",
    "page_count": 5,
    "stale": false
  }
}
```

**Response (200 — stale cache, upstream failed):**
```json
{
  "data": { ... },
  "meta": {
    "slug": "drugnames",
    "source_url": "https://dailymed.nlm.nih.gov/dailymed/services/v2/drugnames.json",
    "fetched_at": "2026-03-04T08:00:00Z",
    "page_count": 5,
    "stale": true,
    "stale_reason": "upstream returned 503"
  }
}
```

**Error Responses:**
- `404` — Endpoint slug not found in config: `{"error": "endpoint not configured", "slug": "nonexistent"}`
- `502` — Upstream failed and no cache exists: `{"error": "upstream unavailable", "slug": "drugnames", "upstream_status": 503, "message": "Service Unavailable"}`

## 6. UI Behavior

N/A — no UI. This is a backend API service.

## 7. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| Upstream returns empty response (200 but no data) | Store the empty response as valid cache. Return to consumer as-is. |
| Upstream returns XML when JSON configured | Store raw response as-is. Content type mismatch logged as warning. |
| Two consumers request same uncached endpoint simultaneously | First request triggers fetch; second should wait or also get the fetched result (no duplicate upstream calls if possible) |
| Path parameter contains special characters | URL-encode the parameter value before upstream call |
| Config file doesn't exist at startup | Fail to start with clear error: "config file not found at {path}" |
| Config has duplicate slugs | Fail to start with error identifying the duplicates |
| Upstream returns redirect (3xx) | Follow redirects (Go http.Client default behavior) |
| Very large aggregated response (100+ pages) | No size limit in v1. Log a warning if aggregated response exceeds 50MB. |

## 8. Dependencies

- MongoDB (running and accessible)
- Upstream APIs accessible from the network (DailyMed for validation)
- Go `net/http` for upstream requests
- Go MongoDB driver (`go.mongodb.org/mongo-driver`)
- YAML parser (`gopkg.in/yaml.v3`)

## 9. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-05 | 0.1.0 | calebdunn | Initial spec from /add:spec interview |

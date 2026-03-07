# Spec: FDA API Integration

**Version:** 0.1.0
**Created:** 2026-03-07
**PRD Reference:** docs/prd.md
**Status:** Complete

## 1. Overview

Add FDA openFDA drug API endpoints to the service via config, with generic fetcher enhancements to support offset-based (skip/limit) pagination and configurable JSON response parsing. FDA APIs use different pagination and response structure than DailyMed — the fetcher must handle both styles driven purely by config.

### User Story

As an internal microservice developer, I want cached access to FDA drug data (NDC codes, approvals, recalls, shortages, labels, adverse events), so that I can enrich drug information without directly calling FDA APIs.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | Config supports `pagination_style` field with values `page` (default) and `offset` | Must |
| AC-002 | When `pagination_style: offset`, fetcher sends `skip=N&limit=N` incrementing skip by pagesize each request | Must |
| AC-003 | Config supports `data_key` field (default: `data`) — fetcher extracts items from this key in JSON response | Must |
| AC-004 | Config supports `total_key` field (default: `metadata.total_pages`) — fetcher reads total from this dot-path to determine if more pages exist | Must |
| AC-005 | Existing DailyMed endpoints work unchanged with new defaults | Must |
| AC-006 | FDA Enforcement endpoint configured with daily prefetch (refresh cron + TTL) | Must |
| AC-007 | FDA Drug Shortages endpoint configured with daily prefetch | Must |
| AC-008 | FDA NDC endpoint configured for on-demand search by brand_name | Must |
| AC-009 | FDA Drugs@FDA endpoint configured for on-demand search by brand_name | Must |
| AC-010 | FDA Drug Labels endpoint configured for on-demand search | Must |
| AC-011 | FDA Adverse Events endpoint configured for on-demand search | Must |
| AC-012 | Offset pagination stops gracefully when skip exceeds API limit (e.g., FDA 25K cap) — no crash, partial data stored | Must |
| AC-013 | E2E tests validate all config.yaml endpoints against real APIs with minimal data (limit=1 or pagesize=1) | Must |
| AC-014 | README documents new config fields (pagination_style, data_key, total_key) with examples | Must |
| AC-015 | OpenAPI spec updated with FDA endpoint slugs and query parameters | Should |
| AC-016 | `total_key` supports dot-notation paths (e.g., `meta.results.total`) for nested JSON fields | Must |
| AC-017 | Offset pagination correctly calculates skip as `(page - 1) * pagesize` for each request | Must |

## 3. User Test Cases

### TC-001: Offset pagination fetches FDA data correctly

**Precondition:** Server running with FDA enforcement endpoint configured
**Steps:**
1. Request `GET /api/cache/fda-enforcement`
2. Fetcher uses skip/limit pagination against FDA API
3. Response cached in MongoDB
**Expected Result:** Returns enforcement recall data with correct item count
**Screenshot Checkpoint:** N/A (API only)
**Maps to:** TBD

### TC-002: On-demand FDA NDC search by brand name

**Precondition:** Server running with fda-ndc-by-name endpoint configured
**Steps:**
1. Request `GET /api/cache/fda-ndc-by-name?BRAND_NAME=aspirin`
2. Fetcher substitutes param into FDA search syntax: `search=brand_name:"aspirin"`
3. FDA returns matching NDC records
**Expected Result:** Returns NDC records matching "aspirin" with FDA response structure (results key)
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-003: Existing DailyMed endpoints still work

**Precondition:** Server running with both DailyMed and FDA endpoints
**Steps:**
1. Request `GET /api/cache/drugnames`
2. Request `GET /api/cache/spls-by-name?DRUGNAME=aspirin`
**Expected Result:** Both return correct DailyMed data — no regression from new config fields
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-004: E2E config validation against real APIs

**Precondition:** Network access to DailyMed and FDA APIs
**Steps:**
1. Run E2E test suite
2. Each configured endpoint is tested with minimal params (limit=1 or pagesize=1)
3. Validates: HTTP 200, response has expected data key, items are returned
**Expected Result:** All endpoints return valid data, confirming config correctness
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-005: Offset pagination handles skip limit gracefully

**Precondition:** Endpoint configured with `pagination_style: offset` and `pagination: all`
**Steps:**
1. Fetcher paginates through data using skip/limit
2. API returns error when skip exceeds its limit (e.g., 25K)
**Expected Result:** Fetcher stops pagination gracefully, stores data collected so far, logs warning
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-006: Configurable data_key extracts items correctly

**Precondition:** Endpoint with `data_key: results` (FDA style)
**Steps:**
1. Fetch from FDA API
2. Fetcher uses `data_key` to locate items in response
**Expected Result:** Items extracted from `results` key, not `data` key
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-007: Configurable total_key with dot-notation

**Precondition:** Endpoint with `total_key: meta.results.total` (FDA style)
**Steps:**
1. Fetch from FDA API
2. Fetcher traverses dot-path to read total count
3. Determines if more pages exist
**Expected Result:** Pagination continues until skip + limit >= total, then stops
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

## 4. Data Model

### Endpoint (config extension)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| pagination_style | string | No | `page` (default) or `offset` — controls pagination behavior |
| data_key | string | No | JSON key containing items array (default: `data`) |
| total_key | string | No | Dot-path to total count in response (default: `metadata.total_pages`) |

All existing fields remain unchanged. New fields have backward-compatible defaults.

### No MongoDB schema changes

FDA responses stored using the same CachedResponse structure — the fetcher normalizes data into the existing format.

## 5. API Contract

### GET /api/cache/fda-enforcement

**Description:** Prefetched FDA drug recall enforcement reports

**Response (200):**
```json
{
  "data": [...],
  "meta": {
    "slug": "fda-enforcement",
    "source_url": "https://api.fda.gov/drug/enforcement.json",
    "fetched_at": "2026-03-07T03:00:00Z",
    "page_count": 174,
    "stale": false
  }
}
```

### GET /api/cache/fda-ndc-by-name?BRAND_NAME={name}

**Description:** On-demand FDA NDC search by brand name

**Response (200):**
```json
{
  "data": [...],
  "meta": {
    "slug": "fda-ndc-by-name",
    "source_url": "https://api.fda.gov/drug/ndc.json",
    "fetched_at": "2026-03-07T15:30:00Z",
    "page_count": 1,
    "stale": false
  }
}
```

### GET /api/cache/fda-shortages

**Description:** Prefetched FDA drug shortage reports

### GET /api/cache/fda-drugsfda-by-name?BRAND_NAME={name}

**Description:** On-demand FDA drug approval search by brand name

### GET /api/cache/fda-labels-by-name?DRUG_NAME={name}

**Description:** On-demand FDA drug label search

### GET /api/cache/fda-events-by-drug?DRUG_NAME={name}

**Description:** On-demand FDA adverse event search

**Error Responses:**
- `404` — Slug not configured
- `502` — FDA API unavailable, no cache available

## 6. UI Behavior

N/A — API only

## 7. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| FDA API returns error at skip=25000 | Fetcher stops gracefully, stores data collected so far, logs warning |
| FDA API rate limited (429) | Fetcher returns error, stale cache served if available |
| data_key missing from response | Fetcher wraps entire response as single item (existing behavior) |
| total_key path doesn't exist in response | Fetcher assumes single page, stops after first request |
| Empty search results from FDA | Returns empty data array with valid meta |
| DailyMed endpoint with no new config fields | Defaults apply, behavior unchanged |
| Offset pagination with pagination: 3 | Fetches 3 pages worth of skip/limit requests |

## 8. Dependencies

- No new Go dependencies required
- FDA APIs are free, no API key needed
- FDA rate limit: ~240 requests/minute
- Network access to api.fda.gov required for E2E tests
- Existing DailyMed config must remain functional (backward compatibility)

## 9. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-07 | 0.1.0 | calebdunn | Initial spec from /add:spec interview |

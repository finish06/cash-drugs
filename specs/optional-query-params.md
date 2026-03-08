# Spec: Optional Query Parameters

**Version:** 0.1.0
**Created:** 2026-03-07
**PRD Reference:** docs/prd.md
**Status:** Complete

## 1. Overview

Endpoints configured with multiple `{PLACEHOLDER}` query parameters should treat them as optional — only parameters the caller provides are sent upstream. Unresolved placeholders (where the caller didn't supply a value) are silently omitted from the upstream request. Static query parameters (no placeholder) are always sent.

This enables a single endpoint config to support multiple search strategies. For example, an FDA NDC endpoint can accept `brand_name`, `generic_name`, or both — the caller chooses which to provide.

### User Story

As an internal microservice developer, I want to call a cached endpoint with only the search parameters I have, so that I don't need separate endpoint configs for every possible parameter combination.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | Query params with `{PLACEHOLDER}` values where the caller provides the placeholder are sent upstream with the resolved value | Must |
| AC-002 | Query params with `{PLACEHOLDER}` values where the caller does NOT provide the placeholder are omitted from the upstream request | Must |
| AC-003 | Static query params (no `{PLACEHOLDER}`) are always sent upstream regardless of caller-provided params | Must |
| AC-004 | When the caller provides ALL placeholder params, all query params are sent upstream | Must |
| AC-005 | When the caller provides NO placeholder params, only static query params are sent upstream | Must |
| AC-006 | Existing endpoints with single `{PLACEHOLDER}` params continue to work unchanged (backward compatible) | Must |
| AC-007 | Pagination params (skip/limit or page/pagesize) are unaffected by this change | Must |

## 3. User Test Cases

### TC-001: Partial placeholder resolution

**Precondition:** Endpoint configured with `brand_name: "{BRAND_NAME}"`, `generic_name: "{GENERIC_NAME}"`, `status: "active"`
**Steps:**
1. Request `GET /api/cache/test-endpoint?BRAND_NAME=Tylenol`
2. Fetcher builds upstream URL
**Expected Result:** Upstream receives `brand_name=Tylenol&status=active` — `generic_name` is omitted
**Screenshot Checkpoint:** N/A (API only)
**Maps to:** TestOptionalQueryParams_UnresolvedPlaceholdersSkipped

### TC-002: All placeholders resolved

**Precondition:** Endpoint configured with `brand_name: "{BRAND_NAME}"`, `generic_name: "{GENERIC_NAME}"`
**Steps:**
1. Request `GET /api/cache/test-endpoint?BRAND_NAME=Tylenol&GENERIC_NAME=Acetaminophen`
2. Fetcher builds upstream URL
**Expected Result:** Upstream receives `brand_name=Tylenol&generic_name=Acetaminophen`
**Screenshot Checkpoint:** N/A (API only)
**Maps to:** TestOptionalQueryParams_AllPlaceholdersResolved

### TC-003: No placeholders provided

**Precondition:** Endpoint configured with `brand_name: "{BRAND_NAME}"`, `generic_name: "{GENERIC_NAME}"`, `status: "active"`
**Steps:**
1. Request `GET /api/cache/test-endpoint` (no query params)
2. Fetcher builds upstream URL
**Expected Result:** Upstream receives only `status=active` — both placeholder params omitted
**Screenshot Checkpoint:** N/A (API only)
**Maps to:** TestOptionalQueryParams_NoPlaceholdersProvided

## 4. Data Model

No new data entities. This is a behavioral change to the URL builder in the fetcher.

### Config Schema (unchanged)

The existing `query_params` map in `config.yaml` already supports `{PLACEHOLDER}` syntax. No config schema changes needed — the behavior change is in how unresolved placeholders are handled.

```yaml
# Example: multiple optional search params
- slug: fda-ndc
  query_params:
    brand_name: "{BRAND_NAME}"
    generic_name: "{GENERIC_NAME}"
    search: "finished:true"  # static — always sent
```

## 5. API Contract

No new endpoints. Existing `GET /api/cache/{slug}` behavior is enhanced — callers can now provide a subset of configured placeholder params and only the provided ones are forwarded upstream.

## 6. UI Behavior

N/A — no UI component.

## 7. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| All query params are placeholders, none provided | No query params sent (except pagination if configured) |
| Mixed static and placeholder params | Static always sent, placeholders only when resolved |
| Placeholder in the middle of a value (e.g., `prefix-{VAL}-suffix`) | If any `{...}` remains after substitution, the param is skipped |
| Empty string provided for a placeholder | Param is sent with empty value (not a placeholder) |
| Pagination params use placeholders | Pagination params are set separately and are unaffected |

## 8. Dependencies

- `config.SubstitutePathParams` — existing function, no changes needed
- `buildURL` in `internal/upstream/fetcher.go` — modified to skip unresolved placeholders

## 9. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-07 | 0.1.0 | calebdunn | Initial spec (retroactive — feature already implemented) |

# Spec: Response Normalization for Nested Upstream Data

**Version:** 0.1.0
**Created:** 2026-03-16
**PRD Reference:** docs/prd.md
**Status:** Complete

## 1. Overview

Some upstream APIs (especially RxNorm) return data that, even after `data_key` extraction, contains nested structures. For example, `rxnorm-all-related` returns an array of `conceptGroup` objects, each containing an inner `conceptProperties` array. Consumers currently need per-endpoint parsing logic to extract the actual items. This feature adds an optional `flatten` config field that post-processes extracted data into a flat array before caching, so all consumers receive a consistent `{"data": [...flat items...]}` response.

### User Story

As a **downstream consumer**, I want all cache responses to contain a flat data array, so I don't need per-endpoint response parsing logic.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | New optional config field `flatten` (boolean, default `false`) is supported on endpoint configuration | Must |
| AC-002 | When `flatten: true`, the fetcher post-processes the `data_key`-extracted data to flatten nested arrays into a single flat array | Must |
| AC-003 | For `rxnorm-all-related`: `conceptGroup[*].conceptProperties[*]` is flattened into a single array, with the `tty` field from each `conceptGroup` preserved on every flattened item | Must |
| AC-004 | For `rxnorm-approximate-match`: data is already flat after `data_key` extraction — enabling `flatten` is a no-op (no error, no change) | Must |
| AC-005 | Endpoints without `flatten` (or `flatten: false`) are completely unchanged — backward compatible | Must |
| AC-006 | Flattening happens at fetch time (before cache storage), not at response time — cached data is already flat | Must |
| AC-007 | `results_count` in the API response `meta` reflects the flattened item count | Must |
| AC-008 | Config parsing accepts the `flatten` field and exposes it on the `config.Endpoint` struct | Must |

## 3. User Test Cases

### TC-001: Flattened rxnorm-all-related response

**Precondition:** `rxnorm-all-related` configured with `flatten: true`
**Steps:**
1. Send `GET /api/cache/rxnorm-all-related?RXCUI=197381`
2. Inspect the `data` array in the response
**Expected Result:** `data` is a flat array of concept objects. Each object has fields from `conceptProperties` (e.g. `rxcui`, `name`, `synonym`) plus the `tty` field from its parent `conceptGroup`.
**Maps to:** AC-003, AC-006, AC-007

### TC-002: Non-flattened endpoint unchanged

**Precondition:** `fda-ndc` configured without `flatten` (default false)
**Steps:**
1. Send `GET /api/cache/fda-ndc?BRAND_NAME=aspirin`
2. Inspect the response
**Expected Result:** Response is identical to pre-feature behavior — no flattening applied.
**Maps to:** AC-005

### TC-003: Flatten on already-flat data is no-op

**Precondition:** `rxnorm-approximate-match` configured with `flatten: true`
**Steps:**
1. Send `GET /api/cache/rxnorm-approximate-match?DRUG_NAME=aspirin`
2. Inspect the `data` array
**Expected Result:** `data` is the same flat array as without `flatten`. No error, no structural change.
**Maps to:** AC-004

### TC-004: results_count reflects flattened count

**Precondition:** `rxnorm-all-related` with `flatten: true`, upstream returns 5 concept groups with varying numbers of properties
**Steps:**
1. Fetch `GET /api/cache/rxnorm-all-related?RXCUI=197381`
2. Check `meta.results_count`
**Expected Result:** `results_count` equals the total number of individual concept properties across all groups (the flat count), not the number of concept groups.
**Maps to:** AC-007

### TC-005: Empty conceptGroups handled gracefully

**Precondition:** `rxnorm-all-related` with `flatten: true`
**Steps:**
1. Fetch with an RXCUI that returns concept groups but some have no `conceptProperties`
2. Inspect response
**Expected Result:** Empty groups are silently skipped. `data` contains only items from groups that had properties. No error.
**Maps to:** AC-003

## 4. Data Model

### Config Extension

New field on `config.Endpoint`:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `flatten` | `bool` | `false` | When true, post-process extracted data to flatten nested arrays |

### Flatten Behavior

The flattener operates on the data after `data_key` extraction:

1. If the extracted data is not an array, return it unchanged.
2. Walk each element of the array.
3. If an element is an object containing an array field, extract items from that inner array and promote them to the top level.
4. For `conceptGroup` specifically: copy the `tty` field from the parent group onto each child `conceptProperties` item.
5. If an element is a primitive or has no inner arrays, include it as-is.

**Example transformation (rxnorm-all-related):**

Before (after `data_key` extraction):
```json
[
  {
    "tty": "BN",
    "conceptProperties": [
      {"rxcui": "12345", "name": "Lipitor", "synonym": "..."}
    ]
  },
  {
    "tty": "IN",
    "conceptProperties": [
      {"rxcui": "67890", "name": "Atorvastatin", "synonym": "..."},
      {"rxcui": "67891", "name": "Atorvastatin Calcium", "synonym": "..."}
    ]
  }
]
```

After flattening:
```json
[
  {"rxcui": "12345", "name": "Lipitor", "synonym": "...", "tty": "BN"},
  {"rxcui": "67890", "name": "Atorvastatin", "synonym": "...", "tty": "IN"},
  {"rxcui": "67891", "name": "Atorvastatin Calcium", "synonym": "...", "tty": "IN"}
]
```

## 5. API Contract

No new endpoints. Existing `GET /api/cache/{slug}` behavior changes only for endpoints with `flatten: true`:

- `data` array contains flattened items instead of nested groups
- `meta.results_count` reflects flattened count
- All other response fields unchanged

### Config YAML

```yaml
- slug: rxnorm-all-related
  base_url: https://rxnav.nlm.nih.gov
  path: /REST/rxcui/{RXCUI}/allrelated.json
  format: json
  data_key: allRelatedGroup.conceptGroup
  flatten: true
  ttl: "336h"
```

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| `conceptGroup` array is empty | Return empty `data` array, `results_count: 0` |
| A `conceptGroup` has no `conceptProperties` field | Skip that group silently, continue with remaining groups |
| A `conceptGroup` has `conceptProperties: null` | Treat as empty — skip that group |
| A `conceptGroup` has `conceptProperties: []` (empty array) | Skip that group (no items to flatten) |
| Mixed term types in flattened output | Each item carries its `tty` from the parent group — consumer can filter by `tty` |
| `flatten: true` on an endpoint with no nested arrays | No-op — data passes through unchanged |
| `flatten: true` on an endpoint with `format: raw` | Ignored — flattening only applies to JSON format data |
| `data_key` extraction returns a non-array (e.g. a single object) | No flattening — return as-is |
| Upstream returns unexpected structure (no `conceptProperties` key) | Graceful degradation — items without recognizable inner arrays pass through unchanged |

## 7. Dependencies

- `internal/config` — add `Flatten` field to `Endpoint` struct, parse from YAML
- `internal/upstream` — apply flatten post-processing after `data_key` extraction, before returning result
- No new external dependencies

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-16 | 0.1.0 | calebdunn | Initial spec |

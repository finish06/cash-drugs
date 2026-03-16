# Spec: Parameterized Query Warmup

**Version:** 0.1.0
**Created:** 2026-03-16
**PRD Reference:** docs/prd.md
**Status:** Draft

## 1. Overview

Extend the warmup system to pre-cache parameterized endpoint queries using a `warmup-queries.yaml` config file. The existing warmup handles scheduled (non-parameterized) endpoints like `drugnames` and `fda-enforcement`. This spec adds the ability to warm parameterized endpoints (`fda-ndc?GENERIC_NAME=METFORMIN`, `rxnorm-find-drug?DRUG_NAME=atorvastatin`, etc.) from a curated list of the top 100 most prescribed drugs.

### User Story

As a **downstream service consumer**, I want the most common drug lookups pre-cached on startup, so that the first request for popular drugs like metformin or atorvastatin returns instantly from cache instead of waiting for an upstream fetch.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | A `warmup-queries.yaml` file defines parameterized queries grouped by slug | Must |
| AC-002 | Each entry under a slug specifies query parameter key-value pairs (e.g., `GENERIC_NAME: METFORMIN`) | Must |
| AC-003 | `POST /api/warmup` with no body warms both scheduled endpoints AND all parameterized queries from `warmup-queries.yaml` | Must |
| AC-004 | `POST /api/warmup` with `{"slugs": ["fda-ndc"]}` warms scheduled endpoint (if applicable) AND parameterized queries for that slug from `warmup-queries.yaml` | Must |
| AC-005 | `POST /api/warmup` with `{"slugs": ["fda-ndc"], "skip_queries": true}` warms only the scheduled endpoint, skipping parameterized queries | Should |
| AC-006 | Parameterized warmup queries run concurrently with a configurable concurrency cap (default: 5) to avoid overwhelming upstream APIs | Must |
| AC-007 | `GET /ready` progress includes parameterized queries in the total count (e.g., `"progress": "12/196"`) | Must |
| AC-008 | Parameterized warmup happens AFTER scheduled endpoint warmup completes (scheduled data may be needed by consumers first) | Must |
| AC-009 | Failed parameterized warmup queries are logged but do not block overall readiness — `/ready` transitions to 200 even if some queries fail | Must |
| AC-010 | Warmup queries path is configurable via `WARMUP_QUERIES_PATH` env var (default: `warmup-queries.yaml`) | Should |
| AC-011 | If `warmup-queries.yaml` doesn't exist, parameterized warmup is silently skipped — no error, no crash | Must |
| AC-012 | Duplicate queries in `warmup-queries.yaml` are deduplicated (same slug + same params = one fetch) | Should |
| AC-013 | Prometheus counter `cashdrugs_warmup_queries_total` with labels `slug` and `result` (success/error) tracks warmup query outcomes | Should |
| AC-014 | Prometheus gauge `cashdrugs_warmup_queries_pending` tracks how many queries are still in progress | Should |
| AC-015 | Warmup respects circuit breaker state — if a slug's circuit is open, skip that slug's queries and log a warning | Must |

## 3. User Test Cases

### TC-001: Full warmup includes parameterized queries

**Precondition:** Service is starting. `warmup-queries.yaml` exists with 196 queries across 4 slugs.
**Steps:**
1. Start the service
2. Poll `GET /ready` until 200
**Expected Result:** Progress counts up through scheduled endpoints (5), then parameterized queries (196). Total progress shows `"progress": "N/201"`. All 196 queries are cached in MongoDB + LRU.
**Maps to:** TBD

### TC-002: Warmup specific slug includes its parameterized queries

**Precondition:** Service is running. Cache is empty for `fda-ndc`.
**Steps:**
1. `POST /api/warmup` with `{"slugs": ["fda-ndc"]}`
2. Poll `GET /ready`
**Expected Result:** Warms `fda-ndc` scheduled endpoint (if any) plus all 86 `fda-ndc` queries from `warmup-queries.yaml`. Progress reflects total for that slug.
**Maps to:** TBD

### TC-003: Missing warmup-queries.yaml is not an error

**Precondition:** Service is starting. No `warmup-queries.yaml` file exists.
**Steps:**
1. Start the service
2. Wait for ready
**Expected Result:** Only scheduled endpoints warm. No error logs about missing file. `/ready` returns 200 after scheduled warmup completes.
**Maps to:** TBD

### TC-004: Failed queries don't block readiness

**Precondition:** Service is starting. RxNorm API is unreachable (circuit open).
**Steps:**
1. Start the service with `warmup-queries.yaml` containing `rxnorm-find-drug` queries
2. Poll `GET /ready`
**Expected Result:** Scheduled endpoints warm normally. RxNorm queries are skipped (circuit open). Log shows warnings. `/ready` transitions to 200 despite skipped queries.
**Maps to:** TBD

### TC-005: Concurrent query execution respects cap

**Precondition:** Service is running. 86 `fda-ndc` queries in warmup file.
**Steps:**
1. Trigger warmup
2. Monitor upstream request rate
**Expected Result:** At most 5 concurrent upstream requests at any time (default cap). Queries complete in ~17 batches of 5.
**Maps to:** TBD

### TC-006: skip_queries flag skips parameterized warmup

**Precondition:** Service is running. `warmup-queries.yaml` exists.
**Steps:**
1. `POST /api/warmup` with `{"slugs": ["fda-ndc"], "skip_queries": true}`
2. Check cache for parameterized entries
**Expected Result:** Only the scheduled `fda-ndc` bulk endpoint is warmed. No parameterized queries executed. Cache has no entries for individual drug lookups.
**Maps to:** TBD

## 4. Data Model

### warmup-queries.yaml Schema

```yaml
# Top-level keys are endpoint slugs
fda-ndc:
  - GENERIC_NAME: METFORMIN        # Each entry is a map of param: value
  - GENERIC_NAME: LISINOPRIL
  - BRAND_NAME: ASPIRIN            # Different param keys allowed

rxnorm-find-drug:
  - DRUG_NAME: metformin
  - DRUG_NAME: lisinopril
```

### WarmupQuery (in-memory)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| Slug | string | Yes | Endpoint slug from config |
| Params | map[string]string | Yes | Query parameter key-value pairs |

### WarmupConfig (parsed from YAML)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| Queries | map[string][]map[string]string | Yes | Slug → list of param maps |

## 5. API Contract

### POST /api/warmup (updated)

**Request (with queries):**
```json
{
  "slugs": ["fda-ndc", "rxnorm-find-drug"],
  "skip_queries": false
}
```

**Request (skip parameterized):**
```json
{
  "slugs": ["fda-ndc"],
  "skip_queries": true
}
```

**Response (202 Accepted):**
```json
{
  "status": "accepted",
  "warming": 5,
  "warming_queries": 136
}
```

### GET /ready (updated progress)

**During warmup (503):**
```json
{
  "status": "warming",
  "progress": "12/201",
  "phase": "queries"
}
```

Phase values: `"scheduled"` (bulk endpoints), `"queries"` (parameterized), `"ready"`.

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| warmup-queries.yaml missing | Skip parameterized warmup silently |
| warmup-queries.yaml malformed YAML | Log parse error, skip parameterized warmup |
| Slug in warmup-queries.yaml not in config.yaml | Log warning, skip that slug's queries |
| Empty query list for a slug | Skip that slug, no error |
| Same query appears twice (duplicate) | Deduplicate, fetch once |
| Upstream rate-limits during bulk warmup | Circuit breaker handles it — skip remaining queries for that slug |
| Service restart mid-warmup | Warmup restarts from scratch (in-memory state) |
| warmup-queries.yaml updated while running | Changes picked up on next warmup trigger (file re-read each time) |
| 500+ queries in file | Concurrency cap (5) prevents overwhelming upstream; warmup takes longer but completes |

## 7. Dependencies

- Existing `POST /api/warmup` endpoint (specs/readiness-warmup.md)
- Existing `GET /ready` endpoint (specs/readiness-warmup.md)
- Existing circuit breaker system (specs/upstream-resilience.md)
- `warmup-queries.yaml` file (already created with top 100 drugs)
- `gopkg.in/yaml.v3` (already a dependency)

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-16 | 0.1.0 | calebdunn | Initial spec from client enhancement feedback |

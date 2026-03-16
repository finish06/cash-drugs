# Spec: MongoDB Query Optimization

**Version:** 0.1.0
**Created:** 2026-03-15
**PRD Reference:** docs/prd.md (M10)
**Status:** Complete

## 1. Overview

Replace regex-based `cache_key` matching with indexed exact-match queries for multi-page response lookups. Currently, `buildRegexFilter()` in `internal/cache/mongo.go` uses `$regex: "^{key}(:|$)"` to find a cache key and its page variants (e.g., `drugnames` matches `drugnames`, `drugnames:page:1`, `drugnames:page:2`). MongoDB cannot efficiently use the `idx_cache_key` index for regex patterns, resulting in collection scans proportional to document count. This spec adds a `base_key` field to every document, creates a compound index on it, and switches all lookups from regex to `$eq` matching on `base_key`.

### User Story

As an **operator**, I want MongoDB cache lookups to use indexed exact-match queries instead of regex scans, so that response latency remains predictable as the collection grows and CPU load from regex evaluation is eliminated.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | A new `base_key` field is added to the `CachedResponse` model (`bson:"base_key"`) containing the cache key without any `:page:N` suffix | Must |
| AC-002 | `ensureIndexes()` creates a new compound index `idx_base_key_page` on `{base_key: 1, page: 1}` | Must |
| AC-003 | `Get()` uses `bson.M{"base_key": cacheKey}` with sort `{page: 1}` instead of `buildRegexFilter()` | Must |
| AC-004 | `FetchedAt()` uses `bson.M{"base_key": cacheKey}` instead of `buildRegexFilter()` | Must |
| AC-005 | `Upsert()` for multi-page responses sets `base_key` to `resp.CacheKey` on each page document | Must |
| AC-006 | `Upsert()` for single-document responses sets `base_key` equal to `cache_key` | Must |
| AC-007 | `Upsert()` stale page deletion uses `bson.M{"base_key": cacheKey, "page": bson.M{"$gt": pageCount}}` instead of regex | Must |
| AC-008 | A migration function `backfillBaseKey()` populates `base_key` for all existing documents: documents with `cache_key` containing `:page:` get the prefix before `:page:` as `base_key`; documents without `:page:` get `cache_key` as `base_key` | Must |
| AC-009 | `backfillBaseKey()` is called once during `NewMongoRepository()` startup, is idempotent (skips documents where `base_key` already exists), and logs the number of documents updated | Must |
| AC-010 | The old `buildRegexFilter()` function and `escapeRegex()` helper are removed after migration is in place | Should |
| AC-011 | The original `idx_cache_key` unique index is retained (existing upserts depend on it) | Must |
| AC-012 | All existing unit and integration tests pass without modification (backward compatible) | Must |
| AC-013 | A benchmark test demonstrates that `Get()` with the new index is faster than regex for a collection with 500+ documents | Should |

## 3. User Test Cases

### TC-001: Multi-page lookup uses exact match

**Precondition:** MongoDB contains documents for `drugnames:page:1`, `drugnames:page:2`, `drugnames:page:3` with `base_key: "drugnames"`
**Steps:**
1. Call `repo.Get("drugnames")`
2. Inspect MongoDB query log or explain plan
**Expected Result:** Query uses `{base_key: "drugnames"}` filter. Returns combined data from all 3 pages in order.
**Maps to:** AC-003

### TC-002: Single-document lookup uses exact match

**Precondition:** MongoDB contains one document with `cache_key: "fda-ndc:NDC=12345"` and `base_key: "fda-ndc:NDC=12345"`
**Steps:**
1. Call `repo.Get("fda-ndc:NDC=12345")`
**Expected Result:** Returns the single document. No regex in the query.
**Maps to:** AC-003, AC-006

### TC-003: Migration backfills existing documents

**Precondition:** MongoDB contains 10 documents: 3 with `:page:N` suffixes, 7 single-document entries. None have `base_key` set.
**Steps:**
1. Call `backfillBaseKey()` or restart the service
**Expected Result:** All 10 documents now have `base_key` set correctly. Log shows "backfilled base_key for 10 documents".
**Maps to:** AC-008, AC-009

### TC-004: Stale page cleanup uses exact match

**Precondition:** MongoDB contains `drugnames:page:1` through `drugnames:page:5` with `base_key: "drugnames"`
**Steps:**
1. Upsert a new response for `drugnames` with only 3 pages
**Expected Result:** Pages 4 and 5 are deleted using `{base_key: "drugnames", page: {$gt: 3}}`. Pages 1-3 are updated.
**Maps to:** AC-007

### TC-005: Idempotent migration on restart

**Precondition:** All documents already have `base_key` populated
**Steps:**
1. Restart the service (triggers `backfillBaseKey()` again)
**Expected Result:** No documents are modified. Log shows "backfilled base_key for 0 documents" or skips silently.
**Maps to:** AC-009

## 4. Data Model

### Modified: `CachedResponse` (`internal/model/cache.go`)

| Field | Type | BSON Tag | Description |
|-------|------|----------|-------------|
| `BaseKey` | `string` | `base_key` | Cache key without `:page:N` suffix. For single docs, equals `CacheKey`. For page docs, equals the parent cache key. |

### New Index: `idx_base_key_page`

```
{base_key: 1, page: 1}
```

Non-unique compound index. Supports both multi-page lookups (`base_key` only) and ordered page retrieval (`base_key` + `page` sort).

### Existing Index Retained: `idx_cache_key`

```
{cache_key: 1}  (unique)
```

Still required for upsert filter uniqueness on individual page documents.

## 5. API Contract

No API changes. This is a storage-layer optimization transparent to consumers.

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| Document has `base_key` already set during migration | Skip it (idempotent) |
| `cache_key` contains multiple `:page:` substrings (e.g., `some:page:key:page:2`) | Split on last `:page:` occurrence to extract the base |
| Empty collection during migration | No-op, log "0 documents" |
| Migration fails midway (e.g., MongoDB disconnects) | Partial update is safe — next startup will resume. Already-set documents are skipped. |
| `base_key` index creation fails (e.g., insufficient permissions) | Return error from `NewMongoRepository()`, same as existing index failure path |
| Cache key with special regex characters (e.g., `fda-ndc:NDC=12345-6789`) | No longer relevant since regex is eliminated; exact match handles all characters |

## 7. Dependencies

- `internal/model/cache.go` — add `BaseKey` field
- `internal/cache/mongo.go` — modify `Get`, `Upsert`, `FetchedAt`, `ensureIndexes`, add `backfillBaseKey`
- `internal/cache/repository.go` — no interface change (Get/Upsert signatures unchanged)
- Existing MongoDB data must be migrated (backfill runs at startup)

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-15 | 0.1.0 | calebdunn | Initial spec for M10 |

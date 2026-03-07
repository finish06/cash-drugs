# Spec: MongoDB Persistence

**Version:** 0.1.0
**Created:** 2026-03-05
**PRD Reference:** docs/prd.md
**Status:** Complete

## 1. Overview

Implement the MongoDB persistence layer for drugs. This replaces the in-memory cache stub with a real MongoDB-backed `cache.Repository` implementation using the official Go MongoDB driver. Includes connection management, automatic index creation, Docker Compose setup for local development, and a deep health check endpoint. The connection string is sourced from the `MONGO_URI` environment variable with fallback to the YAML config file.

### User Story

As an internal microservice developer, I want cached upstream API responses to persist in MongoDB, so that cached data survives service restarts and can be shared across multiple service instances.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | A `MongoRepository` struct implements the `cache.Repository` interface (`Get`, `Upsert`) using the official Go MongoDB driver | Must |
| AC-002 | `Get(cacheKey)` returns the matching `CachedResponse` document from the `cached_responses` collection, or `nil` if not found | Must |
| AC-003 | `Upsert(resp)` inserts a new document or updates an existing one matched by `cache_key`, setting `created_at` on insert and `updated_at` on every write | Must |
| AC-004 | Connection string is read from `MONGO_URI` env var first; if not set, falls back to `database.uri` field in config.yaml | Must |
| AC-005 | If neither `MONGO_URI` nor config fallback is set, the service fails to start with a clear error message | Must |
| AC-006 | The service pings MongoDB on startup and fails fast with a clear error if the database is unreachable | Must |
| AC-007 | On startup, the service creates a unique index on `cache_key` in the `cached_responses` collection | Must |
| AC-008 | `GET /health` pings MongoDB and returns `{"status":"ok","db":"connected"}` (200) when healthy, or `{"status":"degraded","db":"disconnected"}` (503) when MongoDB is unreachable | Must |
| AC-009 | A `docker-compose.yml` defines MongoDB and drugs services for local development, with MongoDB accessible on default port and drugs on port 8080 | Must |
| AC-010 | A `Dockerfile` builds the drugs Go binary in a multi-stage build (build + scratch/distroless) | Should |
| AC-011 | MongoDB database name defaults to `drugs` and collection name is fixed as `cached_responses` | Must |
| AC-012 | The `MongoRepository` uses a configurable context timeout for all database operations (default: 5 seconds) | Should |

## 3. User Test Cases

### TC-001: Service starts and connects to MongoDB

**Precondition:** MongoDB is running via Docker Compose. `MONGO_URI` is set.
**Steps:**
1. Start the drugs service
2. Observe startup logs
**Expected Result:** Log line: "Connected to MongoDB at {uri}". Service starts successfully on port 8080.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-002: Upsert stores document in MongoDB

**Precondition:** Service is running, connected to MongoDB.
**Steps:**
1. Send `GET /api/cache/drugnames` (triggers upstream fetch)
2. Query MongoDB `cached_responses` collection directly
**Expected Result:** Document exists with `cache_key: "drugnames"`, `slug: "drugnames"`, `fetched_at` timestamp, `data` field populated, `created_at` and `updated_at` set.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-003: Get retrieves cached document

**Precondition:** TC-002 has run — document exists in MongoDB.
**Steps:**
1. Send `GET /api/cache/drugnames` again
2. Observe no upstream call is made
**Expected Result:** 200 response with data from MongoDB. `stale: false`.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-004: Upsert updates existing document

**Precondition:** Document exists for `drugnames` in MongoDB.
**Steps:**
1. Trigger a fresh upstream fetch (e.g., via `_force=true`)
2. Query MongoDB directly
**Expected Result:** Same document (matched by `cache_key`), `updated_at` is newer than `created_at`, `fetched_at` reflects the new fetch.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-005: Health check reports MongoDB status

**Precondition:** Service is running with MongoDB connected.
**Steps:**
1. Send `GET /health`
**Expected Result:** `200 {"status":"ok","db":"connected"}`
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-006: Health check reports degraded when MongoDB is down

**Precondition:** Service is running. MongoDB is stopped after startup.
**Steps:**
1. Stop the MongoDB container
2. Send `GET /health`
**Expected Result:** `503 {"status":"degraded","db":"disconnected"}`
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-007: Service fails to start without MongoDB

**Precondition:** MongoDB is not running. `MONGO_URI` points to unreachable host.
**Steps:**
1. Attempt to start the drugs service
**Expected Result:** Service exits with error: "failed to connect to MongoDB: {details}"
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-008: Service fails to start without connection string

**Precondition:** `MONGO_URI` is not set. No `database.uri` in config.yaml.
**Steps:**
1. Attempt to start the drugs service
**Expected Result:** Service exits with error: "MongoDB URI not configured: set MONGO_URI or database.uri in config"
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-009: Unique index prevents duplicate cache keys

**Precondition:** Service is running with MongoDB.
**Steps:**
1. Insert two documents with the same `cache_key` directly via MongoDB client
**Expected Result:** Second insert fails with duplicate key error. The unique index enforces cache_key uniqueness.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-010: Data persists across service restarts

**Precondition:** TC-002 has run — cached data exists in MongoDB.
**Steps:**
1. Stop the drugs service
2. Restart the drugs service
3. Send `GET /api/cache/drugnames`
**Expected Result:** 200 response with previously cached data. No upstream fetch needed.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

## 4. Data Model

### Config Extension (YAML config file)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| database.uri | string | No | MongoDB connection string fallback (e.g., `mongodb://localhost:27017/drugs`). Overridden by `MONGO_URI` env var. |

### CachedResponse (MongoDB collection: `cached_responses`)

Unchanged from `specs/upstream-api-config.md` — same schema, same `bson` tags. This spec implements the persistence of that model.

### Indexes

| Collection | Index | Fields | Type | Purpose |
|------------|-------|--------|------|---------|
| cached_responses | idx_cache_key | `cache_key` | Unique | Fast lookup and upsert correctness |

## 5. API Contract

### GET /health (updated)

**Description:** Reports service health including MongoDB connectivity.

**Response (200 — healthy):**
```json
{
  "status": "ok",
  "db": "connected"
}
```

**Response (503 — degraded):**
```json
{
  "status": "degraded",
  "db": "disconnected"
}
```

## 6. UI Behavior

N/A — no UI. This is a backend API service.

## 7. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| MongoDB connection drops after startup | Operations return errors; health check returns 503. Service stays running. |
| MongoDB reconnects after transient failure | Operations resume normally. Health check returns 200. (Driver handles reconnection.) |
| Very large document (>16MB BSON limit) | MongoDB rejects the write. Error returned to caller. Log warning. |
| Concurrent upserts for same cache_key | MongoDB's atomic upsert ensures last-write-wins. No corruption. |
| Database name not in URI | Default to `drugs` database |

## 8. Dependencies

- Go MongoDB driver (`go.mongodb.org/mongo-driver/v2`)
- Docker and Docker Compose (for local dev)
- Existing `cache.Repository` interface (`internal/cache/repository.go`)
- Existing `model.CachedResponse` struct (`internal/model/cache.go`)

## 9. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-05 | 0.1.0 | calebdunn | Initial spec from /add:spec interview |

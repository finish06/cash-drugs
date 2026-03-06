# Implementation Plan: MongoDB Persistence

**Spec:** specs/mongodb-persistence.md
**Created:** 2026-03-05
**Status:** Active

## Task Breakdown

### Phase 1: Config Extension (AC-004, AC-005)

1. **Extend config loader for database URI**
   - Add `Database` struct with `URI` field to config file schema
   - Add `ResolveMongoURI()` function: check `MONGO_URI` env var, fallback to config
   - Error if neither is set
   - Files: `internal/config/loader.go`

### Phase 2: MongoDB Repository (AC-001, AC-002, AC-003, AC-011, AC-012)

2. **Implement MongoRepository**
   - `NewMongoRepository(uri string, timeout time.Duration)` — connect, ping, ensure indexes
   - `Get(cacheKey)` — find one by cache_key, return nil if not found
   - `Upsert(resp)` — upsert by cache_key with `$set` + `$setOnInsert` for created_at
   - `Ping()` — health check ping
   - `Close()` — disconnect
   - Files: `internal/cache/mongo.go`

### Phase 3: Index Management (AC-007)

3. **Auto-create indexes at startup**
   - `EnsureIndexes()` called from constructor
   - Unique index on `cache_key`
   - Files: `internal/cache/mongo.go`

### Phase 4: Health Check (AC-008)

4. **Deep health check handler**
   - Ping MongoDB, return status JSON
   - 200 when connected, 503 when disconnected
   - Files: `internal/handler/health.go`

### Phase 5: Server Integration (AC-006)

5. **Update server entrypoint**
   - Resolve MongoDB URI from config
   - Create MongoRepository (fail fast on connection error)
   - Wire health handler with DB ping
   - Replace memory repo
   - Files: `cmd/server/main.go`

### Phase 6: Docker Compose (AC-009, AC-010)

6. **Docker Compose + Dockerfile**
   - Multi-stage Dockerfile
   - docker-compose.yml with MongoDB + drugs
   - Files: `Dockerfile`, `docker-compose.yml`

## Spec Traceability

| Task | ACs Covered |
|------|-------------|
| 1. Config extension | AC-004, AC-005 |
| 2. MongoRepository | AC-001, AC-002, AC-003, AC-011, AC-012 |
| 3. Index management | AC-007 |
| 4. Health check | AC-008 |
| 5. Server integration | AC-006 |
| 6. Docker Compose | AC-009, AC-010 |

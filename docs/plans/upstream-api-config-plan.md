# Implementation Plan: Upstream API Configuration

**Spec:** specs/upstream-api-config.md
**Created:** 2026-03-05
**Status:** Active

## Task Breakdown

### Phase 1: Data Models & Config Loading (AC-001, AC-002, AC-003, AC-012)

1. **Define data models** (`internal/model/`)
   - `EndpointConfig` struct matching YAML schema
   - `CachedResponse` struct matching MongoDB schema
   - API response envelope (`data` + `meta`)
   - Files: `internal/model/config.go`, `internal/model/cache.go`, `internal/model/response.go`

2. **Config loader** (`internal/config/`)
   - Load and parse YAML config file
   - Validate required fields (slug, base_url, path, format)
   - Detect duplicate slugs
   - Return typed config or error
   - Files: `internal/config/loader.go`

### Phase 2: Cache Layer (AC-007, AC-008)

3. **MongoDB cache repository** (`internal/cache/`)
   - Connect to MongoDB
   - Upsert cached response by cache_key
   - Find cached response by cache_key
   - Build cache_key from slug + params
   - Files: `internal/cache/repository.go`

### Phase 3: Upstream Fetcher (AC-004, AC-005, AC-006, AC-003, AC-011)

4. **Upstream HTTP client** (`internal/upstream/`)
   - Build upstream URL from config + path params
   - Fetch single page
   - Auto-paginate: walk pages and aggregate results
   - Respect pagination limit (numeric or "all")
   - Handle pagination failure midway (discard partial)
   - Files: `internal/upstream/fetcher.go`

### Phase 4: HTTP Handlers (AC-008, AC-009, AC-010, AC-013, AC-014)

5. **Cache handler** (`internal/handler/`)
   - `GET /api/cache/{slug}` route
   - Extract slug and query params
   - Lookup config by slug (404 if not found)
   - Check cache → return if exists
   - Fetch upstream → cache → return
   - Fallback to stale cache on upstream error
   - Return 502 if no cache and upstream fails
   - Files: `internal/handler/cache.go`

### Phase 5: Server Entrypoint (AC-001, AC-012, AC-015)

6. **Main server** (`cmd/server/`)
   - Load config at startup (fail fast on invalid)
   - Initialize MongoDB connection
   - Register routes
   - Seed config with DailyMed endpoints
   - Files: `cmd/server/main.go`

### Phase 6: Seed Config (AC-015)

7. **Seed YAML config**
   - DailyMed `/v2/drugnames` endpoint
   - DailyMed `/v2/spls/{SETID}` endpoint
   - Files: `config.yaml`

## Test Strategy

- **Unit tests:** Config loading/validation, cache key building, URL construction, pagination logic
- **Integration tests:** MongoDB cache operations, full handler flow with mocked upstream
- **E2E tests:** Full server with real MongoDB (Docker Compose)

## Spec Traceability

| Task | ACs Covered |
|------|-------------|
| 1. Data models | AC-002, AC-003, AC-007 |
| 2. Config loader | AC-001, AC-002, AC-012 |
| 3. Cache repository | AC-007, AC-008 |
| 4. Upstream fetcher | AC-004, AC-005, AC-006, AC-003, AC-011 |
| 5. Cache handler | AC-008, AC-009, AC-010, AC-013, AC-014 |
| 6. Server entrypoint | AC-001, AC-012 |
| 7. Seed config | AC-015 |

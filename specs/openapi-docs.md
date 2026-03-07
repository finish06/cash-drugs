# Spec: OpenAPI Documentation

**Version:** 0.1.0
**Created:** 2026-03-07
**PRD Reference:** docs/prd.md
**Status:** Complete

## 1. Overview

Auto-generated OpenAPI 3.0 documentation for all API endpoints, served at runtime via Swagger UI. Uses swaggo/swag to generate the spec from Go code annotations. Consumers can explore and test the API interactively from a browser.

### User Story

As an internal microservice developer, I want interactive API documentation served from the drugs service itself, so that I can discover available endpoints, understand request/response formats, and test calls without reading source code.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | `GET /openapi.json` returns a valid OpenAPI 3.0 JSON spec | Must |
| AC-002 | `GET /swagger/` serves Swagger UI with the generated spec loaded | Must |
| AC-003 | All handler functions have swaggo annotations documenting route, method, parameters, and responses | Must |
| AC-004 | `GET /api/cache/{slug}` is documented with path parameter, query parameters, and all response schemas (200 OK, 404 Not Found, 502 Bad Gateway) | Must |
| AC-005 | `GET /health` is documented with response schemas (200 OK, 503 Service Unavailable) | Must |
| AC-006 | Response model schemas (`APIResponse`, `ErrorResponse`, `ResponseMeta`) are documented via swaggo struct annotations | Must |
| AC-007 | The OpenAPI spec includes service metadata: title ("drugs API"), description, version, and base URL | Must |
| AC-008 | `swag init` generates the spec without errors from the annotated code | Must |
| AC-009 | The `/swagger/` and `/openapi.json` endpoints are themselves listed in the spec | Should |
| AC-010 | The generated spec validates against the OpenAPI 3.0 schema (no validation errors) | Should |

## 3. User Test Cases

### TC-001: View Swagger UI in browser

**Precondition:** Service is running.
**Steps:**
1. Open `http://localhost:8080/swagger/` in a browser
**Expected Result:** Swagger UI loads showing all documented endpoints. Endpoints are grouped and described.
**Screenshot Checkpoint:** N/A
**Maps to:** AC-002

### TC-002: Retrieve raw OpenAPI spec

**Precondition:** Service is running.
**Steps:**
1. `curl http://localhost:8080/openapi.json`
**Expected Result:** Returns valid JSON with `"openapi": "3.0"` at the top level. Contains paths for `/api/cache/{slug}`, `/health`, `/swagger/`, `/openapi.json`.
**Screenshot Checkpoint:** N/A
**Maps to:** AC-001, AC-009

### TC-003: Try API call from Swagger UI

**Precondition:** Service is running with cached data.
**Steps:**
1. Open Swagger UI
2. Expand `GET /api/cache/{slug}`
3. Enter `drugnames` as the slug
4. Click "Execute"
**Expected Result:** Response shows 200 with cached data, matching the documented `APIResponse` schema.
**Screenshot Checkpoint:** N/A
**Maps to:** AC-004

### TC-004: Error responses documented

**Precondition:** Service is running.
**Steps:**
1. Open Swagger UI
2. Expand `GET /api/cache/{slug}`
3. Check response schemas
**Expected Result:** 200, 404, and 502 responses are documented with example bodies matching `APIResponse` and `ErrorResponse` schemas.
**Screenshot Checkpoint:** N/A
**Maps to:** AC-004, AC-006

### TC-005: Health endpoint documented

**Precondition:** Service is running.
**Steps:**
1. Open Swagger UI
2. Find `GET /health`
**Expected Result:** Endpoint is listed with 200 and 503 response schemas.
**Screenshot Checkpoint:** N/A
**Maps to:** AC-005

### TC-006: Spec generation from annotations

**Precondition:** swaggo/swag installed.
**Steps:**
1. Run `swag init -g cmd/server/main.go`
**Expected Result:** Generates `docs/swagger.json` and `docs/swagger.yaml` without errors.
**Screenshot Checkpoint:** N/A
**Maps to:** AC-008

## 4. Data Model

No new data entities. Documents existing response models:

| Model | Fields | Usage |
|-------|--------|-------|
| APIResponse | data (object), meta (ResponseMeta) | 200 OK response envelope |
| ResponseMeta | slug, source_url, fetched_at, page_count, stale, stale_reason | Metadata in API response |
| ErrorResponse | error, slug, upstream_status, message | 404 and 502 error responses |
| HealthResponse | status, db | Health check response (inline in handler) |

## 5. API Contract

### New Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | /openapi.json | Returns the generated OpenAPI 3.0 spec as JSON |
| GET | /swagger/ | Serves Swagger UI (HTML + JS) with spec loaded |

### Documented Existing Endpoints

| Method | Path | Parameters | Responses |
|--------|------|------------|-----------|
| GET | /api/cache/{slug} | slug (path, required), SETID etc. (query, optional), _force (query, optional) | 200 APIResponse, 404 ErrorResponse, 502 ErrorResponse |
| GET | /health | none | 200 {"status":"ok","db":"connected"}, 503 {"status":"degraded","db":"disconnected"} |

## 6. UI Behavior

N/A — Swagger UI is provided by the swaggo library, no custom UI work.

## 7. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| Swagger UI accessed before any data is cached | UI loads fine, API calls return 502 if upstream is also down |
| Invalid slug entered in Swagger UI "Try it out" | Returns 404 with documented ErrorResponse schema |
| Service running without swag init having been run | Build should include generated docs; if missing, endpoint returns 404 or empty |

## 8. Dependencies

- `github.com/swaggo/swag` — annotation parser and spec generator
- `github.com/swaggo/http-swagger` — Swagger UI middleware for net/http
- Existing handler and model packages (annotations added to existing code)

## 9. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-07 | 0.1.0 | calebdunn | Initial spec from /add:spec interview |

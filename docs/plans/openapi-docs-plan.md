# Implementation Plan: OpenAPI Documentation (M3)

**Spec:** specs/openapi-docs.md
**Created:** 2026-03-07

## Overview

Add OpenAPI 3.0 documentation with Swagger UI and an endpoint discovery API. Uses swaggo/swag for annotation-based spec generation.

## Task Breakdown

### Task 1: Add swaggo dependencies
- `go get github.com/swaggo/swag/cmd/swag`
- `go get github.com/swaggo/http-swagger/v2`
- Files: go.mod, go.sum
- AC: AC-008

### Task 2: Add swag annotations to main.go
- Add general API info annotations (title, version, description, basePath)
- Files: cmd/server/main.go
- AC: AC-007

### Task 3: Add swag annotations to handlers
- Annotate CacheHandler.ServeHTTP with route, params, responses
- Annotate HealthHandler.ServeHTTP with route, responses
- Add swagger struct tags to model types
- Files: internal/handler/cache.go, internal/handler/health.go, internal/model/response.go
- AC: AC-003, AC-004, AC-005, AC-006

### Task 4: Generate spec and wire Swagger UI
- Run `swag init -g cmd/server/main.go`
- Register `/swagger/` route with httpSwagger handler
- Serve `/openapi.json` from generated docs
- Files: cmd/server/main.go, docs/ (generated)
- AC: AC-001, AC-002, AC-008

### Task 5: Endpoint discovery handler
- `GET /api/endpoints` returns list of configured slugs with metadata
- Files: internal/handler/endpoints.go, cmd/server/main.go
- AC: PRD M3 success criteria

### Task 6: Write tests
- Test `/openapi.json` returns valid JSON with openapi field
- Test `/swagger/` returns HTML
- Test `/api/endpoints` returns configured slugs
- Files: internal/handler/openapi_test.go, internal/handler/endpoints_test.go
- AC: AC-001, AC-002, AC-010

### Task 7: Update Dockerfile
- Install swag in build stage, run `swag init` during build
- Files: Dockerfile
- AC: AC-008

## Test Strategy

- Unit tests for endpoint discovery handler
- Integration tests for swagger UI and openapi.json serving
- Validate generated spec structure

## Dependencies

- Tasks 1-3 before Task 4
- Task 4 before Task 6
- Task 5 independent of Tasks 1-4

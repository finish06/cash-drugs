# Implementation Plan: FDA API Integration

**Spec Version**: 0.1.0
**Created**: 2026-03-07
**Team Size**: Solo
**Estimated Duration**: 3-4 days

## Overview

Extend the generic fetcher to support offset-based (skip/limit) pagination and configurable JSON response parsing, then add 6 FDA openFDA drug API endpoints via config.yaml. All FDA-specific behavior lives in config — the fetcher remains generic.

## Objectives

- Support offset-based pagination alongside existing page-based pagination
- Make JSON response data extraction configurable (data_key, total_key with dot-notation)
- Add 6 FDA drug endpoints (2 prefetch, 4 on-demand search)
- Maintain full backward compatibility with existing DailyMed endpoints
- E2E tests validate all config.yaml endpoints against real APIs

## Success Criteria

- All 17 acceptance criteria implemented and tested
- Code coverage >= 80%
- All quality gates passing
- Zero regressions on existing DailyMed endpoints
- E2E tests pass against live APIs

## Acceptance Criteria Analysis

### AC-001: pagination_style config field
- **Complexity**: Simple
- **Effort**: 30min
- **Tasks**: Add field to Endpoint struct, validate in loader
- **Dependencies**: None
- **Testing**: Unit test config parsing

### AC-002: Offset pagination sends skip/limit
- **Complexity**: Medium
- **Effort**: 2h
- **Tasks**: Modify buildURL to emit skip/limit when pagination_style=offset, modify fetchJSON loop to calculate skip
- **Dependencies**: AC-001
- **Testing**: Unit test URL building, mock fetch test

### AC-003: data_key config field
- **Complexity**: Simple
- **Effort**: 30min
- **Tasks**: Add field to Endpoint struct with default "data"
- **Dependencies**: None
- **Testing**: Unit test config parsing

### AC-004: total_key config field
- **Complexity**: Simple
- **Effort**: 30min
- **Tasks**: Add field to Endpoint struct with default "metadata.total_pages"
- **Dependencies**: None
- **Testing**: Unit test config parsing

### AC-005: Existing DailyMed endpoints unchanged
- **Complexity**: Simple
- **Effort**: 30min
- **Tasks**: Verify defaults match current hardcoded behavior
- **Dependencies**: AC-001, AC-003, AC-004
- **Testing**: Run existing tests, E2E against DailyMed

### AC-006–AC-011: 6 FDA endpoints in config.yaml
- **Complexity**: Simple (config-only)
- **Effort**: 1h
- **Tasks**: Add 6 endpoint blocks to config.yaml with correct FDA API params
- **Dependencies**: AC-001, AC-002, AC-003, AC-004
- **Testing**: E2E against live FDA APIs

### AC-012: Graceful skip limit handling
- **Complexity**: Medium
- **Effort**: 1.5h
- **Tasks**: Detect error/empty response when skip exceeds API cap, stop pagination, store partial data, log warning
- **Dependencies**: AC-002
- **Testing**: Unit test with mock returning error at high skip

### AC-013: E2E tests against real APIs
- **Complexity**: Medium
- **Effort**: 2h
- **Tasks**: Write E2E test suite iterating config.yaml endpoints, minimal params (limit=1)
- **Dependencies**: AC-006–AC-011
- **Testing**: E2E test itself

### AC-014: README documents new config fields
- **Complexity**: Simple
- **Effort**: 30min
- **Tasks**: Add pagination_style, data_key, total_key sections to README
- **Dependencies**: AC-001, AC-003, AC-004

### AC-015: OpenAPI spec updated
- **Complexity**: Simple
- **Effort**: 30min
- **Tasks**: Add FDA endpoint slugs and query params to OpenAPI YAML
- **Dependencies**: AC-006–AC-011

### AC-016: total_key dot-notation traversal
- **Complexity**: Medium
- **Effort**: 1h
- **Tasks**: Implement dot-path traversal helper, replace hardcoded metadata.total_pages in hasMorePages
- **Dependencies**: AC-004
- **Testing**: Unit test with nested JSON structures

### AC-017: Offset skip calculation
- **Complexity**: Simple
- **Effort**: 30min
- **Tasks**: Calculate skip as (page - 1) * pagesize in buildURL
- **Dependencies**: AC-002
- **Testing**: Unit test URL building with offset pagination

## Implementation Phases

### Phase 0: Preparation (0.5 day)

| Task ID | Description | Effort | Dependencies | AC |
|---------|-------------|--------|--------------|-----|
| TASK-001 | Add `PaginationStyle`, `DataKey`, `TotalKey` fields to `config.Endpoint` struct | 30min | — | AC-001, AC-003, AC-004 |
| TASK-002 | Add defaults in `ApplyDefaults`: DataKey="data", TotalKey="metadata.total_pages", PaginationStyle="page" | 15min | TASK-001 | AC-005 |
| TASK-003 | Add config validation for PaginationStyle (must be "page" or "offset") | 15min | TASK-001 | AC-001 |
| TASK-004 | Write unit tests for new config fields parsing and defaults | 30min | TASK-001 | AC-001, AC-003, AC-004, AC-005 |

**Phase Duration**: 0.5 day
**Blockers**: None

### Phase 1: Core Fetcher Enhancements (1-1.5 days)

| Task ID | Description | Effort | Dependencies | AC |
|---------|-------------|--------|--------------|-----|
| TASK-005 | Implement `resolveByDotPath(parsed map[string]interface{}, dotPath string)` helper function | 1h | — | AC-016 |
| TASK-006 | Write unit tests for dot-path traversal (nested keys, missing keys, single key) | 30min | TASK-005 | AC-016 |
| TASK-007 | Refactor `fetchJSONPage` to use `ep.DataKey` instead of hardcoded `"data"` | 30min | TASK-002 | AC-003 |
| TASK-008 | Refactor `hasMorePages` to use `resolveByDotPath(parsed, ep.TotalKey)` instead of hardcoded path | 30min | TASK-005, TASK-002 | AC-004, AC-016 |
| TASK-009 | Modify `buildURL` to emit `skip` and `limit` params when `PaginationStyle == "offset"` | 1h | TASK-002 | AC-002, AC-017 |
| TASK-010 | Modify `fetchJSON` loop: when offset style, calculate skip as `(page-1) * pagesize` | 1h | TASK-009 | AC-002, AC-017 |
| TASK-011 | Add graceful skip-limit handling: detect HTTP error or empty results at high skip values, stop pagination, log warning, return partial data | 1.5h | TASK-010 | AC-012 |
| TASK-012 | Write unit tests for offset pagination URL building | 30min | TASK-009 | AC-002, AC-017 |
| TASK-013 | Write unit tests for offset pagination loop with mock HTTP server | 1h | TASK-010, TASK-011 | AC-002, AC-012 |
| TASK-014 | Write unit tests verifying page-based pagination still works (backward compat) | 30min | TASK-007, TASK-008 | AC-005 |

**Phase Duration**: 1.5 days
**Blockers**: TASK-002 (config defaults) must be done first
**Critical path**: TASK-005 → TASK-008 → TASK-010 → TASK-011 → TASK-013

### Phase 2: FDA Config & E2E Tests (1 day)

| Task ID | Description | Effort | Dependencies | AC |
|---------|-------------|--------|--------------|-----|
| TASK-015 | Add `fda-enforcement` endpoint to config.yaml (prefetch, daily cron, offset pagination) | 15min | TASK-010 | AC-006 |
| TASK-016 | Add `fda-shortages` endpoint to config.yaml (prefetch, daily cron, offset pagination) | 15min | TASK-010 | AC-007 |
| TASK-017 | Add `fda-ndc-by-name` endpoint to config.yaml (on-demand, search by BRAND_NAME) | 15min | TASK-010 | AC-008 |
| TASK-018 | Add `fda-drugsfda-by-name` endpoint to config.yaml (on-demand, search by BRAND_NAME) | 15min | TASK-010 | AC-009 |
| TASK-019 | Add `fda-labels-by-name` endpoint to config.yaml (on-demand, search by DRUG_NAME) | 15min | TASK-010 | AC-010 |
| TASK-020 | Add `fda-events-by-drug` endpoint to config.yaml (on-demand, search by DRUG_NAME) | 15min | TASK-010 | AC-011 |
| TASK-021 | Write E2E test suite: iterate all config.yaml endpoints, test with minimal params (limit=1/pagesize=1), validate HTTP 200 + expected data key + items returned | 2h | TASK-015–TASK-020 | AC-013 |
| TASK-022 | Verify E2E tests pass for all existing DailyMed endpoints (no regression) | 30min | TASK-021 | AC-005 |

**Phase Duration**: 1 day
**Blockers**: Phase 1 must be complete (fetcher must handle offset pagination)

### Phase 3: Documentation & Polish (0.5 day)

| Task ID | Description | Effort | Dependencies | AC |
|---------|-------------|--------|--------------|-----|
| TASK-023 | Update README with `pagination_style`, `data_key`, `total_key` fields and examples | 30min | Phase 1 | AC-014 |
| TASK-024 | Update OpenAPI spec with FDA endpoint slugs and query parameters | 30min | TASK-015–TASK-020 | AC-015 |
| TASK-025 | Run full quality gates (`/add:verify`) — lint, vet, tests, coverage | 30min | Phase 2 | All |

**Phase Duration**: 0.5 day

## Effort Summary

| Phase | Estimated Hours | Days (solo) |
|-------|-----------------|-------------|
| Phase 0: Preparation | 1.5 | 0.5 |
| Phase 1: Core Fetcher | 8 | 1.5 |
| Phase 2: FDA Config & E2E | 4 | 1 |
| Phase 3: Documentation | 1.5 | 0.5 |
| **Total** | **15** | **3.5** |

## Dependencies

### External Dependencies
- FDA APIs are free, no API key needed
- FDA rate limit: ~240 requests/minute (E2E tests use limit=1, well within rate)
- Network access to api.fda.gov required for E2E tests

### Internal Dependencies
- No upstream work required
- Existing DailyMed config must remain functional (backward compatibility)

## File Changes

### Modified Files
| File | Changes |
|------|---------|
| `internal/config/loader.go` | Add `PaginationStyle`, `DataKey`, `TotalKey` to Endpoint struct; update `ApplyDefaults`; add validation |
| `internal/upstream/fetcher.go` | Refactor `fetchJSONPage` (configurable data_key), `hasMorePages` (configurable total_key with dot-path), `buildURL` (offset pagination), `fetchJSON` (skip calculation, graceful stop) |
| `config.yaml` | Add 6 FDA endpoint blocks |

### New Files
| File | Purpose |
|------|---------|
| `internal/upstream/dotpath.go` | `resolveByDotPath` helper for dot-notation JSON traversal |
| `internal/upstream/dotpath_test.go` | Unit tests for dot-path traversal |
| `internal/upstream/fetcher_test.go` | Extended unit tests for offset pagination, data_key, total_key |
| `tests/e2e/config_endpoints_test.go` | E2E test iterating all config.yaml endpoints against live APIs |

### Existing Test Files (Modified)
| File | Changes |
|------|---------|
| `internal/config/loader_test.go` | Tests for new config fields, defaults, validation |

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| FDA API rate limiting during E2E | Low | Low | Tests use limit=1, well under 240/min |
| FDA API changes response structure | Low | Medium | E2E tests detect this immediately; config can be updated |
| Backward compat regression on DailyMed | Low | High | Defaults match current hardcoded values; E2E tests validate both |
| FDA 25K skip cap varies by endpoint | Medium | Low | Graceful stop handles any cap; partial data stored |
| FDA API temporarily unavailable | Low | Low | E2E tests can be skipped with `-short` flag |

## Testing Strategy

1. **Unit Tests** (Phase 0-1)
   - Config parsing: new fields, defaults, validation
   - Dot-path traversal: nested, missing, single-key paths
   - URL building: offset vs page style
   - Pagination loop: offset calculation, graceful stop
   - Data extraction: configurable data_key
   - Backward compat: existing page-based behavior unchanged

2. **E2E Tests** (Phase 2)
   - All config.yaml endpoints tested against live APIs
   - Minimal data requests (limit=1 or pagesize=1)
   - Validates: HTTP 200, expected data key present, items returned
   - Both DailyMed and FDA endpoints covered

3. **Coverage Target**: >= 80% (currently at 81%)

## Deliverables

### Code
- `internal/config/loader.go` — Extended Endpoint struct with 3 new fields
- `internal/upstream/fetcher.go` — Generic offset pagination, configurable data/total keys
- `internal/upstream/dotpath.go` — Dot-notation path traversal utility
- `config.yaml` — 6 FDA endpoint configurations

### Tests
- Unit tests for config, fetcher, dot-path traversal
- E2E tests for all config.yaml endpoints

### Documentation
- README with new config fields documented
- OpenAPI spec with FDA endpoints

## Success Metrics

- [ ] All 17 acceptance criteria implemented
- [ ] Unit tests written and passing
- [ ] E2E tests passing against live APIs
- [ ] Code coverage >= 80%
- [ ] All quality gates passing
- [ ] DailyMed endpoints work unchanged (zero regression)

## Next Steps

1. Review and approve this plan
2. Run `/add:tdd-cycle specs/fda-api-integration.md` to execute
3. Start with Phase 0 (config struct changes)

## Plan History

- 2026-03-07: Initial plan created

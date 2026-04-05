# Implementation Plan: rx-dag NDC Migration

**Spec Version:** 0.1.0
**Spec:** specs/rxdag-ndc-migration.md
**Created:** 2026-04-04
**Team Size:** Solo
**Estimated Duration:** 1.5 days

## Overview

Migrate the `fda-ndc` slug from the public openFDA API to the internal rx-dag ndc-loader, add three new rx-dag query slugs, and introduce a generic `headers` config field with environment variable interpolation for upstream auth.

## Objectives

- Transparent `fda-ndc` upstream swap (no consumer-facing changes)
- Three new rx-dag slugs for richer NDC queries
- Generic `headers` config mechanism reusable by any future endpoint
- `${ENV_VAR}` interpolation in header values

## Success Criteria

- All 16 acceptance criteria implemented and tested
- Existing FDA and DailyMed slugs unaffected
- Code coverage >= 80%
- All quality gates passing

## Acceptance Criteria Analysis

### AC-001: fda-ndc transparent swap
- **Complexity**: Simple
- **Tasks**: Config-only change — update `base_url` and `path`
- **Risk**: None — rx-dag's openFDA endpoint is drop-in compatible

### AC-002 + AC-003 + AC-014: Generic headers config with env var interpolation
- **Complexity**: Medium
- **Tasks**: Add `Headers` field to Endpoint struct, add env var resolver, pass headers to HTTP requests
- **Risk**: Must not break existing endpoints without headers
- **Key files**: `internal/config/loader.go`, `internal/upstream/fetcher.go`

### AC-004 + AC-008: API key header on rx-dag slugs
- **Complexity**: Simple
- **Tasks**: Add `headers` block to 4 config entries

### AC-005 + AC-006 + AC-007: New rx-dag slugs
- **Complexity**: Simple
- **Tasks**: Config entries only — the generic fetcher already handles query_params and path params

### AC-009 + AC-010: Stale-serve and 502 on rx-dag failure
- **Complexity**: None — existing behavior, no code changes
- **Testing**: Verify via test cases

### AC-013: Missing API key warning
- **Complexity**: Simple
- **Tasks**: Add startup validation that logs warning if `RXDAG_API_KEY` is empty

### AC-015 + AC-016: OpenAPI and discovery updates
- **Complexity**: Simple
- **Tasks**: Update swagger annotations, endpoint discovery auto-includes new slugs

## Implementation Phases

### Phase 1: Config Extension — `headers` field (core feature)

This is the only code change. Everything else is config.

| Task ID | Description | Effort | Dependencies | AC |
|---------|-------------|--------|--------------|-----|
| TASK-001 | Add `Headers map[string]string` field to `config.Endpoint` struct with `yaml:"headers"` tag | 15min | — | AC-002, AC-014 |
| TASK-002 | Add `ResolveHeaders()` function in `internal/config/loader.go` — iterates header values, replaces `${VAR}` with `os.Getenv(VAR)`, logs warning for empty vars | 30min | TASK-001 | AC-003, AC-013 |
| TASK-003 | Add `doRequest(method, url string, ep config.Endpoint) (*http.Response, error)` helper in `internal/upstream/fetcher.go` — creates `http.NewRequest`, applies resolved headers from `ep.Headers`, calls `f.Client.Do(req)` | 30min | TASK-001 | AC-002, AC-008 |
| TASK-004 | Replace `f.Client.Get(reqURL)` with `f.doRequest("GET", reqURL, ep)` at all 3 call sites: `fetchRaw` (line 204), `fetchJSONPage` (line 248). Pass `ep` through to `fetchJSONPage`. | 30min | TASK-003 | AC-002 |

**Phase Duration**: ~2 hours
**Key insight**: `fetchJSONPage` currently only takes `(reqURL, dataKey)`. It needs the `Endpoint` (or at least resolved headers) passed through. The cleanest approach is to add `ep config.Endpoint` as a parameter to `fetchJSONPage` and the concurrent page fetch call site.

### Phase 2: Config Changes

| Task ID | Description | Effort | Dependencies | AC |
|---------|-------------|--------|--------------|-----|
| TASK-005 | Update `fda-ndc` in `config.yaml`: change `base_url` to `http://192.168.1.145:8081`, `path` to `/api/openfda/ndc.json`, add `headers: { "X-API-Key": "${RXDAG_API_KEY}" }` | 15min | TASK-001 | AC-001, AC-004 |
| TASK-006 | Add `rx-dag-ndc-search` slug: `base_url: http://192.168.1.145:8081`, `path: /api/ndc/search`, `query_params: { q: "{Q}", limit: "{LIMIT}", offset: "{OFFSET}" }`, headers with API key | 15min | TASK-001 | AC-005, AC-008 |
| TASK-007 | Add `rx-dag-ndc-lookup` slug: `path: /api/ndc/{NDC}`, headers with API key | 10min | TASK-001 | AC-006, AC-008 |
| TASK-008 | Add `rx-dag-ndc-packages` slug: `path: /api/ndc/{NDC}/packages`, headers with API key | 10min | TASK-001 | AC-007, AC-008 |

**Phase Duration**: ~50 minutes

### Phase 3: Testing

| Task ID | Description | Effort | Dependencies | AC |
|---------|-------------|--------|--------------|-----|
| TASK-009 | Unit test: `ResolveHeaders()` — env var present, env var missing (empty string + warning), no headers (nil map), mixed | 1h | TASK-002 | AC-003, AC-013, AC-014 |
| TASK-010 | Unit test: `doRequest()` — headers applied to outgoing request, no headers = no crash, verify method and URL | 1h | TASK-003 | AC-002, AC-008 |
| TASK-011 | Unit test: `fetchJSONPage` still works with endpoint that has no headers (backward compat) | 30min | TASK-004 | AC-012, AC-014 |
| TASK-012 | Integration test: config loads with `headers` field, config loads without `headers` field (existing endpoints) | 30min | TASK-001 | AC-002, AC-014 |
| TASK-013 | E2E test: `fda-ndc` against rx-dag returns openFDA-shaped response (if rx-dag available) | 30min | TASK-005 | AC-001 |
| TASK-014 | E2E test: `rx-dag-ndc-search`, `rx-dag-ndc-lookup`, `rx-dag-ndc-packages` against live rx-dag | 45min | TASK-006–008 | AC-005–007 |
| TASK-015 | Regression test: existing `fda-enforcement`, `fda-shortages`, `fda-label`, DailyMed slugs still work | 30min | TASK-004 | AC-012 |

**Phase Duration**: ~5 hours

### Phase 4: Documentation & Polish

| Task ID | Description | Effort | Dependencies | AC |
|---------|-------------|--------|--------------|-----|
| TASK-016 | Update swagger annotations for new rx-dag slugs | 30min | TASK-006–008 | AC-015 |
| TASK-017 | Verify `GET /api/endpoints` includes new slugs with param docs (auto from config) | 15min | TASK-006–008 | AC-016 |
| TASK-018 | Add `RXDAG_API_KEY` to `.env.example` and document `headers` config field in README | 15min | TASK-002 | AC-003 |

**Phase Duration**: ~1 hour

## Effort Summary

| Phase | Estimated Hours | Description |
|-------|-----------------|-------------|
| Phase 1 | 2h | Config extension — headers + doRequest helper |
| Phase 2 | 1h | Config.yaml changes |
| Phase 3 | 5h | Tests |
| Phase 4 | 1h | Docs & polish |
| **Total** | **9h** | **~1.5 days** |

## Dependencies

### External
- rx-dag ndc-loader running at `192.168.1.145:8081` (needed for E2E tests only)
- Valid `RXDAG_API_KEY` env var

### Internal
- No upstream code changes needed beyond this spec
- No MongoDB schema changes
- No new Go dependencies

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|-----------|
| `fetchJSONPage` signature change breaks callers | Low | Medium | Grep all callers — only called from `fetchJSON` and concurrent loop, both in same file |
| rx-dag unavailable during E2E tests | Medium | Low | E2E tests already handle upstream unavailability gracefully; can test with build tag |
| Header env var interpolation edge cases | Low | Low | Simple `${VAR}` → `os.Getenv` replacement; no nested or recursive expansion needed |
| Existing endpoints regress from `doRequest` refactor | Low | High | Unit tests for backward compat (TASK-011); all existing tests run in CI |

## Critical Path

```
TASK-001 (Endpoint struct)
  → TASK-002 (ResolveHeaders) + TASK-003 (doRequest helper)
    → TASK-004 (replace Client.Get calls)
      → TASK-005–008 (config changes)
        → TASK-009–015 (tests)
          → TASK-016–018 (docs)
```

All tasks are sequential (solo developer). Total critical path: ~9 hours.

## File Change Summary

| File | Action | Description |
|------|--------|-------------|
| `internal/config/loader.go` | Modify | Add `Headers` field to `Endpoint`, add `ResolveHeaders()` |
| `internal/upstream/fetcher.go` | Modify | Add `doRequest()` helper, update `fetchJSONPage` signature, replace `Client.Get` calls |
| `config.yaml` | Modify | Update `fda-ndc`, add 3 new rx-dag slugs |
| `internal/config/loader_test.go` | Modify | Tests for headers parsing |
| `internal/upstream/fetcher_test.go` | Modify | Tests for doRequest, backward compat |
| `tests/e2e/` | Modify | E2E tests for rx-dag endpoints |
| `.env.example` | Modify | Add `RXDAG_API_KEY` |

## Spec Traceability

| AC | Tasks |
|----|-------|
| AC-001 | TASK-005, TASK-013 |
| AC-002 | TASK-001, TASK-003, TASK-004, TASK-010, TASK-012 |
| AC-003 | TASK-002, TASK-009 |
| AC-004 | TASK-005 |
| AC-005 | TASK-006, TASK-014 |
| AC-006 | TASK-007, TASK-014 |
| AC-007 | TASK-008, TASK-014 |
| AC-008 | TASK-005–008, TASK-010 |
| AC-009 | TASK-015 (existing behavior, no code change) |
| AC-010 | TASK-015 (existing behavior, no code change) |
| AC-011 | TASK-005–008 (config only) |
| AC-012 | TASK-011, TASK-015 |
| AC-013 | TASK-002, TASK-009 |
| AC-014 | TASK-001, TASK-011, TASK-012 |
| AC-015 | TASK-016 |
| AC-016 | TASK-017 |

## Next Steps

1. Review and approve plan
2. Run `/add:tdd-cycle specs/rxdag-ndc-migration.md` to execute

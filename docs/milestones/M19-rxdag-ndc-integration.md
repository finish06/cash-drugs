# M19 — rx-dag NDC Integration

## Goal

Migrate NDC queries from the public openFDA API to the internal rx-dag ndc-loader service. Add richer query endpoints (full-text search, direct NDC lookup, package listing). Introduce generic upstream auth headers.

## Status: IN_PROGRESS

## Appetite: 2 days

## Success Criteria

- [ ] `fda-ndc` slug transparently swapped to rx-dag (no consumer-facing changes)
- [ ] 3 new rx-dag slugs operational (search, lookup, packages)
- [ ] Generic `headers` config with `${ENV_VAR}` interpolation working
- [ ] All existing slugs unaffected (backward compatible)
- [ ] Test coverage >= 85%
- [ ] PR created and reviewed

## Hill Chart

| Feature | Position | PR |
|---------|----------|----|
| Generic headers config | PLANNED | — |
| fda-ndc upstream swap | PLANNED | — |
| rx-dag-ndc-search slug | PLANNED | — |
| rx-dag-ndc-lookup slug | PLANNED | — |
| rx-dag-ndc-packages slug | PLANNED | — |

## Dependencies

- rx-dag ndc-loader running at 192.168.1.145:8081
- Valid `RXDAG_API_KEY` env var for E2E tests

## Risks

| Risk | Mitigation |
|------|-----------|
| rx-dag unavailable during E2E tests | Unit/integration tests don't need live service; E2E deferred |
| fetchJSONPage signature change | Only called internally in fetcher.go; all callers updated together |

## Retrospective

_To be filled at milestone completion._

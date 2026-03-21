# Cycle 7 — M14: Observability & Operational Foundation

**Milestone:** M14 — Observability & Operational Foundation
**Maturity:** Beta
**Status:** COMPLETE
**Started:** 2026-03-20
**Completed:** 2026-03-20
**Duration Budget:** 1-2 days

## Work Items

| Feature | Current Pos | Target Pos | Assigned | Est. Effort | Validation |
|---------|-------------|-----------|----------|-------------|------------|
| Request correlation IDs | SHAPED | DONE | Agent-1 | ~2 hours | X-Request-ID in every log line and response header |
| Error taxonomy | SHAPED | DONE | Agent-1 | ~3 hours | Stable error codes, JSON error envelope, error metrics |
| Prometheus alerting rules | SHAPED | DONE | Agent-1 | ~2 hours | 7+ rules with Alertmanager templates |
| Cache status endpoint | SHAPED | DONE | Agent-1 | ~2 hours | GET /api/cache/status returns per-slug health |

## Dependencies & Serialization

All features are independent — no serialization required.

```
Request correlation IDs ──┐
Error taxonomy ───────────┤── all parallel, no dependencies
Alerting rules ───────────┤
Cache status endpoint ────┘
```

## Parallel Strategy

Single-agent serial execution. Features advance sequentially within one agent session.

Recommended order:
1. Request correlation IDs (infrastructure — other features benefit from tracing)
2. Error taxonomy (defines error codes used in alerting rules)
3. Cache status endpoint (new handler, self-contained)
4. Alerting rules (references metrics from above, config-only)

## Validation Criteria

### Per-Item Validation
- **Request correlation IDs:** X-Request-ID generated on ingress, threaded through slog fields, returned in response headers. Existing tests pass.
- **Error taxonomy:** Error codes (CD-U001, CD-M001 etc) in logs and API responses. JSON error envelope with error_code, message, request_id, retry_after. cashdrugs_errors_total metric with code/category/slug labels.
- **Cache status endpoint:** GET /api/cache/status returns JSON with per-slug freshness (doc count, last refresh, staleness, TTL remaining, health score 0-100).
- **Alerting rules:** 7+ rules in docs/grafana/alerts.yml covering cache latency, upstream errors, circuit breaker, MongoDB down, scheduler stalled, high memory, concurrency exhaustion.

### Cycle Success Criteria
- [x] All features reach DONE position
- [x] All acceptance criteria verified
- [x] All existing tests pass + new tests for each feature (13 packages, all OK)
- [x] Coverage remains >= 85% (93.8%)
- [x] go vet clean
- [x] No regressions

## Agent Autonomy & Checkpoints

**Mode:** Autonomous execution. Agent implements all 4 features, runs tests, checks in with results when complete.

**Checkpoint:** Single check-in after all features are implemented and tested.

## Notes

- SLA doc already created during cycle 6 — M14's first item is already DONE
- Request IDs should use middleware pattern for clean separation
- Error taxonomy should be backward-compatible — existing error responses get codes, new responses use the envelope
- Alerting rules are config files, not code — low risk
- Cache status endpoint is a new handler — clean addition

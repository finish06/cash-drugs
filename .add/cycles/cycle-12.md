# Cycle 12 — M17: Intelligent Data Layer (Search + Autocomplete + Filtering)

**Milestone:** M17 — Intelligent Data Layer
**Maturity:** Beta
**Status:** IN_PROGRESS
**Started:** 2026-03-22
**Completed:** TBD
**Duration Budget:** 4+ hours (away mode)

## Work Items

| Feature | Current Pos | Target Pos | Assigned | Est. Effort | Validation |
|---------|-------------|-----------|----------|-------------|------------|
| Cross-slug search | SHAPED | DONE | Agent-1 | ~4 hours | GET /api/search?q=metformin returns grouped results <100ms |
| Autocomplete | SHAPED | DONE | Agent-2 | ~2 hours | GET /api/autocomplete?q=met returns prefix matches <20ms |
| Field filtering | SHAPED | DONE | Agent-3 | ~2 hours | GET /api/cache/{slug}?fields=a,b returns partial payload |

## Dependencies & Serialization

Search must be implemented first — autocomplete shares the MongoDB text index.

```
Cross-slug search (Agent-1)
    ↓ (autocomplete depends on text index from search)
Autocomplete (Agent-1, after search)

Field filtering (Agent-3) — parallel, independent
```

## Parallel Strategy

2 parallel tracks: search+autocomplete (serial), field filtering (independent).

### File Reservations
- **Agent-1 (search + autocomplete):** internal/handler/search.go, internal/handler/search_test.go, internal/handler/autocomplete.go, internal/handler/autocomplete_test.go, internal/cache/mongo.go (text index)
- **Agent-3 (field filtering):** internal/handler/cache.go (add fields param), internal/handler/cache_test.go

### Merge Sequence
1. Field filtering (modifies existing handler, merge first)
2. Cross-slug search (new handler + text index)
3. Autocomplete (new handler, depends on text index)

## Validation Criteria

### Per-Item Validation
- **Cross-slug search:** GET /api/search?q=term returns results grouped by slug. MongoDB text index on cached data. Results ranked: exact > prefix > contains.
- **Autocomplete:** GET /api/autocomplete?q=prefix&limit=10 returns fast prefix matches from drugnames/fda-ndc caches.
- **Field filtering:** GET /api/cache/{slug}?...&fields=product_ndc,brand_name returns only requested fields. Reduces payload size.

### Cycle Success Criteria
- [ ] All 3 features reach DONE
- [ ] All existing tests pass + new tests
- [ ] Coverage remains >= 95%
- [ ] go vet clean
- [ ] No regressions
- [ ] M17 milestone complete (6/6)

## Agent Autonomy & Checkpoints

**Mode:** Away mode — autonomous execution. Commit, push, check in with results.

## Notes

- Text index: MongoDB text indexes work on string fields. Cached data is interface{} — need to extract searchable text fields during indexing or search
- Simpler approach for search: query MongoDB with regex on known fields rather than full text index (more pragmatic for interface{} data)
- Field filtering: apply projection at the Go level after retrieving from cache, not at MongoDB query level (data shape varies per slug)
- Autocomplete: can use a simple in-memory prefix trie built from cached slugnames, or query MongoDB with regex prefix
- Update AllowMethods middleware if needed (all three are GET)
- Register new routes in main.go
- Update k6 smoke test with new endpoints

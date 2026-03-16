# M11 — RxNorm API Integration

**Goal:** Integrate NLM RxNorm REST API endpoints and add parameterized query warmup for top 100 drugs — config-driven endpoints plus pre-caching popular lookups on startup.

**Appetite:** Small-Medium — config-driven endpoints + warmup infrastructure

**Target Maturity:** Beta

**Status:** NOW

## Success Criteria

- [ ] 6 RxNorm endpoints configured in config.yaml
- [ ] All 6 endpoints return valid cached data via `/api/cache/{slug}`
- [ ] `rxnorm-find-drug?DRUG_NAME=metformin` returns RxCUI identifiers
- [ ] `rxnorm-approximate-match?DRUG_NAME=ambienn` returns fuzzy matches with scores
- [ ] `rxnorm-spelling-suggestions?DRUG_NAME=ambienn` returns "ambien"
- [ ] `rxnorm-ndcs?RXCUI=861004` returns NDC codes
- [ ] `rxnorm-generic-product?RXCUI=213269` maps branded to generic
- [ ] `rxnorm-all-related?RXCUI=161` returns ingredient/brand/form relationships
- [ ] TTLs configured appropriately (7d for NDCs, 14d for relationships, 30d for stable lookups)
- [x] No code changes needed for RxNorm endpoints — existing fetcher handles all patterns
- [x] E2E tests validate all 6 endpoints against live RxNorm API (17/17 pass)
- [ ] `warmup-queries.yaml` pre-caches top 100 drugs across fda-ndc, fda-label, rxnorm-find-drug, rxnorm-approximate-match
- [ ] `POST /api/warmup` supports parameterized queries from `warmup-queries.yaml`
- [ ] `/ready` progress includes parameterized query count
- [ ] Failed warmup queries don't block readiness

## Endpoints

| Slug | RxNorm Endpoint | Params | TTL | Use Case |
|------|----------------|--------|-----|----------|
| `rxnorm-find-drug` | `/REST/rxcui.json` | DRUG_NAME | 30d | Drug name → RxCUI lookup |
| `rxnorm-approximate-match` | `/REST/approximateTerm.json` | DRUG_NAME | 7d | Fuzzy/typo-tolerant search |
| `rxnorm-spelling-suggestions` | `/REST/spellingsuggestions.json` | DRUG_NAME | 30d | Spelling correction |
| `rxnorm-ndcs` | `/REST/rxcui/{RXCUI}/ndcs.json` | RXCUI | 7d | RxCUI → NDC mapping |
| `rxnorm-generic-product` | `/REST/rxcui/{RXCUI}/generic.json` | RXCUI | 30d | Brand → Generic |
| `rxnorm-all-related` | `/REST/rxcui/{RXCUI}/allrelated.json` | RXCUI | 14d | All drug relationships |

## Key Characteristics

- **No authentication required** — RxNorm is open/free from NLM
- **No pagination** — All endpoints return complete results in one response
- **No scheduled refresh** — All endpoints are parameterized (on-demand only)
- **Monthly data updates** — NLM releases RxNorm updates first Monday of each month
- **Nested responses** — `rxnorm-all-related` returns `conceptGroup` arrays grouped by term type (IN, BN, SCD, SBD, etc.)

## Risks

| Risk | Mitigation |
|------|------------|
| RxNorm API undocumented rate limits | Singleflight dedup + LRU cache reduce upstream calls; circuit breaker protects against blocking |
| Nested conceptGroup arrays may confuse consumers | Document response structure in API docs; consider future flattening feature |
| `data_key` dot-path may not extract nested arrays correctly | Test each endpoint's data_key extraction against live API |

## Validation

Since this is config-only, validation is:
1. Deploy updated config.yaml
2. Test each endpoint via curl
3. Verify data_key extraction produces expected results
4. Add E2E test cases for all 6 endpoints

## Parameterized Warmup

Pre-cache the top 100 most prescribed drugs (ClinCalc/IQVIA 2023) on startup via `warmup-queries.yaml`:

| Slug | Queries | Source |
|------|---------|--------|
| fda-ndc | 86 | Top 86 by GENERIC_NAME |
| fda-label | 30 | Top 30 by GENERIC_NAME |
| rxnorm-find-drug | 50 | Top 50 drug name lookups |
| rxnorm-approximate-match | 30 | Top 30 fuzzy search |

- **Spec:** `specs/parameterized-warmup.md` (15 ACs)
- **Config:** `warmup-queries.yaml` (196 total queries)
- **Concurrency:** 5 concurrent upstream requests during warmup
- **Ordering:** Scheduled endpoints first, then parameterized queries
- **Resilience:** Failed queries logged but don't block readiness

## Cycles

**Cycle 3:** RxNorm config + parameterized warmup

## Dependencies

- Requires existing fetcher to handle RxNorm's JSON response structure
- `data_key` dot-path traversal must work for paths like `idGroup.rxnormId` and `suggestionGroup.suggestionList.suggestion`
- Requires readiness-warmup endpoints (M12, merged)

---
*Milestone for cash-drugs. Adds NLM RxNorm standardized drug data + parameterized warmup for top 100 drugs.*

# Performance

## SLA Targets (from PRD Section 8)

| Metric | Target |
|--------|--------|
| Cached response P95 | < 50ms |
| LRU cache hit | sub-5ms |
| Autocomplete | < 20ms |
| Cross-slug search | < 100ms |

## Profiling Tools

- `pprof` on internal port `:6060` (M17)
- Benchmark suite: `go test -bench . -benchmem ./internal/cache ./internal/handler`
- Committed baselines: `tests/benchmarks/baseline-{cache,handler}.txt` (captured at cycle 11, commit `fafeb0a`, 2026-03-21, Apple M4 Pro / darwin-arm64)

## Baseline Drift — 2026-04-18 (post-M19, branch `feature/stack-health-version-spec`)

Hardware matches baseline (Apple M4 Pro, darwin/arm64). Re-ran `go test -bench` after M18+M19+M20 work.

### `internal/cache` — all within ±5% of baseline (no regressions)

All 19 cache benchmarks drifted ≤ 5% (noise band). Allocation counts unchanged across the board.

### `internal/handler` — +1 alloc on cache hit path

| Benchmark | Baseline | Current | Δ ns | Δ B/op | Δ allocs |
|-----------|---------:|--------:|-----:|-------:|---------:|
| `CacheHandler_LRUHit` | 910.7 ns | 968.4 ns | +6.3% | +48 | +1 |
| `CacheHandler_MongoHit` | 876.8 ns | 924.0 ns | +5.4% | +48 | +1 |
| `CacheHandler_404` | 554.3 ns | 582.8 ns | +5.1% | 0 | 0 |
| `CacheHandler_LargeResponse` | 375.3 µs | 387.7 µs | +3.3% | +949 | +1 |
| `JSONEncode_APIResponse` | 20.16 µs | 20.37 µs | +1.0% | 0 | 0 |

**Root cause:** baseline was committed at `fafeb0a` (before `7eb19af` — M17 field filtering). The +1 alloc / +48 B on cache handler paths is the expected cost of the `fields=` query-parameter branch added in that commit, not a regression. All paths remain sub-1µs handler overhead — three orders of magnitude under the 50 ms SLA.

**Action:** re-snapshot baselines after M20 lands on main so future comparisons are against the current feature set.

## M20 `/health` — new benchmark

Added `BenchmarkHealthHandler` and `BenchmarkHealthHandler_Unhealthy` in `internal/handler/benchmark_test.go` to cover the stack-compliant health contract.

| Benchmark | ns/op | B/op | allocs/op |
|-----------|------:|-----:|----------:|
| `HealthHandler` (healthy, 2ms simulated ping) | 624.3 | 1361 | 13 |
| `HealthHandler_Unhealthy` (Mongo disconnected) | 644.4 | 1409 | 14 |

**Analysis:** handler overhead is ~625 ns — faster than the cache-hit path. Real-world `/health` latency is dominated by `MongoRepository.PingWithLatency()` (network RTT to MongoDB). The added `Dependencies` array, `uptime`, and `start_time` fields contribute negligible CPU cost. No optimization needed.

## Optimization History

- **2026-03-15** — M10: MongoDB `base_key` exact-match + compound index, 16-shard LRU, parallel page fetches
- **2026-03-14** — M9: concurrency limiter (503 + Retry-After), gzip, singleflight, circuit breakers, 256 MB LRU
- **2026-03-21** — M17: pprof on `:6060`, `tests/benchmarks/baseline-*.txt` committed, MongoDB TTL indexes, field filtering
- **2026-04-18** — M20 `/health` benchmark added; baseline drift attributed to M17 field filtering (not a regression)

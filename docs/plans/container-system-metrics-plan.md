# Implementation Plan: Container System Metrics

**Spec Version:** 0.1.0
**Spec:** specs/container-system-metrics.md
**Created:** 2026-03-14
**Team Size:** Solo (1 agent)
**Estimated Duration:** 5-6 hours

## Overview

Add a `SystemCollector` that reads CPU, memory, disk, and network stats from procfs/cgroup and exports them as Prometheus gauges/counters on the existing `/metrics` endpoint. Follows the `MongoCollector` pattern (background goroutine, `Start()`/`Stop()`, configurable interval). Linux-only via build tags; unit tests use file-based fixtures.

## Objectives

- Export container CPU, memory, disk, and network metrics to Prometheus
- Follow the existing `MongoCollector` lifecycle pattern
- Linux-only implementation with `//go:build linux` build tags
- Testable via file-based fixtures (no live procfs in tests)
- Configurable collection interval via `config.yaml` + env var override

## Acceptance Criteria Analysis

### AC-001, AC-002: CPU metrics (Must)
- **Complexity:** Medium
- **Effort:** 45min
- **Approach:** `syscall.Getrusage` for user+system CPU time (cross-arch, no procfs parsing needed). `runtime.NumCPU()` for core count.

### AC-003, AC-004: Memory RSS/VMS (Must/Should)
- **Complexity:** Medium
- **Effort:** 45min
- **Approach:** Parse `/proc/self/status` for `VmRSS` and `VmSize` fields. Simple line-by-line parsing with `strings.HasPrefix`.

### AC-005, AC-006: Memory cgroup limit + ratio (Should)
- **Complexity:** Medium
- **Effort:** 30min
- **Approach:** Read `/sys/fs/cgroup/memory.max` (cgroup v2). If missing or `"max"`, try `/sys/fs/cgroup/memory/memory.limit_in_bytes` (v1). Parse as int64. Ratio = RSS / limit.

### AC-007: Disk usage (Must)
- **Complexity:** Simple
- **Effort:** 20min
- **Approach:** `syscall.Statfs("/")` — total = blocks * bsize, free = bfree * bsize, used = total - free.

### AC-008, AC-009: Network I/O (Must/Should)
- **Complexity:** Medium
- **Effort:** 45min
- **Approach:** Parse `/proc/net/dev` — skip header lines, split by `|` and whitespace, extract receive/transmit bytes and packets per interface.

### AC-010, AC-014: Background collector lifecycle (Must)
- **Complexity:** Simple
- **Effort:** 30min
- **Approach:** Clone `MongoCollector` pattern — `Start()` goroutine with ticker, `Stop()` via `sync.Once` + done channel, `collect()` method calls individual readers.

### AC-011: Config field + env override (Should)
- **Complexity:** Simple
- **Effort:** 15min
- **Approach:** Add `SystemMetricsInterval` to `AppConfig`. Resolve in `main.go` with `SYSTEM_METRICS_INTERVAL` env var override.

### AC-012: Build tags + fixtures (Must)
- **Complexity:** Medium
- **Effort:** 45min
- **Approach:** Procfs/cgroup parsing in `_linux.go` files. A `SystemSource` interface (like `CollectorSource`) with a `fileSystemSource` for production and `fixtureSource` for tests. Test fixtures are embedded strings matching procfs format.

### AC-013: Metric naming prefix (Must)
- **Complexity:** Simple
- **Effort:** Covered by metric registration

## Implementation Phases

### Phase 1: RED — Write Failing Tests (1.5h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-001 | Create test fixtures: sample `/proc/self/status`, `/proc/net/dev`, cgroup memory files as string constants | 20min | AC-012 | — |
| TASK-002 | Write unit tests for CPU collection — verify `cpu_usage_seconds_total` and `cpu_cores_available` gauges are set | 15min | AC-001, AC-002 | TASK-001 |
| TASK-003 | Write unit tests for memory collection — verify RSS, VMS, limit, and ratio gauges from fixture data | 20min | AC-003, AC-004, AC-005, AC-006 | TASK-001 |
| TASK-004 | Write unit tests for disk collection — verify total/free/used gauges are set with sensible values | 10min | AC-007 | — |
| TASK-005 | Write unit tests for network parsing — verify per-interface bytes and packets from fixture `/proc/net/dev` | 15min | AC-008, AC-009 | TASK-001 |
| TASK-006 | Write unit tests for collector lifecycle — Start/Stop, interval-based collection, panic recovery | 10min | AC-010, AC-014 | — |

**Phase Output:** All tests fail (RED).

### Phase 2: GREEN — Minimal Implementation (2.5h)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-007 | Add `SystemMetricsInterval` field to `AppConfig` in `internal/config/loader.go` | 10min | AC-011 | — |
| TASK-008 | Add container metric gauges/counters to `internal/metrics/metrics.go` — 12 new collectors under `cashdrugs_container_` prefix | 30min | AC-013 | — |
| TASK-009 | Define `SystemSource` interface in `internal/metrics/system.go` with methods: `CPUUsage()`, `MemoryInfo()`, `DiskUsage()`, `NetworkStats()` | 15min | AC-012 | — |
| TASK-010 | Implement `procfsSource` in `internal/metrics/system_linux.go` (`//go:build linux`) — reads from configurable base paths (default `/proc`, `/sys/fs/cgroup`) for testability | 45min | AC-001–AC-009, AC-012 | TASK-009 |
| TASK-011 | Implement `SystemCollector` in `internal/metrics/system_collector.go` — background goroutine, `Start()`/`Stop()`, calls source methods and sets Prometheus gauges | 30min | AC-010, AC-014 | TASK-008, TASK-009 |
| TASK-012 | Wire `SystemCollector` in `cmd/server/main.go` — read config, resolve env override, create and start collector, stop on shutdown | 15min | AC-010, AC-011 | TASK-007, TASK-008, TASK-011 |
| TASK-013 | Implement test fixture source (satisfies `SystemSource` interface with canned data) for unit tests | 15min | AC-012 | TASK-009 |

**Phase Output:** All tests pass (GREEN).

### Phase 3: REFACTOR (30min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-014 | Review parsing code for robustness — handle unexpected procfs formats, missing fields, partial reads | 15min | — | Phase 2 |
| TASK-015 | Run full test suite, `go vet ./...`, verify no regressions | 15min | AC-015 | Phase 2 |

### Phase 4: VERIFY (30min)

| Task ID | Description | Effort | AC | Dependencies |
|---------|-------------|--------|----|--------------|
| TASK-016 | Run `make test-coverage` — verify ≥ 80% | 10min | — | Phase 3 |
| TASK-017 | Spec compliance check — verify each AC has a passing test | 10min | — | Phase 3 |
| TASK-018 | Build Docker image and verify `cashdrugs_container_*` metrics appear in `/metrics` output | 10min | — | Phase 3 |

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/config/loader.go` | Modify | Add `SystemMetricsInterval` to `AppConfig` |
| `internal/config/loader_test.go` | Modify | Test new config field |
| `internal/metrics/metrics.go` | Modify | Add 12 container metric collectors |
| `internal/metrics/system.go` | Create | `SystemSource` interface, shared types (`MemoryInfo`, `DiskUsage`, `NetworkStat`) |
| `internal/metrics/system_linux.go` | Create | `procfsSource` implementation (`//go:build linux`) |
| `internal/metrics/system_collector.go` | Create | `SystemCollector` with Start/Stop lifecycle |
| `internal/metrics/system_collector_test.go` | Create | Unit tests with fixture source |
| `internal/metrics/system_test.go` | Create | Procfs/cgroup parsing tests with fixture data |
| `internal/metrics/testdata/proc_self_status` | Create | Fixture: sample `/proc/self/status` |
| `internal/metrics/testdata/proc_net_dev` | Create | Fixture: sample `/proc/net/dev` |
| `internal/metrics/testdata/cgroup_memory_max` | Create | Fixture: sample cgroup memory.max |
| `cmd/server/main.go` | Modify | Wire SystemCollector, read config, shutdown |

## Effort Summary

| Phase | Estimated Hours |
|-------|-----------------|
| Phase 1: RED | 1.5h |
| Phase 2: GREEN | 2.5h |
| Phase 3: REFACTOR | 0.5h |
| Phase 4: VERIFY | 0.5h |
| **Total** | **5h** |

## Dependencies

### External
- None — stdlib `syscall` + procfs file reads

### Internal
- Existing `internal/metrics` package (add new collectors, follows MongoCollector pattern)
- Existing `internal/config` package (add 1 field)

## Architecture Decision: Source Interface

Following the `MongoCollector` pattern, the `SystemCollector` reads from a `SystemSource` interface:

```go
type SystemSource interface {
    CPUUsage() (userSec, sysSec float64, err error)
    MemoryInfo() (*MemInfo, error)
    DiskUsage(path string) (*DiskInfo, error)
    NetworkStats() ([]NetStat, error)
}
```

- Production: `procfsSource` reads from `/proc` and `/sys/fs/cgroup`
- Tests: fixture source returns canned data
- The `procfsSource` accepts base paths (`procPath`, `cgroupPath`) so tests can point to `testdata/` directories instead of live procfs

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Procfs format varies between kernel versions | Low | Medium | Parse defensively — skip unknown lines, don't panic on format changes |
| cgroup v1 vs v2 detection fails | Low | Low | Try v2 path first, fall back to v1, fall back to -1. Only affects `memory_limit_bytes` |
| Tests can't run on macOS CI (build tags) | Medium | Medium | Use fixture-based source that doesn't need `//go:build linux`; only the `procfsSource` has the build tag |
| `syscall.Statfs` not available on all Linux versions | Very Low | Low | Part of POSIX — available on all Docker-supported kernels |

## Testing Strategy

1. **Unit tests** (Phase 1) — fixture-based, no live procfs dependency
   - Parse `/proc/self/status` from fixture string → verify RSS, VMS extraction
   - Parse `/proc/net/dev` from fixture string → verify per-interface byte/packet counts
   - Parse cgroup memory files → verify limit detection (v1, v2, unlimited)
   - `syscall.Getrusage` → verify CPU seconds conversion
   - `syscall.Statfs` → verify disk calculation (can mock via interface)
   - Collector lifecycle → verify Start/Stop, interval timing

2. **Integration test** (Phase 4) — Docker build + `curl /metrics | grep cashdrugs_container_`

3. **Regression** (Phase 3) — full `make test-unit`

## Spec Traceability

| AC | Tasks | Test Coverage |
|----|-------|---------------|
| AC-001 | TASK-002, TASK-010 | system_test.go, system_collector_test.go |
| AC-002 | TASK-002, TASK-010 | system_test.go |
| AC-003 | TASK-003, TASK-010 | system_test.go |
| AC-004 | TASK-003, TASK-010 | system_test.go |
| AC-005 | TASK-003, TASK-010 | system_test.go |
| AC-006 | TASK-003, TASK-010 | system_test.go |
| AC-007 | TASK-004, TASK-010 | system_test.go |
| AC-008 | TASK-005, TASK-010 | system_test.go |
| AC-009 | TASK-005, TASK-010 | system_test.go |
| AC-010 | TASK-006, TASK-011 | system_collector_test.go |
| AC-011 | TASK-007, TASK-012 | loader_test.go |
| AC-012 | TASK-001, TASK-010, TASK-013 | system_test.go (fixtures) |
| AC-013 | TASK-008 | system_collector_test.go |
| AC-014 | TASK-006, TASK-011 | system_collector_test.go |
| AC-015 | TASK-015 | existing test suite |

## Next Steps

1. Review and approve this plan
2. Run `/add:tdd-cycle specs/container-system-metrics.md` to execute

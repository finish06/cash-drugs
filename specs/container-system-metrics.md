# Spec: Container System Metrics

**Version:** 0.1.0
**Created:** 2026-03-14
**PRD Reference:** docs/prd.md (M9)
**Status:** Complete

## 1. Overview

Export container-level system metrics (CPU, memory, disk, and network) via the existing `/metrics` Prometheus endpoint. The default Go Prometheus collectors provide runtime-level stats (goroutines, GC, heap) but not OS/container-level resource usage. This spec adds host/container metrics read from procfs (`/proc`) and cgroup filesystems so operators can monitor resource consumption alongside application metrics in Grafana without deploying a separate node_exporter or cAdvisor sidecar. The service is always deployed as a Linux Docker container (amd64 or arm64) — no cross-platform support is needed.

### User Story

As an **operator**, I want CPU, memory, disk, and network metrics from the container running cash-drugs, so that I can correlate application performance with resource consumption in a single Grafana dashboard without deploying additional exporters.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | CPU usage gauge `cashdrugs_container_cpu_usage_seconds_total` reports cumulative CPU time (user + system) from `/proc/self/stat` or Go `syscall.Getrusage` | Must |
| AC-002 | CPU core count gauge `cashdrugs_container_cpu_cores_available` reports `runtime.NumCPU()` | Must |
| AC-003 | Memory RSS gauge `cashdrugs_container_memory_rss_bytes` reports resident set size from `/proc/self/status` or equivalent | Must |
| AC-004 | Memory VMS gauge `cashdrugs_container_memory_vms_bytes` reports virtual memory size | Should |
| AC-005 | Memory limit gauge `cashdrugs_container_memory_limit_bytes` reports cgroup memory limit from `/sys/fs/cgroup/memory.max` (or `-1` if unlimited/unavailable) | Should |
| AC-006 | Memory usage percentage gauge `cashdrugs_container_memory_usage_ratio` reports RSS / cgroup limit (0-1 range, omitted if limit unavailable) | Should |
| AC-007 | Disk usage gauges `cashdrugs_container_disk_total_bytes`, `cashdrugs_container_disk_free_bytes`, `cashdrugs_container_disk_used_bytes` report filesystem stats for the root volume (`/`) via `syscall.Statfs` | Must |
| AC-008 | Network I/O counters `cashdrugs_container_network_receive_bytes_total` and `cashdrugs_container_network_transmit_bytes_total` with label `interface` from `/proc/net/dev` | Must |
| AC-009 | Network packet counters `cashdrugs_container_network_receive_packets_total` and `cashdrugs_container_network_transmit_packets_total` with label `interface` | Should |
| AC-010 | All container metrics are collected via a background goroutine on a configurable interval (default: 15s), not on every `/metrics` scrape | Must |
| AC-011 | Collection interval configurable via `system_metrics_interval` field in `config.yaml` with `SYSTEM_METRICS_INTERVAL` env var override (default: `15s`) | Should |
| AC-012 | Implementation targets Linux only (amd64/arm64). Build tags restrict procfs/cgroup code to `linux` — unit tests use file-based fixtures, not live procfs | Must |
| AC-013 | All metric names use the `cashdrugs_container_` prefix | Must |
| AC-014 | Collector has a `Start()` / `Stop()` lifecycle consistent with the existing `MongoCollector` pattern | Must |
| AC-015 | No regression in existing tests or functionality | Must |

## 3. User Test Cases

### TC-001: CPU metrics reported

**Precondition:** Service is running in a Docker container
**Steps:**
1. `curl http://localhost:8080/metrics | grep cashdrugs_container_cpu`
**Expected Result:** Output includes `cashdrugs_container_cpu_usage_seconds_total` with a value > 0, and `cashdrugs_container_cpu_cores_available` matching container CPU allocation.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-002: Memory metrics reported

**Precondition:** Service is running in a Docker container
**Steps:**
1. `curl http://localhost:8080/metrics | grep cashdrugs_container_memory`
**Expected Result:** Output includes `cashdrugs_container_memory_rss_bytes` > 0. If cgroup limit is set, `cashdrugs_container_memory_limit_bytes` shows the limit and `cashdrugs_container_memory_usage_ratio` is between 0 and 1.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-003: Disk metrics reported

**Precondition:** Service is running
**Steps:**
1. `curl http://localhost:8080/metrics | grep cashdrugs_container_disk`
**Expected Result:** Output includes `cashdrugs_container_disk_total_bytes`, `cashdrugs_container_disk_free_bytes`, and `cashdrugs_container_disk_used_bytes` with sensible values (total > used > 0, free > 0).
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-004: Network metrics reported with interface labels

**Precondition:** Service is running in a Docker container
**Steps:**
1. `curl http://localhost:8080/metrics | grep cashdrugs_container_network`
**Expected Result:** Output includes `cashdrugs_container_network_receive_bytes_total{interface="eth0"}` and `cashdrugs_container_network_transmit_bytes_total{interface="eth0"}` (or equivalent container interface name).
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-005: Metrics update on collection interval

**Precondition:** Service is running with `system_metrics_interval: 5s`
**Steps:**
1. `curl /metrics` — note `cashdrugs_container_cpu_usage_seconds_total` value
2. Run a CPU-intensive request (e.g., force-refresh of a paginated endpoint)
3. Wait 5s, `curl /metrics` again
**Expected Result:** CPU usage value has increased. Metrics are not stale from startup.
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

## 4. Data Model

No persistent data. All metrics are in-memory gauges/counters registered with Prometheus.

### System Collector (in-memory)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| interval | time.Duration | Yes | Collection frequency (default 15s) |
| done | chan struct{} | Yes | Shutdown signal channel |
| metrics | *metrics.Metrics | Yes | Shared metrics registry |

## 5. API Contract

No new endpoints. Existing `/metrics` endpoint gains additional `cashdrugs_container_*` metric families.

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| `/sys/fs/cgroup/memory.max` reads `max` (no limit set) | `memory_limit_bytes` set to -1, `usage_ratio` omitted |
| cgroup v1 vs v2 | Check `/sys/fs/cgroup/memory.max` (v2) first, fall back to `/sys/fs/cgroup/memory/memory.limit_in_bytes` (v1) |
| Container has no network interfaces | Network metrics omitted |
| Loopback interface (`lo`) | Included with `interface="lo"` label (operator can filter in Grafana) |
| Collection goroutine panics | Recover, log error, continue collecting on next interval |
| Running on arm64 vs amd64 | No difference — procfs/cgroup paths are architecture-independent |

## 7. Dependencies

- `syscall` / `golang.org/x/sys/unix` (stdlib / extended stdlib) for Getrusage, Statfs
- `/proc` and `/sys/fs/cgroup` filesystems (Linux Docker containers, amd64/arm64)
- Existing `internal/metrics` package for Prometheus registration
- Follows `MongoCollector` pattern from `internal/metrics/collector.go`

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-14 | 0.1.0 | calebdunn | Initial spec from /add:spec interview |

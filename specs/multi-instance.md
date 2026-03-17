# Spec: Multi-Instance Support

**Version:** 0.1.0
**Created:** 2026-03-16
**PRD Reference:** docs/prd.md
**Status:** Draft

## 1. Overview

Enable running multiple cash-drugs instances against the same MongoDB database for horizontal scaling. All instances handle requests, read/write cache, and fetch from upstream on cache miss. Only one instance (the leader) runs the scheduler and startup warmup to avoid duplicate cron fetches and upstream rate limit issues.

### User Story

As an **operator**, I want to run multiple cash-drugs instances behind a load balancer, so that I can handle higher traffic and provide redundancy without duplicate scheduled fetches hitting upstream APIs.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | `ENABLE_SCHEDULER` env var controls whether the instance runs the scheduler and startup warmup (default: `true` for backward compatibility) | Must |
| AC-002 | When `ENABLE_SCHEDULER=false`, the scheduler is not started and no cron jobs are registered | Must |
| AC-003 | When `ENABLE_SCHEDULER=false`, the startup warmup (scheduled endpoints + parameterized queries) does not run | Must |
| AC-004 | When `ENABLE_SCHEDULER=false`, `POST /api/warmup` still works (manual warmup on any instance) | Must |
| AC-005 | When `ENABLE_SCHEDULER=false`, `/ready` returns 200 immediately (no warmup to wait for) | Must |
| AC-006 | All instances (leader and replica) handle `GET /api/cache/{slug}` identically — cache miss triggers upstream fetch and MongoDB write | Must |
| AC-007 | All instances share the same MongoDB database and collections — a write from one instance is visible to all | Must |
| AC-008 | `GET /version` includes `"leader": true/false` field to identify which instance is the leader | Should |
| AC-009 | `enable_scheduler` field also supported in `config.yaml` (env var takes precedence) | Should |
| AC-010 | Prometheus metric `cashdrugs_instance_leader` gauge (1=leader, 0=replica) for alerting if no leader is running | Should |
| AC-011 | No regression — single-instance deployments work identically with default `ENABLE_SCHEDULER=true` | Must |

## 3. User Test Cases

### TC-001: Leader instance runs scheduler

**Precondition:** Instance started with `ENABLE_SCHEDULER=true` (or no env var set)
**Steps:**
1. Start the service
2. Check logs for scheduler registration
3. Wait for cron tick
**Expected Result:** Scheduled endpoints are fetched on cron. Startup warmup runs. `/ready` shows warming progress then ready.
**Maps to:** TBD

### TC-002: Replica instance skips scheduler

**Precondition:** Instance started with `ENABLE_SCHEDULER=false`
**Steps:**
1. Start the service
2. Check logs
3. Check `/ready`
**Expected Result:** No scheduler logs. No warmup. `/ready` returns 200 immediately. `GET /version` shows `"leader": false`.
**Maps to:** TBD

### TC-003: Replica handles cache miss

**Precondition:** Replica instance running. MongoDB cache is empty for a slug.
**Steps:**
1. `GET /api/cache/fda-ndc?GENERIC_NAME=ASPIRIN` on replica
**Expected Result:** Replica fetches from upstream, writes to MongoDB, returns data. Subsequent requests from any instance return cached data.
**Maps to:** TBD

### TC-004: Manual warmup on replica

**Precondition:** Replica instance running with `ENABLE_SCHEDULER=false`
**Steps:**
1. `POST /api/warmup` on replica
**Expected Result:** Warmup runs (scheduled endpoints + parameterized queries). `/ready` shows progress. Returns 202.
**Maps to:** TBD

### TC-005: Leader and replica share MongoDB cache

**Precondition:** Two instances running against same MongoDB. Leader warms cache.
**Steps:**
1. Leader warms `drugnames` via scheduler
2. Replica receives `GET /api/cache/drugnames`
**Expected Result:** Replica serves cached data written by leader (MongoDB hit, no upstream fetch).
**Maps to:** TBD

## 4. Data Model

No schema changes. All instances use the same MongoDB collections and document format.

### Instance Configuration

| Field | Source | Default | Description |
|-------|--------|---------|-------------|
| EnableScheduler | `ENABLE_SCHEDULER` env / `enable_scheduler` config | `true` | Controls scheduler + startup warmup |

## 5. API Contract

### GET /version (updated)

```json
{
  "version": "v0.9.1",
  "leader": true,
  "git_commit": "abc1234",
  ...
}
```

New `leader` field indicates if this instance runs the scheduler.

## 6. Deployment Examples

### Staging (2 instances)

```yaml
services:
  drugs-leader:
    image: dockerhub.calebdunn.tech/finish06/cash-drugs:beta
    environment:
      - ENABLE_SCHEDULER=true
      - MONGO_URI=mongodb://cash-drugs-mongo:27017/drugs_staging
    ports:
      - "8083:8080"

  drugs-replica:
    image: dockerhub.calebdunn.tech/finish06/cash-drugs:beta
    environment:
      - ENABLE_SCHEDULER=false
      - MONGO_URI=mongodb://cash-drugs-mongo:27017/drugs_staging
    ports:
      - "8084:8080"
```

### Production (4 instances, behind load balancer)

```yaml
services:
  drugs-1:
    image: dockerhub.calebdunn.tech/finish06/cash-drugs:latest
    environment:
      - ENABLE_SCHEDULER=true    # leader
      - MONGO_URI=mongodb://mongo:27017/drugs

  drugs-2:
    environment:
      - ENABLE_SCHEDULER=false   # replica

  drugs-3:
    environment:
      - ENABLE_SCHEDULER=false   # replica

  drugs-4:
    environment:
      - ENABLE_SCHEDULER=false   # replica
```

## 7. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| No leader running (all `ENABLE_SCHEDULER=false`) | Service works but no scheduled refresh — cache goes stale after TTL expires. Prometheus alert on `cashdrugs_instance_leader == 0`. |
| Multiple leaders (two instances with `ENABLE_SCHEDULER=true`) | Both run schedulers — duplicate upstream fetches. Harmless but wasteful. Fetchlock prevents concurrent fetches within an instance but not across instances. |
| Leader crashes, replica still running | Replica continues serving cached data. Cache goes stale after TTL. Operator promotes a replica by setting `ENABLE_SCHEDULER=true`. |
| MongoDB connection lost | All instances degrade — serve from LRU cache, upstream fetches fail to persist. Same as single-instance behavior. |
| Replica starts before leader has warmed cache | Replica serves cache misses via upstream fetch. Once leader warms, replica benefits from shared MongoDB cache. |

## 8. Dependencies

- Existing scheduler (`internal/scheduler/scheduler.go`)
- Existing warmup orchestrator (`internal/handler/warmup_orchestrator.go`)
- Existing config loader (`internal/config/loader.go`)
- `cmd/server/main.go` wiring

## 9. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-16 | 0.1.0 | calebdunn | Initial spec for multi-instance support |

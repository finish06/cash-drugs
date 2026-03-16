# cash-drugs Sequence Diagrams

## Cache Lookup Flow (Happy Path)

```mermaid
sequenceDiagram
    actor Client as Internal Service
    participant GZ as Gzip Middleware
    participant LIM as Concurrency Limiter
    participant CH as CacheHandler
    participant LRU as Sharded LRU Cache<br/>(16 shards, FNV-1a)
    participant DB as MongoDB<br/>(cached_responses)

    Client->>GZ: GET /api/cache/{slug}?param=value
    GZ->>LIM: pass through
    LIM->>CH: ServeHTTP(w, r)
    CH->>CH: Build cache key<br/>(slug + sorted params)
    CH->>LRU: Get(cacheKey)
    Note over LRU: FNV-1a hash → shard N<br/>lock shard N mutex
    alt LRU hit
        LRU-->>CH: CachedResponse (fresh)
    end
    alt LRU miss
        LRU-->>CH: nil
        CH->>DB: Get(cacheKey) via base_key index
        Note over DB: Compound index: base_key + page<br/>exact match (no regex)
        DB-->>CH: CachedResponse (fresh)
        CH->>LRU: Set(cacheKey, response)
    end
    CH->>CH: Set X-Cache-Stale: false
    CH-->>GZ: 200 {"data": [...], "meta": {slug, fetched_at, results_count, stale: false}}
    GZ-->>Client: 200 (gzip compressed)
```

## Cache Miss — Upstream Fetch Flow

```mermaid
sequenceDiagram
    actor Client as Internal Service
    participant CH as CacheHandler
    participant SF as Singleflight
    participant FL as FetchLock
    participant CB as Circuit Breaker
    participant UF as HTTPFetcher
    participant API as Upstream API
    participant LRU as Sharded LRU Cache
    participant DB as MongoDB

    Client->>CH: GET /api/cache/{slug}
    CH->>LRU: Get(cacheKey)
    LRU-->>CH: nil (LRU miss)
    CH->>DB: Get(cacheKey) via base_key index
    DB-->>CH: nil (cache miss)

    CH->>SF: Do(cacheKey, fetchFn)
    Note over SF: Dedup concurrent requests<br/>for same key

    SF->>FL: Lock(slug)
    FL-->>SF: Acquired

    SF->>CB: Allow(slug)?
    alt Circuit closed
        CB-->>SF: Allowed
        SF->>UF: Fetch(endpoint, params)

        Note over UF,API: Page 1 fetched sequentially
        UF->>API: GET {base_url}{path}?page=1
        API-->>UF: 200 {data, total_pages: N}

        alt Multi-page response (N > 1)
            Note over UF,API: Pages 2..N fetched concurrently<br/>semaphore cap = 3 goroutines
            par Parallel page fetches
                UF->>API: GET ?page=2
                API-->>UF: {data}
            and
                UF->>API: GET ?page=3
                API-->>UF: {data}
            and
                UF->>API: GET ?page=4
                API-->>UF: {data}
            end
            Note over UF: Remaining pages queued behind semaphore
        end

        UF->>UF: Extract data via data_key
        UF-->>SF: CachedResponse{Pages}

        alt Upstream success (data present)
            SF->>CB: RecordSuccess(slug)
            SF->>DB: Upsert(response) with base_key
            SF->>LRU: Set(cacheKey, response)
            SF->>FL: Unlock(slug)
            SF-->>CH: CachedResponse
            CH-->>Client: 200 {"data": [...], "meta": {results_count, stale: false}}
        end

        alt Upstream success (empty data)
            Note over UF: API returned 200 but empty results
            SF->>CB: RecordSuccess(slug)
            SF->>FL: Unlock(slug)
            SF-->>CH: CachedResponse (empty)
            CH-->>Client: 200 {"data": [], "meta": {results_count: 0, stale: false}}
        end

        alt Upstream failure
            API-->>UF: 5xx / timeout / error
            UF-->>SF: error
            SF->>CB: RecordFailure(slug)
            SF->>DB: Get(cacheKey) — stale fallback
            alt Stale cache exists
                DB-->>SF: CachedResponse (stale)
                SF->>FL: Unlock(slug)
                SF-->>CH: CachedResponse (stale)
                CH-->>Client: 200 {"data": [...], "meta": {stale: true, stale_reason: "..."}}
            end
            alt No cache at all
                SF->>FL: Unlock(slug)
                SF-->>CH: error
                CH-->>Client: 502 {"error": "upstream_error"}
            end
        end
    end
```

## Stale-While-Revalidate Flow

```mermaid
sequenceDiagram
    actor Client as Internal Service
    participant CH as CacheHandler
    participant DB as MongoDB
    participant FL as FetchLock
    participant UF as HTTPFetcher
    participant API as Upstream API

    Client->>CH: GET /api/cache/{slug}
    CH->>DB: Get(cacheKey)
    DB-->>CH: CachedResponse (stale, past TTL)

    CH->>CH: Set X-Cache-Stale: true
    CH-->>Client: 200 {"data": [...], "meta": {stale: true}}

    Note over CH,API: Background revalidation (goroutine)
    CH-)FL: Lock(slug)
    CH-)UF: Fetch(endpoint, params)
    UF-)API: GET {base_url}{path}
    API--)UF: 200 {fresh data}
    UF--)CH: CachedResponse
    CH-)DB: Upsert(response)
    CH-)FL: Unlock(slug)
    Note over DB: Cache now fresh for next request
```

## Concurrency Limiter Flow

```mermaid
sequenceDiagram
    actor Client as Internal Service
    participant GZ as Gzip Middleware
    participant LIM as Concurrency Limiter<br/>(max 50 in-flight)
    participant MUX as ServeMux

    Client->>GZ: GET /api/cache/{slug}
    GZ->>LIM: pass through

    alt Under limit (< 50 in-flight)
        LIM->>LIM: Acquire semaphore
        LIM->>MUX: ServeHTTP(w, r)
        MUX-->>LIM: response
        LIM->>LIM: Release semaphore
        LIM-->>GZ: response
        GZ-->>Client: 200 (gzip compressed)
    end

    alt At limit (50 in-flight)
        LIM->>LIM: Cannot acquire
        LIM-->>GZ: 503 {"error": "server_busy"}<br/>+ Retry-After: 5
        GZ-->>Client: 503 (gzip compressed)
    end

    Note over LIM: Exempt paths: /health, /metrics, /version<br/>bypass the limiter entirely
```

## Circuit Breaker Flow

```mermaid
sequenceDiagram
    participant CH as CacheHandler
    participant CB as Circuit Breaker<br/>(per endpoint)
    participant UF as HTTPFetcher
    participant API as Upstream API
    participant DB as MongoDB

    Note over CB: State machine per slug

    alt Circuit CLOSED (normal operation)
        CH->>CB: Allow(slug)?
        CB-->>CH: Allowed
        CH->>UF: Fetch(endpoint, params)
        UF->>API: GET upstream
        alt Success
            API-->>UF: 200
            UF-->>CH: response
            CH->>CB: RecordSuccess(slug)
        end
        alt Failure (repeated)
            API-->>UF: 5xx
            UF-->>CH: error
            CH->>CB: RecordFailure(slug)
            Note over CB: After N consecutive failures<br/>→ transition to OPEN
        end
    end

    alt Circuit OPEN (blocking requests)
        CH->>CB: Allow(slug)?
        CB-->>CH: Rejected (circuit open)
        CH->>DB: Get(cacheKey) — stale fallback
        alt Stale cache exists
            DB-->>CH: CachedResponse (stale)
            CH-->>CH: Return stale with X-Circuit-State: open
        end
        alt No cache
            CH-->>CH: 503 {"error": "circuit_open"}
        end
        Note over CB: After open_duration elapses<br/>→ transition to HALF-OPEN
    end

    alt Circuit HALF-OPEN (probing)
        CH->>CB: Allow(slug)?
        CB-->>CH: Allowed (probe request)
        CH->>UF: Fetch(endpoint, params)
        UF->>API: GET upstream
        alt Probe success
            API-->>UF: 200
            CH->>CB: RecordSuccess(slug)
            Note over CB: → transition to CLOSED
        end
        alt Probe failure
            API-->>UF: 5xx
            CH->>CB: RecordFailure(slug)
            Note over CB: → transition back to OPEN
        end
    end
```

## Force-Refresh Cooldown Flow

```mermaid
sequenceDiagram
    actor Client as Internal Service
    participant CH as CacheHandler
    participant CD as CooldownTracker<br/>(30s per slug)
    participant UF as HTTPFetcher
    participant API as Upstream API

    Client->>CH: GET /api/cache/{slug}?_force=true

    CH->>CD: IsOnCooldown(slug)?

    alt Not on cooldown
        CD-->>CH: false
        CH->>UF: Fetch(endpoint, params)
        UF->>API: GET upstream
        API-->>UF: 200 {data}
        UF-->>CH: CachedResponse
        CH->>CD: StartCooldown(slug)
        CH-->>Client: 200 (fresh data)
    end

    alt On cooldown
        CD-->>CH: true (remaining: 18s)
        CH->>CH: Ignore _force flag
        CH->>CH: Set X-Force-Cooldown: 18
        Note over CH: Serve from cache normally<br/>(LRU → MongoDB)
        CH-->>Client: 200 (cached data)<br/>+ X-Force-Cooldown: 18
    end
```

## Container System Metrics Flow

```mermaid
sequenceDiagram
    participant SC as SystemCollector<br/>(background)
    participant PROC as /proc filesystem
    participant CG as cgroup
    participant REG as prometheus.Registry

    Note over SC: Collection interval (configurable)

    SC->>PROC: Read /proc/stat
    PROC-->>SC: CPU times (user, system, idle)
    SC->>REG: Set cashdrugs_system_cpu_usage_percent

    SC->>CG: Read memory.current / memory.max
    alt cgroup available (container)
        CG-->>SC: usage, limit
        SC->>REG: Set cashdrugs_system_memory_usage_bytes
        SC->>REG: Set cashdrugs_system_memory_limit_bytes
    end
    alt cgroup unavailable (bare metal)
        SC->>PROC: Read /proc/meminfo
        PROC-->>SC: MemTotal, MemAvailable
        SC->>REG: Set cashdrugs_system_memory_usage_bytes
    end

    SC->>PROC: Read /proc/diskstats
    PROC-->>SC: sectors read/written
    SC->>REG: Set cashdrugs_system_disk_read_bytes
    SC->>REG: Set cashdrugs_system_disk_write_bytes

    SC->>PROC: Read /proc/net/dev
    PROC-->>SC: rx_bytes, tx_bytes
    SC->>REG: Set cashdrugs_system_network_receive_bytes
    SC->>REG: Set cashdrugs_system_network_transmit_bytes
```

## Paginated Fetch Flow (Parallel)

```mermaid
sequenceDiagram
    participant UF as HTTPFetcher<br/>(FetchConcurrency: 3)
    participant SEM as Semaphore<br/>(cap 3)
    participant API as Upstream API

    Note over UF,API: Page-style pagination (parallel)
    UF->>API: GET ?page=1&pagesize=100 (sequential)
    API-->>UF: {data: [...], metadata: {total_pages: 5}}
    Note over UF: total_pages=5 → spawn 4 goroutines

    par Pages 2-4 (concurrent, semaphore slots available)
        UF->>SEM: acquire
        SEM-->>UF: granted
        UF->>API: GET ?page=2&pagesize=100
        API-->>UF: {data: [...]}
    and
        UF->>SEM: acquire
        SEM-->>UF: granted
        UF->>API: GET ?page=3&pagesize=100
        API-->>UF: {data: [...]}
    and
        UF->>SEM: acquire
        SEM-->>UF: granted
        UF->>API: GET ?page=4&pagesize=100
        API-->>UF: {data: [...]}
    end

    Note over UF,SEM: Page 5 waits for a semaphore slot
    UF->>SEM: acquire (blocks until slot free)
    SEM-->>UF: granted
    UF->>API: GET ?page=5&pagesize=100
    API-->>UF: {data: [...]}

    Note over UF: Combine all pages into CachedResponse.Pages

    Note over UF,API: Offset-style pagination (FDA, same parallel strategy)
    UF->>API: GET ?skip=0&limit=100 (sequential)
    API-->>UF: {results: [...], meta: {results: {total: 350}}}
    Note over UF: total=350 → 3 more pages needed

    par Remaining offsets (concurrent)
        UF->>API: GET ?skip=100&limit=100
        API-->>UF: {results: [...]}
    and
        UF->>API: GET ?skip=200&limit=100
        API-->>UF: {results: [...]}
    and
        UF->>API: GET ?skip=300&limit=100
        API-->>UF: {results: [...]}
    end

    Note over UF: Combine via configurable data_key + total_key
```

## Scheduled Refresh Flow

```mermaid
sequenceDiagram
    participant CRON as Scheduler<br/>(robfig/cron)
    participant FL as FetchLock
    participant UF as HTTPFetcher
    participant API as Upstream API
    participant DB as MongoDB

    Note over CRON: Cron fires (e.g., "0 */6 * * *")
    CRON->>DB: FetchedAt(cacheKey)
    DB-->>CRON: timestamp

    alt Cache still fresh (within TTL)
        Note over CRON: Skip — no fetch needed
    end

    alt Cache stale or missing
        CRON->>FL: Lock(slug)
        FL-->>CRON: Acquired
        CRON->>UF: Fetch(endpoint, nil)
        UF->>API: GET {base_url}{path}
        API-->>UF: 200 {data}
        UF-->>CRON: CachedResponse
        CRON->>DB: Upsert(response)
        CRON->>FL: Unlock(slug)
    end

    alt Lock already held (dedup)
        CRON->>FL: Lock(slug)
        FL-->>CRON: Already locked — skip
        Note over CRON: Another fetch in progress, skip
    end
```

## Health Check Flow

```mermaid
sequenceDiagram
    actor Client as Internal Service
    participant GW as cash-drugs<br/>:8080
    participant HC as HealthHandler
    participant DB as MongoDB

    Client->>GW: GET /health
    GW->>HC: ServeHTTP(w, r)
    HC->>DB: Ping(ctx)

    alt MongoDB reachable
        DB-->>HC: ok
        HC-->>Client: 200 {"status": "ok", "db": "connected", "version": "v0.8.0"}
    end

    alt MongoDB unreachable
        DB-->>HC: error
        HC-->>Client: 503 {"status": "degraded", "db": "disconnected", "version": "v0.8.0"}
    end
```

## Endpoint Discovery Flow

```mermaid
sequenceDiagram
    actor Client as Internal Service
    participant GW as cash-drugs<br/>:8080
    participant EH as EndpointsHandler

    Client->>GW: GET /api/endpoints
    GW->>EH: ServeHTTP(w, r)
    EH->>EH: Build endpoint list from config
    EH-->>Client: 200 [{slug, base_url, path, format, refresh, ttl}, ...]
```

## Prometheus Metrics Flow

```mermaid
sequenceDiagram
    actor Prom as Prometheus
    participant GW as cash-drugs<br/>:8080
    participant MH as promhttp.Handler
    participant REG as prometheus.Registry
    participant MC as MongoCollector<br/>(background)
    participant SC as SystemCollector<br/>(background)
    participant DB as MongoDB

    Note over MC,DB: Every 30 seconds
    MC->>DB: Ping(ctx)
    DB-->>MC: ok
    MC->>REG: Set mongodb_up=1, ping_duration
    MC->>DB: Aggregate({$group: {_id: "$slug", count: {$sum: 1}}})
    DB-->>MC: [{slug, count}, ...]
    MC->>REG: Set mongodb_documents_total per slug

    Note over SC: Periodic collection
    SC->>REG: Set system_cpu, memory, disk, network metrics

    Prom->>GW: GET /metrics
    GW->>MH: ServeHTTP(w, r)
    MH->>REG: Gather()
    REG-->>MH: All metric families
    MH-->>Prom: text/plain (Prometheus exposition format)
    Note over Prom: Includes cashdrugs_* + Go runtime<br/>+ system container metrics
```

## Version Endpoint Flow

```mermaid
sequenceDiagram
    actor Client as Internal Service
    participant GW as cash-drugs<br/>:8080
    participant VH as VersionHandler

    Client->>GW: GET /version
    Note over GW: Exempt from concurrency limiter
    GW->>VH: ServeHTTP(w, r)
    VH->>VH: Collect runtime info<br/>(version, git_commit, git_branch,<br/>build_date, go_version, os, arch,<br/>hostname, GOMAXPROCS, uptime)
    VH-->>Client: 200 {"version": "v0.8.0", "git_commit": "abc1234",<br/>"uptime_seconds": 3600, "endpoint_count": 17, ...}

    Note over VH: Prometheus gauges updated independently:<br/>cashdrugs_build_info (labels: version, commit, go, date)<br/>cashdrugs_uptime_seconds (updated every 15s)
```

## Empty Upstream Result Flow

```mermaid
sequenceDiagram
    actor Client as Internal Service
    participant CH as CacheHandler
    participant UF as HTTPFetcher
    participant API as Upstream API

    Client->>CH: GET /api/cache/{slug}?search=nonexistent
    CH->>CH: LRU miss → MongoDB miss → fetch upstream

    CH->>UF: Fetch(endpoint, params)
    UF->>API: GET {base_url}{path}?search=nonexistent
    API-->>UF: 200 {data: [], metadata: {total_pages: 0}}
    Note over UF: Valid 200 response, but empty data array

    UF-->>CH: CachedResponse (empty data)

    Note over CH: NOT an error — upstream responded successfully<br/>Return 200 with empty data + results_count: 0
    CH-->>Client: 200 {"data": [], "meta": {results_count: 0, stale: false}}

    Note over CH: Previous behavior returned 502 for empty results.<br/>Now correctly distinguishes "no results" from "upstream error".
```

## RxNorm Lookup Flow

```mermaid
sequenceDiagram
    actor Client as Internal Service
    participant CH as CacheHandler
    participant LRU as Sharded LRU Cache
    participant DB as MongoDB
    participant SF as Singleflight
    participant RX as RxNorm API<br/>(rxnav.nlm.nih.gov)

    Client->>CH: GET /api/cache/{rxnorm-slug}?name=aspirin

    CH->>LRU: Get(cacheKey)
    alt LRU hit (subsequent request)
        LRU-->>CH: CachedResponse (fresh)
        CH-->>Client: 200 {"data": [...], "meta": {stale: false}}
        Note over LRU,CH: Fast path — no MongoDB or API call
    end

    alt LRU miss
        LRU-->>CH: nil
        CH->>DB: Get(cacheKey) via base_key index
        alt MongoDB hit
            DB-->>CH: CachedResponse (fresh)
            CH->>LRU: Set(cacheKey, response)
            CH-->>Client: 200 {"data": [...], "meta": {stale: false}}
        end

        alt MongoDB miss (first request)
            DB-->>CH: nil
            CH->>SF: Do(cacheKey, fetchFn)
            Note over SF,RX: Single request — no pagination<br/>RxNorm returns all results in one response

            SF->>RX: GET /REST/{path}?name=aspirin
            RX-->>SF: 200 {rxnormdata: {idGroup: {rxnormId: [...]}}}

            Note over SF: Dot-path data_key extraction<br/>(e.g., "rxnormdata.idGroup.rxnormId")
            SF->>SF: Extract nested data via data_key

            SF->>DB: Upsert(response) with base_key
            SF->>LRU: Set(cacheKey, response)
            Note over LRU,DB: Cache populated for subsequent requests

            SF-->>CH: CachedResponse
            CH-->>Client: 200 {"data": [...], "meta": {results_count: N, stale: false}}
        end
    end
```

## System Overview

```mermaid
sequenceDiagram
    actor Dev as Developer
    actor Svc as Internal Service
    actor Prom as Prometheus
    participant SW as Swagger UI
    participant GZ as Gzip
    participant LIM as Limiter
    participant CD as cash-drugs<br/>:8080
    participant LRU as Sharded LRU Cache<br/>(16 shards)
    participant DB as MongoDB
    participant API1 as DailyMed API
    participant API2 as openFDA API
    participant API3 as RxNorm API

    Dev->>CD: GET /swagger/
    CD-->>Dev: Swagger UI
    Dev->>CD: GET /openapi.json
    CD-->>Dev: OpenAPI spec

    Svc->>GZ: GET /api/cache/drugnames
    GZ->>LIM: pass through
    LIM->>CD: ServeHTTP
    CD->>LRU: Hash key → shard → check
    LRU-->>CD: miss
    CD->>DB: base_key exact match (indexed)
    DB-->>CD: Cache miss
    Note over CD: Singleflight + circuit breaker
    CD->>API1: GET /dailymed/services/v2/drugnames?page=1
    API1-->>CD: {data: [...], metadata: {total_pages: N}}
    Note over CD,API1: Pages 2..N fetched in parallel (cap 3)
    CD->>DB: Store response with base_key (multi-page)
    CD->>LRU: Populate shard
    CD-->>GZ: {"data": [...], "meta": {results_count: M, stale: false}}
    GZ-->>Svc: 200 (gzip compressed)

    Svc->>GZ: GET /api/cache/fda-enforcement
    GZ->>LIM: pass through
    LIM->>CD: ServeHTTP
    CD->>LRU: Hash key → shard → check
    LRU-->>CD: Cache hit (fresh)
    CD-->>GZ: {"data": [...], "meta": {results_count: M, stale: false}}
    GZ-->>Svc: 200 (gzip compressed)

    Svc->>GZ: GET /api/cache/rxnorm-interaction
    GZ->>LIM: pass through
    LIM->>CD: ServeHTTP
    CD->>LRU: Hash key → shard → check
    LRU-->>CD: miss
    CD->>DB: base_key exact match (indexed)
    DB-->>CD: Cache miss
    Note over CD: Single request — no pagination
    CD->>API3: GET /REST/interaction/list.json?rxcui=123
    API3-->>CD: {interactionTypeGroup: [...]}
    Note over CD: Dot-path data_key extraction
    CD->>DB: Store response with base_key
    CD->>LRU: Populate shard
    CD-->>GZ: {"data": [...], "meta": {results_count: M, stale: false}}
    GZ-->>Svc: 200 (gzip compressed)

    Note over CD,DB: Scheduler runs in background
    CD->>API2: GET /drug/enforcement.json?skip=0&limit=100
    API2-->>CD: {results: [...], meta: {results: {total: 500}}}
    CD->>DB: Upsert cached response with base_key

    Svc->>CD: GET /version
    Note over LIM: Exempt — bypasses limiter
    CD-->>Svc: {"version": "v0.8.0", "uptime_seconds": 3600, ...}

    Svc->>CD: GET /health
    Note over LIM: Exempt — bypasses limiter
    CD-->>Svc: {"status": "ok", "db": "connected", "version": "v0.8.0"}

    Prom->>CD: GET /metrics
    Note over LIM: Exempt — bypasses limiter
    CD-->>Prom: cashdrugs_* + build_info + uptime + system metrics
```

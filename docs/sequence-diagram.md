# cash-drugs Sequence Diagrams

## Cache Lookup Flow (Happy Path)

```mermaid
sequenceDiagram
    actor Client as Internal Service
    participant GZ as Gzip Middleware
    participant LIM as Concurrency Limiter
    participant CH as CacheHandler
    participant LRU as LRU Cache<br/>(in-memory)
    participant DB as MongoDB<br/>(cached_responses)

    Client->>GZ: GET /api/cache/{slug}?param=value
    GZ->>LIM: pass through
    LIM->>CH: ServeHTTP(w, r)
    CH->>CH: Build cache key<br/>(slug + sorted params)
    CH->>LRU: Get(cacheKey)
    alt LRU hit
        LRU-->>CH: CachedResponse (fresh)
    end
    alt LRU miss
        LRU-->>CH: nil
        CH->>DB: Get(cacheKey)
        DB-->>CH: CachedResponse (fresh)
        CH->>LRU: Set(cacheKey, response)
    end
    CH->>CH: Set X-Cache-Stale: false
    CH-->>GZ: 200 {"data": [...], "meta": {slug, fetched_at, stale: false}}
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
    participant LRU as LRU Cache
    participant DB as MongoDB

    Client->>CH: GET /api/cache/{slug}
    CH->>LRU: Get(cacheKey)
    LRU-->>CH: nil (LRU miss)
    CH->>DB: Get(cacheKey)
    DB-->>CH: nil (cache miss)

    CH->>SF: Do(cacheKey, fetchFn)
    Note over SF: Dedup concurrent requests<br/>for same key

    SF->>FL: Lock(slug)
    FL-->>SF: Acquired

    SF->>CB: Allow(slug)?
    alt Circuit closed
        CB-->>SF: Allowed
        SF->>UF: Fetch(endpoint, params)
        UF->>API: GET {base_url}{path}?params

        alt Upstream success
            API-->>UF: 200 {data}
            UF->>UF: Extract data via data_key
            UF->>UF: Paginate if needed (page/offset style)
            UF-->>SF: CachedResponse{Pages}
            SF->>CB: RecordSuccess(slug)
            SF->>DB: Upsert(response)
            SF->>LRU: Set(cacheKey, response)
            SF->>FL: Unlock(slug)
            SF-->>CH: CachedResponse
            CH-->>Client: 200 {"data": [...], "meta": {stale: false}}
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

    Note over LIM: Exempt paths: /health, /metrics<br/>bypass the limiter entirely
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

## Paginated Fetch Flow

```mermaid
sequenceDiagram
    participant UF as HTTPFetcher
    participant API as Upstream API

    Note over UF,API: Page-style pagination
    UF->>API: GET ?page=1&pagesize=100
    API-->>UF: {data: [...], metadata: {total_pages: 5}}
    UF->>API: GET ?page=2&pagesize=100
    API-->>UF: {data: [...]}
    UF->>API: GET ?page=3&pagesize=100
    API-->>UF: {data: [...]}
    UF->>API: GET ?page=4&pagesize=100
    API-->>UF: {data: [...]}
    UF->>API: GET ?page=5&pagesize=100
    API-->>UF: {data: [...]}
    Note over UF: Combine all pages into CachedResponse.Pages

    Note over UF,API: Offset-style pagination (FDA)
    UF->>API: GET ?skip=0&limit=100
    API-->>UF: {results: [...], meta: {results: {total: 350}}}
    UF->>API: GET ?skip=100&limit=100
    API-->>UF: {results: [...]}
    UF->>API: GET ?skip=200&limit=100
    API-->>UF: {results: [...]}
    UF->>API: GET ?skip=300&limit=100
    API-->>UF: {results: [...]}
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
        HC-->>Client: 200 {"status": "ok", "db": "connected", "version": "v0.6.1"}
    end

    alt MongoDB unreachable
        DB-->>HC: error
        HC-->>Client: 503 {"status": "degraded", "db": "disconnected", "version": "v0.6.1"}
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
    participant LRU as LRU Cache
    participant DB as MongoDB
    participant API1 as DailyMed API
    participant API2 as openFDA API

    Dev->>CD: GET /swagger/
    CD-->>Dev: Swagger UI
    Dev->>CD: GET /openapi.json
    CD-->>Dev: OpenAPI spec

    Svc->>GZ: GET /api/cache/drugnames
    GZ->>LIM: pass through
    LIM->>CD: ServeHTTP
    CD->>LRU: Check LRU
    LRU-->>CD: miss
    CD->>DB: Check MongoDB
    DB-->>CD: Cache miss
    Note over CD: Singleflight + circuit breaker
    CD->>API1: GET /dailymed/services/v2/drugnames
    API1-->>CD: {data: [...], metadata: {total_pages: N}}
    CD->>DB: Store response (multi-page)
    CD->>LRU: Populate LRU
    CD-->>GZ: {"data": [...], "meta": {stale: false}}
    GZ-->>Svc: 200 (gzip compressed)

    Svc->>GZ: GET /api/cache/fda-enforcement
    GZ->>LIM: pass through
    LIM->>CD: ServeHTTP
    CD->>LRU: Check LRU
    LRU-->>CD: Cache hit (fresh)
    CD-->>GZ: {"data": [...], "meta": {stale: false}}
    GZ-->>Svc: 200 (gzip compressed)

    Note over CD,DB: Scheduler runs in background
    CD->>API2: GET /drug/enforcement.json?skip=0&limit=100
    API2-->>CD: {results: [...], meta: {results: {total: 500}}}
    CD->>DB: Upsert cached response

    Svc->>CD: GET /health
    Note over LIM: Exempt — bypasses limiter
    CD-->>Svc: {"status": "ok", "db": "connected", "version": "v0.6.1"}

    Prom->>CD: GET /metrics
    Note over LIM: Exempt — bypasses limiter
    CD-->>Prom: cashdrugs_* + system metrics (Prometheus exposition format)
```

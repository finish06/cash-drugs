# cash-drugs Sequence Diagrams

## Cache Lookup Flow (Happy Path)

```mermaid
sequenceDiagram
    actor Client as Internal Service
    participant GW as cash-drugs<br/>:8080
    participant CH as CacheHandler
    participant DB as MongoDB<br/>(cached_responses)

    Client->>GW: GET /api/cache/{slug}?param=value
    GW->>CH: ServeHTTP(w, r)
    CH->>CH: Build cache key<br/>(slug + sorted params)
    CH->>DB: Get(cacheKey)
    DB-->>CH: CachedResponse (fresh)
    CH->>CH: Set X-Cache-Stale: false
    CH-->>Client: 200 {"data": [...], "meta": {slug, fetched_at, stale: false}}
```

## Cache Miss — Upstream Fetch Flow

```mermaid
sequenceDiagram
    actor Client as Internal Service
    participant CH as CacheHandler
    participant FL as FetchLock
    participant UF as HTTPFetcher
    participant API as Upstream API
    participant DB as MongoDB

    Client->>CH: GET /api/cache/{slug}
    CH->>DB: Get(cacheKey)
    DB-->>CH: nil (cache miss)

    CH->>FL: Lock(slug)
    FL-->>CH: Acquired

    CH->>UF: Fetch(endpoint, params)
    UF->>API: GET {base_url}{path}?params

    alt Upstream success
        API-->>UF: 200 {data}
        UF->>UF: Extract data via data_key
        UF->>UF: Paginate if needed (page/offset style)
        UF-->>CH: CachedResponse{Pages}
        CH->>DB: Upsert(response)
        CH->>FL: Unlock(slug)
        CH-->>Client: 200 {"data": [...], "meta": {stale: false}}
    end

    alt Upstream failure
        API-->>UF: 5xx / timeout / error
        UF-->>CH: error
        CH->>DB: Get(cacheKey) — stale fallback
        alt Stale cache exists
            DB-->>CH: CachedResponse (stale)
            CH->>FL: Unlock(slug)
            CH-->>Client: 200 {"data": [...], "meta": {stale: true, stale_reason: "..."}}
        end
        alt No cache at all
            CH->>FL: Unlock(slug)
            CH-->>Client: 502 {"error": "upstream_error"}
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
        HC-->>Client: 200 {"status": "ok", "db": "connected", "version": "v0.5.0"}
    end

    alt MongoDB unreachable
        DB-->>HC: error
        HC-->>Client: 503 {"status": "degraded", "db": "disconnected", "version": "v0.5.0"}
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

## System Overview

```mermaid
sequenceDiagram
    actor Dev as Developer
    actor Svc as Internal Service
    participant SW as Swagger UI
    participant CD as cash-drugs<br/>:8080
    participant DB as MongoDB
    participant API1 as DailyMed API
    participant API2 as openFDA API

    Dev->>CD: GET /swagger/
    CD-->>Dev: Swagger UI
    Dev->>CD: GET /openapi.json
    CD-->>Dev: OpenAPI spec

    Svc->>CD: GET /api/cache/drugnames
    CD->>DB: Check cache
    DB-->>CD: Cache miss
    CD->>API1: GET /dailymed/services/v2/drugnames
    API1-->>CD: {data: [...], metadata: {total_pages: N}}
    CD->>DB: Store response (multi-page)
    CD-->>Svc: {"data": [...], "meta": {stale: false}}

    Svc->>CD: GET /api/cache/fda-enforcement
    CD->>DB: Check cache
    DB-->>CD: Cache hit (fresh)
    CD-->>Svc: {"data": [...], "meta": {stale: false}}

    Note over CD,DB: Scheduler runs in background
    CD->>API2: GET /drug/enforcement.json?skip=0&limit=100
    API2-->>CD: {results: [...], meta: {results: {total: 500}}}
    CD->>DB: Upsert cached response

    Svc->>CD: GET /health
    CD-->>Svc: {"status": "ok", "db": "connected", "version": "v0.5.0"}
```

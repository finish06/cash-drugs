# drugs

**Configure once, cache forever, use everywhere.**

Point drugs at any REST API — it fetches the data, stores it in MongoDB, and serves it back instantly. Your microservices hit drugs instead of upstream APIs. One cache, zero redundant calls.

```yaml
# config.yaml — that's it
endpoints:
  - slug: users
    base_url: https://api.example.com
    path: /v1/users
    format: json
```

```bash
# Every service gets the same cached data
curl http://localhost:8080/api/cache/users
```

## Why

Every internal service calling the same external API means:
- **Redundant requests** — 10 services × same API = 10× the calls
- **Rate limit pain** — one service burns the quota, all services fail
- **Cascade failures** — upstream goes down, everything goes down

drugs sits in between. Call once, cache in MongoDB, serve to everyone. When upstream goes down, your services keep running on cached data.

## Quick Start

```bash
docker-compose up
```

Service starts at **http://localhost:8080**. Explore:
- **Swagger UI:** http://localhost:8080/swagger/
- **All endpoints:** http://localhost:8080/api/endpoints
- **Health:** http://localhost:8080/health

## Configure Any API

Add entries to `config.yaml`. Each entry becomes a cached endpoint at `/api/cache/{slug}`.

### Simple (fetch on demand)

```yaml
endpoints:
  - slug: products
    base_url: https://api.example.com
    path: /v1/products
    format: json
```

→ `GET /api/cache/products`

### Auto-paginated (fetch all pages)

```yaml
  - slug: all-items
    base_url: https://api.example.com
    path: /v1/items
    format: json
    pagination: all
    pagesize: 50
```

drugs walks every page automatically and stores them as separate MongoDB documents (no 16MB limit). Consumers get one combined response.

### Scheduled refresh

```yaml
  - slug: inventory
    base_url: https://api.example.com
    path: /v1/inventory
    format: json
    pagination: all
    refresh: "0 */4 * * *"   # Every 4 hours
    ttl: "4h"                 # Serve stale + background refresh after 4h
```

Cache stays fresh automatically. When TTL expires, stale data is served instantly while a background fetch runs — consumers never wait.

### With parameters

```yaml
  - slug: user-detail
    base_url: https://api.example.com
    path: /v1/users/{USER_ID}
    format: json
```

→ `GET /api/cache/user-detail?USER_ID=123`

Parameters work in paths and query values:

```yaml
  - slug: search
    base_url: https://api.example.com
    path: /v1/search
    format: json
    query_params:
      q: "{QUERY}"
      category: "{CAT}"
```

→ `GET /api/cache/search?QUERY=aspirin&CAT=drugs`

### Raw format (XML, HTML, binary)

```yaml
  - slug: report-xml
    base_url: https://api.example.com
    path: /v1/reports/{ID}.xml
    format: raw
```

Stores the response body as-is with the original content type. No JSON envelope.

## How It Works

```
Your Services → drugs → MongoDB cache
                  ↕
            Upstream APIs
```

1. **First request** → cache miss → fetch from upstream → store in MongoDB → return
2. **Subsequent requests** → served from cache (< 50ms)
3. **Scheduled refresh** → cron job fetches in background → cache stays fresh
4. **TTL expired** → serve stale immediately → background refresh → next request gets fresh data
5. **Upstream down** → serve last cached response with `stale: true`
6. **No cache at all + upstream down** → 502

## Response Format

JSON endpoints return:

```json
{
  "data": [ ... ],
  "meta": {
    "slug": "products",
    "source_url": "https://api.example.com/v1/products",
    "fetched_at": "2026-03-07T15:59:13Z",
    "page_count": 12,
    "stale": false
  }
}
```

## Go Client

```go
resp, _ := http.Get("http://drugs:8080/api/cache/products")
defer resp.Body.Close()

var result struct {
    Data []Product `json:"data"`
    Meta struct {
        Stale bool `json:"stale"`
    } `json:"meta"`
}
json.NewDecoder(resp.Body).Decode(&result)
```

## Configuration Reference

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `slug` | yes | — | Unique name, becomes `/api/cache/{slug}` |
| `base_url` | yes | — | Upstream base URL |
| `path` | yes | — | Path, supports `{PARAM}` placeholders |
| `format` | yes | — | `json`, `xml`, or `raw` |
| `query_params` | no | — | Static or `{PARAM}` query parameters |
| `pagination` | no | `1` | `"all"` or max page count |
| `page_param` | no | `page` | Query param for page number |
| `pagesize_param` | no | `pagesize` | Query param for page size |
| `pagesize` | no | `100` | Items per page |
| `refresh` | no | — | Cron expression for background refresh |
| `ttl` | no | — | Go duration (`1h`, `30m`) for staleness |
| `log_level` | no | `warn` | Log level: `debug`, `info`, `warn`, `error` |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CONFIG_PATH` | `config.yaml` | Path to config file |
| `MONGO_URI` | — | MongoDB connection string |
| `LISTEN_ADDR` | `:8080` | Server listen address |
| `LOG_LEVEL` | `warn` | Overrides config file log level |
| `LOG_FORMAT` | `json` | `json` or `text` |

## Development

```bash
# Run with Docker
docker-compose up

# Run directly
MONGO_URI=mongodb://localhost:27017/drugs go run ./cmd/server

# Tests
make test-unit          # Fast, no Docker
make test-integration   # Full suite with MongoDB
make test-coverage      # Coverage report

# Lint
go vet ./...
```

## Tech Stack

Go 1.22+ · MongoDB · Docker Compose · `log/slog` · swaggo/swag

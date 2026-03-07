# drugs

API cache/proxy microservice — fetches from upstream REST APIs on demand and on schedule, stores responses in MongoDB, and serves cached data to internal consumers.

## Quick Start

```bash
docker-compose up
```

The service starts at **http://localhost:8080** and immediately begins warming cached data in the background.

### Explore the API

- **Swagger UI:** http://localhost:8080/swagger/
- **OpenAPI spec:** http://localhost:8080/openapi.json
- **Endpoint discovery:** http://localhost:8080/api/endpoints

## Usage

### Get cached data

```bash
# Get all drug names (cached, auto-paginated)
curl http://localhost:8080/api/cache/drugnames | jq '.meta'

# Get all SPLs
curl http://localhost:8080/api/cache/spls | jq '.data | length'

# Get a specific SPL detail by set ID
curl "http://localhost:8080/api/cache/spl-detail?SETID=7a8d2539-4422-4312-0298-207ef1a2948e"

# Get raw SPL XML
curl "http://localhost:8080/api/cache/spl-xml?SETID=7a8d2539-4422-4312-0298-207ef1a2948e"
```

### Response format

JSON endpoints return data wrapped in an envelope:

```json
{
  "data": [ ... ],
  "meta": {
    "slug": "drugnames",
    "source_url": "https://dailymed.nlm.nih.gov/dailymed/services/v2/drugnames",
    "fetched_at": "2026-03-07T15:59:13Z",
    "page_count": 1045,
    "stale": false
  }
}
```

When data is stale (past TTL or upstream is down), `meta.stale` is `true` and `meta.stale_reason` explains why. Raw/XML endpoints return the upstream response body directly with the original content type.

### Discover available endpoints

```bash
curl http://localhost:8080/api/endpoints | jq .
```

Returns all configured endpoints with their parameters, pagination settings, and schedule status.

### Health check

```bash
curl http://localhost:8080/health
# {"status":"ok","db":"connected"}
```

## Adding a New Upstream API

Edit `config.yaml` and add a new entry under `endpoints:`. Restart the service to pick up changes.

### Minimal example (on-demand, single page)

```yaml
endpoints:
  - slug: my-api              # URL slug: GET /api/cache/my-api
    base_url: https://api.example.com
    path: /v1/data
    format: json               # json or raw
```

### With pagination (fetch all pages)

```yaml
  - slug: all-items
    base_url: https://api.example.com
    path: /v1/items
    format: json
    pagination: all            # Fetch every page ("all" or a number like 5)
    page_param: page           # Query param name for page number (default: "page")
    pagesize_param: per_page   # Query param name for page size (default: "pagesize")
    pagesize: 50               # Items per page (default: 100)
```

The service walks pages until the upstream says there are no more (via `metadata.total_pages` in the response).

### With scheduled refresh

```yaml
  - slug: products
    base_url: https://api.example.com
    path: /v1/products
    format: json
    pagination: all
    refresh: "0 */4 * * *"    # Cron expression: refresh every 4 hours
    ttl: "4h"                  # Serve stale after 4 hours, trigger background refresh
```

- `refresh` — Cron schedule for automatic background refresh. Only works for endpoints without parameters.
- `ttl` — Go duration string (`1h`, `30m`, `6h`). After TTL expires, the next request serves stale data immediately and triggers a background refresh.

### With path parameters

```yaml
  - slug: user-detail
    base_url: https://api.example.com
    path: /v1/users/{USER_ID}
    format: json
```

Consumers pass the parameter as a query string: `GET /api/cache/user-detail?USER_ID=123`

### With query parameter placeholders

```yaml
  - slug: search
    base_url: https://api.example.com
    path: /v1/search
    format: json
    query_params:
      q: "{QUERY}"
      category: "{CAT}"
```

Consumer request: `GET /api/cache/search?QUERY=aspirin&CAT=drugs`

The `{PARAM}` placeholders in both `path` and `query_params` values are substituted at request time.

### Raw format (XML, HTML, binary)

```yaml
  - slug: document-xml
    base_url: https://api.example.com
    path: /v1/documents/{DOC_ID}.xml
    format: raw
```

`format: raw` stores the upstream response body as-is with the original content type. The cached response is served directly (no JSON envelope).

### Configuration reference

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `slug` | string | yes | — | Unique identifier, used in consumer URL |
| `base_url` | string | yes | — | Upstream API base URL |
| `path` | string | yes | — | Endpoint path, supports `{PARAM}` placeholders |
| `format` | string | yes | — | `json` or `raw` |
| `query_params` | map | no | — | Static or `{PARAM}` query parameters |
| `pagination` | string/int | no | `1` | `"all"` or max page count |
| `page_param` | string | no | `page` | Query param name for page number |
| `pagesize_param` | string | no | `pagesize` | Query param name for page size |
| `pagesize` | int | no | `100` | Items per page |
| `refresh` | string | no | — | Cron expression for scheduled refresh |
| `ttl` | string | no | — | Go duration for cache staleness threshold |

## How It Works

1. **First request** — No cache exists. Service fetches from upstream, auto-paginates if configured, stores in MongoDB, returns to consumer.
2. **Subsequent requests** — Served directly from MongoDB cache (sub-50ms).
3. **Scheduled refresh** — Cron job fetches fresh data in the background. Consumers always get fast cached responses.
4. **Stale-while-revalidate** — When TTL expires, the stale response is served immediately while a background goroutine fetches fresh data. No consumer waits for upstream.
5. **Upstream failure** — If upstream is down, the last cached response is returned with `stale: true`. If no cache exists at all, returns 502.
6. **Large datasets** — Multi-page responses are stored as separate MongoDB documents per page (avoiding the 16MB limit) and reassembled transparently on read.

## Go Client Example

```go
package main

import (
    "encoding/json"
    "fmt"
    "net/http"
)

type CacheResponse struct {
    Data interface{} `json:"data"`
    Meta struct {
        Slug      string `json:"slug"`
        FetchedAt string `json:"fetched_at"`
        PageCount int    `json:"page_count"`
        Stale     bool   `json:"stale"`
    } `json:"meta"`
}

func main() {
    resp, _ := http.Get("http://localhost:8080/api/cache/drugnames")
    defer resp.Body.Close()

    var result CacheResponse
    json.NewDecoder(resp.Body).Decode(&result)

    fmt.Printf("Slug: %s, Pages: %d, Stale: %v\n",
        result.Meta.Slug, result.Meta.PageCount, result.Meta.Stale)
}
```

## Development

```bash
# Run directly (requires local MongoDB)
MONGO_URI=mongodb://localhost:27017/drugs go run ./cmd/server

# Run tests
go test ./...            # All tests
go test ./... -short     # Unit tests only

# Regenerate OpenAPI spec after changing annotations
swag init -g cmd/server/main.go -o docs --parseDependency

# Lint
golangci-lint run ./...
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CONFIG_PATH` | `config.yaml` | Path to endpoint configuration file |
| `MONGO_URI` | — | MongoDB connection URI (or set `database.uri` in config) |
| `LISTEN_ADDR` | `:8080` | Server listen address |

## Architecture

```
Consumer → drugs API → MongoDB cache
                ↕
          Upstream APIs (DailyMed, etc.)
```

- **Language:** Go 1.22+ (stdlib `net/http`)
- **Database:** MongoDB
- **Docs:** swaggo/swag + Swagger UI
- **Containers:** Docker Compose

# Prometheus Metrics Setup

cash-drugs exposes a `/metrics` endpoint in Prometheus exposition format. This guide covers scrape configuration, available metrics, alerting rules, and Grafana dashboard setup.

## Endpoint

```
GET http://localhost:8080/metrics
Content-Type: text/plain; version=0.0.4; charset=utf-8
```

No authentication required. The endpoint is served by `promhttp.Handler()` and includes both `cashdrugs_*` application metrics and Go runtime metrics (goroutines, memory, GC).

## Prometheus Scrape Configuration

Add a scrape job to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: cash-drugs
    scrape_interval: 15s
    static_configs:
      - targets: ['cash-drugs:8080']
        labels:
          environment: production
```

For Docker Compose deployments, use the service name as the target:

```yaml
scrape_configs:
  - job_name: cash-drugs
    scrape_interval: 15s
    static_configs:
      - targets: ['drugs:8080']
```

### Docker Compose with Prometheus

To add Prometheus to your existing `docker-compose.yml`:

```yaml
services:
  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    depends_on:
      - drugs
    restart: unless-stopped

volumes:
  prometheus-data:
```

Create `prometheus.yml` alongside your compose file:

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: cash-drugs
    static_configs:
      - targets: ['drugs:8080']
```

## Available Metrics

All application metrics use the `cashdrugs_` namespace prefix.

### HTTP Requests

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `cashdrugs_http_requests_total` | Counter | `slug`, `method`, `status_code` | Total HTTP requests to `/api/cache/{slug}` |
| `cashdrugs_http_request_duration_seconds` | Histogram | `slug`, `method` | Request latency (includes cache lookup and optional upstream fetch) |

### Cache Performance

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `cashdrugs_cache_hits_total` | Counter | `slug`, `outcome` | Cache outcomes. `outcome` is `hit`, `miss`, or `stale` |

**Cache hit ratio per slug:**
```promql
sum by (slug) (rate(cashdrugs_cache_hits_total{outcome="hit"}[5m]))
/
sum by (slug) (rate(cashdrugs_cache_hits_total[5m]))
```

### Upstream API

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `cashdrugs_upstream_fetch_duration_seconds` | Histogram | `slug` | Time spent fetching from upstream APIs |
| `cashdrugs_upstream_fetch_errors_total` | Counter | `slug` | Failed upstream fetch attempts |
| `cashdrugs_upstream_fetch_pages_total` | Counter | `slug` | Total pages fetched (paginated endpoints) |

### MongoDB

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `cashdrugs_mongodb_up` | Gauge | — | MongoDB health: `1` = healthy, `0` = unhealthy |
| `cashdrugs_mongodb_ping_duration_seconds` | Gauge | — | Last MongoDB ping latency in seconds |
| `cashdrugs_mongodb_documents_total` | Gauge | `slug` | Document count per slug in the cache collection |

MongoDB metrics are collected by a background goroutine every 30 seconds (not on every `/metrics` scrape).

### Scheduler

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `cashdrugs_scheduler_runs_total` | Counter | `slug`, `result` | Scheduler job executions. `result` is `success` or `error` |
| `cashdrugs_scheduler_run_duration_seconds` | Histogram | `slug` | Scheduler job duration (fetch + store) |

### Fetch Lock

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `cashdrugs_fetchlock_dedup_total` | Counter | `slug` | Deduplicated concurrent fetch attempts (lock already held) |

### Go Runtime

Standard Go collector metrics are included automatically:

- `go_goroutines` — Number of goroutines
- `go_memstats_alloc_bytes` — Bytes allocated and in use
- `go_gc_duration_seconds` — GC pause duration
- `process_cpu_seconds_total` — Total CPU time
- `process_resident_memory_bytes` — Resident memory size

## Useful PromQL Queries

### Request Rate
```promql
sum(rate(cashdrugs_http_requests_total[5m]))
```

### Request Rate by Slug
```promql
sum by (slug) (rate(cashdrugs_http_requests_total[5m]))
```

### P95 Latency by Slug
```promql
histogram_quantile(0.95, sum by (slug, le) (rate(cashdrugs_http_request_duration_seconds_bucket[5m])))
```

### Cache Hit Ratio (overall)
```promql
sum(rate(cashdrugs_cache_hits_total{outcome="hit"}[5m]))
/
sum(rate(cashdrugs_cache_hits_total[5m]))
```

### Upstream Error Rate
```promql
sum by (slug) (rate(cashdrugs_upstream_fetch_errors_total[5m]))
```

### Scheduler Success Rate
```promql
sum(rate(cashdrugs_scheduler_runs_total{result="success"}[5m]))
/
sum(rate(cashdrugs_scheduler_runs_total[5m]))
```

### MongoDB Health
```promql
cashdrugs_mongodb_up
```

### Total Cached Documents
```promql
sum(cashdrugs_mongodb_documents_total)
```

## Grafana Dashboard

An importable Grafana dashboard is provided at [`docs/grafana/cash-drugs-dashboard.json`](grafana/cash-drugs-dashboard.json).

### Import Steps

1. Open Grafana and navigate to **Dashboards > Import**
2. Click **Upload JSON file** and select `docs/grafana/cash-drugs-dashboard.json`
3. Select your Prometheus datasource from the dropdown
4. Click **Import**

### Dashboard Panels

The dashboard includes the following sections:

**Overview Row**
- MongoDB status (UP/DOWN)
- MongoDB ping latency
- Request rate (req/s)
- Cache hit ratio (%)
- Upstream errors (5m window)
- Total cached documents

**HTTP Requests Row**
- Request rate by slug (time series)
- Request latency P95 by slug (time series)
- Request rate by status code (bar chart)

**Cache Performance Row**
- Cache outcomes by slug — stacked hit/miss/stale (time series)
- Cache hit ratio by slug (gauge)

**Upstream APIs Row**
- Upstream fetch duration P95 by slug (time series)
- Upstream errors by slug (bar chart)
- Pages fetched by slug (time series)

**Scheduler Row**
- Scheduler runs by slug and result — stacked success/error (bar chart)
- Scheduler run duration P95 by slug (time series)

**MongoDB Row**
- Documents per slug (table)
- Fetch lock deduplications by slug (time series)

### Dashboard Variables

The dashboard includes a `$slug` template variable that lets you filter all panels to specific endpoints. It auto-populates from `label_values(cashdrugs_http_requests_total, slug)`.

## Example Alert Rules

Add to your Prometheus alerting rules:

```yaml
groups:
  - name: cash-drugs
    rules:
      - alert: MongoDBDown
        expr: cashdrugs_mongodb_up == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "cash-drugs MongoDB is down"
          description: "MongoDB has been unreachable for more than 1 minute."

      - alert: HighUpstreamErrorRate
        expr: sum by (slug) (rate(cashdrugs_upstream_fetch_errors_total[5m])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High upstream error rate for {{ $labels.slug }}"
          description: "Upstream API errors for {{ $labels.slug }} exceed 0.1/s for 5 minutes."

      - alert: LowCacheHitRatio
        expr: >
          (sum by (slug) (rate(cashdrugs_cache_hits_total{outcome="hit"}[15m]))
          /
          sum by (slug) (rate(cashdrugs_cache_hits_total[15m]))) < 0.5
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "Low cache hit ratio for {{ $labels.slug }}"
          description: "Cache hit ratio for {{ $labels.slug }} is below 50% over 15 minutes."

      - alert: HighRequestLatency
        expr: >
          histogram_quantile(0.95, sum by (slug, le) (rate(cashdrugs_http_request_duration_seconds_bucket[5m]))) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High P95 latency for {{ $labels.slug }}"
          description: "P95 request latency for {{ $labels.slug }} exceeds 1 second."

      - alert: SchedulerFailures
        expr: sum by (slug) (rate(cashdrugs_scheduler_runs_total{result="error"}[1h])) > 0
        for: 30m
        labels:
          severity: warning
        annotations:
          summary: "Scheduler failures for {{ $labels.slug }}"
          description: "Scheduler has been failing for {{ $labels.slug }} for 30+ minutes."
```

## Verifying the Setup

After starting cash-drugs with Prometheus scraping:

```bash
# 1. Check the metrics endpoint directly
curl -s http://localhost:8080/metrics | head -20

# 2. Verify Prometheus is scraping
curl -s http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | select(.labels.job=="cash-drugs")'

# 3. Query a metric
curl -s 'http://localhost:9090/api/v1/query?query=cashdrugs_mongodb_up' | jq '.data.result'

# 4. Generate some traffic to populate metrics
curl http://localhost:8080/api/cache/drugnames
curl http://localhost:8080/api/cache/nonexistent
curl http://localhost:8080/metrics | grep cashdrugs_http_requests_total
```

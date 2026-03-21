# Chaos Tests

Integration tests that exercise failure scenarios against a running Docker stack. These tests verify that resilience features (stale-serve, circuit breakers, concurrency limiting, graceful shutdown) work correctly.

## Prerequisites

- Docker stack running: `docker-compose up -d`
- Service healthy: `curl http://localhost:8080/health`

## Run

```bash
go test -tags chaos -v ./tests/chaos/
```

These tests are excluded from normal CI by the `chaos` build tag. They only run when explicitly requested.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CHAOS_BASE_URL` | `http://localhost:8080` | Target service URL |

## Test Summary

| Test | Automated | What it verifies |
|------|-----------|------------------|
| `TestChaos_StaleServeWhenMongoDBDown` | Yes (uses docker compose) | LRU/stale serve when MongoDB stops |
| `TestChaos_ConcurrencyExhaustion` | Yes | 503 + CD-S001 + Retry-After when concurrency limit exceeded |
| `TestChaos_MethodEnforcement` | Yes | 405 + CD-H004 for wrong HTTP methods |
| `TestChaos_ParamValidation` | Yes | 400 + CD-H003 for missing required params |
| `TestChaos_UnknownSlug` | Yes | 404 + CD-H001 for unconfigured slugs |
| `TestChaos_RequestIDPropagation` | Yes | X-Request-ID echoed or generated |
| `TestChaos_HealthBypassesLimiter` | Yes | /health, /ready, /metrics always reachable |
| `TestChaos_GracefulShutdown` | Manual | In-flight requests complete after SIGTERM |

## Manual Scenarios

### MongoDB Failure

```bash
# 1. Run the stale-serve test (primes cache first)
go test -tags chaos -v -run TestChaos_StaleServeWhenMongoDBDown ./tests/chaos/

# Or manually:
# Prime cache
curl http://localhost:8080/api/cache/drugclasses

# Kill MongoDB
docker compose stop mongodb

# Verify stale serve (data returned from LRU)
curl -v http://localhost:8080/api/cache/drugclasses

# Restore
docker compose start mongodb
```

### Concurrency Saturation

To force 503s more reliably, lower the concurrency limit:

```bash
# Restart service with low limit
MAX_CONCURRENT_REQUESTS=5 docker compose up -d drugs

# Run test
go test -tags chaos -v -run TestChaos_ConcurrencyExhaustion ./tests/chaos/

# Restore
docker compose up -d drugs
```

### Graceful Shutdown

```bash
# Start a slow request in the background
curl -v http://localhost:8080/api/cache/drugnames &

# Send SIGTERM while request is in flight
docker compose kill -s SIGTERM drugs

# Observe: the curl request should complete with 200
# New requests should fail with connection refused

# Restart
docker compose up -d drugs
```

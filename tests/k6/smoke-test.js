import http from 'k6/http';
import { check } from 'k6';

// Quick smoke test — verifies all endpoints respond correctly
// Usage: BASE_URL=http://localhost:8080 k6 run tests/k6/smoke-test.js
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate==1.0'], // all checks must pass
  },
};

export default function () {
  // Health
  const health = http.get(`${BASE_URL}/health`);
  check(health, {
    'GET /health: 200': (r) => r.status === 200,
    'GET /health: db connected': (r) => JSON.parse(r.body).db === 'connected',
  });

  // Readiness
  const ready = http.get(`${BASE_URL}/ready`);
  check(ready, {
    'GET /ready: 200': (r) => r.status === 200,
  });

  // Version
  const version = http.get(`${BASE_URL}/version`);
  check(version, {
    'GET /version: 200': (r) => r.status === 200,
    'GET /version: has version field': (r) => JSON.parse(r.body).version !== undefined,
    'GET /version: has X-Request-ID': (r) => r.headers['X-Request-Id'] !== undefined,
  });

  // Endpoints list
  const endpoints = http.get(`${BASE_URL}/api/endpoints`);
  check(endpoints, {
    'GET /api/endpoints: 200': (r) => r.status === 200,
    'GET /api/endpoints: returns array': (r) => Array.isArray(JSON.parse(r.body)),
  });

  // Cache status
  const status = http.get(`${BASE_URL}/api/cache/status`);
  check(status, {
    'GET /api/cache/status: 200': (r) => r.status === 200,
    'GET /api/cache/status: has slugs': (r) => JSON.parse(r.body).slugs !== undefined,
    'GET /api/cache/status: has total_slugs': (r) => JSON.parse(r.body).total_slugs > 0,
  });

  // Cached data endpoint
  const cached = http.get(`${BASE_URL}/api/cache/drugnames`);
  check(cached, {
    'GET /api/cache/drugnames: 200': (r) => r.status === 200,
    'GET /api/cache/drugnames: has data': (r) => JSON.parse(r.body).data !== undefined,
    'GET /api/cache/drugnames: has meta': (r) => JSON.parse(r.body).meta !== undefined,
    'GET /api/cache/drugnames: has X-Request-ID': (r) => r.headers['X-Request-Id'] !== undefined,
  });

  // Unknown slug returns 404 with error code
  const notFound = http.get(`${BASE_URL}/api/cache/nonexistent-slug`);
  check(notFound, {
    'GET /api/cache/unknown: 404': (r) => r.status === 404,
    'GET /api/cache/unknown: has error_code': (r) => JSON.parse(r.body).error_code === 'CD-H001',
  });

  // Metrics endpoint
  const metrics = http.get(`${BASE_URL}/metrics`);
  check(metrics, {
    'GET /metrics: 200': (r) => r.status === 200,
    'GET /metrics: has cashdrugs prefix': (r) => r.body.includes('cashdrugs_'),
  });

  // Swagger
  const swagger = http.get(`${BASE_URL}/swagger/index.html`);
  check(swagger, {
    'GET /swagger: 200': (r) => r.status === 200,
  });
}

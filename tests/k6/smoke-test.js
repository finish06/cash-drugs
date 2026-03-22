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

  // --- M15: New Endpoints ---

  // Per-slug metadata
  const meta = http.get(`${BASE_URL}/api/cache/drugnames/_meta`);
  check(meta, {
    'GET /_meta: 200': (r) => r.status === 200,
    'GET /_meta: has slug': (r) => JSON.parse(r.body).slug === 'drugnames',
    'GET /_meta: has is_stale': (r) => JSON.parse(r.body).is_stale !== undefined,
    'GET /_meta: has record_count': (r) => JSON.parse(r.body).record_count !== undefined,
    'GET /_meta: has circuit_state': (r) => JSON.parse(r.body).circuit_state !== undefined,
  });

  // Per-slug metadata — unknown slug
  const metaNotFound = http.get(`${BASE_URL}/api/cache/nonexistent/_meta`);
  check(metaNotFound, {
    'GET /_meta unknown: 404': (r) => r.status === 404,
    'GET /_meta unknown: CD-H001': (r) => JSON.parse(r.body).error_code === 'CD-H001',
  });

  // Bulk query API
  const bulkPayload = JSON.stringify({
    queries: [
      { params: { DRUGNAME: 'aspirin' } },
      { params: { DRUGNAME: 'ibuprofen' } },
    ],
  });
  const bulkRes = http.post(`${BASE_URL}/api/cache/spls-by-name/bulk`, bulkPayload, {
    headers: { 'Content-Type': 'application/json' },
  });
  check(bulkRes, {
    'POST /bulk: 200': (r) => r.status === 200,
    'POST /bulk: has results': (r) => JSON.parse(r.body).results !== undefined,
    'POST /bulk: has total_queries': (r) => JSON.parse(r.body).total_queries === 2,
    'POST /bulk: has request_id': (r) => JSON.parse(r.body).request_id !== undefined,
  });

  // Bulk query — empty queries
  const bulkEmpty = http.post(`${BASE_URL}/api/cache/drugnames/bulk`,
    JSON.stringify({ queries: [] }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  check(bulkEmpty, {
    'POST /bulk empty: 200': (r) => r.status === 200,
    'POST /bulk empty: total_queries 0': (r) => JSON.parse(r.body).total_queries === 0,
  });

  // Bulk query — unknown slug
  const bulkNotFound = http.post(`${BASE_URL}/api/cache/nonexistent/bulk`,
    JSON.stringify({ queries: [{ params: {} }] }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  check(bulkNotFound, {
    'POST /bulk unknown: 404': (r) => r.status === 404,
  });

  // Enhanced endpoints discovery
  const endpointsEnhanced = http.get(`${BASE_URL}/api/endpoints`);
  const epData = JSON.parse(endpointsEnhanced.body);
  check(endpointsEnhanced, {
    'GET /api/endpoints: has params as objects': (r) => {
      const ep = epData.find(e => e.params && e.params.length > 0);
      return ep && ep.params[0].name !== undefined;
    },
    'GET /api/endpoints: has example_url': (r) => epData[0].example_url !== undefined,
    'GET /api/endpoints: has cache_status': (r) => {
      const scheduled = epData.find(e => e.scheduled);
      return scheduled && scheduled.cache_status !== undefined;
    },
  });

  // Method enforcement — POST to GET endpoint returns 405
  const methodCheck = http.post(`${BASE_URL}/api/cache/drugnames/_meta`);
  check(methodCheck, {
    'POST to /_meta: 405': (r) => r.status === 405,
  });

  // Method enforcement — GET to bulk returns 405
  const bulkGetCheck = http.get(`${BASE_URL}/api/cache/drugnames/bulk`);
  check(bulkGetCheck, {
    'GET to /bulk: 405': (r) => r.status === 405,
  });

  // Missing params returns 400
  const missingParams = http.get(`${BASE_URL}/api/cache/spls-by-name`);
  check(missingParams, {
    'GET missing params: 400': (r) => r.status === 400,
    'GET missing params: CD-H003': (r) => JSON.parse(r.body).error_code === 'CD-H003',
  });

  // --- M17: Search, Autocomplete, Field Filtering ---

  // Cross-slug search
  const search = http.get(`${BASE_URL}/api/search?q=aspirin&limit=5`);
  check(search, {
    'GET /api/search: 200': (r) => r.status === 200,
    'GET /api/search: has query': (r) => JSON.parse(r.body).query === 'aspirin',
    'GET /api/search: has results': (r) => JSON.parse(r.body).results !== undefined,
    'GET /api/search: has total_matches': (r) => JSON.parse(r.body).total_matches !== undefined,
  });

  // Search — no query returns 400
  const searchNoQ = http.get(`${BASE_URL}/api/search`);
  check(searchNoQ, {
    'GET /api/search no q: 400': (r) => r.status === 400,
  });

  // Autocomplete
  const autocomplete = http.get(`${BASE_URL}/api/autocomplete?q=asp&limit=5`);
  check(autocomplete, {
    'GET /api/autocomplete: 200': (r) => r.status === 200,
    'GET /api/autocomplete: has suggestions': (r) => Array.isArray(JSON.parse(r.body).suggestions),
  });

  // Field filtering
  const filtered = http.get(`${BASE_URL}/api/cache/drugnames?fields=drug_name`);
  check(filtered, {
    'GET with fields: 200': (r) => r.status === 200,
    'GET with fields: has data': (r) => JSON.parse(r.body).data !== undefined,
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

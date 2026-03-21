import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const cacheHitRate = new Rate('cache_hits');
const staleDuration = new Trend('stale_response_time', true);

// Configuration — override with env vars:
//   K6_BASE_URL=http://192.168.1.145:8083 k6 run tests/k6/load-test.js
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export const options = {
  scenarios: {
    // Ramp up to steady state, hold, ramp down
    load: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '15s', target: 10 },   // ramp up
        { duration: '30s', target: 10 },   // steady state
        { duration: '30s', target: 30 },   // push higher
        { duration: '30s', target: 30 },   // hold at peak
        { duration: '15s', target: 0 },    // ramp down
      ],
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<2000', 'p(99)<5000'],                // P95 < 2s, P99 < 5s (includes upstream fetches)
    errors: ['rate<0.10'],                                          // error rate < 10% (force-refresh timeouts expected)
    'http_req_duration{endpoint:cached}': ['p(95)<500'],            // cached responses < 500ms P95 (homelab network)
    'http_req_duration{endpoint:status}': ['p(95)<200'],            // status endpoints < 200ms P95
    'http_req_duration{endpoint:meta}': ['p(95)<200'],              // metadata endpoints < 200ms P95
    'http_req_duration{endpoint:bulk}': ['p(95)<2000'],             // bulk queries < 2s P95
  },
};

// Endpoints to test — mix of cached and parameterized
const CACHED_ENDPOINTS = [
  '/api/cache/drugnames',
  '/api/cache/spls',
];

const PARAM_ENDPOINTS = [
  '/api/cache/spls-by-name?DRUGNAME=aspirin',
  '/api/cache/spls-by-name?DRUGNAME=ibuprofen',
  '/api/cache/spls-by-name?DRUGNAME=metformin',
];

const STATUS_ENDPOINTS = [
  '/health',
  '/ready',
  '/version',
  '/api/cache/status',
  '/api/endpoints',
];

const META_ENDPOINTS = [
  '/api/cache/drugnames/_meta',
  '/api/cache/spls/_meta',
  '/api/cache/fda-ndc/_meta',
];

export default function () {
  const scenario = Math.random();

  if (scenario < 0.3) {
    // 30% — cached bulk endpoints (should be fast, from LRU/MongoDB)
    testCachedEndpoint();
  } else if (scenario < 0.5) {
    // 20% — parameterized queries
    testParamEndpoint();
  } else if (scenario < 0.65) {
    // 15% — status/health endpoints
    testStatusEndpoint();
  } else if (scenario < 0.8) {
    // 15% — metadata endpoints (new M15)
    testMetaEndpoint();
  } else if (scenario < 0.9) {
    // 10% — bulk query (new M15)
    testBulkEndpoint();
  } else {
    // 10% — force refresh (triggers upstream fetch)
    testForceRefresh();
  }

  sleep(0.1 + Math.random() * 0.3); // 100-400ms between requests
}

function testCachedEndpoint() {
  const endpoint = CACHED_ENDPOINTS[Math.floor(Math.random() * CACHED_ENDPOINTS.length)];
  const res = http.get(`${BASE_URL}${endpoint}`, {
    tags: { endpoint: 'cached', slug: endpoint.split('/').pop() },
  });

  const passed = check(res, {
    'cached: status 200': (r) => r.status === 200,
    'cached: has data': (r) => {
      try { return JSON.parse(r.body).data !== undefined; }
      catch { return false; }
    },
    'cached: has meta': (r) => {
      try { return JSON.parse(r.body).meta !== undefined; }
      catch { return false; }
    },
    'cached: has X-Request-ID': (r) => r.headers['X-Request-Id'] !== undefined,
  });

  errorRate.add(!passed);

  // Track cache hit vs stale
  try {
    const body = JSON.parse(res.body);
    if (body.meta) {
      cacheHitRate.add(!body.meta.stale);
      if (body.meta.stale) {
        staleDuration.add(res.timings.duration);
      }
    }
  } catch {}
}

function testParamEndpoint() {
  const endpoint = PARAM_ENDPOINTS[Math.floor(Math.random() * PARAM_ENDPOINTS.length)];
  const res = http.get(`${BASE_URL}${endpoint}`, {
    tags: { endpoint: 'parameterized' },
  });

  const passed = check(res, {
    'param: status 200, 400, or 404': (r) => r.status === 200 || r.status === 400 || r.status === 404,
    'param: has X-Request-ID': (r) => r.headers['X-Request-Id'] !== undefined,
    'param: valid JSON': (r) => {
      try { JSON.parse(r.body); return true; }
      catch { return false; }
    },
  });

  // 404 from upstream is expected, not an error
  if (res.status === 404) {
    try {
      const body = JSON.parse(res.body);
      check(res, {
        'param 404: has error_code': () => body.error_code !== undefined,
      });
    } catch {}
  }

  errorRate.add(res.status >= 500);
}

function testStatusEndpoint() {
  const endpoint = STATUS_ENDPOINTS[Math.floor(Math.random() * STATUS_ENDPOINTS.length)];
  const res = http.get(`${BASE_URL}${endpoint}`, {
    tags: { endpoint: 'status' },
  });

  const passed = check(res, {
    'status: responds 200': (r) => r.status === 200,
    'status: valid JSON': (r) => {
      try { JSON.parse(r.body); return true; }
      catch { return false; }
    },
  });

  errorRate.add(!passed);
}

function testMetaEndpoint() {
  const endpoint = META_ENDPOINTS[Math.floor(Math.random() * META_ENDPOINTS.length)];
  const res = http.get(`${BASE_URL}${endpoint}`, {
    tags: { endpoint: 'meta' },
  });

  const passed = check(res, {
    'meta: responds 200': (r) => r.status === 200,
    'meta: has slug': (r) => {
      try { return JSON.parse(r.body).slug !== undefined; }
      catch { return false; }
    },
    'meta: has is_stale': (r) => {
      try { return JSON.parse(r.body).is_stale !== undefined; }
      catch { return false; }
    },
    'meta: has X-Request-ID': (r) => r.headers['X-Request-Id'] !== undefined,
  });

  errorRate.add(!passed);
}

function testBulkEndpoint() {
  const drugs = ['aspirin', 'ibuprofen', 'metformin', 'lisinopril', 'atorvastatin'];
  const queryCount = 2 + Math.floor(Math.random() * 4); // 2-5 queries
  const queries = [];
  for (let i = 0; i < queryCount; i++) {
    queries.push({ params: { DRUGNAME: drugs[Math.floor(Math.random() * drugs.length)] } });
  }

  const res = http.post(`${BASE_URL}/api/cache/spls-by-name/bulk`,
    JSON.stringify({ queries }),
    {
      headers: { 'Content-Type': 'application/json' },
      tags: { endpoint: 'bulk' },
    },
  );

  const passed = check(res, {
    'bulk: responds 200': (r) => r.status === 200,
    'bulk: has results': (r) => {
      try { return JSON.parse(r.body).results !== undefined; }
      catch { return false; }
    },
    'bulk: total_queries matches': (r) => {
      try { return JSON.parse(r.body).total_queries === queryCount; }
      catch { return false; }
    },
    'bulk: has request_id': (r) => {
      try { return JSON.parse(r.body).request_id !== ''; }
      catch { return false; }
    },
  });

  errorRate.add(!passed);
}

function testForceRefresh() {
  // Force refresh a parameterized endpoint — triggers upstream fetch
  const res = http.get(`${BASE_URL}/api/cache/spls-by-name?DRUGNAME=tylenol&_force=true`, {
    tags: { endpoint: 'force_refresh' },
    timeout: '10s',
  });

  const passed = check(res, {
    'force: responds': (r) => r.status < 500,
    'force: has X-Request-ID': (r) => r.headers['X-Request-Id'] !== undefined,
  });

  errorRate.add(res.status >= 500);
}


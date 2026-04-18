package handler_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/handler"
	"github.com/finish06/cash-drugs/internal/model"
)

// ---------------------------------------------------------------------------
// Handler Benchmarks — cache hit path (no upstream, no MongoDB)
// ---------------------------------------------------------------------------

// benchRepo is a minimal Repository that returns a pre-built response.
type benchRepo struct {
	resp *model.CachedResponse
}

func (r *benchRepo) Get(cacheKey string) (*model.CachedResponse, error) { return r.resp, nil }
func (r *benchRepo) Upsert(resp *model.CachedResponse) error           { return nil }
func (r *benchRepo) FetchedAt(cacheKey string) (time.Time, bool, error) {
	if r.resp != nil {
		return r.resp.FetchedAt, true, nil
	}
	return time.Time{}, false, nil
}

// benchFetcher is a no-op fetcher (should not be called on cache hits).
type benchFetcher struct{}

func (f *benchFetcher) Fetch(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
	return nil, nil
}

// BenchmarkCacheHandler_LRUHit measures the full handler path with an LRU cache hit.
// This is the fastest path: LRU hit -> JSON encode -> response.
func BenchmarkCacheHandler_LRUHit(b *testing.B) {
	b.ReportAllocs()

	resp := &model.CachedResponse{
		Slug:        "drugclasses",
		CacheKey:    "drugclasses",
		Data:        []interface{}{"class1", "class2", "class3"},
		ContentType: "application/json",
		FetchedAt:   time.Now(),
		SourceURL:   "http://api.fda.gov/drug/drugsfda.json",
		HTTPStatus:  200,
		PageCount:   1,
	}

	lru := cache.NewLRUCache(256 * 1024 * 1024)
	lru.Set("drugclasses", resp, 5*time.Minute)

	h := handler.NewCacheHandler(
		[]config.Endpoint{{Slug: "drugclasses", TTLDuration: 6 * time.Hour}},
		&benchRepo{resp: resp},
		&benchFetcher{},
		handler.WithLRU(lru),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugclasses", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}
}

// BenchmarkCacheHandler_MongoHit measures the handler path with an LRU miss but
// MongoDB cache hit (repo returns data directly).
func BenchmarkCacheHandler_MongoHit(b *testing.B) {
	b.ReportAllocs()

	resp := &model.CachedResponse{
		Slug:        "drugclasses",
		CacheKey:    "drugclasses",
		Data:        []interface{}{"class1", "class2", "class3"},
		ContentType: "application/json",
		FetchedAt:   time.Now(),
		SourceURL:   "http://api.fda.gov/drug/drugsfda.json",
		HTTPStatus:  200,
		PageCount:   1,
	}

	// No LRU -> falls through to repo
	h := handler.NewCacheHandler(
		[]config.Endpoint{{Slug: "drugclasses", TTLDuration: 6 * time.Hour}},
		&benchRepo{resp: resp},
		&benchFetcher{},
	)

	req := httptest.NewRequest("GET", "/api/cache/drugclasses", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}
}

// BenchmarkCacheHandler_404 measures the not-found endpoint path.
func BenchmarkCacheHandler_404(b *testing.B) {
	b.ReportAllocs()

	h := handler.NewCacheHandler(
		[]config.Endpoint{},
		&benchRepo{},
		&benchFetcher{},
	)

	req := httptest.NewRequest("GET", "/api/cache/nonexistent", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}
}

// BenchmarkCacheHandler_LargeResponse measures serialization of a large dataset.
func BenchmarkCacheHandler_LargeResponse(b *testing.B) {
	b.ReportAllocs()

	// Build a response with 1000 items
	items := make([]interface{}, 1000)
	for i := range items {
		items[i] = map[string]interface{}{
			"name":         "Drug Name Here",
			"type":         "ESTABLISHED_PHARMACOLOGIC_CLASS",
			"code":         "N0000175557",
			"codingSystem": "NDF-RT",
		}
	}

	resp := &model.CachedResponse{
		Slug:        "drugclasses",
		CacheKey:    "drugclasses",
		Data:        items,
		ContentType: "application/json",
		FetchedAt:   time.Now(),
		SourceURL:   "http://api.fda.gov/drug/drugsfda.json",
		HTTPStatus:  200,
		PageCount:   1,
	}

	lru := cache.NewLRUCache(256 * 1024 * 1024)
	lru.Set("drugclasses", resp, 5*time.Minute)

	h := handler.NewCacheHandler(
		[]config.Endpoint{{Slug: "drugclasses", TTLDuration: 6 * time.Hour}},
		&benchRepo{resp: resp},
		&benchFetcher{},
		handler.WithLRU(lru),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugclasses", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}
}

// BenchmarkHealthHandler measures the M20 stack-compliant /health path with a
// healthy MongoDB ping (2ms simulated latency — typical localhost).
func BenchmarkHealthHandler(b *testing.B) {
	b.ReportAllocs()

	h := handler.NewHealthHandler(
		&mockPinger{healthy: true, latency: 2 * time.Millisecond},
		handler.WithVersion("v1.0.0"),
		handler.WithHealthStartTime(time.Now().Add(-1*time.Hour)),
		handler.WithCacheSlugCount(20),
		handler.WithHealthLeader(true),
	)

	req := httptest.NewRequest("GET", "/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}
}

// BenchmarkHealthHandler_Unhealthy measures the /health error path (MongoDB
// disconnected). Hot-looped load balancers must still get fast 503s.
func BenchmarkHealthHandler_Unhealthy(b *testing.B) {
	b.ReportAllocs()

	h := handler.NewHealthHandler(
		&mockPinger{healthy: false},
		handler.WithVersion("v1.0.0"),
		handler.WithHealthStartTime(time.Now().Add(-1*time.Hour)),
		handler.WithCacheSlugCount(20),
		handler.WithHealthLeader(true),
	)

	req := httptest.NewRequest("GET", "/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}
}

// BenchmarkJSONEncode_APIResponse isolates JSON encoding cost for the response envelope.
func BenchmarkJSONEncode_APIResponse(b *testing.B) {
	b.ReportAllocs()

	items := make([]interface{}, 100)
	for i := range items {
		items[i] = map[string]interface{}{
			"name": "Drug",
			"code": "N123",
		}
	}

	resp := model.APIResponse{
		Data: items,
		Meta: model.ResponseMeta{
			Slug:         "drugclasses",
			SourceURL:    "http://api.fda.gov/drug/drugsfda.json",
			FetchedAt:    time.Now().Format("2006-01-02T15:04:05Z"),
			PageCount:    1,
			ResultsCount: len(items),
			Stale:        false,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		_ = json.NewEncoder(w).Encode(resp)
	}
}

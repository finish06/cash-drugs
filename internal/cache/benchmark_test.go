package cache

import (
	"fmt"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ---------------------------------------------------------------------------
// LRU Cache Benchmarks
// ---------------------------------------------------------------------------

// BenchmarkLRUGet_Hit measures Get latency on a populated cache (hot path).
func BenchmarkLRUGet_Hit(b *testing.B) {
	b.ReportAllocs()
	lru := NewShardedLRUCache(256*1024*1024, 16)
	resp := &model.CachedResponse{
		Slug:      "bench",
		CacheKey:  "bench-key",
		Data:      []interface{}{"item1", "item2", "item3"},
		FetchedAt: time.Now(),
		PageCount: 1,
	}
	lru.Set("bench-key", resp, 5*time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lru.Get("bench-key")
	}
}

// BenchmarkLRUGet_Miss measures Get latency when the key does not exist.
func BenchmarkLRUGet_Miss(b *testing.B) {
	b.ReportAllocs()
	lru := NewShardedLRUCache(256*1024*1024, 16)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lru.Get("nonexistent")
	}
}

// BenchmarkLRUSet measures Set throughput on a single key (update path).
func BenchmarkLRUSet(b *testing.B) {
	b.ReportAllocs()
	lru := NewShardedLRUCache(256*1024*1024, 16)
	resp := &model.CachedResponse{
		Slug:      "bench",
		CacheKey:  "bench-key",
		Data:      []interface{}{"item1"},
		PageCount: 1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lru.Set("bench-key", resp, 5*time.Minute)
	}
}

// BenchmarkLRUSet_UniqueKeys measures Set with unique keys to exercise eviction.
func BenchmarkLRUSet_UniqueKeys(b *testing.B) {
	b.ReportAllocs()
	// Small cache to trigger eviction frequently
	lru := NewShardedLRUCache(1*1024*1024, 16)
	resp := &model.CachedResponse{
		Slug:      "bench",
		CacheKey:  "bench-key",
		Data:      []interface{}{"item1"},
		PageCount: 1,
	}

	keys := make([]string, b.N)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lru.Set(keys[i], resp, 5*time.Minute)
	}
}

// BenchmarkLRUConcurrent simulates realistic mixed read/write load.
// 90% reads, 10% writes across 1000 pre-populated keys.
func BenchmarkLRUConcurrent(b *testing.B) {
	b.ReportAllocs()
	lru := NewShardedLRUCache(256*1024*1024, 16)
	resp := &model.CachedResponse{
		Slug:      "bench",
		Data:      []interface{}{"item"},
		PageCount: 1,
	}
	// Pre-populate
	for i := 0; i < 1000; i++ {
		lru.Set(fmt.Sprintf("key-%d", i), resp, 5*time.Minute)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%1000)
			if i%10 == 0 {
				lru.Set(key, resp, 5*time.Minute)
			} else {
				lru.Get(key)
			}
			i++
		}
	})
}

// BenchmarkLRUInvalidate measures Invalidate latency on existing keys.
func BenchmarkLRUInvalidate(b *testing.B) {
	b.ReportAllocs()
	lru := NewShardedLRUCache(256*1024*1024, 16)
	resp := &model.CachedResponse{
		Slug:      "bench",
		CacheKey:  "bench-key",
		PageCount: 1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		lru.Set(key, resp, 5*time.Minute)
		lru.Invalidate(key)
	}
}

// BenchmarkLRUSizeBytes measures SizeBytes aggregation across all shards.
func BenchmarkLRUSizeBytes(b *testing.B) {
	b.ReportAllocs()
	lru := NewShardedLRUCache(256*1024*1024, 16)
	resp := &model.CachedResponse{
		Slug:      "bench",
		PageCount: 1,
	}
	for i := 0; i < 100; i++ {
		lru.Set(fmt.Sprintf("key-%d", i), resp, 5*time.Minute)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lru.SizeBytes()
	}
}

// ---------------------------------------------------------------------------
// reassemblePages Benchmarks
// ---------------------------------------------------------------------------

// BenchmarkReassemblePages_10Pages benchmarks reassembly of 10 pages x 100 items.
func BenchmarkReassemblePages_10Pages(b *testing.B) {
	b.ReportAllocs()
	docs := make([]model.CachedResponse, 10)
	for i := range docs {
		items := make(bson.A, 100)
		for j := range items {
			items[j] = fmt.Sprintf("item-%d-%d", i, j)
		}
		docs[i] = model.CachedResponse{
			Slug:     "bench",
			CacheKey: fmt.Sprintf("bench:page:%d", i+1),
			Page:     i + 1,
			Data:     items,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reassemblePages(docs)
	}
}

// BenchmarkReassemblePages_SingleDoc benchmarks the fast path (single non-paginated doc).
func BenchmarkReassemblePages_SingleDoc(b *testing.B) {
	b.ReportAllocs()
	docs := []model.CachedResponse{
		{
			Slug:     "bench",
			CacheKey: "bench-key",
			Page:     0,
			Data:     "raw data",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reassemblePages(docs)
	}
}

// BenchmarkReassemblePages_100Pages benchmarks large-scale page reassembly.
func BenchmarkReassemblePages_100Pages(b *testing.B) {
	b.ReportAllocs()
	docs := make([]model.CachedResponse, 100)
	for i := range docs {
		items := make(bson.A, 50)
		for j := range items {
			items[j] = fmt.Sprintf("item-%d-%d", i, j)
		}
		docs[i] = model.CachedResponse{
			Slug: "bench",
			Page: i + 1,
			Data: items,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reassemblePages(docs)
	}
}

// ---------------------------------------------------------------------------
// estimateSize Benchmarks
// ---------------------------------------------------------------------------

// BenchmarkEstimateSize measures the heuristic size estimator.
func BenchmarkEstimateSize(b *testing.B) {
	b.ReportAllocs()
	resp := &model.CachedResponse{
		Slug:        "benchmark-slug",
		CacheKey:    "benchmark-cache-key",
		SourceURL:   "http://example.com/api/v1/data",
		ContentType: "application/json",
		PageCount:   5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		estimateSize(resp)
	}
}

// BenchmarkEstimateSize_NoPages measures estimateSize with nil Data, zero PageCount.
func BenchmarkEstimateSize_NoPages(b *testing.B) {
	b.ReportAllocs()
	resp := &model.CachedResponse{
		Slug:     "bench",
		CacheKey: "bench-key",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		estimateSize(resp)
	}
}

// ---------------------------------------------------------------------------
// BuildCacheKey Benchmarks
// ---------------------------------------------------------------------------

// BenchmarkBuildCacheKey_WithParams measures key construction with sorted params.
func BenchmarkBuildCacheKey_WithParams(b *testing.B) {
	b.ReportAllocs()
	params := map[string]string{"BRAND_NAME": "Tylenol", "NDC": "12345"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildCacheKey("fda-ndc", params)
	}
}

// BenchmarkBuildCacheKey_NoParams measures the slug-only fast path.
func BenchmarkBuildCacheKey_NoParams(b *testing.B) {
	b.ReportAllocs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildCacheKey("drugclasses", nil)
	}
}

// BenchmarkBuildCacheKey_ManyParams measures key construction with many params.
func BenchmarkBuildCacheKey_ManyParams(b *testing.B) {
	b.ReportAllocs()
	params := map[string]string{
		"BRAND_NAME":   "Tylenol",
		"NDC":          "12345",
		"GENERIC_NAME": "acetaminophen",
		"PHARM_CLASS":  "analgesic",
		"MANUFACTURER": "McNeil",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildCacheKey("fda-ndc", params)
	}
}

// ---------------------------------------------------------------------------
// extractBaseKey Benchmarks
// ---------------------------------------------------------------------------

// BenchmarkExtractBaseKey_WithPageSuffix measures stripping :page:N suffix.
func BenchmarkExtractBaseKey_WithPageSuffix(b *testing.B) {
	b.ReportAllocs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractBaseKey("drugnames:page:42")
	}
}

// BenchmarkExtractBaseKey_NoSuffix measures the no-op path (no :page: in key).
func BenchmarkExtractBaseKey_NoSuffix(b *testing.B) {
	b.ReportAllocs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractBaseKey("fda-ndc:NDC=12345")
	}
}

// ---------------------------------------------------------------------------
// buildUpsertFilter / buildSingleUpdate Benchmarks
// ---------------------------------------------------------------------------

// BenchmarkBuildUpsertFilter measures filter document construction.
func BenchmarkBuildUpsertFilter(b *testing.B) {
	b.ReportAllocs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildUpsertFilter("fda-ndc:NDC=12345")
	}
}

// BenchmarkBuildSingleUpdate measures full update document construction.
func BenchmarkBuildSingleUpdate(b *testing.B) {
	b.ReportAllocs()
	now := time.Now()
	resp := &model.CachedResponse{
		Slug:        "fda-ndc",
		Params:      map[string]string{"NDC": "12345"},
		CacheKey:    "fda-ndc:NDC=12345",
		Data:        []interface{}{"item1", "item2"},
		ContentType: "application/json",
		FetchedAt:   now.Add(-time.Hour),
		SourceURL:   "http://api.fda.gov/drug/ndc.json",
		HTTPStatus:  200,
		PageCount:   1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildSingleUpdate(resp, now)
	}
}

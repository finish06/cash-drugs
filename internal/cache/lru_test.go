package cache_test

import (
	"sync"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/model"
)

// AC: LRU Get/Set basic operations
func TestLRU_GetSetBasic(t *testing.T) {
	lru := cache.NewLRUCache(1024 * 1024) // 1MB

	resp := &model.CachedResponse{
		Slug:     "test",
		CacheKey: "test",
		Data:     map[string]interface{}{"items": []interface{}{"drug1"}},
	}

	lru.Set("test-key", resp, 5*time.Minute)

	got, ok := lru.Get("test-key")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Slug != "test" {
		t.Errorf("expected slug 'test', got '%s'", got.Slug)
	}
}

// AC: TTL expiry — expired entries return miss
func TestLRU_TTLExpiry(t *testing.T) {
	lru := cache.NewLRUCache(1024 * 1024)

	resp := &model.CachedResponse{
		Slug:     "test",
		CacheKey: "test",
		Data:     "data",
	}

	lru.Set("expire-key", resp, 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	_, ok := lru.Get("expire-key")
	if ok {
		t.Error("expected cache miss for expired entry")
	}
}

// AC: Size eviction — when full, LRU entry evicted
func TestLRU_SizeEviction(t *testing.T) {
	// Very small cache — 500 bytes
	lru := cache.NewLRUCache(500)

	resp1 := &model.CachedResponse{
		Slug:     "first",
		CacheKey: "first",
		Data:     string(make([]byte, 200)),
	}
	resp2 := &model.CachedResponse{
		Slug:     "second",
		CacheKey: "second",
		Data:     string(make([]byte, 200)),
	}
	resp3 := &model.CachedResponse{
		Slug:     "third",
		CacheKey: "third",
		Data:     string(make([]byte, 200)),
	}

	lru.Set("key1", resp1, 5*time.Minute)
	lru.Set("key2", resp2, 5*time.Minute)
	lru.Set("key3", resp3, 5*time.Minute) // Should evict key1

	_, ok := lru.Get("key1")
	if ok {
		t.Error("expected key1 to be evicted (LRU)")
	}

	_, ok = lru.Get("key3")
	if !ok {
		t.Error("expected key3 to still be in cache")
	}
}

// AC: SizeBytes() returns current usage
func TestLRU_SizeBytes(t *testing.T) {
	lru := cache.NewLRUCache(1024 * 1024)

	if lru.SizeBytes() != 0 {
		t.Errorf("expected 0 bytes for empty cache, got %d", lru.SizeBytes())
	}

	resp := &model.CachedResponse{
		Slug:     "test",
		CacheKey: "test",
		Data:     "some data",
	}
	lru.Set("key1", resp, 5*time.Minute)

	if lru.SizeBytes() <= 0 {
		t.Error("expected SizeBytes > 0 after adding entry")
	}
}

// AC: Invalidate — removes entry
func TestLRU_Invalidate(t *testing.T) {
	lru := cache.NewLRUCache(1024 * 1024)

	resp := &model.CachedResponse{
		Slug:     "test",
		CacheKey: "test",
		Data:     "data",
	}

	lru.Set("inv-key", resp, 5*time.Minute)

	_, ok := lru.Get("inv-key")
	if !ok {
		t.Fatal("expected cache hit before invalidation")
	}

	lru.Invalidate("inv-key")

	_, ok = lru.Get("inv-key")
	if ok {
		t.Error("expected cache miss after invalidation")
	}
}

// AC: Disabled when maxBytes = 0 (returns miss always)
func TestLRU_DisabledWhenZeroBytes(t *testing.T) {
	lru := cache.NewLRUCache(0)

	resp := &model.CachedResponse{
		Slug:     "test",
		CacheKey: "test",
		Data:     "data",
	}

	lru.Set("key1", resp, 5*time.Minute)

	_, ok := lru.Get("key1")
	if ok {
		t.Error("expected cache miss when LRU is disabled (maxBytes=0)")
	}

	if lru.SizeBytes() != 0 {
		t.Error("expected 0 size when LRU is disabled")
	}
}

// AC: Concurrent access safety
func TestLRU_ConcurrentAccess(t *testing.T) {
	lru := cache.NewLRUCache(1024 * 1024)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp := &model.CachedResponse{
				Slug:     "test",
				CacheKey: "test",
				Data:     "data",
			}
			key := "concurrent-key"
			lru.Set(key, resp, 5*time.Minute)
			lru.Get(key)
			lru.Invalidate(key)
			lru.SizeBytes()
		}(i)
	}
	wg.Wait()
	// No panic = pass
}

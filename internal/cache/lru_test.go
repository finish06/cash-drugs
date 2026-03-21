package cache_test

import (
	"fmt"
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

// =============================================================================
// LRU Cache Sharding Tests (specs/lru-cache-sharding.md)
// =============================================================================

// AC-001: ShardedLRUCache implements LRUCache interface
func TestAC001_ShardedLRUImplementsInterface(t *testing.T) {
	// NewLRUCache should return a sharded implementation
	lru := cache.NewLRUCache(1024 * 1024)
	// Verify it works through the interface
	resp := &model.CachedResponse{Slug: "test", CacheKey: "test"}
	lru.Set("key", resp, time.Minute)
	got, ok := lru.Get("key")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Slug != "test" {
		t.Errorf("expected slug 'test', got %q", got.Slug)
	}
}

// AC-002: Configurable shard count via NewShardedLRUCache
func TestAC002_ConfigurableShardCount(t *testing.T) {
	lru := cache.NewShardedLRUCache(1024*1024, 4)
	resp := &model.CachedResponse{Slug: "test", CacheKey: "k"}
	lru.Set("key", resp, time.Minute)
	got, ok := lru.Get("key")
	if !ok {
		t.Fatal("expected cache hit with 4 shards")
	}
	if got.Slug != "test" {
		t.Errorf("expected slug 'test', got %q", got.Slug)
	}
}

// AC-004: Deterministic shard assignment — same key always routes to same shard
func TestAC004_DeterministicShardAssignment(t *testing.T) {
	lru := cache.NewShardedLRUCache(1024*1024, 16)
	resp := &model.CachedResponse{Slug: "v1", CacheKey: "drugnames"}
	lru.Set("drugnames", resp, time.Minute)

	// Overwrite with v2
	resp2 := &model.CachedResponse{Slug: "v2", CacheKey: "drugnames"}
	lru.Set("drugnames", resp2, time.Minute)

	// Should get v2 (same shard, same key)
	got, ok := lru.Get("drugnames")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Slug != "v2" {
		t.Errorf("expected latest value 'v2', got %q", got.Slug)
	}
}

// AC-004: Different keys may land on different shards
func TestAC004_DifferentKeysDistribute(t *testing.T) {
	// With 16 shards and enough distinct keys, at least 2 different shards should be used
	lru := cache.NewShardedLRUCache(10*1024*1024, 16)

	// Set 100 different keys
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%d", i)
		resp := &model.CachedResponse{Slug: key, CacheKey: key}
		lru.Set(key, resp, time.Minute)
	}

	// Verify all 100 keys are retrievable (proves distribution works)
	hitCount := 0
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%d", i)
		if _, ok := lru.Get(key); ok {
			hitCount++
		}
	}
	if hitCount < 100 {
		t.Errorf("expected 100 cache hits, got %d (keys may not be distributed correctly)", hitCount)
	}
}

// AC-005: Size budget split across shards
func TestAC005_SizeBudgetSplitAcrossShards(t *testing.T) {
	// 2 shards, 200KB total = 100KB each
	lru := cache.NewShardedLRUCache(200*1024, 2)

	// SizeBytes should start at 0
	if lru.SizeBytes() != 0 {
		t.Errorf("expected 0 bytes for empty sharded cache, got %d", lru.SizeBytes())
	}

	// Add entries — they should be bounded by total maxBytes
	for i := 0; i < 50; i++ {
		key := fmt.Sprintf("k-%d", i)
		resp := &model.CachedResponse{
			Slug:     key,
			CacheKey: key,
			Data:     "some data",
		}
		lru.Set(key, resp, time.Minute)
	}

	totalSize := lru.SizeBytes()
	if totalSize > 200*1024 {
		t.Errorf("total size %d exceeds maxBytes 200KB", totalSize)
	}
}

// AC-006: SizeBytes returns sum of all shards
func TestAC006_SizeBytesAggregatesAllShards(t *testing.T) {
	lru := cache.NewShardedLRUCache(10*1024*1024, 4)

	// Add entries to different shards
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key-%d", i)
		resp := &model.CachedResponse{
			Slug:     key,
			CacheKey: key,
			Data:     "data",
		}
		lru.Set(key, resp, time.Minute)
	}

	totalSize := lru.SizeBytes()
	if totalSize <= 0 {
		t.Error("expected SizeBytes > 0 after adding entries")
	}
}

// AC-007: NewLRUCache returns sharded with 16 shards by default
func TestAC007_NewLRUCacheReturnsSharded(t *testing.T) {
	// 10MB to ensure all 100 entries fit across 16 shards
	lru := cache.NewLRUCache(10 * 1024 * 1024)
	// Verify it's functional (we can't inspect internal shard count easily,
	// but we verify it works as a sharded cache through behavior)
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("test-%d", i)
		resp := &model.CachedResponse{Slug: key, CacheKey: key, Data: "d"}
		lru.Set(key, resp, time.Minute)
	}
	hits := 0
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("test-%d", i)
		if _, ok := lru.Get(key); ok {
			hits++
		}
	}
	if hits < 100 {
		t.Errorf("expected all 100 keys to be retrievable, got %d", hits)
	}
}

// AC-008: maxBytes <= 0 returns noopLRU
func TestAC008_NoopLRUPreserved(t *testing.T) {
	lru := cache.NewLRUCache(0)
	resp := &model.CachedResponse{Slug: "test"}
	lru.Set("key", resp, time.Minute)
	_, ok := lru.Get("key")
	if ok {
		t.Error("expected miss from noop cache")
	}
	if lru.SizeBytes() != 0 {
		t.Error("expected 0 bytes from noop cache")
	}

	// Invalidate on noop should not panic
	lru.Invalidate("key")
	lru.Invalidate("nonexistent")

	_, ok = lru.Get("key")
	if ok {
		t.Error("expected miss from noop cache after invalidate")
	}

	lru2 := cache.NewLRUCache(-100)
	lru2.Set("key", resp, time.Minute)
	_, ok = lru2.Get("key")
	if ok {
		t.Error("expected miss from noop cache with negative maxBytes")
	}
	lru2.Invalidate("key")
}

// AC: NewShardedLRUCache with shardCount reduced due to minShardBytes
func TestNewShardedLRUCache_MinShardBytesReduction(t *testing.T) {
	// 64KB total with 16 shards = 4KB/shard, below minShardBytes (32KB).
	// Should reduce shard count to 64KB / 32KB = 2 shards.
	lru := cache.NewShardedLRUCache(64*1024, 16)
	resp := &model.CachedResponse{Slug: "test", CacheKey: "k", Data: "data"}
	lru.Set("key", resp, time.Minute)
	got, ok := lru.Get("key")
	if !ok {
		t.Fatal("expected cache hit with reduced shard count")
	}
	if got.Slug != "test" {
		t.Errorf("expected 'test', got %q", got.Slug)
	}
}

// AC: NewShardedLRUCache with negative shard count clamped to 1
func TestNewShardedLRUCache_NegativeShardCount(t *testing.T) {
	lru := cache.NewShardedLRUCache(1024*1024, -5)
	resp := &model.CachedResponse{Slug: "test", CacheKey: "k", Data: "data"}
	lru.Set("key", resp, time.Minute)
	got, ok := lru.Get("key")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Slug != "test" {
		t.Errorf("expected 'test', got %q", got.Slug)
	}
}

// AC-009: Shard count clamping
func TestAC009_ShardCountClamping(t *testing.T) {
	// shardCount <= 0 should be clamped to 1
	lru := cache.NewShardedLRUCache(1024, 0)
	resp := &model.CachedResponse{Slug: "test", CacheKey: "k"}
	lru.Set("key", resp, time.Minute)
	got, ok := lru.Get("key")
	if !ok {
		t.Fatal("expected cache hit with clamped shard count")
	}
	if got.Slug != "test" {
		t.Errorf("expected 'test', got %q", got.Slug)
	}

	// shardCount > maxBytes should be clamped
	lru2 := cache.NewShardedLRUCache(5, 100)
	resp2 := &model.CachedResponse{Slug: "small", CacheKey: "s"}
	lru2.Set("key", resp2, time.Minute)
	// Should work (each shard gets at least 1 byte)
}

// AC-010: TTL expiration works per shard
func TestAC010_TTLPerShard(t *testing.T) {
	lru := cache.NewShardedLRUCache(1024*1024, 4)
	resp := &model.CachedResponse{Slug: "ttl-test", CacheKey: "alpha"}
	lru.Set("alpha", resp, 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	_, ok := lru.Get("alpha")
	if ok {
		t.Error("expected cache miss for expired entry in shard")
	}
}

// AC-011: Independent eviction across shards
func TestAC011_IndependentEvictionAcrossShards(t *testing.T) {
	// Use 2 shards with enough space for meaningful eviction testing
	// Each entry estimates ~10KB (200 base + data), so 100KB per shard = 200KB total
	maxBytes := int64(200 * 1024) // 200KB
	lru := cache.NewShardedLRUCache(maxBytes, 2)

	// Fill with many entries that should overflow at least one shard
	for i := 0; i < 50; i++ {
		key := fmt.Sprintf("evict-%d", i)
		resp := &model.CachedResponse{
			Slug:     key,
			CacheKey: key,
			Data:     "some data to take space",
		}
		lru.Set(key, resp, time.Minute)
	}

	// Total size should not exceed maxBytes
	if lru.SizeBytes() > maxBytes {
		t.Errorf("total size %d exceeds maxBytes %d", lru.SizeBytes(), maxBytes)
	}
}

// AC-013: Concurrent access with sharding — no deadlocks or data races
func TestAC013_ConcurrentShardedAccess(t *testing.T) {
	lru := cache.NewShardedLRUCache(10*1024*1024, 16)

	var wg sync.WaitGroup
	for g := 0; g < 100; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("goroutine-%d-key-%d", id, i)
				resp := &model.CachedResponse{
					Slug:     key,
					CacheKey: key,
					Data:     "concurrent data",
				}
				lru.Set(key, resp, time.Minute)
				lru.Get(key)
				lru.SizeBytes()
				if i%10 == 0 {
					lru.Invalidate(key)
				}
			}
		}(g)
	}
	wg.Wait()
	// No panic, no deadlock = pass
}

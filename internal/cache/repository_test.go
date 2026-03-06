package cache_test

import (
	"testing"

	"github.com/finish06/drugs/internal/cache"
)

// AC-007: Build cache key from slug only
func TestAC007_CacheKeySlugOnly(t *testing.T) {
	key := cache.BuildCacheKey("drugnames", nil)
	if key != "drugnames" {
		t.Errorf("expected cache key 'drugnames', got '%s'", key)
	}
}

// AC-007: Build cache key from slug + params
func TestAC007_CacheKeyWithParams(t *testing.T) {
	params := map[string]string{"SETID": "abc-123"}
	key := cache.BuildCacheKey("spl-detail", params)
	if key != "spl-detail:SETID=abc-123" {
		t.Errorf("expected cache key 'spl-detail:SETID=abc-123', got '%s'", key)
	}
}

// AC-007: Cache key with multiple params is deterministic
func TestAC007_CacheKeyMultipleParamsDeterministic(t *testing.T) {
	params := map[string]string{"B": "2", "A": "1"}
	key1 := cache.BuildCacheKey("test", params)
	key2 := cache.BuildCacheKey("test", params)
	if key1 != key2 {
		t.Errorf("cache keys should be deterministic, got '%s' and '%s'", key1, key2)
	}
	// Should be sorted alphabetically
	expected := "test:A=1:B=2"
	if key1 != expected {
		t.Errorf("expected '%s', got '%s'", expected, key1)
	}
}

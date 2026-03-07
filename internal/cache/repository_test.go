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

// BuildCacheKey edge case: empty slug with no params
func TestBuildCacheKey_EmptySlug(t *testing.T) {
	key := cache.BuildCacheKey("", nil)
	if key != "" {
		t.Errorf("expected empty string for empty slug with nil params, got %q", key)
	}
}

// BuildCacheKey edge case: empty slug with params
func TestBuildCacheKey_EmptySlugWithParams(t *testing.T) {
	params := map[string]string{"key": "val"}
	key := cache.BuildCacheKey("", params)
	expected := ":key=val"
	if key != expected {
		t.Errorf("expected %q, got %q", expected, key)
	}
}

// BuildCacheKey edge case: single param
func TestBuildCacheKey_SingleParam(t *testing.T) {
	params := map[string]string{"SETID": "abc-123"}
	key := cache.BuildCacheKey("my-slug", params)
	expected := "my-slug:SETID=abc-123"
	if key != expected {
		t.Errorf("expected %q, got %q", expected, key)
	}
}

// BuildCacheKey edge case: params with special characters (regex metacharacters)
func TestBuildCacheKey_ParamsWithSpecialChars(t *testing.T) {
	params := map[string]string{"q": "foo.bar+baz*"}
	key := cache.BuildCacheKey("search", params)
	// BuildCacheKey does not escape; it stores raw values
	expected := "search:q=foo.bar+baz*"
	if key != expected {
		t.Errorf("expected %q, got %q", expected, key)
	}
}

// BuildCacheKey edge case: params are sorted alphabetically regardless of insertion order
func TestBuildCacheKey_ParamsSortedAlphabetically(t *testing.T) {
	params := map[string]string{"zebra": "z", "alpha": "a", "middle": "m"}
	key := cache.BuildCacheKey("slug", params)
	expected := "slug:alpha=a:middle=m:zebra=z"
	if key != expected {
		t.Errorf("expected %q, got %q", expected, key)
	}
}

// BuildCacheKey edge case: empty map (not nil) behaves same as nil
func TestBuildCacheKey_EmptyMap(t *testing.T) {
	key := cache.BuildCacheKey("slug", map[string]string{})
	if key != "slug" {
		t.Errorf("expected 'slug' for empty map, got %q", key)
	}
}

// BuildCacheKey edge case: param values containing colons and equals signs
func TestBuildCacheKey_ParamsWithDelimiterChars(t *testing.T) {
	params := map[string]string{"url": "http://example.com:8080/path?q=1"}
	key := cache.BuildCacheKey("api", params)
	expected := "api:url=http://example.com:8080/path?q=1"
	if key != expected {
		t.Errorf("expected %q, got %q", expected, key)
	}
}

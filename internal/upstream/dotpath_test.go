package upstream

import "testing"

func TestResolveByDotPath_SingleKey(t *testing.T) {
	parsed := map[string]interface{}{
		"total": float64(42),
	}
	val, ok := resolveByDotPath(parsed, "total")
	if !ok {
		t.Fatal("expected ok=true for single key")
	}
	if val != float64(42) {
		t.Errorf("expected 42, got %v", val)
	}
}

func TestResolveByDotPath_NestedKeys(t *testing.T) {
	parsed := map[string]interface{}{
		"meta": map[string]interface{}{
			"results": map[string]interface{}{
				"total": float64(100),
			},
		},
	}
	val, ok := resolveByDotPath(parsed, "meta.results.total")
	if !ok {
		t.Fatal("expected ok=true for nested path")
	}
	if val != float64(100) {
		t.Errorf("expected 100, got %v", val)
	}
}

func TestResolveByDotPath_MissingKey(t *testing.T) {
	parsed := map[string]interface{}{
		"meta": map[string]interface{}{},
	}
	_, ok := resolveByDotPath(parsed, "meta.results.total")
	if ok {
		t.Fatal("expected ok=false for missing nested key")
	}
}

func TestResolveByDotPath_NonMapIntermediate(t *testing.T) {
	parsed := map[string]interface{}{
		"meta": "not a map",
	}
	_, ok := resolveByDotPath(parsed, "meta.results.total")
	if ok {
		t.Fatal("expected ok=false when intermediate is not a map")
	}
}

func TestResolveByDotPath_TwoLevelPath(t *testing.T) {
	parsed := map[string]interface{}{
		"metadata": map[string]interface{}{
			"total_pages": float64(5),
		},
	}
	val, ok := resolveByDotPath(parsed, "metadata.total_pages")
	if !ok {
		t.Fatal("expected ok=true for two-level path")
	}
	if val != float64(5) {
		t.Errorf("expected 5, got %v", val)
	}
}

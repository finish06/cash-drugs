package handler_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/finish06/cash-drugs/internal/handler"
)

// AC-001: warmup-queries.yaml defines parameterized queries grouped by slug
func TestAC001_LoadWarmupQueries_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "warmup-queries.yaml")
	content := `fda-ndc:
  - GENERIC_NAME: METFORMIN
  - GENERIC_NAME: LISINOPRIL
rxnorm-find-drug:
  - DRUG_NAME: metformin
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	queries, err := handler.LoadWarmupQueries(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(queries) != 2 {
		t.Errorf("expected 2 slugs, got %d", len(queries))
	}
	if len(queries["fda-ndc"]) != 2 {
		t.Errorf("expected 2 fda-ndc queries, got %d", len(queries["fda-ndc"]))
	}
	if len(queries["rxnorm-find-drug"]) != 1 {
		t.Errorf("expected 1 rxnorm-find-drug query, got %d", len(queries["rxnorm-find-drug"]))
	}

	// Verify param values
	if queries["fda-ndc"][0]["GENERIC_NAME"] != "METFORMIN" {
		t.Errorf("expected METFORMIN, got %s", queries["fda-ndc"][0]["GENERIC_NAME"])
	}
}

// AC-011: Missing warmup-queries.yaml returns empty map, no error
func TestAC011_LoadWarmupQueries_MissingFile(t *testing.T) {
	queries, err := handler.LoadWarmupQueries("/nonexistent/warmup-queries.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if len(queries) != 0 {
		t.Errorf("expected empty map for missing file, got %d slugs", len(queries))
	}
}

// Edge case: malformed YAML returns error
func TestLoadWarmupQueries_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "warmup-queries.yaml")
	content := `{{{invalid yaml`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := handler.LoadWarmupQueries(path)
	if err == nil {
		t.Error("expected error for malformed YAML, got nil")
	}
}

// AC-012: Duplicate queries are deduplicated
func TestAC012_LoadWarmupQueries_Deduplication(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "warmup-queries.yaml")
	content := `fda-ndc:
  - GENERIC_NAME: METFORMIN
  - GENERIC_NAME: METFORMIN
  - GENERIC_NAME: LISINOPRIL
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	queries, err := handler.LoadWarmupQueries(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(queries["fda-ndc"]) != 2 {
		t.Errorf("expected 2 unique fda-ndc queries after dedup, got %d", len(queries["fda-ndc"]))
	}
}

// AC-002: Each entry specifies query parameter key-value pairs
func TestAC002_LoadWarmupQueries_MultipleParamKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "warmup-queries.yaml")
	content := `fda-ndc:
  - GENERIC_NAME: METFORMIN
  - BRAND_NAME: ASPIRIN
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	queries, err := handler.LoadWarmupQueries(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(queries["fda-ndc"]) != 2 {
		t.Errorf("expected 2 queries, got %d", len(queries["fda-ndc"]))
	}

	// Second entry should have BRAND_NAME key
	if queries["fda-ndc"][1]["BRAND_NAME"] != "ASPIRIN" {
		t.Errorf("expected BRAND_NAME=ASPIRIN, got %v", queries["fda-ndc"][1])
	}
}

// Edge case: empty query list for a slug
func TestLoadWarmupQueries_EmptySlug(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "warmup-queries.yaml")
	content := `fda-ndc:
rxnorm-find-drug:
  - DRUG_NAME: metformin
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	queries, err := handler.LoadWarmupQueries(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Empty slug should have 0 entries (or be absent)
	if len(queries["fda-ndc"]) != 0 {
		t.Errorf("expected 0 queries for empty slug, got %d", len(queries["fda-ndc"]))
	}
	if len(queries["rxnorm-find-drug"]) != 1 {
		t.Errorf("expected 1 query for rxnorm-find-drug, got %d", len(queries["rxnorm-find-drug"]))
	}
}

// TotalQueryCount returns the total number of queries across all slugs
func TestTotalQueryCount(t *testing.T) {
	queries := map[string][]map[string]string{
		"fda-ndc":          {{"GENERIC_NAME": "METFORMIN"}, {"GENERIC_NAME": "LISINOPRIL"}},
		"rxnorm-find-drug": {{"DRUG_NAME": "metformin"}},
	}
	count := handler.TotalQueryCount(queries)
	if count != 3 {
		t.Errorf("expected total count 3, got %d", count)
	}
}

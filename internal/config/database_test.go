package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/finish06/drugs/internal/config"
)

// AC-004: Connection string from MONGO_URI env var
func TestAC004_MongoURIFromEnvVar(t *testing.T) {
	t.Setenv("MONGO_URI", "mongodb://envhost:27017/drugs")

	uri, err := config.ResolveMongoURI("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uri != "mongodb://envhost:27017/drugs" {
		t.Errorf("expected URI from env var, got '%s'", uri)
	}
}

// AC-004: Fallback to config file when MONGO_URI not set
func TestAC004_MongoURIFallbackToConfig(t *testing.T) {
	t.Setenv("MONGO_URI", "")

	cfgPath := writeTestConfigWithDB(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
database:
  uri: mongodb://confighost:27017/drugs
`)

	uri, err := config.ResolveMongoURI(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uri != "mongodb://confighost:27017/drugs" {
		t.Errorf("expected URI from config, got '%s'", uri)
	}
}

// AC-004: Env var takes precedence over config
func TestAC004_EnvVarTakesPrecedence(t *testing.T) {
	t.Setenv("MONGO_URI", "mongodb://envhost:27017/drugs")

	cfgPath := writeTestConfigWithDB(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
database:
  uri: mongodb://confighost:27017/drugs
`)

	uri, err := config.ResolveMongoURI(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uri != "mongodb://envhost:27017/drugs" {
		t.Errorf("expected env var URI to take precedence, got '%s'", uri)
	}
}

// AC-005: Error when neither env var nor config has URI
func TestAC005_NoURIConfigured(t *testing.T) {
	t.Setenv("MONGO_URI", "")

	cfgPath := writeTestConfigWithDB(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

	_, err := config.ResolveMongoURI(cfgPath)
	if err == nil {
		t.Fatal("expected error when no MongoDB URI is configured")
	}
}

// AC-005: Error when config file doesn't exist and no env var
func TestAC005_NoConfigFileNoEnvVar(t *testing.T) {
	t.Setenv("MONGO_URI", "")

	_, err := config.ResolveMongoURI("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error when config file doesn't exist and no env var")
	}
}

func writeTestConfigWithDB(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

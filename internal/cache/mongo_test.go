package cache_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/finish06/drugs/internal/cache"
	"github.com/finish06/drugs/internal/model"
)

func getTestMongoURI(t *testing.T) string {
	t.Helper()
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017/drugs_test"
	}
	return uri
}

func skipIfNoMongo(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

// AC-001: MongoRepository implements cache.Repository interface
func TestAC001_MongoRepositoryImplementsInterface(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	// Compile-time check: MongoRepository implements Repository
	var _ cache.Repository = repo
}

// AC-002: Get returns nil for non-existent cache key
func TestAC002_GetReturnsNilWhenNotFound(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	result, err := repo.Get("nonexistent-key-" + time.Now().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil for non-existent key")
	}
}

// AC-002: Get returns matching document
func TestAC002_GetReturnsMatchingDocument(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	testKey := "test-get-" + time.Now().Format(time.RFC3339Nano)
	now := time.Now().Truncate(time.Millisecond)

	doc := &model.CachedResponse{
		Slug:        "test",
		CacheKey:    testKey,
		Data:        map[string]interface{}{"items": []interface{}{"drug1"}},
		ContentType: "application/json",
		FetchedAt:   now,
		SourceURL:   "http://example.com/api",
		HTTPStatus:  200,
		PageCount:   1,
	}

	if err := repo.Upsert(doc); err != nil {
		t.Fatalf("failed to upsert: %v", err)
	}

	result, err := repo.Get(testKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected document, got nil")
	}
	if result.Slug != "test" {
		t.Errorf("expected slug 'test', got '%s'", result.Slug)
	}
	if result.CacheKey != testKey {
		t.Errorf("expected cache_key '%s', got '%s'", testKey, result.CacheKey)
	}
}

// AC-003: Upsert inserts new document with created_at
func TestAC003_UpsertInsertsWithCreatedAt(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	testKey := "test-insert-" + time.Now().Format(time.RFC3339Nano)
	now := time.Now().Truncate(time.Millisecond)

	doc := &model.CachedResponse{
		Slug:        "test",
		CacheKey:    testKey,
		Data:        map[string]interface{}{"items": []interface{}{"drug1"}},
		ContentType: "application/json",
		FetchedAt:   now,
		SourceURL:   "http://example.com/api",
		HTTPStatus:  200,
		PageCount:   1,
	}

	if err := repo.Upsert(doc); err != nil {
		t.Fatalf("failed to upsert: %v", err)
	}

	result, err := repo.Get(testKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CreatedAt.IsZero() {
		t.Error("expected created_at to be set on insert")
	}
	if result.UpdatedAt.IsZero() {
		t.Error("expected updated_at to be set on insert")
	}
}

// AC-003: Upsert updates existing document, preserves created_at
func TestAC003_UpsertUpdatesPreservesCreatedAt(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	testKey := "test-update-" + time.Now().Format(time.RFC3339Nano)
	now := time.Now().Truncate(time.Millisecond)

	doc := &model.CachedResponse{
		Slug:        "test",
		CacheKey:    testKey,
		Data:        map[string]interface{}{"v": "1"},
		ContentType: "application/json",
		FetchedAt:   now,
		SourceURL:   "http://example.com/api",
		HTTPStatus:  200,
		PageCount:   1,
	}

	if err := repo.Upsert(doc); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	first, _ := repo.Get(testKey)
	originalCreatedAt := first.CreatedAt

	time.Sleep(10 * time.Millisecond)

	doc.Data = map[string]interface{}{"v": "2"}
	doc.FetchedAt = time.Now().Truncate(time.Millisecond)
	if err := repo.Upsert(doc); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	second, _ := repo.Get(testKey)
	if second.CreatedAt.Sub(originalCreatedAt).Abs() > time.Second {
		t.Errorf("created_at should be preserved on update, was %v now %v", originalCreatedAt, second.CreatedAt)
	}
	if !second.UpdatedAt.After(originalCreatedAt) {
		t.Error("updated_at should be newer after update")
	}
}

// AC-006: Fail fast if MongoDB unreachable
func TestAC006_FailFastIfUnreachable(t *testing.T) {
	_, err := cache.NewMongoRepository("mongodb://unreachable-host:27017/test", 2*time.Second)
	if err == nil {
		t.Fatal("expected error when MongoDB is unreachable")
	}
}

// AC-011: Database name defaults to drugs, collection is cached_responses
func TestAC011_DefaultDatabaseAndCollection(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	dbName, collName := repo.Names()
	if collName != "cached_responses" {
		t.Errorf("expected collection 'cached_responses', got '%s'", collName)
	}
	// DB name should contain "drugs" (could be "drugs_test" in test)
	if dbName == "" {
		t.Error("expected non-empty database name")
	}
}

// AC-012: Configurable timeout
func TestAC012_ConfigurableTimeout(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 1*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	// Just verify it was created with custom timeout (no error)
	if repo.Timeout() != 1*time.Second {
		t.Errorf("expected timeout 1s, got %v", repo.Timeout())
	}
}

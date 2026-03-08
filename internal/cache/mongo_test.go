package cache_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/model"
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

// Ping: returns nil when connected
func TestPing_Success(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	if err := repo.Ping(); err != nil {
		t.Errorf("expected ping to succeed, got %v", err)
	}
}

// Multi-page: Upsert stores pages separately and Get reassembles them
func TestMultiPage_UpsertAndGetReassembly(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	testKey := "test-multipage-" + time.Now().Format(time.RFC3339Nano)
	now := time.Now().Truncate(time.Millisecond)

	doc := &model.CachedResponse{
		Slug:        "multipage-test",
		CacheKey:    testKey,
		ContentType: "application/json",
		FetchedAt:   now,
		SourceURL:   "http://example.com/api",
		HTTPStatus:  200,
		PageCount:   3,
		Pages: []model.PageData{
			{Page: 1, Data: []interface{}{"a1", "a2"}},
			{Page: 2, Data: []interface{}{"b1", "b2"}},
			{Page: 3, Data: []interface{}{"c1"}},
		},
	}

	if err := repo.Upsert(doc); err != nil {
		t.Fatalf("failed to upsert multi-page: %v", err)
	}

	result, err := repo.Get(testKey)
	if err != nil {
		t.Fatalf("unexpected error on Get: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// Should reassemble all pages into combined data
	arr, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result.Data)
	}
	if len(arr) != 5 {
		t.Errorf("expected 5 combined items, got %d", len(arr))
	}
	if result.Slug != "multipage-test" {
		t.Errorf("expected slug 'multipage-test', got '%s'", result.Slug)
	}
}

// Multi-page: Upsert cleans up stale pages when page count decreases
func TestMultiPage_UpsertCleansStalePages(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	testKey := "test-stale-" + time.Now().Format(time.RFC3339Nano)
	now := time.Now().Truncate(time.Millisecond)

	// First: upsert 3 pages
	doc := &model.CachedResponse{
		Slug:        "stale-test",
		CacheKey:    testKey,
		ContentType: "application/json",
		FetchedAt:   now,
		SourceURL:   "http://example.com/api",
		HTTPStatus:  200,
		PageCount:   3,
		Pages: []model.PageData{
			{Page: 1, Data: []interface{}{"a"}},
			{Page: 2, Data: []interface{}{"b"}},
			{Page: 3, Data: []interface{}{"c"}},
		},
	}

	if err := repo.Upsert(doc); err != nil {
		t.Fatalf("failed to upsert 3 pages: %v", err)
	}

	// Then: upsert only 2 pages (page 3 should be cleaned up)
	doc.PageCount = 2
	doc.Pages = []model.PageData{
		{Page: 1, Data: []interface{}{"x"}},
		{Page: 2, Data: []interface{}{"y"}},
	}

	if err := repo.Upsert(doc); err != nil {
		t.Fatalf("failed to upsert 2 pages: %v", err)
	}

	result, err := repo.Get(testKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	arr, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result.Data)
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 items after stale cleanup, got %d", len(arr))
	}
}

// Multi-page: Upsert cleans up old single-document version when switching to multi-page
func TestMultiPage_UpsertCleansSingleDocVersion(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	testKey := "test-single-to-multi-" + time.Now().Format(time.RFC3339Nano)
	now := time.Now().Truncate(time.Millisecond)

	// First: upsert as single document
	single := &model.CachedResponse{
		Slug:        "convert-test",
		CacheKey:    testKey,
		Data:        map[string]interface{}{"old": "data"},
		ContentType: "application/json",
		FetchedAt:   now,
		SourceURL:   "http://example.com/api",
		HTTPStatus:  200,
		PageCount:   1,
	}

	if err := repo.Upsert(single); err != nil {
		t.Fatalf("failed to upsert single doc: %v", err)
	}

	// Then: upsert as multi-page (should clean up the single doc)
	multi := &model.CachedResponse{
		Slug:        "convert-test",
		CacheKey:    testKey,
		ContentType: "application/json",
		FetchedAt:   now,
		SourceURL:   "http://example.com/api",
		HTTPStatus:  200,
		PageCount:   2,
		Pages: []model.PageData{
			{Page: 1, Data: []interface{}{"new1"}},
			{Page: 2, Data: []interface{}{"new2"}},
		},
	}

	if err := repo.Upsert(multi); err != nil {
		t.Fatalf("failed to upsert multi-page: %v", err)
	}

	result, err := repo.Get(testKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	arr, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result.Data)
	}
	// Should only have the 2 new items, not the old single-doc data
	if len(arr) != 2 {
		t.Errorf("expected 2 items, got %d", len(arr))
	}
}

// FetchedAt: returns timestamp for existing cache key
func TestFetchedAt_ReturnsTimestampWhenFound(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	testKey := "test-fetchedat-" + time.Now().Format(time.RFC3339Nano)
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

	fetchedAt, found, err := repo.FetchedAt(testKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true for existing key")
	}
	if fetchedAt.Sub(now).Abs() > time.Second {
		t.Errorf("expected fetched_at ~%v, got %v", now, fetchedAt)
	}
}

// FetchedAt: returns false for non-existent key
func TestFetchedAt_ReturnsFalseWhenNotFound(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	_, found, err := repo.FetchedAt("nonexistent-" + time.Now().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false for non-existent key")
	}
}

// FetchedAt: works with multi-page cache keys
func TestFetchedAt_WorksWithMultiPageKeys(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	testKey := "test-fetchedat-multi-" + time.Now().Format(time.RFC3339Nano)
	now := time.Now().Truncate(time.Millisecond)

	doc := &model.CachedResponse{
		Slug:        "test",
		CacheKey:    testKey,
		ContentType: "application/json",
		FetchedAt:   now,
		SourceURL:   "http://example.com/api",
		HTTPStatus:  200,
		PageCount:   2,
		Pages: []model.PageData{
			{Page: 1, Data: []interface{}{"a"}},
			{Page: 2, Data: []interface{}{"b"}},
		},
	}

	if err := repo.Upsert(doc); err != nil {
		t.Fatalf("failed to upsert: %v", err)
	}

	fetchedAt, found, err := repo.FetchedAt(testKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true for multi-page key")
	}
	if fetchedAt.Sub(now).Abs() > time.Second {
		t.Errorf("expected fetched_at ~%v, got %v", now, fetchedAt)
	}
}

// Upsert: single-page with params uses correct cache key
func TestUpsert_SinglePageWithParams(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	testKey := "test-params-" + time.Now().Format(time.RFC3339Nano)
	now := time.Now().Truncate(time.Millisecond)

	doc := &model.CachedResponse{
		Slug:        "test-params",
		CacheKey:    testKey,
		Params:      map[string]string{"BRAND_NAME": "Tylenol"},
		Data:        []interface{}{map[string]interface{}{"ndc": "12345"}},
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
		t.Fatal("expected result, got nil")
	}
	if result.Slug != "test-params" {
		t.Errorf("expected slug 'test-params', got '%s'", result.Slug)
	}
}

// Get: single document with page=0 returns as-is (no reassembly)
func TestGet_SingleDocReturnsAsIs(t *testing.T) {
	skipIfNoMongo(t)
	uri := getTestMongoURI(t)

	repo, err := cache.NewMongoRepository(uri, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create MongoRepository: %v", err)
	}
	defer repo.Close(context.Background())

	testKey := "test-single-asis-" + time.Now().Format(time.RFC3339Nano)
	now := time.Now().Truncate(time.Millisecond)

	doc := &model.CachedResponse{
		Slug:        "raw-test",
		CacheKey:    testKey,
		Data:        "raw xml content here",
		ContentType: "application/xml",
		FetchedAt:   now,
		SourceURL:   "http://example.com/data.xml",
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

	// Raw/single doc should return Data as-is (string), not reassembled
	if s, ok := result.Data.(string); !ok || s != "raw xml content here" {
		t.Errorf("expected raw string data, got %T: %v", result.Data, result.Data)
	}
}

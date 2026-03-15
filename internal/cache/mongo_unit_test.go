package cache

import (
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// --- reassemblePages tests ---

func TestReassemblePages_EmptySlice(t *testing.T) {
	result := reassemblePages(nil)
	if result != nil {
		t.Error("expected nil for empty docs")
	}

	result = reassemblePages([]model.CachedResponse{})
	if result != nil {
		t.Error("expected nil for empty slice")
	}
}

func TestReassemblePages_SingleDocPageZero(t *testing.T) {
	docs := []model.CachedResponse{
		{
			Slug:     "test",
			CacheKey: "test-key",
			Page:     0,
			Data:     "raw data",
		},
	}
	result := reassemblePages(docs)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Slug != "test" {
		t.Errorf("expected slug 'test', got %q", result.Slug)
	}
	if s, ok := result.Data.(string); !ok || s != "raw data" {
		t.Errorf("expected raw data string, got %T: %v", result.Data, result.Data)
	}
}

func TestReassemblePages_SingleDocWithPageNonZero(t *testing.T) {
	// A single doc with Page != 0 should still go through reassembly
	docs := []model.CachedResponse{
		{
			Slug:     "test",
			CacheKey: "test-key:page:1",
			Page:     1,
			Data:     []interface{}{"item1", "item2"},
		},
	}
	result := reassemblePages(docs)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	arr, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result.Data)
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 items, got %d", len(arr))
	}
	if result.Page != 0 {
		t.Errorf("expected combined Page=0, got %d", result.Page)
	}
}

func TestReassemblePages_MultiplePages(t *testing.T) {
	docs := []model.CachedResponse{
		{
			Slug:     "multi",
			CacheKey: "key:page:1",
			Page:     1,
			Data:     []interface{}{"a1", "a2"},
		},
		{
			Slug:     "multi",
			CacheKey: "key:page:2",
			Page:     2,
			Data:     []interface{}{"b1"},
		},
		{
			Slug:     "multi",
			CacheKey: "key:page:3",
			Page:     3,
			Data:     []interface{}{"c1", "c2", "c3"},
		},
	}
	result := reassemblePages(docs)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	arr, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result.Data)
	}
	if len(arr) != 6 {
		t.Errorf("expected 6 combined items, got %d", len(arr))
	}
	if result.Page != 0 {
		t.Errorf("expected combined Page=0, got %d", result.Page)
	}
	if result.Slug != "multi" {
		t.Errorf("expected slug 'multi', got %q", result.Slug)
	}
}

func TestReassemblePages_BsonAData(t *testing.T) {
	// Simulates what MongoDB returns: bson.A instead of []interface{}
	docs := []model.CachedResponse{
		{
			Slug:     "bson-test",
			CacheKey: "key:page:1",
			Page:     1,
			Data:     bson.A{"x1", "x2"},
		},
		{
			Slug:     "bson-test",
			CacheKey: "key:page:2",
			Page:     2,
			Data:     bson.A{"y1"},
		},
	}
	result := reassemblePages(docs)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	arr, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result.Data)
	}
	if len(arr) != 3 {
		t.Errorf("expected 3 combined items, got %d", len(arr))
	}
}

func TestReassemblePages_MixedDataTypes(t *testing.T) {
	// One page has bson.A, another has []interface{}
	docs := []model.CachedResponse{
		{
			Slug: "mixed",
			Page: 1,
			Data: bson.A{"a"},
		},
		{
			Slug: "mixed",
			Page: 2,
			Data: []interface{}{"b"},
		},
	}
	result := reassemblePages(docs)
	arr, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result.Data)
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 items, got %d", len(arr))
	}
}

func TestReassemblePages_NonArrayData(t *testing.T) {
	// Pages with non-array data (e.g., a string) are skipped in reassembly
	docs := []model.CachedResponse{
		{
			Slug: "non-array",
			Page: 1,
			Data: "string data",
		},
		{
			Slug: "non-array",
			Page: 2,
			Data: 42,
		},
	}
	result := reassemblePages(docs)
	// allData will be nil since neither is an array
	arr, ok := result.Data.([]interface{})
	if ok && len(arr) != 0 {
		t.Errorf("expected empty/nil data for non-array pages, got %v", arr)
	}
}

func TestReassemblePages_PreservesBaseFields(t *testing.T) {
	now := time.Now()
	docs := []model.CachedResponse{
		{
			Slug:        "preserve",
			CacheKey:    "key:page:1",
			Page:        1,
			PageCount:   2,
			ContentType: "application/json",
			FetchedAt:   now,
			SourceURL:   "http://example.com",
			HTTPStatus:  200,
			Data:        []interface{}{"item"},
		},
		{
			Slug:     "preserve",
			CacheKey: "key:page:2",
			Page:     2,
			Data:     []interface{}{"item2"},
		},
	}
	result := reassemblePages(docs)
	if result.ContentType != "application/json" {
		t.Errorf("expected content type from base doc, got %q", result.ContentType)
	}
	if result.SourceURL != "http://example.com" {
		t.Errorf("expected source URL from base doc, got %q", result.SourceURL)
	}
	if result.HTTPStatus != 200 {
		t.Errorf("expected HTTP status 200 from base doc, got %d", result.HTTPStatus)
	}
	if result.PageCount != 2 {
		t.Errorf("expected page count 2, got %d", result.PageCount)
	}
}

// --- buildUpsertFilter tests ---

func TestBuildUpsertFilter(t *testing.T) {
	filter := buildUpsertFilter("my-key")
	if filter["cache_key"] != "my-key" {
		t.Errorf("expected cache_key 'my-key', got %v", filter["cache_key"])
	}
}

func TestBuildUpsertFilter_EmptyKey(t *testing.T) {
	filter := buildUpsertFilter("")
	if filter["cache_key"] != "" {
		t.Errorf("expected empty cache_key, got %v", filter["cache_key"])
	}
}

// --- buildSingleUpdate tests ---

func TestBuildSingleUpdate(t *testing.T) {
	now := time.Now()
	resp := &model.CachedResponse{
		Slug:        "test-slug",
		Params:      map[string]string{"key": "val"},
		CacheKey:    "test-key",
		Data:        []interface{}{"item1"},
		ContentType: "application/json",
		FetchedAt:   now.Add(-time.Hour),
		SourceURL:   "http://example.com",
		HTTPStatus:  200,
		PageCount:   1,
	}

	update := buildSingleUpdate(resp, now)

	setFields, ok := update["$set"].(bson.M)
	if !ok {
		t.Fatal("expected $set to be bson.M")
	}

	if setFields["slug"] != "test-slug" {
		t.Errorf("expected slug 'test-slug', got %v", setFields["slug"])
	}
	if setFields["cache_key"] != "test-key" {
		t.Errorf("expected cache_key 'test-key', got %v", setFields["cache_key"])
	}
	if setFields["content_type"] != "application/json" {
		t.Errorf("expected content_type 'application/json', got %v", setFields["content_type"])
	}
	if setFields["http_status"] != 200 {
		t.Errorf("expected http_status 200, got %v", setFields["http_status"])
	}
	if setFields["page_count"] != 1 {
		t.Errorf("expected page_count 1, got %v", setFields["page_count"])
	}
	if setFields["updated_at"] != now {
		t.Errorf("expected updated_at to be now")
	}

	setOnInsert, ok := update["$setOnInsert"].(bson.M)
	if !ok {
		t.Fatal("expected $setOnInsert to be bson.M")
	}
	if setOnInsert["created_at"] != now {
		t.Errorf("expected created_at to be now")
	}
}

// --- buildPageUpdate tests ---

func TestBuildPageUpdate(t *testing.T) {
	now := time.Now()
	resp := &model.CachedResponse{
		Slug:        "page-slug",
		Params:      map[string]string{"p": "1"},
		CacheKey:    "base-key",
		ContentType: "application/json",
		FetchedAt:   now.Add(-time.Hour),
		SourceURL:   "http://example.com/api",
		HTTPStatus:  200,
		PageCount:   3,
	}
	page := model.PageData{
		Page: 2,
		Data: []interface{}{"item1", "item2"},
	}
	pageKey := "base-key:page:2"

	update := buildPageUpdate(resp, page, pageKey, now)

	setFields, ok := update["$set"].(bson.M)
	if !ok {
		t.Fatal("expected $set to be bson.M")
	}

	if setFields["slug"] != "page-slug" {
		t.Errorf("expected slug 'page-slug', got %v", setFields["slug"])
	}
	if setFields["cache_key"] != "base-key:page:2" {
		t.Errorf("expected cache_key 'base-key:page:2', got %v", setFields["cache_key"])
	}
	if setFields["page"] != 2 {
		t.Errorf("expected page 2, got %v", setFields["page"])
	}
	if setFields["page_count"] != 3 {
		t.Errorf("expected page_count 3, got %v", setFields["page_count"])
	}

	data, ok := setFields["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data to be []interface{}, got %T", setFields["data"])
	}
	if len(data) != 2 {
		t.Errorf("expected 2 data items, got %d", len(data))
	}
}

// --- buildRegexFilter tests ---

func TestBuildRegexFilter_SimpleKey(t *testing.T) {
	filter := buildRegexFilter("mykey")
	regex, ok := filter["cache_key"].(bson.M)
	if !ok {
		t.Fatal("expected cache_key to be bson.M")
	}
	expected := "^mykey(:|$)"
	if regex["$regex"] != expected {
		t.Errorf("expected regex %q, got %v", expected, regex["$regex"])
	}
}

func TestBuildRegexFilter_KeyWithSpecialChars(t *testing.T) {
	filter := buildRegexFilter("key.with+special")
	regex, ok := filter["cache_key"].(bson.M)
	if !ok {
		t.Fatal("expected cache_key to be bson.M")
	}
	expected := `^key\.with\+special(:|$)`
	if regex["$regex"] != expected {
		t.Errorf("expected regex %q, got %v", expected, regex["$regex"])
	}
}

func TestBuildRegexFilter_KeyWithColons(t *testing.T) {
	filter := buildRegexFilter("slug:param=value")
	regex, ok := filter["cache_key"].(bson.M)
	if !ok {
		t.Fatal("expected cache_key to be bson.M")
	}
	expected := "^slug:param=value(:|$)"
	if regex["$regex"] != expected {
		t.Errorf("expected regex %q, got %v", expected, regex["$regex"])
	}
}

// --- MongoRepository accessor tests ---

func TestMongoRepository_Names(t *testing.T) {
	repo := &MongoRepository{
		dbName:   "testdb",
		collName: "testcoll",
	}
	db, coll := repo.Names()
	if db != "testdb" {
		t.Errorf("expected db 'testdb', got %q", db)
	}
	if coll != "testcoll" {
		t.Errorf("expected coll 'testcoll', got %q", coll)
	}
}

func TestMongoRepository_Timeout(t *testing.T) {
	repo := &MongoRepository{
		timeout: 5 * time.Second,
	}
	if repo.Timeout() != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", repo.Timeout())
	}
}

func TestMongoRepository_Client_Nil(t *testing.T) {
	repo := &MongoRepository{}
	if repo.Client() != nil {
		t.Error("expected nil client")
	}
}

func TestMongoRepository_Database_Nil(t *testing.T) {
	repo := &MongoRepository{}
	if repo.Database() != nil {
		t.Error("expected nil database")
	}
}

// --- estimateSize edge case tests ---

func TestEstimateSize_WithPageCount(t *testing.T) {
	resp := &model.CachedResponse{
		Slug:      "test",
		CacheKey:  "test",
		PageCount: 3,
	}
	size := estimateSize(resp)
	// Base 200 + len("test") + len("test") + 3*50000
	expected := int64(200 + 4 + 4 + 150000)
	if size != expected {
		t.Errorf("expected size %d, got %d", expected, size)
	}
}

func TestEstimateSize_WithDataNoPageCount(t *testing.T) {
	resp := &model.CachedResponse{
		Slug:     "test",
		CacheKey: "test",
		Data:     "some data",
	}
	size := estimateSize(resp)
	// Base 200 + len("test") + len("test") + 10000 (default for non-nil data)
	expected := int64(200 + 4 + 4 + 10000)
	if size != expected {
		t.Errorf("expected size %d, got %d", expected, size)
	}
}

func TestEstimateSize_NilDataNoPageCount(t *testing.T) {
	resp := &model.CachedResponse{
		Slug:     "test",
		CacheKey: "test",
	}
	size := estimateSize(resp)
	// Base 200 + len("test") + len("test"), no data contribution
	expected := int64(200 + 4 + 4)
	if size != expected {
		t.Errorf("expected size %d, got %d", expected, size)
	}
}

func TestEstimateSize_IncludesAllStringFields(t *testing.T) {
	resp := &model.CachedResponse{
		Slug:        "my-slug",
		CacheKey:    "my-cache-key",
		SourceURL:   "http://example.com/api/v1/data",
		ContentType: "application/json",
	}
	size := estimateSize(resp)
	expected := int64(200 + len("my-slug") + len("my-cache-key") + len("http://example.com/api/v1/data") + len("application/json"))
	if size != expected {
		t.Errorf("expected size %d, got %d", expected, size)
	}
}

// --- noopLRU tests ---

func TestNoopLRU_Invalidate(t *testing.T) {
	lru := NewLRUCache(0)
	// Should not panic
	lru.Invalidate("nonexistent")
}

func TestNoopLRU_Set(t *testing.T) {
	lru := NewLRUCache(-1) // negative also triggers noop
	resp := &model.CachedResponse{Slug: "test"}
	lru.Set("key", resp, time.Minute)
	// Should not panic, should not store
	_, ok := lru.Get("key")
	if ok {
		t.Error("expected miss from noop cache")
	}
}

// --- LRU estimateSize with zero page count ---

func TestEstimateSize_ZeroPageCountWithData(t *testing.T) {
	resp := &model.CachedResponse{
		Slug:      "s",
		CacheKey:  "k",
		PageCount: 0,
		Data:      map[string]interface{}{"key": "value"},
	}
	size := estimateSize(resp)
	// Has data but no page count -> 10000 default
	if size < 10000 {
		t.Errorf("expected size >= 10000 for data with no page count, got %d", size)
	}
}

// --- Default constants ---

func TestDefaultConstants(t *testing.T) {
	if defaultDBName != "drugs" {
		t.Errorf("expected default DB name 'drugs', got %q", defaultDBName)
	}
	if defaultCollectionName != "cached_responses" {
		t.Errorf("expected default collection 'cached_responses', got %q", defaultCollectionName)
	}
}

// --- MongoRepository.Get with mock collection ---

func newTestRepo(coll collection) *MongoRepository {
	return &MongoRepository{
		collection: coll,
		timeout:    5 * time.Second,
		dbName:     "test",
		collName:   "test_coll",
	}
}

func TestGet_FindError(t *testing.T) {
	mock := &mockCollection{findErr: errMock}
	repo := newTestRepo(mock)

	_, err := repo.Get("some-key")
	if err == nil {
		t.Fatal("expected error from Get when Find fails")
	}
	if !mock.findCalled {
		t.Error("expected Find to be called")
	}
}

func TestGet_EmptyResult(t *testing.T) {
	mock := &mockCollection{findDocs: []any{}}
	repo := newTestRepo(mock)

	result, err := repo.Get("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil for empty result")
	}
}

func TestGet_SingleDocument(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	doc := model.CachedResponse{
		Slug:        "test",
		CacheKey:    "test-key",
		Page:        0,
		Data:        "raw data",
		ContentType: "text/plain",
		FetchedAt:   now,
		SourceURL:   "http://example.com",
		HTTPStatus:  200,
	}
	mock := &mockCollection{findDocs: []any{doc}}
	repo := newTestRepo(mock)

	result, err := repo.Get("test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Slug != "test" {
		t.Errorf("expected slug 'test', got %q", result.Slug)
	}
}

func TestGet_MultiPageDocuments(t *testing.T) {
	doc1 := model.CachedResponse{
		Slug:     "multi",
		CacheKey: "key:page:1",
		Page:     1,
		Data:     bson.A{"a1", "a2"},
	}
	doc2 := model.CachedResponse{
		Slug:     "multi",
		CacheKey: "key:page:2",
		Page:     2,
		Data:     bson.A{"b1"},
	}
	mock := &mockCollection{findDocs: []any{doc1, doc2}}
	repo := newTestRepo(mock)

	result, err := repo.Get("key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	arr, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result.Data)
	}
	if len(arr) != 3 {
		t.Errorf("expected 3 items, got %d", len(arr))
	}
}

// --- MongoRepository.Upsert with mock collection ---

func TestUpsert_SingleDocument(t *testing.T) {
	mock := &mockCollection{}
	repo := newTestRepo(mock)

	resp := &model.CachedResponse{
		Slug:        "test",
		CacheKey:    "test-key",
		Data:        "data",
		ContentType: "text/plain",
		FetchedAt:   time.Now(),
		SourceURL:   "http://example.com",
		HTTPStatus:  200,
		PageCount:   1,
	}

	err := repo.Upsert(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.updateOneCalled != 1 {
		t.Errorf("expected UpdateOne called once, got %d", mock.updateOneCalled)
	}
}

func TestUpsert_SingleDocument_Error(t *testing.T) {
	mock := &mockCollection{updateErr: errMock}
	repo := newTestRepo(mock)

	resp := &model.CachedResponse{
		Slug:     "test",
		CacheKey: "test-key",
		Data:     "data",
	}

	err := repo.Upsert(resp)
	if err == nil {
		t.Fatal("expected error from Upsert when UpdateOne fails")
	}
}

func TestUpsert_MultiPage(t *testing.T) {
	mock := &mockCollection{}
	repo := newTestRepo(mock)

	resp := &model.CachedResponse{
		Slug:        "multi",
		CacheKey:    "multi-key",
		ContentType: "application/json",
		FetchedAt:   time.Now(),
		SourceURL:   "http://example.com",
		HTTPStatus:  200,
		PageCount:   2,
		Pages: []model.PageData{
			{Page: 1, Data: []interface{}{"a1"}},
			{Page: 2, Data: []interface{}{"b1"}},
		},
	}

	err := repo.Upsert(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.updateOneCalled != 2 {
		t.Errorf("expected UpdateOne called twice for 2 pages, got %d", mock.updateOneCalled)
	}
	if !mock.deleteManyCalled {
		t.Error("expected DeleteMany called to clean stale pages")
	}
	if !mock.deleteOneCalled {
		t.Error("expected DeleteOne called to clean old single-doc version")
	}
}

func TestUpsert_MultiPage_UpdateError(t *testing.T) {
	mock := &mockCollection{updateErr: errMock}
	repo := newTestRepo(mock)

	resp := &model.CachedResponse{
		Slug:     "multi",
		CacheKey: "multi-key",
		Pages: []model.PageData{
			{Page: 1, Data: []interface{}{"a1"}},
			{Page: 2, Data: []interface{}{"b1"}},
		},
	}

	err := repo.Upsert(resp)
	if err == nil {
		t.Fatal("expected error from Upsert when page UpdateOne fails")
	}
}

func TestUpsert_SinglePage(t *testing.T) {
	// Pages with len <= 1 should take the single-document path
	mock := &mockCollection{}
	repo := newTestRepo(mock)

	resp := &model.CachedResponse{
		Slug:     "single-page",
		CacheKey: "key",
		Data:     "data",
		Pages: []model.PageData{
			{Page: 1, Data: []interface{}{"a1"}},
		},
	}

	err := repo.Upsert(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should use single-doc path, not multi-page path
	if mock.deleteManyCalled {
		t.Error("expected single-doc path, but DeleteMany was called")
	}
}

// --- MongoRepository.FetchedAt with mock collection ---

func TestFetchedAt_Found(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	doc := bson.M{"fetched_at": now}
	mock := &mockCollection{findOneDoc: doc}
	repo := newTestRepo(mock)

	fetchedAt, found, err := repo.FetchedAt("some-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Error("expected found=true")
	}
	if fetchedAt.Sub(now).Abs() > time.Second {
		t.Errorf("expected fetched_at ~%v, got %v", now, fetchedAt)
	}
}

func TestFetchedAt_NotFound(t *testing.T) {
	mock := &mockCollection{findOneDoc: nil} // nil triggers ErrNoDocuments
	repo := newTestRepo(mock)

	_, found, err := repo.FetchedAt("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false for non-existent key")
	}
}

func TestFetchedAt_Error(t *testing.T) {
	mock := &mockCollection{findOneErr: errMock}
	repo := newTestRepo(mock)

	_, _, err := repo.FetchedAt("some-key")
	if err == nil {
		t.Fatal("expected error from FetchedAt when FindOne fails")
	}
}

// --- NewMongoRepository error path ---

func TestNewMongoRepository_InvalidURI(t *testing.T) {
	// Test with a syntactically invalid URI
	_, err := NewMongoRepository("://invalid", 1*time.Second)
	if err == nil {
		t.Fatal("expected error for invalid URI")
	}
}

// --- Get with cursor decode error ---

func TestGet_CursorDecodeError(t *testing.T) {
	// Provide a document that cannot be decoded into model.CachedResponse
	badDoc := bson.M{"slug": 12345} // slug expects string, provide int
	mock := &mockCollection{findDocs: []any{badDoc}}
	repo := newTestRepo(mock)

	_, err := repo.Get("some-key")
	if err == nil {
		t.Fatal("expected decode error from Get")
	}
}

// --- noopLRU full coverage ---

func TestNoopLRU_SetDoesNotStore(t *testing.T) {
	lru := &noopLRU{}
	resp := &model.CachedResponse{Slug: "test"}
	lru.Set("key", resp, time.Minute)
	_, ok := lru.Get("key")
	if ok {
		t.Error("expected miss from noop Set")
	}
}

func TestNoopLRU_InvalidateDoesNotPanic(t *testing.T) {
	lru := &noopLRU{}
	lru.Invalidate("nonexistent") // should be a no-op
}

// --- LRU Set replacing existing entry ---

func TestLRU_SetReplacesExistingEntry(t *testing.T) {
	lru := NewLRUCache(1024 * 1024)

	resp1 := &model.CachedResponse{Slug: "first"}
	resp2 := &model.CachedResponse{Slug: "second"}

	lru.Set("key", resp1, time.Minute)
	lru.Set("key", resp2, time.Minute)

	got, ok := lru.Get("key")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Slug != "second" {
		t.Errorf("expected slug 'second', got %q", got.Slug)
	}
}

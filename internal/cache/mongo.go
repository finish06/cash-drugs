package cache

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/finish06/cash-drugs/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	defaultDBName         = "drugs"
	defaultCollectionName = "cached_responses"
)

// Pinger is the interface for health check pings.
type Pinger interface {
	Ping() error
}

// MongoRepository implements cache.Repository using MongoDB.
type MongoRepository struct {
	client     *mongo.Client
	db         *mongo.Database
	collection collection
	timeout    time.Duration
	dbName     string
	collName   string
}

// reassemblePages combines multiple page documents into a single CachedResponse.
// Returns the first document as-is if it's a single non-paginated document (Page == 0).
// For multi-page results, concatenates all Data arrays into a combined response.
func reassemblePages(docs []model.CachedResponse) *model.CachedResponse {
	if len(docs) == 0 {
		return nil
	}

	// Single document (non-paginated or raw) -- return as-is
	if len(docs) == 1 && docs[0].Page == 0 {
		return &docs[0]
	}

	// Multi-page: reassemble all page data into combined response
	base := docs[0]
	var allData []interface{}
	for _, doc := range docs {
		if arr, ok := doc.Data.(bson.A); ok {
			for _, item := range arr {
				allData = append(allData, item)
			}
		} else if arr, ok := doc.Data.([]interface{}); ok {
			allData = append(allData, arr...)
		}
	}

	base.Data = allData
	base.Page = 0 // combined view
	return &base
}

// buildUpsertFilter returns the filter document for a single-document upsert.
func buildUpsertFilter(cacheKey string) bson.M {
	return bson.M{"cache_key": cacheKey}
}

// buildSingleUpdate returns the update document for a single-document upsert.
func buildSingleUpdate(resp *model.CachedResponse, now time.Time) bson.M {
	return bson.M{
		"$set": bson.M{
			"slug":         resp.Slug,
			"params":       resp.Params,
			"cache_key":    resp.CacheKey,
			"base_key":     resp.CacheKey,
			"data":         resp.Data,
			"content_type": resp.ContentType,
			"fetched_at":   resp.FetchedAt,
			"source_url":   resp.SourceURL,
			"http_status":  resp.HTTPStatus,
			"page_count":   resp.PageCount,
			"updated_at":   now,
		},
		"$setOnInsert": bson.M{
			"created_at": now,
		},
	}
}

// buildPageUpdate returns the update document for a single page upsert.
func buildPageUpdate(resp *model.CachedResponse, page model.PageData, pageKey string, now time.Time) bson.M {
	return bson.M{
		"$set": bson.M{
			"slug":         resp.Slug,
			"params":       resp.Params,
			"cache_key":    pageKey,
			"base_key":     resp.CacheKey,
			"page":         page.Page,
			"page_count":   resp.PageCount,
			"data":         page.Data,
			"content_type": resp.ContentType,
			"fetched_at":   resp.FetchedAt,
			"source_url":   resp.SourceURL,
			"http_status":  resp.HTTPStatus,
			"updated_at":   now,
		},
		"$setOnInsert": bson.M{
			"created_at": now,
		},
	}
}

// buildBaseKeyFilter creates a MongoDB exact-match filter on the base_key field.
func buildBaseKeyFilter(cacheKey string) bson.M {
	return bson.M{"base_key": cacheKey}
}

// extractBaseKey returns the cache key without any :page:N suffix.
// For keys like "drugnames:page:2", returns "drugnames".
// For keys like "some:page:key:page:2", splits on the last ":page:" occurrence.
// For keys without ":page:", returns the key as-is.
func extractBaseKey(cacheKey string) string {
	const sep = ":page:"
	idx := strings.LastIndex(cacheKey, sep)
	if idx < 0 {
		return cacheKey
	}
	return cacheKey[:idx]
}

// NewMongoRepository connects to MongoDB, pings it, and ensures indexes.
func NewMongoRepository(uri string, timeout time.Duration) (*MongoRepository, error) {
	clientOpts := options.Client().ApplyURI(uri)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	// Extract database name from URI or use default
	dbName := defaultDBName

	db := client.Database(dbName)
	coll := db.Collection(defaultCollectionName)

	repo := &MongoRepository{
		client:     client,
		db:         db,
		collection: &mongoCollectionAdapter{coll: coll},
		timeout:    timeout,
		dbName:     dbName,
		collName:   defaultCollectionName,
	}

	if err := repo.ensureIndexes(ctx); err != nil {
		return nil, fmt.Errorf("failed to create indexes: %w", err)
	}

	if err := repo.backfillBaseKey(ctx); err != nil {
		slog.Warn("backfillBaseKey failed (non-fatal)", "error", err)
	}

	return repo, nil
}

// Get retrieves a cached response by cache key. For multi-page responses,
// reassembles all pages into a single combined Data slice.
func (r *MongoRepository) Get(cacheKey string) (*model.CachedResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	pageFilter := buildBaseKeyFilter(cacheKey)

	cursor, err := r.collection.Find(ctx, pageFilter, options.Find().SetSort(bson.D{{Key: "page", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("failed to query cached responses: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []model.CachedResponse
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("failed to decode cached responses: %w", err)
	}

	return reassemblePages(docs), nil
}

// Upsert inserts or updates a cached response. For multi-page responses (Pages populated),
// stores each page as a separate document to avoid MongoDB's 16MB limit.
func (r *MongoRepository) Upsert(resp *model.CachedResponse) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	now := time.Now()

	// Multi-page: store each page separately
	if len(resp.Pages) > 1 {
		// Delete any stale pages beyond current page count
		staleFilter := bson.M{
			"base_key": resp.CacheKey,
			"page":     bson.M{"$gt": len(resp.Pages)},
		}
		r.collection.DeleteMany(ctx, staleFilter)

		// Also delete any old single-document version
		r.collection.DeleteOne(ctx, bson.M{"cache_key": resp.CacheKey})

		// Upsert each page
		for _, page := range resp.Pages {
			pageKey := fmt.Sprintf("%s:page:%d", resp.CacheKey, page.Page)
			filter := buildUpsertFilter(pageKey)
			update := buildPageUpdate(resp, page, pageKey, now)

			opts := options.UpdateOne().SetUpsert(true)
			if _, err := r.collection.UpdateOne(ctx, filter, update, opts); err != nil {
				return fmt.Errorf("failed to upsert page %d for %s: %w", page.Page, resp.Slug, err)
			}
		}
		return nil
	}

	// Single document (non-paginated, raw, or single-page)
	filter := buildUpsertFilter(resp.CacheKey)
	update := buildSingleUpdate(resp, now)

	opts := options.UpdateOne().SetUpsert(true)
	_, err := r.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to upsert cached response: %w", err)
	}

	return nil
}

// FetchedAt returns the fetched_at timestamp for the first page of a cache key.
// Returns (time, true, nil) if found, (zero, false, nil) if not found.
func (r *MongoRepository) FetchedAt(cacheKey string) (time.Time, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	// Look for exact key or first page using indexed base_key
	filter := buildBaseKeyFilter(cacheKey)

	opts := options.FindOne().SetProjection(bson.M{"fetched_at": 1})
	var result struct {
		FetchedAt time.Time `bson:"fetched_at"`
	}
	err := r.collection.FindOne(ctx, filter, opts).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, fmt.Errorf("failed to check fetched_at: %w", err)
	}
	return result.FetchedAt, true, nil
}

// Ping checks MongoDB connectivity.
func (r *MongoRepository) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	return r.client.Ping(ctx, nil)
}

// Close disconnects from MongoDB.
func (r *MongoRepository) Close(ctx context.Context) error {
	return r.client.Disconnect(ctx)
}

// Names returns the database and collection names.
func (r *MongoRepository) Names() (dbName, collName string) {
	return r.dbName, r.collName
}

// Client returns the underlying MongoDB client.
func (r *MongoRepository) Client() *mongo.Client {
	return r.client
}

// Database returns the underlying MongoDB database.
func (r *MongoRepository) Database() *mongo.Database {
	return r.db
}

// Timeout returns the configured operation timeout.
func (r *MongoRepository) Timeout() time.Duration {
	return r.timeout
}

func (r *MongoRepository) ensureIndexes(ctx context.Context) error {
	indexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "cache_key", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("idx_cache_key"),
	}

	if _, err := r.collection.Indexes().CreateOne(ctx, indexModel); err != nil {
		return err
	}

	// Compound index on base_key + page for efficient multi-page lookups
	baseKeyIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "base_key", Value: 1}, {Key: "page", Value: 1}},
		Options: options.Index().SetName("idx_base_key_page"),
	}

	_, err := r.collection.Indexes().CreateOne(ctx, baseKeyIndex)
	return err
}

// backfillBaseKey populates the base_key field on documents that are missing it.
// This is idempotent -- documents with base_key already set are skipped.
func (r *MongoRepository) backfillBaseKey(ctx context.Context) error {
	// Find all documents where base_key is empty or missing
	filter := bson.M{
		"$or": bson.A{
			bson.M{"base_key": ""},
			bson.M{"base_key": bson.M{"$exists": false}},
		},
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return fmt.Errorf("backfillBaseKey: failed to find documents: %w", err)
	}
	defer cursor.Close(ctx)

	var updated int64
	for cursor.Next(ctx) {
		var doc struct {
			CacheKey string `bson:"cache_key"`
		}
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		baseKey := extractBaseKey(doc.CacheKey)
		updateFilter := bson.M{"cache_key": doc.CacheKey}
		update := bson.M{"$set": bson.M{"base_key": baseKey}}

		if _, err := r.collection.UpdateOne(ctx, updateFilter, update); err != nil {
			slog.Warn("backfillBaseKey: failed to update document",
				"cache_key", doc.CacheKey, "error", err)
			continue
		}
		updated++
	}

	slog.Info("backfillBaseKey complete", "documents_updated", updated)
	return nil
}

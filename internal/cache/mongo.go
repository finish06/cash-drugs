package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/finish06/drugs/internal/model"
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
	collection *mongo.Collection
	timeout    time.Duration
	dbName     string
	collName   string
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
		collection: coll,
		timeout:    timeout,
		dbName:     dbName,
		collName:   defaultCollectionName,
	}

	if err := repo.ensureIndexes(ctx); err != nil {
		return nil, fmt.Errorf("failed to create indexes: %w", err)
	}

	return repo, nil
}

// Get retrieves a cached response by cache key. Returns nil if not found.
func (r *MongoRepository) Get(cacheKey string) (*model.CachedResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	filter := bson.M{"cache_key": cacheKey}
	var result model.CachedResponse

	err := r.collection.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get cached response: %w", err)
	}

	return &result, nil
}

// Upsert inserts or updates a cached response by cache key.
// Sets created_at on insert, updated_at on every write.
func (r *MongoRepository) Upsert(resp *model.CachedResponse) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	now := time.Now()
	filter := bson.M{"cache_key": resp.CacheKey}
	update := bson.M{
		"$set": bson.M{
			"slug":         resp.Slug,
			"params":       resp.Params,
			"cache_key":    resp.CacheKey,
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

	opts := options.UpdateOne().SetUpsert(true)
	_, err := r.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to upsert cached response: %w", err)
	}

	return nil
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

// Timeout returns the configured operation timeout.
func (r *MongoRepository) Timeout() time.Duration {
	return r.timeout
}

func (r *MongoRepository) ensureIndexes(ctx context.Context) error {
	indexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "cache_key", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("idx_cache_key"),
	}

	_, err := r.collection.Indexes().CreateOne(ctx, indexModel)
	return err
}

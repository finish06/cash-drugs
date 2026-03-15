package cache

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// collection abstracts MongoDB collection operations for testability.
type collection interface {
	Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (*mongo.Cursor, error)
	FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) *mongo.SingleResult
	UpdateOne(ctx context.Context, filter any, update any, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error)
	DeleteOne(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (*mongo.DeleteResult, error)
	DeleteMany(ctx context.Context, filter any, opts ...options.Lister[options.DeleteManyOptions]) (*mongo.DeleteResult, error)
	Indexes() mongo.IndexView
}

// mongoCollectionAdapter wraps *mongo.Collection to implement the collection interface.
// This exists solely so MongoRepository can use the interface internally,
// enabling unit tests with mock collections.
type mongoCollectionAdapter struct {
	coll *mongo.Collection
}

func (a *mongoCollectionAdapter) Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (*mongo.Cursor, error) {
	return a.coll.Find(ctx, filter, opts...)
}

func (a *mongoCollectionAdapter) FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) *mongo.SingleResult {
	return a.coll.FindOne(ctx, filter, opts...)
}

func (a *mongoCollectionAdapter) UpdateOne(ctx context.Context, filter any, update any, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error) {
	return a.coll.UpdateOne(ctx, filter, update, opts...)
}

func (a *mongoCollectionAdapter) DeleteOne(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (*mongo.DeleteResult, error) {
	return a.coll.DeleteOne(ctx, filter, opts...)
}

func (a *mongoCollectionAdapter) DeleteMany(ctx context.Context, filter any, opts ...options.Lister[options.DeleteManyOptions]) (*mongo.DeleteResult, error) {
	return a.coll.DeleteMany(ctx, filter, opts...)
}

func (a *mongoCollectionAdapter) Indexes() mongo.IndexView {
	return a.coll.Indexes()
}

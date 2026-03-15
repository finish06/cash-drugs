package cache

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// mockCollection implements the collection interface for unit testing.
type mockCollection struct {
	// Find behavior
	findDocs []any
	findErr  error

	// FindOne behavior
	findOneDoc any
	findOneErr error

	// UpdateOne behavior
	updateResult *mongo.UpdateResult
	updateErr    error

	// DeleteOne/DeleteMany behavior
	deleteResult *mongo.DeleteResult
	deleteErr    error

	// Track calls for assertions
	findCalled      bool
	findOneCalled   bool
	updateOneCalled int
	deleteOneCalled bool
	deleteManyCalled bool

	// Capture filters and updates for assertions
	lastFindFilter      any
	lastFindOneFilter   any
	lastDeleteManyFilter any
	updateOneFilters    []any
	updateOneUpdates    []any
}

func (m *mockCollection) Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (*mongo.Cursor, error) {
	m.findCalled = true
	m.lastFindFilter = filter
	if m.findErr != nil {
		return nil, m.findErr
	}
	cursor, err := mongo.NewCursorFromDocuments(m.findDocs, nil, nil)
	return cursor, err
}

func (m *mockCollection) FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) *mongo.SingleResult {
	m.findOneCalled = true
	m.lastFindOneFilter = filter
	if m.findOneErr != nil {
		return mongo.NewSingleResultFromDocument(nil, m.findOneErr, nil)
	}
	if m.findOneDoc == nil {
		// Return a SingleResult that will decode as ErrNoDocuments
		return mongo.NewSingleResultFromDocument(bson.D{}, mongo.ErrNoDocuments, nil)
	}
	return mongo.NewSingleResultFromDocument(m.findOneDoc, nil, nil)
}

func (m *mockCollection) UpdateOne(ctx context.Context, filter any, update any, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error) {
	m.updateOneCalled++
	m.updateOneFilters = append(m.updateOneFilters, filter)
	m.updateOneUpdates = append(m.updateOneUpdates, update)
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	if m.updateResult != nil {
		return m.updateResult, nil
	}
	return &mongo.UpdateResult{UpsertedCount: 1}, nil
}

func (m *mockCollection) DeleteOne(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (*mongo.DeleteResult, error) {
	m.deleteOneCalled = true
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}
	if m.deleteResult != nil {
		return m.deleteResult, nil
	}
	return &mongo.DeleteResult{DeletedCount: 1}, nil
}

func (m *mockCollection) DeleteMany(ctx context.Context, filter any, opts ...options.Lister[options.DeleteManyOptions]) (*mongo.DeleteResult, error) {
	m.deleteManyCalled = true
	m.lastDeleteManyFilter = filter
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}
	if m.deleteResult != nil {
		return m.deleteResult, nil
	}
	return &mongo.DeleteResult{DeletedCount: 0}, nil
}

func (m *mockCollection) Indexes() mongo.IndexView {
	// This won't be called in unit tests; only used by ensureIndexes during NewMongoRepository
	return mongo.IndexView{}
}

// errMock is a reusable test error.
var errMock = errors.New("mock error")

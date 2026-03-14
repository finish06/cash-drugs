package metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// SlugCount represents a document count for a single slug.
type SlugCount struct {
	Slug  string
	Count int64
}

// CollectorSource abstracts MongoDB operations for testability.
type CollectorSource interface {
	Ping(ctx context.Context) error
	CountBySlug(ctx context.Context) ([]SlugCount, error)
}

// MongoCollectorSource implements CollectorSource using a real MongoDB connection.
type MongoCollectorSource struct {
	client   *mongo.Client
	db       *mongo.Database
	collName string
}

// NewMongoCollectorSource creates a CollectorSource backed by MongoDB.
func NewMongoCollectorSource(client *mongo.Client, db *mongo.Database, collName string) *MongoCollectorSource {
	return &MongoCollectorSource{client: client, db: db, collName: collName}
}

// Ping checks MongoDB connectivity.
func (s *MongoCollectorSource) Ping(ctx context.Context) error {
	return s.client.Ping(ctx, nil)
}

// CountBySlug returns document counts grouped by slug.
func (s *MongoCollectorSource) CountBySlug(ctx context.Context) ([]SlugCount, error) {
	coll := s.db.Collection(s.collName)
	pipeline := bson.A{
		bson.M{"$group": bson.M{
			"_id":   "$slug",
			"count": bson.M{"$sum": 1},
		}},
	}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []struct {
		Slug  string `bson:"_id"`
		Count int64  `bson:"count"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	counts := make([]SlugCount, len(results))
	for i, r := range results {
		counts[i] = SlugCount{Slug: r.Slug, Count: r.Count}
	}
	return counts, nil
}

// MongoCollector periodically collects MongoDB health and document count metrics.
type MongoCollector struct {
	source   CollectorSource
	metrics  *Metrics
	interval time.Duration
	stopCh   chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// NewMongoCollector creates a new background MongoDB metrics collector.
func NewMongoCollector(client *mongo.Client, db *mongo.Database, collName string, m *Metrics, interval time.Duration) *MongoCollector {
	var source CollectorSource
	if client != nil {
		source = NewMongoCollectorSource(client, db, collName)
	}
	return &MongoCollector{
		source:   source,
		metrics:  m,
		interval: interval,
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// NewMongoCollectorWithSource creates a collector with a custom source (for testing).
func NewMongoCollectorWithSource(source CollectorSource, m *Metrics, interval time.Duration) *MongoCollector {
	return &MongoCollector{
		source:   source,
		metrics:  m,
		interval: interval,
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start begins the background collection loop.
func (c *MongoCollector) Start() {
	go func() {
		defer close(c.done)

		// Collect once immediately
		c.collect()

		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.collect()
			case <-c.stopCh:
				return
			}
		}
	}()
}

// Stop signals the collector to stop and waits for the goroutine to exit.
func (c *MongoCollector) Stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
	<-c.done
}

func (c *MongoCollector) collect() {
	if c.source == nil {
		c.metrics.MongoDBUp.Set(0)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ping MongoDB
	pingStart := time.Now()
	err := c.source.Ping(ctx)
	pingDuration := time.Since(pingStart).Seconds()

	if err != nil {
		c.metrics.MongoDBUp.Set(0)
		slog.Debug("mongodb ping failed", "component", "metrics", "error", err)
		return
	}

	c.metrics.MongoDBUp.Set(1)
	c.metrics.MongoDBPingDuration.Set(pingDuration)

	// Count documents per slug
	counts, err := c.source.CountBySlug(ctx)
	if err != nil {
		slog.Debug("mongodb count by slug failed", "component", "metrics", "error", err)
		return
	}

	// Reset before setting to clear stale slug label values
	c.metrics.MongoDBDocuments.Reset()
	for _, sc := range counts {
		c.metrics.MongoDBDocuments.WithLabelValues(sc.Slug).Set(float64(sc.Count))
	}
}

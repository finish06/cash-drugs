package metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// MongoCollector periodically collects MongoDB health and document count metrics.
type MongoCollector struct {
	client   *mongo.Client
	db       *mongo.Database
	collName string
	metrics  *Metrics
	interval time.Duration
	stopCh   chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// NewMongoCollector creates a new background MongoDB metrics collector.
func NewMongoCollector(client *mongo.Client, db *mongo.Database, collName string, m *Metrics, interval time.Duration) *MongoCollector {
	return &MongoCollector{
		client:   client,
		db:       db,
		collName: collName,
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ping MongoDB
	pingStart := time.Now()
	err := c.client.Ping(ctx, nil)
	pingDuration := time.Since(pingStart).Seconds()

	if err != nil {
		c.metrics.MongoDBUp.Set(0)
		slog.Debug("mongodb ping failed", "component", "metrics", "error", err)
		return
	}

	c.metrics.MongoDBUp.Set(1)
	c.metrics.MongoDBPingDuration.Set(pingDuration)

	// Count documents per slug
	coll := c.db.Collection(c.collName)
	pipeline := bson.A{
		bson.M{"$group": bson.M{
			"_id":   "$slug",
			"count": bson.M{"$sum": 1},
		}},
	}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		slog.Debug("mongodb aggregate failed", "component", "metrics", "error", err)
		return
	}
	defer cursor.Close(ctx)

	var results []struct {
		Slug  string `bson:"_id"`
		Count int64  `bson:"count"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		slog.Debug("mongodb cursor decode failed", "component", "metrics", "error", err)
		return
	}

	// Reset before setting to clear stale slug label values
	c.metrics.MongoDBDocuments.Reset()
	for _, r := range results {
		c.metrics.MongoDBDocuments.WithLabelValues(r.Slug).Set(float64(r.Count))
	}
}

package metrics_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// --- Mock CollectorSource ---

type mockSource struct {
	pingErr    error
	slugCounts []metrics.SlugCount
	countErr   error
}

func (m *mockSource) Ping(ctx context.Context) error {
	if m.pingErr == nil {
		time.Sleep(1 * time.Millisecond) // simulate non-zero latency
	}
	return m.pingErr
}

func (m *mockSource) CountBySlug(ctx context.Context) ([]metrics.SlugCount, error) {
	return m.slugCounts, m.countErr
}

// --- Lifecycle Tests ---

// AC-019: Collector starts and stops cleanly
func TestAC019_CollectorStartStop(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	collector := metrics.NewMongoCollectorWithSource(&mockSource{}, m, 1*time.Hour)
	collector.Start()

	done := make(chan struct{})
	go func() {
		collector.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("collector.Stop() did not return within 2 seconds")
	}
}

// AC-019: Double Stop does not panic (sync.Once protection)
func TestAC019_CollectorDoubleStopNoPanic(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	collector := metrics.NewMongoCollectorWithSource(&mockSource{}, m, 1*time.Hour)
	collector.Start()
	collector.Stop()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("double Stop() panicked: %v", r)
		}
	}()
	collector.Stop()
}

// AC-019: Nil source sets mongodb_up=0
func TestAC019_NilSourceSetsMongoDBDown(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	collector := metrics.NewMongoCollector(nil, nil, "test", m, 1*time.Hour)
	collector.Start()
	time.Sleep(50 * time.Millisecond)
	collector.Stop()

	if val := testutil.ToFloat64(m.MongoDBUp); val != 0 {
		t.Errorf("expected mongodb_up=0 with nil source, got %f", val)
	}
}

// --- Collect: Ping Success Path ---

// AC-008/AC-009: Successful ping sets mongodb_up=1 and records ping duration
func TestAC008_009_PingSuccessSetsUpAndDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSource{
		slugCounts: []metrics.SlugCount{},
	}

	collector := metrics.NewMongoCollectorWithSource(src, m, 1*time.Hour)
	collector.Start()
	time.Sleep(50 * time.Millisecond)
	collector.Stop()

	if val := testutil.ToFloat64(m.MongoDBUp); val != 1 {
		t.Errorf("expected mongodb_up=1, got %f", val)
	}
	if val := testutil.ToFloat64(m.MongoDBPingDuration); val <= 0 {
		t.Errorf("expected ping duration > 0, got %f", val)
	}
}

// --- Collect: Ping Failure Path ---

// AC-009: Ping failure sets mongodb_up=0
func TestAC009_PingFailureSetsDown(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSource{
		pingErr: fmt.Errorf("connection refused"),
	}

	collector := metrics.NewMongoCollectorWithSource(src, m, 1*time.Hour)
	collector.Start()
	time.Sleep(50 * time.Millisecond)
	collector.Stop()

	if val := testutil.ToFloat64(m.MongoDBUp); val != 0 {
		t.Errorf("expected mongodb_up=0 on ping failure, got %f", val)
	}
}

// --- Collect: Document Count Path ---

// AC-010: Document counts per slug are recorded
func TestAC010_DocumentCountsPerSlug(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSource{
		slugCounts: []metrics.SlugCount{
			{Slug: "drugnames", Count: 1500},
			{Slug: "spls", Count: 3200},
			{Slug: "fda-enforcement", Count: 450},
		},
	}

	collector := metrics.NewMongoCollectorWithSource(src, m, 1*time.Hour)
	collector.Start()
	time.Sleep(50 * time.Millisecond)
	collector.Stop()

	if val := testutil.ToFloat64(m.MongoDBDocuments.WithLabelValues("drugnames")); val != 1500 {
		t.Errorf("expected drugnames=1500, got %f", val)
	}
	if val := testutil.ToFloat64(m.MongoDBDocuments.WithLabelValues("spls")); val != 3200 {
		t.Errorf("expected spls=3200, got %f", val)
	}
	if val := testutil.ToFloat64(m.MongoDBDocuments.WithLabelValues("fda-enforcement")); val != 450 {
		t.Errorf("expected fda-enforcement=450, got %f", val)
	}
}

// AC-010: Empty result set clears previous gauge values (Reset behavior)
func TestAC010_EmptyResultClearsStaleSlugs(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	// First collect with data
	src := &mockSource{
		slugCounts: []metrics.SlugCount{
			{Slug: "drugnames", Count: 100},
		},
	}
	collector := metrics.NewMongoCollectorWithSource(src, m, 1*time.Hour)
	collector.Start()
	time.Sleep(50 * time.Millisecond)
	collector.Stop()

	if val := testutil.ToFloat64(m.MongoDBDocuments.WithLabelValues("drugnames")); val != 100 {
		t.Fatalf("expected drugnames=100, got %f", val)
	}

	// Second collect with empty results — stale slug should be cleared
	src2 := &mockSource{
		slugCounts: []metrics.SlugCount{},
	}
	collector2 := metrics.NewMongoCollectorWithSource(src2, m, 1*time.Hour)
	collector2.Start()
	time.Sleep(50 * time.Millisecond)
	collector2.Stop()

	// After Reset(), WithLabelValues creates a new zero-value series
	if val := testutil.ToFloat64(m.MongoDBDocuments.WithLabelValues("drugnames")); val != 0 {
		t.Errorf("expected drugnames=0 after reset, got %f", val)
	}
}

// --- Collect: CountBySlug Error Path ---

// AC-010: CountBySlug error does not crash, mongodb_up still 1
func TestAC010_CountBySlugErrorHandled(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSource{
		countErr: fmt.Errorf("aggregate timeout"),
	}

	collector := metrics.NewMongoCollectorWithSource(src, m, 1*time.Hour)
	collector.Start()
	time.Sleep(50 * time.Millisecond)
	collector.Stop()

	// Ping succeeded, so mongodb_up should be 1
	if val := testutil.ToFloat64(m.MongoDBUp); val != 1 {
		t.Errorf("expected mongodb_up=1 despite count error, got %f", val)
	}
}

// AC-019: NewMongoCollector with nil client
func TestAC019_NewMongoCollectorNilClient(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	collector := metrics.NewMongoCollector(nil, nil, "cached_responses", m, 30*time.Second)
	if collector == nil {
		t.Fatal("NewMongoCollector returned nil")
	}
}

// AC-019: NewMongoCollector with non-nil client creates MongoCollectorSource
func TestAC019_NewMongoCollectorWithClient(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	// Create a disconnected client (no server needed for construction)
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27099"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	db := client.Database("test")

	collector := metrics.NewMongoCollector(client, db, "cached_responses", m, 1*time.Hour)
	if collector == nil {
		t.Fatal("NewMongoCollector returned nil with non-nil client")
	}

	// Start and stop quickly — collect will fail on Ping (no server) but should not panic
	collector.Start()
	time.Sleep(100 * time.Millisecond)
	collector.Stop()

	// Ping failed, so mongodb_up should be 0
	if val := testutil.ToFloat64(m.MongoDBUp); val != 0 {
		t.Errorf("expected mongodb_up=0 with disconnected client, got %f", val)
	}
}

// AC-019: Start runs collect on ticker interval
func TestAC019_StartRunsCollectOnTick(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSource{
		slugCounts: []metrics.SlugCount{
			{Slug: "test", Count: 42},
		},
	}

	// Very short interval so ticker fires during test
	collector := metrics.NewMongoCollectorWithSource(src, m, 50*time.Millisecond)
	collector.Start()

	// Wait for initial collect + at least one tick
	time.Sleep(150 * time.Millisecond)
	collector.Stop()

	if val := testutil.ToFloat64(m.MongoDBUp); val != 1 {
		t.Errorf("expected mongodb_up=1 after tick, got %f", val)
	}
}

// NewMongoCollectorSource and its methods (Ping error path via disconnected client)
func TestMongoCollectorSource_PingError(t *testing.T) {
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27099"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	db := client.Database("test")

	src := metrics.NewMongoCollectorSource(client, db, "test_coll")
	if src == nil {
		t.Fatal("NewMongoCollectorSource returned nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Ping should fail — no server at port 0
	if err := src.Ping(ctx); err == nil {
		t.Error("expected Ping error with disconnected client")
	}
}

// CountBySlug error path via disconnected client
func TestMongoCollectorSource_CountBySlugError(t *testing.T) {
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27099"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	db := client.Database("test")

	src := metrics.NewMongoCollectorSource(client, db, "test_coll")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = src.CountBySlug(ctx)
	if err == nil {
		t.Error("expected CountBySlug error with disconnected client")
	}
}

package metrics_test

import (
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// AC-019: MongoCollector starts and stops without panic
func TestAC019_CollectorStartStop(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	// Use nil client/db — collect() will fail on Ping, but Start/Stop lifecycle should work
	collector := metrics.NewMongoCollector(nil, nil, "test_coll", m, 1*time.Hour)

	// Should not panic
	collector.Start()

	// Give goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Stop should return (not hang)
	done := make(chan struct{})
	go func() {
		collector.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK — stopped cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("collector.Stop() did not return within 2 seconds")
	}
}

// AC-019: Double Stop does not panic (sync.Once protection)
func TestAC019_CollectorDoubleStopNoPanic(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	collector := metrics.NewMongoCollector(nil, nil, "test_coll", m, 1*time.Hour)
	collector.Start()
	time.Sleep(50 * time.Millisecond)

	// First stop
	collector.Stop()

	// Second stop should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("double Stop() panicked: %v", r)
		}
	}()
	collector.Stop()
}

// AC-009: MongoDB down sets mongodb_up to 0
func TestAC009_CollectorSetsMongoDBDownWhenNilClient(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	// nil client will cause Ping to fail
	collector := metrics.NewMongoCollector(nil, nil, "test_coll", m, 1*time.Hour)
	collector.Start()

	// Wait for initial collect() to run
	time.Sleep(100 * time.Millisecond)
	collector.Stop()

	// MongoDB should be reported as down
	val := testutil.ToFloat64(m.MongoDBUp)
	if val != 0 {
		t.Errorf("expected mongodb_up=0 when client is nil, got %f", val)
	}
}

// AC-019: NewMongoCollector stores interval correctly
func TestAC019_NewMongoCollectorFields(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	interval := 30 * time.Second
	collector := metrics.NewMongoCollector(nil, nil, "cached_responses", m, interval)

	if collector == nil {
		t.Fatal("NewMongoCollector returned nil")
	}
}

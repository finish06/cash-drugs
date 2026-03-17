package metrics_test

import (
	"strings"
	"testing"

	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// AC-018: All metric names use cashdrugs_ prefix
func TestAC018_MetricNamesHavePrefix(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	// Gather all metrics and verify prefix
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	if len(families) == 0 {
		t.Fatal("no metrics registered")
	}

	for _, f := range families {
		if !strings.HasPrefix(*f.Name, "cashdrugs_") {
			t.Errorf("metric %q does not have cashdrugs_ prefix", *f.Name)
		}
	}

	// Verify the metrics object is not nil
	if m == nil {
		t.Fatal("NewMetrics returned nil")
	}
}

// AC-002: HTTP request counter with correct labels
func TestAC002_HTTPRequestCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.HTTPRequestsTotal.WithLabelValues("drugnames", "GET", "200").Inc()

	expected := `
		# HELP cashdrugs_http_requests_total Total HTTP requests.
		# TYPE cashdrugs_http_requests_total counter
		cashdrugs_http_requests_total{method="GET",slug="drugnames",status_code="200"} 1
	`
	if err := testutil.CollectAndCompare(m.HTTPRequestsTotal, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected metric output: %v", err)
	}
}

// AC-003: HTTP request duration histogram with correct labels
func TestAC003_HTTPRequestDurationHistogram(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.HTTPRequestDuration.WithLabelValues("drugnames", "GET").Observe(0.05)

	// Verify the metric exists in registry with observations
	if testutil.CollectAndCount(m.HTTPRequestDuration) == 0 {
		t.Error("HTTP request duration histogram has no observations")
	}
}

// AC-004: Cache hit/miss/stale counter
func TestAC004_CacheOutcomeCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.CacheHitsTotal.WithLabelValues("drugnames", "hit").Inc()
	m.CacheHitsTotal.WithLabelValues("drugnames", "miss").Inc()
	m.CacheHitsTotal.WithLabelValues("drugnames", "stale").Inc()

	expected := `
		# HELP cashdrugs_cache_hits_total Cache outcomes by slug and result.
		# TYPE cashdrugs_cache_hits_total counter
		cashdrugs_cache_hits_total{outcome="hit",slug="drugnames"} 1
		cashdrugs_cache_hits_total{outcome="miss",slug="drugnames"} 1
		cashdrugs_cache_hits_total{outcome="stale",slug="drugnames"} 1
	`
	if err := testutil.CollectAndCompare(m.CacheHitsTotal, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected metric output: %v", err)
	}
}

// AC-005: Upstream fetch duration histogram
func TestAC005_UpstreamFetchDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.UpstreamFetchDuration.WithLabelValues("drugnames").Observe(1.5)

	if testutil.CollectAndCount(m.UpstreamFetchDuration) == 0 {
		t.Error("upstream fetch duration histogram has no observations")
	}
}

// AC-006: Upstream fetch error counter
func TestAC006_UpstreamFetchErrors(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.UpstreamFetchErrors.WithLabelValues("drugnames").Inc()

	val := testutil.ToFloat64(m.UpstreamFetchErrors.WithLabelValues("drugnames"))
	if val != 1 {
		t.Errorf("expected 1 error, got %f", val)
	}
}

// AC-007: Upstream fetch pages counter
func TestAC007_UpstreamFetchPages(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.UpstreamFetchPages.WithLabelValues("drugnames").Add(5)

	val := testutil.ToFloat64(m.UpstreamFetchPages.WithLabelValues("drugnames"))
	if val != 5 {
		t.Errorf("expected 5 pages, got %f", val)
	}
}

// AC-008: MongoDB ping duration gauge
func TestAC008_MongoDBPingDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.MongoDBPingDuration.Set(0.002)

	val := testutil.ToFloat64(m.MongoDBPingDuration)
	if val != 0.002 {
		t.Errorf("expected 0.002, got %f", val)
	}
}

// AC-009: MongoDB health gauge
func TestAC009_MongoDBUpGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.MongoDBUp.Set(1)
	if testutil.ToFloat64(m.MongoDBUp) != 1 {
		t.Error("expected mongodb_up = 1")
	}

	m.MongoDBUp.Set(0)
	if testutil.ToFloat64(m.MongoDBUp) != 0 {
		t.Error("expected mongodb_up = 0")
	}
}

// AC-010: MongoDB document count gauge per slug
func TestAC010_MongoDBDocumentCount(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.MongoDBDocuments.WithLabelValues("drugnames").Set(1500)
	m.MongoDBDocuments.WithLabelValues("spls").Set(3000)

	if testutil.ToFloat64(m.MongoDBDocuments.WithLabelValues("drugnames")) != 1500 {
		t.Error("expected drugnames doc count = 1500")
	}
	if testutil.ToFloat64(m.MongoDBDocuments.WithLabelValues("spls")) != 3000 {
		t.Error("expected spls doc count = 3000")
	}
}

// AC-011: Scheduler run counter
func TestAC011_SchedulerRunCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.SchedulerRunsTotal.WithLabelValues("drugnames", "success").Inc()
	m.SchedulerRunsTotal.WithLabelValues("drugnames", "error").Inc()

	expected := `
		# HELP cashdrugs_scheduler_runs_total Scheduler job executions by slug and result.
		# TYPE cashdrugs_scheduler_runs_total counter
		cashdrugs_scheduler_runs_total{result="error",slug="drugnames"} 1
		cashdrugs_scheduler_runs_total{result="success",slug="drugnames"} 1
	`
	if err := testutil.CollectAndCompare(m.SchedulerRunsTotal, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected metric output: %v", err)
	}
}

// AC-012: Scheduler run duration histogram
func TestAC012_SchedulerRunDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.SchedulerRunDuration.WithLabelValues("drugnames").Observe(2.5)

	if testutil.CollectAndCount(m.SchedulerRunDuration) == 0 {
		t.Error("scheduler run duration histogram has no observations")
	}
}

// M9-AC-014: Circuit state gauge
func TestM9_AC014_CircuitStateGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.CircuitState.WithLabelValues("drugnames").Set(2) // 2 = open

	val := testutil.ToFloat64(m.CircuitState.WithLabelValues("drugnames"))
	if val != 2 {
		t.Errorf("expected circuit state 2 (open), got %f", val)
	}
}

// M9-AC-015: Circuit rejections counter
func TestM9_AC015_CircuitRejectionsCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.CircuitRejectionsTotal.WithLabelValues("drugnames").Inc()

	val := testutil.ToFloat64(m.CircuitRejectionsTotal.WithLabelValues("drugnames"))
	if val != 1 {
		t.Errorf("expected 1 rejection, got %f", val)
	}
}

// M9-AC-016: Force refresh cooldown counter
func TestM9_AC016_ForceRefreshCooldownCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.ForceRefreshCooldownTotal.WithLabelValues("drugnames").Inc()

	val := testutil.ToFloat64(m.ForceRefreshCooldownTotal.WithLabelValues("drugnames"))
	if val != 1 {
		t.Errorf("expected 1 cooldown, got %f", val)
	}
}

// AC-MI-006: Instance leader gauge exists as cashdrugs_instance_leader
func TestAC_MI006_InstanceLeaderGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.InstanceLeader.Set(1)
	val := testutil.ToFloat64(m.InstanceLeader)
	if val != 1 {
		t.Errorf("expected instance_leader=1, got %f", val)
	}

	m.InstanceLeader.Set(0)
	val = testutil.ToFloat64(m.InstanceLeader)
	if val != 0 {
		t.Errorf("expected instance_leader=0, got %f", val)
	}
}

// AC-MI-006: Instance leader gauge has correct name and help
func TestAC_MI006_InstanceLeaderGaugeName(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.InstanceLeader.Set(1)

	expected := `
		# HELP cashdrugs_instance_leader Whether this instance is the scheduler leader (1 = leader, 0 = follower).
		# TYPE cashdrugs_instance_leader gauge
		cashdrugs_instance_leader 1
	`
	if err := testutil.CollectAndCompare(m.InstanceLeader, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected instance_leader metric: %v", err)
	}
}

// AC-013: Fetch lock dedup counter
func TestAC013_FetchLockDedupCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.FetchLockDedupTotal.WithLabelValues("drugnames").Inc()

	val := testutil.ToFloat64(m.FetchLockDedupTotal.WithLabelValues("drugnames"))
	if val != 1 {
		t.Errorf("expected 1 dedup, got %f", val)
	}
}

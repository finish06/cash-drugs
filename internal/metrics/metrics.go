package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const namespace = "cashdrugs"

// Metrics holds all Prometheus metric collectors for the service.
type Metrics struct {
	HTTPRequestsTotal    *prometheus.CounterVec
	HTTPRequestDuration  *prometheus.HistogramVec
	CacheHitsTotal       *prometheus.CounterVec
	UpstreamFetchDuration *prometheus.HistogramVec
	UpstreamFetchErrors  *prometheus.CounterVec
	UpstreamFetchPages   *prometheus.CounterVec
	MongoDBPingDuration  prometheus.Gauge
	MongoDBUp            prometheus.Gauge
	MongoDBDocuments     *prometheus.GaugeVec
	SchedulerRunsTotal   *prometheus.CounterVec
	SchedulerRunDuration *prometheus.HistogramVec
	FetchLockDedupTotal  *prometheus.CounterVec
}

// NewMetrics creates and registers all Prometheus metrics with the given registry.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total HTTP requests.",
			},
			[]string{"slug", "method", "status_code"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request latency in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"slug", "method"},
		),
		CacheHitsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_hits_total",
				Help:      "Cache outcomes by slug and result.",
			},
			[]string{"slug", "outcome"},
		),
		UpstreamFetchDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "upstream_fetch_duration_seconds",
				Help:      "Upstream API fetch latency in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"slug"},
		),
		UpstreamFetchErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "upstream_fetch_errors_total",
				Help:      "Upstream API fetch failures.",
			},
			[]string{"slug"},
		),
		UpstreamFetchPages: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "upstream_fetch_pages_total",
				Help:      "Pages fetched from upstream APIs.",
			},
			[]string{"slug"},
		),
		MongoDBPingDuration: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "mongodb_ping_duration_seconds",
				Help:      "Last MongoDB ping latency in seconds.",
			},
		),
		MongoDBUp: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "mongodb_up",
				Help:      "MongoDB health status (1 = healthy, 0 = unhealthy).",
			},
		),
		MongoDBDocuments: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "mongodb_documents_total",
				Help:      "Document count per slug in MongoDB.",
			},
			[]string{"slug"},
		),
		SchedulerRunsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "scheduler_runs_total",
				Help:      "Scheduler job executions by slug and result.",
			},
			[]string{"slug", "result"},
		),
		SchedulerRunDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "scheduler_run_duration_seconds",
				Help:      "Scheduler job latency in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"slug"},
		),
		FetchLockDedupTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "fetchlock_dedup_total",
				Help:      "Deduplicated concurrent fetch attempts.",
			},
			[]string{"slug"},
		),
	}

	reg.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.CacheHitsTotal,
		m.UpstreamFetchDuration,
		m.UpstreamFetchErrors,
		m.UpstreamFetchPages,
		m.MongoDBPingDuration,
		m.MongoDBUp,
		m.MongoDBDocuments,
		m.SchedulerRunsTotal,
		m.SchedulerRunDuration,
		m.FetchLockDedupTotal,
	)

	return m
}

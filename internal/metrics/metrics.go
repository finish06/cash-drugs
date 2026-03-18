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

	// Container system metrics
	ContainerCPUUsage               prometheus.Gauge
	ContainerCPUCores               prometheus.Gauge
	ContainerMemoryRSS              prometheus.Gauge
	ContainerMemoryVMS              prometheus.Gauge
	ContainerMemoryLimit            prometheus.Gauge
	ContainerMemoryUsageRatio       prometheus.Gauge
	ContainerDiskTotal              prometheus.Gauge
	ContainerDiskFree               prometheus.Gauge
	ContainerDiskUsed               prometheus.Gauge
	ContainerNetworkReceiveBytes    *prometheus.GaugeVec
	ContainerNetworkTransmitBytes   *prometheus.GaugeVec
	ContainerNetworkReceivePackets  *prometheus.GaugeVec
	ContainerNetworkTransmitPackets *prometheus.GaugeVec

	// Circuit breaker metrics
	CircuitState              *prometheus.GaugeVec
	CircuitRejectionsTotal    *prometheus.CounterVec
	ForceRefreshCooldownTotal *prometheus.CounterVec

	// Concurrency limiter metrics
	InFlightRequests      prometheus.Gauge
	RejectedRequestsTotal prometheus.Counter

	// LRU cache metrics
	LRUCacheHitsTotal   *prometheus.CounterVec
	LRUCacheMissesTotal *prometheus.CounterVec
	LRUCacheSizeBytes   prometheus.Gauge

	// Upstream 404 metrics
	Upstream404Total *prometheus.CounterVec

	// Singleflight metrics
	SingleflightDedupTotal *prometheus.CounterVec

	// Warmup query metrics
	WarmupQueriesTotal   *prometheus.CounterVec
	WarmupQueriesPending prometheus.Gauge

	// Build and uptime metrics
	BuildInfo      *prometheus.GaugeVec
	UptimeSeconds  prometheus.Gauge

	// Instance role metrics
	InstanceLeader prometheus.Gauge
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

		// Container system metrics
		ContainerCPUUsage: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "container",
				Name:      "cpu_usage_seconds_total",
				Help:      "Total CPU time consumed by the container in seconds.",
			},
		),
		ContainerCPUCores: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "container",
				Name:      "cpu_cores_available",
				Help:      "Number of CPU cores available to the container.",
			},
		),
		ContainerMemoryRSS: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "container",
				Name:      "memory_rss_bytes",
				Help:      "Resident set size memory in bytes.",
			},
		),
		ContainerMemoryVMS: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "container",
				Name:      "memory_vms_bytes",
				Help:      "Virtual memory size in bytes.",
			},
		),
		ContainerMemoryLimit: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "container",
				Name:      "memory_limit_bytes",
				Help:      "Memory limit in bytes (-1 if unlimited).",
			},
		),
		ContainerMemoryUsageRatio: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "container",
				Name:      "memory_usage_ratio",
				Help:      "Memory usage ratio (RSS / limit).",
			},
		),
		ContainerDiskTotal: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "container",
				Name:      "disk_total_bytes",
				Help:      "Total disk space in bytes.",
			},
		),
		ContainerDiskFree: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "container",
				Name:      "disk_free_bytes",
				Help:      "Free disk space in bytes.",
			},
		),
		ContainerDiskUsed: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "container",
				Name:      "disk_used_bytes",
				Help:      "Used disk space in bytes.",
			},
		),
		ContainerNetworkReceiveBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "container",
				Name:      "network_receive_bytes_total",
				Help:      "Total bytes received per network interface.",
			},
			[]string{"interface"},
		),
		ContainerNetworkTransmitBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "container",
				Name:      "network_transmit_bytes_total",
				Help:      "Total bytes transmitted per network interface.",
			},
			[]string{"interface"},
		),
		ContainerNetworkReceivePackets: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "container",
				Name:      "network_receive_packets_total",
				Help:      "Total packets received per network interface.",
			},
			[]string{"interface"},
		),
		ContainerNetworkTransmitPackets: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "container",
				Name:      "network_transmit_packets_total",
				Help:      "Total packets transmitted per network interface.",
			},
			[]string{"interface"},
		),

		// Circuit breaker metrics
		CircuitState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "circuit_state",
				Help:      "Circuit breaker state per slug (0=closed, 1=half-open, 2=open).",
			},
			[]string{"slug"},
		),
		CircuitRejectionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "circuit_rejections_total",
				Help:      "Requests rejected by open circuit breaker.",
			},
			[]string{"slug"},
		),
		ForceRefreshCooldownTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "force_refresh_cooldown_total",
				Help:      "Force-refresh requests blocked by cooldown.",
			},
			[]string{"slug"},
		),

		// Concurrency limiter metrics
		InFlightRequests: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "inflight_requests",
				Help:      "Current number of in-flight application requests.",
			},
		),
		RejectedRequestsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rejected_requests_total",
				Help:      "Total requests rejected by the concurrency limiter.",
			},
		),

		// LRU cache metrics
		LRUCacheHitsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "lru_cache_hits_total",
				Help:      "Total LRU cache hits by slug.",
			},
			[]string{"slug"},
		),
		LRUCacheMissesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "lru_cache_misses_total",
				Help:      "Total LRU cache misses by slug.",
			},
			[]string{"slug"},
		),
		LRUCacheSizeBytes: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "lru_cache_size_bytes",
				Help:      "Current LRU cache size in bytes.",
			},
		),

		// Upstream 404 metrics
		Upstream404Total: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "upstream_404_total",
				Help:      "Total upstream 404 (not found) responses by slug.",
			},
			[]string{"slug"},
		),

		// Singleflight metrics
		SingleflightDedupTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "singleflight_dedup_total",
				Help:      "Total deduplicated concurrent requests via singleflight.",
			},
			[]string{"slug"},
		),

		// Warmup query metrics
		WarmupQueriesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "warmup_queries_total",
				Help:      "Total warmup query outcomes by slug and result.",
			},
			[]string{"slug", "result"},
		),
		WarmupQueriesPending: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "warmup_queries_pending",
				Help:      "Number of warmup queries still in progress.",
			},
		),

		// Build and uptime metrics
		BuildInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_info",
				Help:      "Build metadata for version tracking.",
			},
			[]string{"version", "git_commit", "go_version", "build_date"},
		),
		UptimeSeconds: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "uptime_seconds",
				Help:      "Process uptime in seconds.",
			},
		),

		// Instance role metrics
		InstanceLeader: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "instance_leader",
				Help:      "Whether this instance is the scheduler leader (1 = leader, 0 = follower).",
			},
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
		m.ContainerCPUUsage,
		m.ContainerCPUCores,
		m.ContainerMemoryRSS,
		m.ContainerMemoryVMS,
		m.ContainerMemoryLimit,
		m.ContainerMemoryUsageRatio,
		m.ContainerDiskTotal,
		m.ContainerDiskFree,
		m.ContainerDiskUsed,
		m.ContainerNetworkReceiveBytes,
		m.ContainerNetworkTransmitBytes,
		m.ContainerNetworkReceivePackets,
		m.ContainerNetworkTransmitPackets,
		m.CircuitState,
		m.CircuitRejectionsTotal,
		m.ForceRefreshCooldownTotal,
		m.InFlightRequests,
		m.RejectedRequestsTotal,
		m.LRUCacheHitsTotal,
		m.LRUCacheMissesTotal,
		m.LRUCacheSizeBytes,
		m.Upstream404Total,
		m.SingleflightDedupTotal,
		m.WarmupQueriesTotal,
		m.WarmupQueriesPending,
		m.BuildInfo,
		m.UptimeSeconds,
		m.InstanceLeader,
	)

	return m
}

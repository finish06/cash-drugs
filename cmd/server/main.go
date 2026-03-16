package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	_ "github.com/finish06/cash-drugs/docs"
	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/fetchlock"
	"github.com/finish06/cash-drugs/internal/handler"
	"github.com/finish06/cash-drugs/internal/logging"
	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/finish06/cash-drugs/internal/middleware"
	"github.com/finish06/cash-drugs/internal/scheduler"
	"github.com/finish06/cash-drugs/internal/upstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// @title        cash-drugs API
// @version      0.8.0
// @description  API cache/proxy — fetches from upstream REST APIs, stores in MongoDB, serves cached data to internal consumers.

// @host      localhost:8080
// @BasePath  /

// Build-time variables set via -ldflags.
var (
	version   = "dev" // overridden at build time via -ldflags "-X main.version=v0.6.0"
	gitCommit string  // -X main.gitCommit=$(git rev-parse --short HEAD)
	gitBranch string  // -X main.gitBranch=$(git rev-parse --abbrev-ref HEAD)
	buildDate string  // -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)
)

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}

	// Load full config for log_level
	appCfg, _ := config.LoadConfig(cfgPath)

	// Resolve log level: LOG_LEVEL env > config.yaml log_level > default (warn)
	logLevelStr := os.Getenv("LOG_LEVEL")
	if logLevelStr == "" && appCfg != nil {
		logLevelStr = appCfg.LogLevel
	}

	logLevel, err := logging.ParseLevel(logLevelStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	logFormat := os.Getenv("LOG_FORMAT")
	logger := logging.Setup(logLevel, logFormat, nil)
	slog.SetDefault(logger)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	slog.Info("config loaded", "component", "server", "endpoints", len(endpoints), "path", cfgPath)

	mongoURI, err := config.ResolveMongoURI(cfgPath)
	if err != nil {
		slog.Error("failed to resolve MongoDB URI", "component", "server", "error", err)
		os.Exit(1)
	}

	repo, err := cache.NewMongoRepository(mongoURI, 5*time.Second)
	if err != nil {
		slog.Error("failed to connect to MongoDB", "component", "cache", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to MongoDB", "component", "cache", "uri", mongoURI)
	defer repo.Close(context.Background())

	fetcher := upstream.NewHTTPFetcher()

	// Shared fetch locks for deduplication between scheduler and handler
	locks := fetchlock.New()

	// Initialize Prometheus metrics
	m := metrics.NewMetrics(prometheus.DefaultRegisterer)

	// Start background MongoDB metrics collector
	_, collName := repo.Names()
	mongoCollector := metrics.NewMongoCollector(repo.Client(), repo.Database(), collName, m, 30*time.Second)
	mongoCollector.Start()

	// Start background system metrics collector (Linux only via procfs)
	sysMetricsInterval := os.Getenv("SYSTEM_METRICS_INTERVAL")
	if sysMetricsInterval == "" && appCfg != nil && appCfg.SystemMetricsInterval != "" {
		sysMetricsInterval = appCfg.SystemMetricsInterval
	}
	if sysMetricsInterval == "" {
		sysMetricsInterval = "15s"
	}
	sysInterval, err := time.ParseDuration(sysMetricsInterval)
	if err != nil {
		slog.Warn("invalid system_metrics_interval, using 15s", "component", "metrics", "value", sysMetricsInterval, "error", err)
		sysInterval = 15 * time.Second
	}
	var sysCollector *metrics.SystemCollector
	if runtime.GOOS == "linux" {
		sysSource := metrics.NewProcfsSource()
		sysCollector = metrics.NewSystemCollector(sysSource, m, sysInterval, "/")
		sysCollector.Start()
		slog.Info("system metrics collector started", "component", "metrics", "interval", sysInterval)
	} else {
		slog.Info("system metrics collector skipped (not linux)", "component", "metrics", "os", runtime.GOOS)
	}

	// Start uptime gauge updater goroutine
	serverStartTime := time.Now()
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			m.UptimeSeconds.Set(time.Since(serverStartTime).Seconds())
			<-ticker.C
		}
	}()

	// Initialize LRU cache
	lruSizeMB := 256 // default
	if appCfg != nil && appCfg.LRUCacheSizeMB > 0 {
		lruSizeMB = appCfg.LRUCacheSizeMB
	}
	if envVal := os.Getenv("LRU_CACHE_SIZE_MB"); envVal != "" {
		if v, err := strconv.Atoi(envVal); err == nil && v >= 0 {
			lruSizeMB = v
		}
	}
	lruCache := cache.NewLRUCache(int64(lruSizeMB) * 1024 * 1024)
	slog.Info("LRU cache configured", "component", "server", "size_mb", lruSizeMB)

	// Circuit breaker configuration
	circuitFailureThreshold := uint32(5)
	if envVal := os.Getenv("CIRCUIT_FAILURE_THRESHOLD"); envVal != "" {
		if v, err := strconv.Atoi(envVal); err == nil && v > 0 {
			circuitFailureThreshold = uint32(v)
		}
	}
	circuitOpenDuration := 30 * time.Second
	if envVal := os.Getenv("CIRCUIT_OPEN_DURATION"); envVal != "" {
		if d, err := time.ParseDuration(envVal); err == nil {
			circuitOpenDuration = d
		}
	}
	circuitReg := upstream.NewCircuitRegistry(circuitFailureThreshold, circuitOpenDuration)
	slog.Info("circuit breaker configured", "component", "server", "failure_threshold", circuitFailureThreshold, "open_duration", circuitOpenDuration)

	// Force-refresh cooldown configuration
	cooldownDuration := 30 * time.Second
	if envVal := os.Getenv("FORCE_REFRESH_COOLDOWN"); envVal != "" {
		if d, err := time.ParseDuration(envVal); err == nil {
			cooldownDuration = d
		}
	}
	cooldownTracker := upstream.NewCooldownTracker(cooldownDuration)
	slog.Info("force-refresh cooldown configured", "component", "server", "duration", cooldownDuration)

	// Start background scheduler
	sched := scheduler.New(endpoints, fetcher, repo, locks, scheduler.WithMetrics(m), scheduler.WithLRU(lruCache), scheduler.WithCircuit(circuitReg))
	sched.Start(context.Background())

	cacheHandler := handler.NewCacheHandler(endpoints, repo, fetcher, handler.WithFetchLocks(locks), handler.WithMetrics(m), handler.WithLRU(lruCache), handler.WithCircuit(circuitReg), handler.WithCooldown(cooldownTracker))
	healthHandler := handler.NewHealthHandler(repo, handler.WithVersion(version))
	endpointsHandler := handler.NewEndpointsHandler(endpoints)
	versionHandler := handler.NewVersionHandler(version, gitCommit, gitBranch, buildDate, len(endpoints))

	// Set build info Prometheus gauge
	m.BuildInfo.WithLabelValues(version, gitCommit, runtime.Version(), buildDate).Set(1)

	// Resolve concurrency limit: env var > config > default (50)
	maxConcurrent := 50
	if appCfg != nil && appCfg.MaxConcurrentRequests > 0 {
		maxConcurrent = appCfg.MaxConcurrentRequests
	}
	if envVal := os.Getenv("MAX_CONCURRENT_REQUESTS"); envVal != "" {
		if v, err := strconv.Atoi(envVal); err == nil && v > 0 {
			maxConcurrent = v
		}
	}
	slog.Info("concurrency limiter configured", "component", "server", "max_concurrent_requests", maxConcurrent)

	limiter := middleware.NewConcurrencyLimiter(maxConcurrent, m)

	// Application routes wrapped with concurrency limiter
	appMux := http.NewServeMux()
	appMux.Handle("/api/cache/", cacheHandler)
	appMux.Handle("/api/endpoints", endpointsHandler)
	appMux.Handle("/swagger/", httpSwagger.WrapHandler)
	appMux.HandleFunc("/openapi.json", handler.ServeOpenAPISpec)

	// Outer mux: exempt paths registered directly, app routes wrapped with limiter
	mux := http.NewServeMux()
	mux.Handle("/health", healthHandler)
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/version", versionHandler)
	mux.Handle("/", limiter.Wrap(appMux))

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	// Wrap outermost handler with gzip middleware (compresses all responses including 503)
	gzipHandler := middleware.GzipMiddleware(mux)

	srv := &http.Server{
		Addr:         addr,
		Handler:      gzipHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down", "component", "server")
		if sysCollector != nil {
			sysCollector.Stop()
		}
		mongoCollector.Stop()
		sched.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	slog.Info("server starting", "component", "server", "addr", addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server failed", "component", "server", "error", err)
		os.Exit(1)
	}
}

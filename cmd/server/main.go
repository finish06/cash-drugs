package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	_ "github.com/finish06/cash-drugs/docs"
	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/fetchlock"
	"github.com/finish06/cash-drugs/internal/handler"
	"github.com/finish06/cash-drugs/internal/logging"
	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/finish06/cash-drugs/internal/scheduler"
	"github.com/finish06/cash-drugs/internal/upstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// @title        cash-drugs API
// @version      0.6.0
// @description  API cache/proxy — fetches from upstream REST APIs, stores in MongoDB, serves cached data to internal consumers.

// @host      localhost:8080
// @BasePath  /

// version is set at build time via -ldflags "-X main.version=v0.5.0".
var version = "dev" // overridden at build time via -ldflags "-X main.version=v0.6.0"

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

	// Start background scheduler
	sched := scheduler.New(endpoints, fetcher, repo, locks, scheduler.WithMetrics(m))
	sched.Start(context.Background())

	cacheHandler := handler.NewCacheHandler(endpoints, repo, fetcher, handler.WithFetchLocks(locks), handler.WithMetrics(m))
	healthHandler := handler.NewHealthHandler(repo, handler.WithVersion(version))
	endpointsHandler := handler.NewEndpointsHandler(endpoints)

	mux := http.NewServeMux()
	mux.Handle("/api/cache/", cacheHandler)
	mux.Handle("/api/endpoints", endpointsHandler)
	mux.Handle("/health", healthHandler)
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/swagger/", httpSwagger.WrapHandler)
	mux.HandleFunc("/openapi.json", handler.ServeOpenAPISpec)

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{Addr: addr, Handler: mux}

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

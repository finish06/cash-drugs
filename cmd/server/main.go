package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/finish06/drugs/docs"
	"github.com/finish06/drugs/internal/cache"
	"github.com/finish06/drugs/internal/config"
	"github.com/finish06/drugs/internal/fetchlock"
	"github.com/finish06/drugs/internal/handler"
	"github.com/finish06/drugs/internal/logging"
	"github.com/finish06/drugs/internal/scheduler"
	"github.com/finish06/drugs/internal/upstream"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// @title        drugs API
// @version      0.5.0
// @description  API cache/proxy — fetches from upstream REST APIs, stores in MongoDB, serves cached data to internal consumers.

// @host      localhost:8080
// @BasePath  /

// version is set at build time via -ldflags "-X main.version=v0.5.0".
var version = "dev"

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

	// Start background scheduler
	sched := scheduler.New(endpoints, fetcher, repo, locks)
	sched.Start(context.Background())

	cacheHandler := handler.NewCacheHandler(endpoints, repo, fetcher, handler.WithFetchLocks(locks))
	healthHandler := handler.NewHealthHandler(repo, handler.WithVersion(version))
	endpointsHandler := handler.NewEndpointsHandler(endpoints)

	mux := http.NewServeMux()
	mux.Handle("/api/cache/", cacheHandler)
	mux.Handle("/api/endpoints", endpointsHandler)
	mux.Handle("/health", healthHandler)
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

package main

import (
	"context"
	"log"
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
	"github.com/finish06/drugs/internal/scheduler"
	"github.com/finish06/drugs/internal/upstream"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// @title        drugs API
// @version      0.2.0
// @description  API cache/proxy — fetches from upstream REST APIs, stores in MongoDB, serves cached data to internal consumers.

// @host      localhost:8080
// @BasePath  /

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("Loaded %d endpoint(s) from %s", len(endpoints), cfgPath)

	mongoURI, err := config.ResolveMongoURI(cfgPath)
	if err != nil {
		log.Fatalf("Failed to resolve MongoDB URI: %v", err)
	}

	repo, err := cache.NewMongoRepository(mongoURI, 5*time.Second)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	log.Printf("Connected to MongoDB at %s", mongoURI)
	defer repo.Close(context.Background())

	fetcher := upstream.NewHTTPFetcher()

	// Shared fetch locks for deduplication between scheduler and handler
	locks := fetchlock.New()

	// Start background scheduler
	sched := scheduler.New(endpoints, fetcher, repo, locks)
	sched.Start(context.Background())

	cacheHandler := handler.NewCacheHandler(endpoints, repo, fetcher, handler.WithFetchLocks(locks))
	healthHandler := handler.NewHealthHandler(repo)
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
		log.Println("Shutting down...")
		sched.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	log.Printf("Starting server on %s", addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}

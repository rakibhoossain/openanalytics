package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"openanalytics/internal/config"
	"openanalytics/internal/currency"
	"openanalytics/internal/integrations/meta"
	"openanalytics/internal/postgres"
	"openanalytics/internal/query"
	"openanalytics/pkg/httputil"
)

func main() {
	cfg := config.Load()

	log.Printf("==================================================")
	log.Printf("Starting OpenAnalytics Query Engine on :%s", cfg.QueryPort)
	log.Printf("Environment: %s", cfg.Env)
	log.Printf("==================================================")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Connect to ClickHouse for analytical queries
	qs, err := query.NewService(ctx, query.Config{
		Addr:     cfg.ClickHouseAddr,
		Database: cfg.ClickHouseDatabase,
		Username: cfg.ClickHouseUsername,
		Password: cfg.ClickHousePassword,
	})
	if err != nil {
		log.Fatalf("[ClickHouse] Fatal: failed to connect to %s: %v", cfg.ClickHouseAddr, err)
	}
	defer func() {
		if err := qs.Close(); err != nil {
			log.Printf("[ClickHouse] Error closing query service: %v", err)
		}
	}()
	log.Printf("[ClickHouse] Connected to analytical engine at %s (DB: %s)", cfg.ClickHouseAddr, cfg.ClickHouseDatabase)

	// 2. Connect to Redis for live streaming and Pub/Sub
	var rdb *redis.Client
	if cfg.RedisAddr != "" {
		rdb = redis.NewClient(&redis.Options{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		})
		if err := rdb.Ping(ctx).Err(); err != nil {
			log.Printf("[Redis] Warning: Redis ping failed to %s: %v", cfg.RedisAddr, err)
		} else {
			log.Printf("[Redis] Connected to %s", cfg.RedisAddr)
		}
		defer rdb.Close()
	}

	// 2b. Connect to PostgreSQL for relational settings and dynamic exchange rates
	var pgPool *pgxpool.Pool
	if cfg.PostgresURL != "" {
		pool, err := postgres.NewPool(ctx, cfg.PostgresURL, cfg.PostgresMaxConns)
		if err != nil {
			log.Printf("[Postgres] Warning: could not connect to PostgreSQL: %v", err)
		} else {
			pgPool = pool
			defer pgPool.Close()
			_ = postgres.Migrate(ctx, pgPool)
			log.Printf("[Postgres] Connected to relational store at %s", cfg.PostgresURL)
		}
	}

	// 2c. Currency Service & Scheduler for dynamic multi-currency analytics
	currencyService := currency.NewService(pgPool, rdb)
	_ = currencyService.LoadRates(ctx)
	currencyService.StartBackgroundSync(ctx, time.Duration(cfg.ExchangeRateSyncHours)*time.Hour)
	if cfg.OpenExchangeRatesAppID != "" {
		currencyScheduler := currency.NewScheduler(cfg.OpenExchangeRatesAppID, pgPool, rdb, currencyService, cfg.ExchangeRateSyncHours)
		currencyScheduler.Start(ctx)
	}
	qs.WithCurrency(currencyService)

	// 2d. Meta CAPI Integration Repository & Client
	metaRepo := meta.NewRepository(pgPool, rdb)
	metaClient := meta.NewClient("")

	// 3. Initialize WebSocket Hub for real-time event streaming
	wsHub := query.NewWebSocketHub(rdb, qs)
	wsHub.Start(ctx)

	// 4. Router setup
	r := chi.NewRouter()

	// Global Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// CORS configuration for dashboard web applications
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"}, // CRITICAL(cors-policy): In production, restrict to merchant domains and dashboard UI origin
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Tenant-ID", "X-Shop-ID", "X-Currency"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// 5. Register HTTP Handlers & WebSocket routes
	handler := query.NewHandler(qs)
	if rdb != nil {
		handler.WithRedis(rdb)
	}
	handler.WithWSHub(wsHub)
	if pgPool != nil {
		handler.WithPostgres(pgPool)
	}
	handler.WithCurrency(currencyService)
	handler.WithMetaIntegration(metaRepo, metaClient)
	handler.RegisterRoutes(r)

	// Root status endpoint (headless analytics engine)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]any{
			"service": "openanalytics-query",
			"status":  "healthy",
			"version": "1.0.0",
		})
	})

	// 5. Start HTTP Server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.QueryPort),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("[Query Engine] HTTP server listening on port %s", cfg.QueryPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Query Engine] Listen error: %v", err)
		}
	}()

	// 6. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Query Engine] Initiating graceful shutdown...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Query Engine] Server shutdown error: %v", err)
	}

	log.Println("[Query Engine] Server stopped cleanly")
}

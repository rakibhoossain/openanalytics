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
	"github.com/redis/go-redis/v9"

	"openanalytics/internal/config"
	"openanalytics/internal/geo"
	"openanalytics/internal/ingest"
	"openanalytics/internal/kafka"
	"openanalytics/pkg/httputil"
)

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Initialize GeoIP Service
	geoService, err := geo.NewService(geo.Config{
		DataDir: cfg.GeoIPDataDir,
	})
	if err != nil {
		log.Printf("[GeoIP] Warning: GeoIP service initialization notice: %v", err)
	} else {
		defer geoService.Close()
		geoService.StartWatcher(ctx, 1*time.Hour)
		log.Printf("[GeoIP] Service active reading databases from %s", cfg.GeoIPDataDir)
	}

	// 2. Initialize Partitioned Kafka Producer
	producer := kafka.NewProducer(kafka.ProducerConfig{
		Brokers:        cfg.KafkaBrokers,
		Topic:          cfg.KafkaEventsTopic,
		BatchSize:      cfg.KafkaBatchSize,
		BatchTimeoutMs: cfg.KafkaBatchTimeoutMs,
	})
	defer func() {
		log.Println("Flushing and closing Kafka producer...")
		if err := producer.Close(); err != nil {
			log.Printf("Error closing Kafka producer: %v", err)
		}
	}()
	log.Printf("[Kafka] Producer connected to %s topic %s", cfg.KafkaBrokers, cfg.KafkaEventsTopic)

	// 3. Initialize Redis Client
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[Redis] Warning: Redis ping failed to %s: %v", cfg.RedisAddr, err)
	} else {
		log.Printf("[Redis] Connected to %s", cfg.RedisAddr)
	}

	// 4. Initialize Ingest Handler
	ingestHandler := ingest.NewHandler(ingest.Config{
		GeoService:  geoService,
		Producer:    producer,
		RedisClient: rdb,
		Salt:        "aicart_openanalytics_salt",
	})

	// 5. Configure Go-Chi Router & Middleware
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(5 * time.Second))

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Shop-Id", "X-Tenant-Id", "openpanel-client-id"},
		AllowCredentials: false,
		MaxAge:           86400 * 7,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]string{
			"status":  "healthy",
			"service": "openanalytics-ingest",
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/track", ingestHandler.HandleTrack)
		r.Post("/replay", ingestHandler.HandleReplay)
		r.Post("/batch", ingestHandler.HandleBatch)
		r.Get("/track/device-id", ingestHandler.HandleDeviceID)
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.IngestPort),
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("OpenAnalytics High-Throughput Ingestion Service listening on :%s", cfg.IngestPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Ingestion service failure: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down ingestion service gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Ingestion service stopped cleanly")
}

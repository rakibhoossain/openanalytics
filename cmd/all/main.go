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

	"openanalytics/internal/clickhouse"
	"openanalytics/internal/config"
	"openanalytics/internal/cron"
	"openanalytics/internal/domain"
	"openanalytics/internal/geo"
	"openanalytics/internal/ingest"
	"openanalytics/internal/kafka"
	"openanalytics/internal/ml"
	"openanalytics/internal/postgres"
	"openanalytics/internal/query"
	"openanalytics/internal/session"
	"openanalytics/pkg/httputil"
)

func main() {
	cfg := config.Load()

	log.Println("==================================================================")
	log.Println(" Starting OpenAnalytics Unified Data Plane (All-In-One)")
	log.Printf(" - Ingestion Engine:  http://127.0.0.1:%s", cfg.IngestPort)
	log.Printf(" - Query & Dashboard: http://127.0.0.1:%s/ui/", cfg.QueryPort)
	log.Printf(" - Stream Worker:     Active (Kafka -> Redis -> ClickHouse)")
	log.Printf(" - Behavioral ML:     Active (Real-time Intent Scorer)")
	log.Println("==================================================================")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ------------------------------------------------------------------
	// 1. Shared Backing Services (Redis, GeoIP, Kafka Producer)
	// ------------------------------------------------------------------
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[Redis] Warning: ping failed to %s: %v", cfg.RedisAddr, err)
	} else {
		log.Printf("[Redis] Connected to %s", cfg.RedisAddr)
	}

	geoService, err := geo.NewService(geo.Config{
		DataDir: cfg.GeoIPDataDir,
	})
	if err != nil {
		log.Printf("[GeoIP] Warning: init notice: %v", err)
	} else {
		defer geoService.Close()
		geoService.StartWatcher(ctx, 1*time.Hour)
	}

	kafkaProducer := kafka.NewProducer(kafka.ProducerConfig{
		Brokers:        cfg.KafkaBrokers,
		Topic:          cfg.KafkaEventsTopic,
		BatchSize:      cfg.KafkaBatchSize,
		BatchTimeoutMs: cfg.KafkaBatchTimeoutMs,
	})
	defer kafkaProducer.Close()

	// ------------------------------------------------------------------
	// 2. ClickHouse Batch Writer & Session State Machine
	// ------------------------------------------------------------------
	chWriter, err := clickhouse.NewBatchWriter(ctx, clickhouse.Config{
		Addr:          cfg.ClickHouseAddr,
		Database:      cfg.ClickHouseDatabase,
		Username:      cfg.ClickHouseUsername,
		Password:      cfg.ClickHousePassword,
		BatchSize:     cfg.KafkaBatchSize,
		FlushInterval: 2 * time.Second,
	})
	if err != nil {
		log.Printf("[Worker] Warning: ClickHouse writer init: %v", err)
	} else {
		defer chWriter.Close()
	}

	sessionMgr := session.NewManager(rdb, time.Duration(cfg.RedisSessionTTLMinutes)*time.Minute)
	sessionReaper := session.NewReaper(rdb, chWriter, time.Duration(cfg.RedisSessionTTLMinutes)*time.Minute, 1*time.Minute)
	sessionReaper.Start(ctx)

	// Start Reporting & Automated Intelligence Schedulers (Hourly Rollup + Daily Insights)
	if chWriter != nil && chWriter.Conn() != nil {
		reportingCron := cron.NewScheduler(chWriter.Conn())
		reportingCron.Start(ctx)
	}

	// ------------------------------------------------------------------
	// 3. Start Ingestion Engine (:8080)
	// ------------------------------------------------------------------
	ingestHandler := ingest.NewHandler(ingest.Config{
		GeoService:  geoService,
		Producer:    kafkaProducer,
		RedisClient: rdb,
		Salt:        "aicart_openanalytics_salt",
		CHWriter:    chWriter,
		SessionMgr:  sessionMgr,
	})

	ingestRouter := chi.NewRouter()
	ingestRouter.Use(middleware.RequestID)
	ingestRouter.Use(middleware.RealIP)
	ingestRouter.Use(middleware.Recoverer)
	ingestRouter.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Shop-Id", "X-Tenant-Id"},
		AllowCredentials: false,
		MaxAge:           86400 * 7,
	}))

	ingestRouter.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]string{
			"status":  "healthy",
			"service": "openanalytics-ingest",
		})
	})
	ingestRouter.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]string{
			"status":  "healthy",
			"service": "openanalytics-ingest",
		})
	})

	ingestRouter.Route("/api/v1", func(r chi.Router) {
		r.Post("/track", ingestHandler.HandleTrack)
		r.Post("/batch", ingestHandler.HandleBatch)
		r.Post("/track/batch", ingestHandler.HandleBatch)
		r.Get("/track/device-id", ingestHandler.HandleDeviceID)
	})

	ingestServer := &http.Server{
		Addr:         ":" + cfg.IngestPort,
		Handler:      ingestRouter,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("[Ingest Service] Listening on port %s", cfg.IngestPort)
		if err := ingestServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Ingest Service] Listen error: %v", err)
		}
	}()

	// ------------------------------------------------------------------
	// 4. Start Query Engine & WebSocket Realtime Hub (:8081)
	// ------------------------------------------------------------------
	pgRepo, err := postgres.NewRepository(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Printf("[Query Engine] Warning: PostgreSQL init error: %v", err)
	} else {
		defer pgRepo.Close()
	}

	qs, err := query.NewService(ctx, query.Config{
		Addr:     cfg.ClickHouseAddr,
		Database: cfg.ClickHouseDatabase,
		Username: cfg.ClickHouseUsername,
		Password: cfg.ClickHousePassword,
	})
	if err != nil {
		log.Printf("[Query Engine] Warning: ClickHouse query service init error: %v", err)
	} else {
		defer qs.Close()
	}

	wsHub := query.NewWebSocketHub(rdb, qs)
	wsHub.Start(ctx)

	queryHandler := query.NewHandler(qs, pgRepo).WithRedis(rdb).WithWSHub(wsHub)

	// ------------------------------------------------------------------
	// 5. Start Stream Worker (Kafka Partition Consumer)
	// ------------------------------------------------------------------
	workerConsumer := kafka.NewConsumer(kafka.ConsumerConfig{
		Brokers:       cfg.KafkaBrokers,
		Topic:         cfg.KafkaEventsTopic,
		ConsumerGroup: cfg.KafkaConsumerGroup,
		WorkerID:      "unified-worker-1",
	})
	defer workerConsumer.Close()

	go func() {
		log.Printf("[Stream Worker] Kafka consumer loop active for topic %s", cfg.KafkaEventsTopic)
		_ = workerConsumer.ConsumeLoop(ctx, func(ctx context.Context, event *domain.Event) error {
			var procErr error
			if sessionMgr != nil && chWriter != nil {
				procErr = sessionMgr.ProcessEventLifecycle(ctx, event, chWriter)
			} else if chWriter != nil {
				procErr = chWriter.AddEvent(ctx, event)
			}

			if procErr == nil && wsHub != nil {
				shopKey := event.ShopID.String()
				wsHub.BroadcastEvents(shopKey, 1)
				if sessionMgr != nil {
					if activeCount, aerr := sessionMgr.GetActiveSessionsCount(ctx, event.ShopID); aerr == nil {
						wsHub.BroadcastVisitors(shopKey, activeCount)
					}
				}
			}
			return procErr
		})
	}()

	// ------------------------------------------------------------------
	// 6. Start Behavioral ML Scorer Worker
	// ------------------------------------------------------------------
	mlScorer, err := ml.NewScorer(cfg.MLModelPath, rdb)
	if err == nil {
		mlConsumer := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers:       cfg.KafkaBrokers,
			Topic:         cfg.KafkaEventsTopic,
			ConsumerGroup: cfg.KafkaConsumerGroup + "-ml",
			WorkerID:      "unified-ml-1",
		})
		defer mlConsumer.Close()

		go func() {
			log.Println("[ML Worker] Behavioral inference stream active")
			_ = mlConsumer.ConsumeLoop(ctx, func(ctx context.Context, event *domain.Event) error {
				score, isHighIntent, _ := mlScorer.ProcessEvent(ctx, event)
				if isHighIntent {
					log.Printf("[ML Intent Alert] High intent (%.1f%%) on device %s (Shop: %s, Event: %s)",
						score*100, event.DeviceID, event.ShopID.String(), event.Name)
				}
				return nil
			})
		}()
	}

	// ------------------------------------------------------------------
	// 7. Configure UI Dashboard & Query Routes (:8081)
	// ------------------------------------------------------------------
	queryRouter := chi.NewRouter()
	queryRouter.Use(middleware.RequestID)
	queryRouter.Use(middleware.RealIP)
	queryRouter.Use(middleware.Recoverer)
	queryRouter.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Tenant-ID", "X-Shop-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	queryHandler.RegisterRoutes(queryRouter)

	// In all-in-one unified mode, also route /api/v1/track on 8081 for direct UI convenience
	queryRouter.Post("/api/v1/track", ingestHandler.HandleTrack)
	queryRouter.Post("/api/v1/batch", ingestHandler.HandleBatch)
	queryRouter.Post("/api/v1/track/batch", ingestHandler.HandleBatch)
	queryRouter.Get("/api/v1/track/device-id", ingestHandler.HandleDeviceID)

	// Redirect root / directly to /ui/
	queryRouter.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusFound)
	})

	// Serve Static UI Assets (resolving from current dir or openanalytics/)
	uiPath := "ui"
	if _, err := os.Stat(uiPath); os.IsNotExist(err) {
		uiPath = "openanalytics/ui"
	}
	uiDir := http.Dir(uiPath)
	queryRouter.Handle("/ui/*", http.StripPrefix("/ui", http.FileServer(uiDir)))
	queryRouter.Get("/ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusMovedPermanently)
	})

	queryServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.QueryPort),
		Handler:      queryRouter,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		log.Printf("[Query Service] Listening on port %s (Dashboard: http://localhost:%s/ui/)", cfg.QueryPort, cfg.QueryPort)
		if err := queryServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Query Service] Listen error: %v", err)
		}
	}()

	// ------------------------------------------------------------------
	// 6. Graceful Termination on SIGINT / SIGTERM
	// ------------------------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Printf("[Unified Runner] Received signal %v, shutting down all services...", sig)
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	_ = ingestServer.Shutdown(shutdownCtx)
	_ = queryServer.Shutdown(shutdownCtx)

	log.Println("[Unified Runner] All services stopped cleanly.")
}

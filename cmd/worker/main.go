package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"openanalytics/internal/clickhouse"
	"openanalytics/internal/config"
	"openanalytics/internal/cron"
	"openanalytics/internal/currency"
	"openanalytics/internal/domain"
	"openanalytics/internal/integrations/meta"
	"openanalytics/internal/kafka"
	"openanalytics/internal/postgres"
	"openanalytics/internal/session"
)

func main() {
	cfg := config.Load()
	workerID := os.Getenv("WORKER_ID")
	if workerID == "" {
		workerID = fmt.Sprintf("worker-%d", time.Now().UnixNano()%1000)
	}

	log.Printf("==================================================")
	log.Printf("Starting OpenAnalytics Stream Worker [%s]", workerID)
	log.Printf("Consumer Group: %s", cfg.KafkaConsumerGroup)
	log.Printf("==================================================")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Connect to Redis (Distributed Session State on port 6380)
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[Redis] Warning: Connection ping failed to %s: %v", cfg.RedisAddr, err)
	} else {
		log.Printf("[Redis] Connected to %s", cfg.RedisAddr)
	}

	// 2. Connect to ClickHouse (Native TCP on port 9000)
	chWriter, err := clickhouse.NewBatchWriter(ctx, clickhouse.Config{
		Addr:          cfg.ClickHouseAddr,
		Database:      cfg.ClickHouseDatabase,
		Username:      cfg.ClickHouseUsername,
		Password:      cfg.ClickHousePassword,
		BatchSize:     cfg.KafkaBatchSize,
		FlushInterval: 2 * time.Second,
	})
	if err != nil {
		log.Fatalf("[ClickHouse] Fatal: Failed to initialize batch writer: %v", err)
	}
	defer func() {
		log.Println("[ClickHouse] Flushing remaining in-memory batches...")
		if err := chWriter.Close(); err != nil {
			log.Printf("[ClickHouse] Error closing batch writer: %v", err)
		}
	}()
	log.Printf("[ClickHouse] Native TCP batch writer connected to %s (DB: %s)", cfg.ClickHouseAddr, cfg.ClickHouseDatabase)

	// 3. Initialize Distributed Session State Machine & Background Reaper
	sessionMgr := session.NewManager(rdb, time.Duration(cfg.RedisSessionTTLMinutes)*time.Minute)
	sessionReaper := session.NewReaper(rdb, chWriter, time.Duration(cfg.RedisSessionTTLMinutes)*time.Minute, 1*time.Minute)
	sessionReaper.Start(ctx)

	// 3b. Initialize Background Reporting & Intelligence Cron Schedulers
	if chWriter.Conn() != nil {
		reportingCron := cron.NewScheduler(chWriter.Conn())
		reportingCron.Start(ctx)
	}

	// 3c. Initialize PostgreSQL & Meta Conversions API (CAPI) Integration Engine
	pgPool, err := postgres.NewPool(ctx, cfg.PostgresURL, cfg.PostgresMaxConns)
	if err != nil {
		log.Printf("[Worker %s] Warning: postgres pool init: %v", workerID, err)
	} else {
		defer pgPool.Close()
		_ = postgres.Migrate(ctx, pgPool)
	}

	metaRepo := meta.NewRepository(pgPool, rdb)
	metaService := meta.NewService(ctx, metaRepo, nil)
	defer metaService.Close()
	log.Printf("[Worker %s] Meta CAPI forwarder service active", workerID)

	// 3d. Initialize Distributed Currency Service & Exchange Rate Scheduler
	currencyService := currency.NewService(pgPool, rdb)
	_ = currencyService.LoadRates(ctx)
	if cfg.OpenExchangeRatesAppID != "" {
		currencyScheduler := currency.NewScheduler(cfg.OpenExchangeRatesAppID, pgPool, rdb, currencyService, cfg.ExchangeRateSyncHours)
		currencyScheduler.Start(ctx)
		log.Printf("[Worker %s] Currency exchange rate scheduler active (sync interval: %dh)", workerID, cfg.ExchangeRateSyncHours)
	}

	// 4. Initialize Kafka Consumer Group Reader
	consumer := kafka.NewConsumer(kafka.ConsumerConfig{
		Brokers:       cfg.KafkaBrokers,
		Topic:         cfg.KafkaEventsTopic,
		ConsumerGroup: cfg.KafkaConsumerGroup,
		WorkerID:      workerID,
	})
	defer consumer.Close()

	// 5. Event processing pipeline (session_start, session_end, raw events, Meta CAPI dispatch, and live WS pub/sub)
	eventHandler := func(ctx context.Context, event *domain.Event) error {
		metaService.DispatchAsync(event)
		procErr := sessionMgr.ProcessEventLifecycle(ctx, event, chWriter)
		if procErr == nil && rdb != nil {
			tenantKey := ""
			if event.TenantID != uuid.Nil {
				tenantKey = event.TenantID.String()
			}
			shopKey := event.ShopID.String()

			// Broadcast live event notification to Redis Pub/Sub for WebSocket Hub
			eventPayload := `{"json":{"count":1}}`
			_ = rdb.Publish(ctx, fmt.Sprintf("analytics:live:events:%s:%s", tenantKey, shopKey), eventPayload).Err()
			if tenantKey != "" {
				_ = rdb.Publish(ctx, fmt.Sprintf("analytics:live:events:%s", shopKey), eventPayload).Err()
			}

			// Broadcast updated live active visitor count
			if activeCount, aerr := sessionMgr.GetActiveSessionsCount(ctx, event.ShopID); aerr == nil {
				visitorPayload := fmt.Sprintf(`{"json":%d}`, activeCount)
				_ = rdb.Publish(ctx, fmt.Sprintf("analytics:live:visitors:%s:%s", tenantKey, shopKey), visitorPayload).Err()
				if tenantKey != "" {
					_ = rdb.Publish(ctx, fmt.Sprintf("analytics:live:visitors:%s", shopKey), visitorPayload).Err()
				}
			}
		}
		return procErr
	}

	// 6. Launch Consumer Loop in background goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := consumer.ConsumeLoop(ctx, eventHandler); err != nil {
			errChan <- err
		}
	}()

	// 7. Handle Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Printf("[Worker %s] Received signal %v, shutting down gracefully...", workerID, sig)
	case err := <-errChan:
		log.Printf("[Worker %s] Consumer loop encountered fatal error: %v", workerID, err)
	}

	cancel() // Stop consumer loop
	log.Printf("[Worker %s] Consumer stopped cleanly", workerID)
}

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"openanalytics/internal/clickhouse"
	"openanalytics/internal/config"
	"openanalytics/internal/cron"
	"openanalytics/internal/domain"
	"openanalytics/internal/kafka"
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

	// 4. Initialize Kafka Consumer Group Reader
	consumer := kafka.NewConsumer(kafka.ConsumerConfig{
		Brokers:       cfg.KafkaBrokers,
		Topic:         cfg.KafkaEventsTopic,
		ConsumerGroup: cfg.KafkaConsumerGroup,
		WorkerID:      workerID,
	})
	defer consumer.Close()

	// 5. Event processing pipeline (session_start, session_end, and raw events)
	eventHandler := func(ctx context.Context, event *domain.Event) error {
		return sessionMgr.ProcessEventLifecycle(ctx, event, chWriter)
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

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

	"openanalytics/internal/config"
	"openanalytics/internal/domain"
	"openanalytics/internal/kafka"
	"openanalytics/internal/ml"
)

func main() {
	cfg := config.Load()
	workerID := os.Getenv("WORKER_ID")
	if workerID == "" {
		workerID = fmt.Sprintf("ml-worker-%d", time.Now().UnixNano()%1000)
	}

	log.Printf("==================================================")
	log.Printf("Starting OpenAnalytics Behavioral ML Worker [%s]", workerID)
	log.Printf("Model Path: %s", cfg.MLModelPath)
	log.Printf("==================================================")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Connect to Redis (Feature Store Cache on port 6380)
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[Redis] Warning: connection ping failed: %v", err)
	} else {
		log.Printf("[Redis] Connected to feature store cache at %s", cfg.RedisAddr)
	}

	// 2. Initialize Real-time Behavioral Inference Scorer
	scorer, err := ml.NewScorer(cfg.MLModelPath, rdb)
	if err != nil {
		log.Fatalf("[ML Worker] Fatal: failed to initialize scorer: %v", err)
	}

	// 3. Initialize Kafka Consumer Group Reader
	consumerGroup := cfg.KafkaConsumerGroup + "-ml"
	consumer := kafka.NewConsumer(kafka.ConsumerConfig{
		Brokers:       cfg.KafkaBrokers,
		Topic:         cfg.KafkaEventsTopic,
		ConsumerGroup: consumerGroup,
		WorkerID:      workerID,
	})
	defer consumer.Close()
	log.Printf("[Kafka] Subscribed to %s via consumer group %s", cfg.KafkaEventsTopic, consumerGroup)

	// 4. ML Stream Processor Handler
	eventHandler := func(ctx context.Context, event *domain.Event) error {
		// CRITICAL(sub-millisecond-inference): Evaluate intent without blocking stream ingestion
		score, isHighIntent, _, err := scorer.ProcessEvent(ctx, event)
		if err != nil {
			log.Printf("[ML Worker] Warning: scoring error for device %s: %v", event.DeviceID, err)
			return nil
		}

		if isHighIntent {
			// CRITICAL(intent-action): High-propensity shopper identified.
			// In production, emit to ai-cart conversion webhook or real-time offer engine.
			log.Printf("[ML Intent Alert] High purchase intent (%.2f%%) detected for device %s (Shop: %s, Event: %s)",
				score*100.0, event.DeviceID, event.ShopID.String(), event.Name)
		}

		return nil
	}

	// 5. Launch Consumer Loop in background goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := consumer.ConsumeLoop(ctx, eventHandler); err != nil {
			errChan <- err
		}
	}()

	// 6. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Printf("[ML Worker %s] Received signal %v, shutting down...", workerID, sig)
	case err := <-errChan:
		log.Printf("[ML Worker %s] Consumer loop error: %v", workerID, err)
	}

	cancel()
	log.Printf("[ML Worker %s] Stopped cleanly", workerID)
}

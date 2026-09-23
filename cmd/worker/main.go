package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"openanalytics/internal/config"
)

func main() {
	cfg := config.Load()
	workerID := os.Getenv("WORKER_ID")
	if workerID == "" {
		workerID = "worker-default"
	}

	log.Printf("Starting OpenAnalytics Stream Worker [%s] consuming group [%s]...", workerID, cfg.KafkaConsumerGroup)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Stream worker event loop placeholder
	go func() {
		log.Printf("Worker [%s] connected to Kafka [%s] topic [%s]", workerID, cfg.KafkaBrokers, cfg.KafkaEventsTopic)
		<-ctx.Done()
	}()

	<-quit
	log.Printf("Stopping worker [%s] gracefully...", workerID)
	cancel()
	log.Printf("Worker [%s] stopped", workerID)
}

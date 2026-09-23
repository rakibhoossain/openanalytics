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
	log.Printf("Starting OpenAnalytics Real-Time ML Scorer (Model: %s)...", cfg.MLModelPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("ML Scorer listening to Kafka [%s] topic [%s] for live inference", cfg.KafkaBrokers, cfg.KafkaEventsTopic)
		<-ctx.Done()
	}()

	<-quit
	log.Println("Stopping ML scorer gracefully...")
	cancel()
	log.Println("ML scorer stopped")
}

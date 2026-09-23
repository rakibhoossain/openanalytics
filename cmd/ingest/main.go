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

	"openanalytics/internal/config"
	"openanalytics/pkg/httputil"
)

func main() {
	cfg := config.Load()

	r := chi.NewRouter()

	// Base middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(5 * time.Second))

	// Routes
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]string{"status": "healthy", "service": "openanalytics-ingest"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/track", func(w http.ResponseWriter, r *http.Request) {
			httputil.JSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
		})
		r.Get("/track/device-id", func(w http.ResponseWriter, r *http.Request) {
			httputil.JSON(w, http.StatusOK, map[string]string{"device_id": ""})
		})
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.IngestPort),
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("OpenAnalytics Ingestion Service starting on :%s", cfg.IngestPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Ingestion service failed to start: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down ingestion service gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Ingestion service exited cleanly")
}

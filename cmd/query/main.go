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

	"openanalytics/internal/config"
	"openanalytics/internal/query"
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

	// 3. Router setup
	r := chi.NewRouter()

	// Global Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS configuration for dashboard web applications
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"}, // CRITICAL(cors-policy): In production, restrict to merchant domains and dashboard UI origin
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Tenant-ID", "X-Shop-ID"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// 4. Register HTTP Handlers
	handler := query.NewHandler(qs)
	handler.RegisterRoutes(r)

	// Redirect root / directly to /ui/
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusFound)
	})

	// Serve Static UI Assets (resolving from current dir or openanalytics/)
	uiPath := "ui"
	if _, err := os.Stat(uiPath); os.IsNotExist(err) {
		uiPath = "openanalytics/ui"
	}
	uiDir := http.Dir(uiPath)
	r.Handle("/ui/*", http.StripPrefix("/ui", http.FileServer(uiDir)))
	r.Get("/ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusMovedPermanently)
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

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

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(15 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]string{"status": "healthy", "service": "openanalytics-query"})
	})

	r.Route("/api/v1/analytics", func(r chi.Router) {
		r.Get("/trends", func(w http.ResponseWriter, r *http.Request) {
			httputil.JSON(w, http.StatusOK, map[string]interface{}{"series": []string{}})
		})
		r.Get("/funnels", func(w http.ResponseWriter, r *http.Request) {
			httputil.JSON(w, http.StatusOK, map[string]interface{}{"steps": []string{}})
		})
		r.Get("/retention", func(w http.ResponseWriter, r *http.Request) {
			httputil.JSON(w, http.StatusOK, map[string]interface{}{"cohorts": []string{}})
		})
		r.Get("/live", func(w http.ResponseWriter, r *http.Request) {
			httputil.JSON(w, http.StatusOK, map[string]interface{}{"active_shoppers": 0})
		})
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.QueryPort),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("OpenAnalytics Query API starting on :%s", cfg.QueryPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Query service failed to start: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down query service gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Query service exited cleanly")
}

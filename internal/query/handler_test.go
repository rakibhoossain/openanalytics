package query

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHandler_Health(t *testing.T) {
	r := chi.NewRouter()
	handler := NewHandler(nil)
	handler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
}

func TestHandler_BuiltinDashboards(t *testing.T) {
	r := chi.NewRouter()
	handler := NewHandler(nil)
	handler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/api/v1/dashboards", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 for dashboards, got %d", rr.Code)
	}
}

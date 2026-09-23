package query

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHandler_Health(t *testing.T) {
	r := chi.NewRouter()
	handler := NewHandler(nil, nil)
	handler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
}

func TestHandler_MissingTenantHeaders(t *testing.T) {
	// CRITICAL(multi-tenant-auth): Ensure requests missing tenant/shop identifiers are rejected with 400.
	r := chi.NewRouter()
	handler := NewHandler(nil, nil)
	handler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/api/v1/query/trends?metric=page_views", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for missing headers, got %d", rr.Code)
	}
}

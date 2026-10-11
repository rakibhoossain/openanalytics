package query

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"openanalytics/internal/currency"
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
	req.Header.Set("X-Tenant-ID", "00000000-0000-0000-0000-000000000001")
	req.Header.Set("X-Shop-ID", "00000000-0000-0000-0000-000000000002")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 for dashboards, got %d", rr.Code)
	}
}

func TestHandler_CurrencyConversion(t *testing.T) {
	cs := currency.NewService(nil, nil)
	handler := NewHandler(nil).WithCurrency(cs)

	req := httptest.NewRequest("GET", "/test?currency=EUR", nil)
	cur := handler.extractCurrency(req)
	if cur != "EUR" {
		t.Fatalf("expected EUR, got %s", cur)
	}

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-Currency", "bdt")
	cur2 := handler.extractCurrency(req2)
	if cur2 != "BDT" {
		t.Fatalf("expected BDT, got %s", cur2)
	}

	// Test cross-currency conversion: 100 USD cents -> EUR cents
	eurCents := cs.Convert(10000, "USD", "EUR")
	if eurCents <= 0 {
		t.Fatalf("expected positive eurCents, got %d", eurCents)
	}
}

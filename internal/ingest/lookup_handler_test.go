package ingest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"openanalytics/internal/geo"
)

func TestHandleLookup_ExplicitPayload(t *testing.T) {
	geoService, _ := geo.NewService(geo.Config{DataDir: "../../data/geo"})
	handler := NewHandler(Config{
		GeoService: geoService,
		Salt:       "test_salt",
	})

	payload := map[string]string{
		"ip":         "103.205.134.10",
		"user_agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/lookup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleLookup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp LookupResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.IP != "103.205.134.10" {
		t.Errorf("expected IP 103.205.134.10, got %s", resp.IP)
	}

	if resp.Browser.Name != "Chrome" {
		t.Errorf("expected Browser Chrome, got %s", resp.Browser.Name)
	}

	if resp.OS.Name != "macOS" {
		t.Errorf("expected OS macOS, got %s", resp.OS.Name)
	}

	if resp.Device.Type != "desktop" {
		t.Errorf("expected Device Type desktop, got %s", resp.Device.Type)
	}

	if resp.Location != nil {
		if resp.Location.Country != "Bangladesh" {
			t.Errorf("expected Country Bangladesh, got %s", resp.Location.Country)
		}
	}
}

func TestHandleLookup_HeaderFallback(t *testing.T) {
	handler := NewHandler(Config{
		Salt: "test_salt",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/lookup", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "192.168.1.50")
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1")
	w := httptest.NewRecorder()

	handler.HandleLookup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp LookupResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.IP != "192.168.1.50" {
		t.Errorf("expected IP 192.168.1.50 from header, got %s", resp.IP)
	}

	if resp.Browser.Name != "Mobile Safari" && resp.Browser.Name != "Safari" {
		t.Errorf("expected Safari browser, got %s", resp.Browser.Name)
	}

	if resp.OS.Name != "iOS" {
		t.Errorf("expected OS iOS, got %s", resp.OS.Name)
	}

	if resp.Device.Type != "mobile" {
		t.Errorf("expected Device Type mobile, got %s", resp.Device.Type)
	}
}

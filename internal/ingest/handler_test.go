package ingest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"openanalytics/internal/domain"
	"openanalytics/internal/geo"
	"openanalytics/internal/kafka"
)

func TestHandleTrackValidation(t *testing.T) {
	// Initialize real Geo service with data/geo if available
	geoSvc, _ := geo.NewService(geo.Config{DataDir: "../../data/geo"})
	if geoSvc != nil {
		defer geoSvc.Close()
	}

	// Mock Kafka Producer
	producer := kafka.NewProducer(kafka.ProducerConfig{
		Brokers:        "91.99.83.171:9092",
		Topic:          "analytics.events.raw",
		BatchSize:      100,
		BatchTimeoutMs: 10,
	})
	defer producer.Close()

	handler := NewHandler(Config{
		GeoService: geoSvc,
		Producer:   producer,
		Salt:       "test_salt",
	})

	shopID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name           string
		payload        map[string]interface{}
		expectedStatus int
	}{
		{
			name: "Valid event",
			payload: map[string]interface{}{
				"shop_id": shopID.String(),
				"name":    "view_product",
				"path":    "/products/shoes-sneakers",
			},
			expectedStatus: http.StatusAccepted,
		},
		{
			name: "Missing event name",
			payload: map[string]interface{}{
				"shop_id": shopID.String(),
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Invalid shop ID",
			payload: map[string]interface{}{
				"shop_id": "not-a-valid-uuid",
				"name":    "view_product",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/track", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
			req.Header.Set("CF-Connecting-IP", "8.8.8.8")

			w := httptest.NewRecorder()
			handler.HandleTrack(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("HandleTrack() status = %d, want %d (body: %s)", w.Code, tt.expectedStatus, w.Body.String())
			}

			if tt.expectedStatus == http.StatusAccepted {
				var resp struct {
					Success bool          `json:"success"`
					Data    TrackResponse `json:"data"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if resp.Data.DeviceID == "" {
					t.Errorf("expected device_id in response")
				}
				if resp.Data.EventID == "" {
					t.Errorf("expected event_id in response")
				}
			}
		})
	}
}

func TestHandleDeviceID(t *testing.T) {
	handler := NewHandler(Config{
		Salt: "test_salt",
	})

	shopID := uuid.Must(uuid.NewV7())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/track/device-id?shop_id="+shopID.String(), nil)
	req.Header.Set("User-Agent", "TestBrowser/1.0")
	req.RemoteAddr = "192.168.1.100:54321"

	w := httptest.NewRecorder()
	handler.HandleDeviceID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandleDeviceID() status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Success bool             `json:"success"`
		Data    DeviceIDResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Data.DeviceID == "" {
		t.Errorf("expected non-empty device_id")
	}
}

// Suppress unused domain import
var _ = domain.Event{}

package meta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"openanalytics/internal/domain"
	"openanalytics/internal/ingest"
)

func TestNormalizer(t *testing.T) {
	// 1. Email Normalization
	rawEmail := " John.Doe+testing@Example.COM "
	expectedEmailHash := HashSHA256("john.doe+testing@example.com")
	if got := NormalizeEmail(rawEmail); got != expectedEmailHash {
		t.Errorf("NormalizeEmail() = %s, want %s", got, expectedEmailHash)
	}

	// 2. Phone Normalization (E.164: removes formatting and leading zeros)
	rawPhone := "+1 (555) 123-4567"
	expectedPhoneHash := HashSHA256("15551234567")
	if got := NormalizePhone(rawPhone); got != expectedPhoneHash {
		t.Errorf("NormalizePhone() = %s, want %s", got, expectedPhoneHash)
	}

	// 3. Country Normalization (2-letter lowercase)
	rawCountry := "USA"
	expectedCountryHash := HashSHA256("us")
	if got := NormalizeCountry(rawCountry); got != expectedCountryHash {
		t.Errorf("NormalizeCountry() = %s, want %s", got, expectedCountryHash)
	}

	// 4. City Normalization (trimmed lowercase alphanumeric)
	rawCity := "New York!"
	expectedCityHash := HashSHA256("newyork")
	if got := NormalizeAlpha(rawCity); got != expectedCityHash {
		t.Errorf("NormalizeAlpha() = %s, want %s", got, expectedCityHash)
	}
}

func TestMapper(t *testing.T) {
	shopID := uuid.Must(uuid.NewV7())
	eventID := "evt_order_12345"
	revCents := int64(4999) // $49.99

	items := []ingest.ECommerceItem{
		{
			ItemID:       "sku_99",
			ItemName:     "Performance Running Shoes",
			Price:        ptrFloat(49.99),
			Quantity:     ptrInt64(1),
			ItemCategory: "Footwear",
			ItemBrand:    "AcroSport",
		},
	}
	itemsJSON, _ := json.Marshal(items)

	userData := ingest.UserData{
		Email:           "shopper@domain.com",
		Phone:           "+15550001122",
		FirstName:       "Jane",
		LastName:        "Doe",
		City:            "San Francisco",
		CountryCode:     "US",
		Fbp:             "fb.1.1700000000.11112222",
		Fbc:             "fb.1.1700000000.AbCdEf123",
		ClientIPAddress: "24.120.55.8",
		ClientUserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)",
	}
	udJSON, _ := json.Marshal(userData)

	event := &domain.Event{
		ID:        uuid.Must(uuid.NewV7()),
		ShopID:    shopID,
		Name:      "purchase",
		Revenue:   &revCents,
		Currency:  "USD",
		Path:      "/checkout/thank-you",
		Origin:    "https://store.example.com",
		CreatedAt: time.Now().UTC(),
		Properties: map[string]string{
			"event_id":       eventID,
			"_user_data":     string(udJSON),
			"items":          string(itemsJSON),
			"transaction_id": "ord_12345",
		},
	}

	capiEvent, ok := MapToMetaEvent(event)
	if !ok {
		t.Fatalf("expected event to be mapped, got false")
	}

	if capiEvent.EventName != "Purchase" {
		t.Errorf("expected EventName = 'Purchase', got %q", capiEvent.EventName)
	}
	if capiEvent.EventID != eventID {
		t.Errorf("expected EventID = %q, got %q", eventID, capiEvent.EventID)
	}
	if capiEvent.EventSourceURL != "https://store.example.com/checkout/thank-you" {
		t.Errorf("expected EventSourceURL = 'https://store.example.com/checkout/thank-you', got %q", capiEvent.EventSourceURL)
	}

	// Verify User Data
	if len(capiEvent.UserData.EM) == 0 || capiEvent.UserData.EM[0] != NormalizeEmail("shopper@domain.com") {
		t.Errorf("user_data.em not correctly hashed")
	}
	if capiEvent.UserData.Fbp != "fb.1.1700000000.11112222" {
		t.Errorf("user_data.fbp mismatch, got %q", capiEvent.UserData.Fbp)
	}
	if capiEvent.UserData.Fbc != "fb.1.1700000000.AbCdEf123" {
		t.Errorf("user_data.fbc mismatch, got %q", capiEvent.UserData.Fbc)
	}
	if capiEvent.UserData.ClientIPAddress != "24.120.55.8" {
		t.Errorf("client_ip_address mismatch, got %q", capiEvent.UserData.ClientIPAddress)
	}

	// Verify Custom Data
	if capiEvent.CustomData == nil {
		t.Fatalf("expected custom_data to be populated")
	}
	if capiEvent.CustomData.Value != 49.99 {
		t.Errorf("expected custom_data.value = 49.99, got %f", capiEvent.CustomData.Value)
	}
	if capiEvent.CustomData.Currency != "USD" {
		t.Errorf("expected custom_data.currency = 'USD', got %s", capiEvent.CustomData.Currency)
	}
	if capiEvent.CustomData.OrderID != "ord_12345" {
		t.Errorf("expected custom_data.order_id = 'ord_12345', got %s", capiEvent.CustomData.OrderID)
	}
	if len(capiEvent.CustomData.Contents) != 1 || capiEvent.CustomData.Contents[0].ID != "sku_99" {
		t.Errorf("expected 1 content item with id 'sku_99', got %+v", capiEvent.CustomData.Contents)
	}
}

func TestClientWithMockServer(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test_access_token_123" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "Bad Content-Type", http.StatusBadRequest)
			return
		}

		var payload CAPIPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}

		if len(payload.Data) != 1 || payload.Data[0].EventName != "Purchase" {
			http.Error(w, "Unexpected event data", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CAPIResponse{
			EventsReceived: 1,
			FBTraceID:      "mock_trace_xyz",
		})
	}))
	defer mockServer.Close()

	client := NewClient(mockServer.URL)
	events := []CAPIEvent{
		{
			EventName:    "Purchase",
			EventTime:    time.Now().Unix(),
			ActionSource: "website",
			UserData: CAPIUserData{
				EM: []string{NormalizeEmail("test@example.com")},
			},
		},
	}

	resp, err := client.SendEvents(context.Background(), "pixel_98765", "test_access_token_123", "TEST1234", events)
	if err != nil {
		t.Fatalf("client.SendEvents() error = %v", err)
	}

	if resp.EventsReceived != 1 {
		t.Errorf("expected events_received = 1, got %d", resp.EventsReceived)
	}
	if resp.FBTraceID != "mock_trace_xyz" {
		t.Errorf("expected fbtrace_id = 'mock_trace_xyz', got %q", resp.FBTraceID)
	}
}

func ptrFloat(f float64) *float64 { return &f }
func ptrInt64(i int64) *int64     { return &i }

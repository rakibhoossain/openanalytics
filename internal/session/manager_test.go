package session

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"openanalytics/internal/domain"
	"openanalytics/pkg/uuidv7"
)

func TestSessionManager_NilRedisFallback(t *testing.T) {
	// WEAK_POINT(fallback-on-nil-redis): Verify fail-open fallback behavior when Redis is unavailable.
	mgr := NewManager(nil, 30*time.Minute)

	shopID := uuidv7.MustNew()
	event := &domain.Event{
		ID:        uuidv7.MustNew(),
		TenantID:  uuidv7.MustNew(),
		ShopID:    shopID,
		DeviceID:  "device-test-123",
		Name:      "page_view",
		Path:      "/products/item-1",
		Referrer:  "https://google.com",
		CreatedAt: time.Now().UTC(),
	}

	res, err := mgr.Ingest(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error on nil redis: %v", err)
	}

	if res.Kind != "new" {
		t.Errorf("expected Kind to be 'new', got %s", res.Kind)
	}
	if event.SessionID == uuid.Nil {
		t.Errorf("expected non-nil SessionID on event")
	}
	if res.SessionID != event.SessionID {
		t.Errorf("expected res.SessionID == event.SessionID")
	}
}

package query

import (
	"context"
	"testing"
	"time"

	"openanalytics/internal/domain"
	"openanalytics/pkg/uuidv7"
)

func TestService_GetLiveVisitors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	svc, err := NewService(ctx, Config{
		Addr:     "127.0.0.1:9000",
		Database: "openpanel",
		Username: "openpanel",
		Password: "openpanel",
	})
	if err != nil {
		t.Skipf("Skipping ClickHouse test: %v", err)
		return
	}
	defer svc.Close()

	tenantID := uuidv7.MustNew()
	shopID := uuidv7.MustNew()

	live, err := svc.GetLiveVisitors(ctx, tenantID, shopID, 5)
	if err != nil {
		t.Fatalf("unexpected error in GetLiveVisitors: %v", err)
	}

	if live == nil {
		t.Fatalf("expected non-nil LiveVisitorsResult")
	}
	if live.WindowMinutes != 5 {
		t.Errorf("expected WindowMinutes=5, got %d", live.WindowMinutes)
	}
}

func TestService_GetTrends(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	svc, err := NewService(ctx, Config{
		Addr:     "127.0.0.1:9000",
		Database: "openpanel",
		Username: "openpanel",
		Password: "openpanel",
	})
	if err != nil {
		t.Skipf("Skipping ClickHouse test: %v", err)
		return
	}
	defer svc.Close()

	tenantID := uuidv7.MustNew()
	shopID := uuidv7.MustNew()

	pts, err := svc.GetTrends(ctx, tenantID, shopID, "page_views", "today", "hour")
	if err != nil {
		t.Fatalf("unexpected error in GetTrends: %v", err)
	}
	if pts == nil {
		pts = []domain.TrendDataPoint{}
	}
}

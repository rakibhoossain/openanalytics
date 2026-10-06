package query

import (
	"context"
	"testing"
	"time"

	"openanalytics/internal/domain"
	"openanalytics/pkg/uuidv7"

	"github.com/google/uuid"
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

func TestService_GetUserJourney(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

	tenantID := uuid.MustParse("019f5bfa-f6e4-76c0-9929-ed0daba7b14b")
	shopID := uuid.MustParse("019fc2f1-6be1-7a2d-9ebf-9f7dced8ccc1")

	journey, err := svc.GetUserJourney(ctx, tenantID, shopID, "all", "", "", 5)
	if err != nil {
		t.Fatalf("unexpected error in GetUserJourney: %v", err)
	}

	if journey == nil {
		t.Fatalf("expected non-nil UserJourneyResult")
	}

	t.Logf("UserJourney result: %d nodes, %d links", len(journey.Nodes), len(journey.Links))
	for _, n := range journey.Nodes {
		t.Logf("Node: ID=%s, Label=%s, Step=%d, Value=%d, Percentage=%.1f%%", n.ID, n.Label, n.Step, n.Value, n.Percentage)
	}
	for _, l := range journey.Links {
		t.Logf("Link: %s -> %s (value=%d)", l.Source, l.Target, l.Value)
	}
}

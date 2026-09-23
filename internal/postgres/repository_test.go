package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"openanalytics/internal/domain"
	"openanalytics/pkg/uuidv7"
)

func TestPostgresRepository_CRUD(t *testing.T) {
	dsn := "postgresql://openpanel:openpanel@localhost:5435/openpanel?sslmode=disable"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo, err := NewRepository(ctx, dsn)
	if err != nil {
		t.Skipf("Skipping postgres test: postgres not reachable on port 5435: %v", err)
		return
	}
	defer repo.Close()

	tenantID := uuidv7.MustNew()
	shopID := uuidv7.MustNew()

	// 1. Create Dashboard
	dash := &domain.Dashboard{
		ID:          uuidv7.MustNew(),
		TenantID:    tenantID,
		ShopID:      shopID,
		Name:        "Executive Overview",
		Description: "Primary revenue and funnel tracking",
		IsDefault:   true,
		LayoutGrid:  json.RawMessage(`[{"w":6,"h":4,"x":0,"y":0}]`),
	}

	if err := repo.CreateDashboard(ctx, dash); err != nil {
		t.Fatalf("failed to create dashboard: %v", err)
	}

	// 2. Fetch Dashboard
	fetched, err := repo.GetDashboard(ctx, tenantID, shopID, dash.ID)
	if err != nil {
		t.Fatalf("failed to get dashboard: %v", err)
	}
	if fetched == nil || fetched.Name != dash.Name {
		t.Fatalf("expected dashboard name %s, got %v", dash.Name, fetched)
	}

	// 3. Create Widget
	widget := &domain.ChartWidget{
		ID:          uuidv7.MustNew(),
		DashboardID: dash.ID,
		TenantID:    tenantID,
		ShopID:      shopID,
		Title:       "Conversion Funnel",
		ChartType:   "funnel",
		MetricType:  "conversion_rate",
		TimeRange:   "7d",
		FilterRules: json.RawMessage(`[]`),
		PositionX:   0,
		PositionY:   0,
		Width:       6,
		Height:      4,
	}

	if err := repo.CreateWidget(ctx, widget); err != nil {
		t.Fatalf("failed to create widget: %v", err)
	}

	// 4. List Widgets
	widgets, err := repo.ListWidgets(ctx, tenantID, shopID, dash.ID)
	if err != nil {
		t.Fatalf("failed to list widgets: %v", err)
	}
	if len(widgets) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(widgets))
	}

	// 5. Delete Dashboard (cascades)
	if err := repo.DeleteDashboard(ctx, tenantID, shopID, dash.ID); err != nil {
		t.Fatalf("failed to delete dashboard: %v", err)
	}
}

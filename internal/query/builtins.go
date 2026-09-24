package query

import (
	"time"

	"github.com/google/uuid"
)

// BuiltinReport defines a built-in analytical chart or report.
type BuiltinReport struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	ChartType  string   `json:"chartType"`
	Metric     string   `json:"metric,omitempty"`
	Range      string   `json:"range,omitempty"`
	Interval   string   `json:"interval,omitempty"`
	Series     []any    `json:"series"`
	Breakdowns []string `json:"breakdowns,omitempty"`
}

// BuiltinDashboard defines a curated out-of-the-box e-commerce dashboard.
type BuiltinDashboard struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	IsDefault   bool            `json:"isDefault"`
	Reports     []BuiltinReport `json:"reports"`
}

const (
	DashboardOverviewID    = "018e69d0-0000-7000-8000-000000000001"
	DashboardFunnelID      = "018e69d0-0000-7000-8000-000000000002"
	DashboardMLIntentID    = "018e69d0-0000-7000-8000-000000000003"
	DashboardProductsID    = "018e69d0-0000-7000-8000-000000000004"
	DashboardAcquisitionID = "018e69d0-0000-7000-8000-000000000005"
)

var defaultDashboards = []BuiltinDashboard{
	{
		ID:          DashboardOverviewID,
		Name:        "Executive Overview",
		Description: "Core e-commerce metrics: revenue, conversion rate, orders, and visitor trends",
		IsDefault:   true,
		Reports: []BuiltinReport{
			{
				ID:        "rep-overview-revenue",
				Name:      "Gross Revenue (GMV)",
				ChartType: "linear",
				Metric:    "sum",
				Range:     "30d",
				Interval:  "day",
				Series: []any{
					map[string]any{"type": "event", "id": "A", "name": "purchase", "segment": "revenue"},
				},
			},
			{
				ID:        "rep-overview-conversion",
				Name:      "Conversion Rate Trend",
				ChartType: "linear",
				Range:     "30d",
				Interval:  "day",
				Series: []any{
					map[string]any{"type": "event", "id": "A", "name": "purchase", "segment": "event"},
				},
			},
			{
				ID:        "rep-overview-sessions",
				Name:      "Total Sessions & Visitors",
				ChartType: "bar",
				Range:     "30d",
				Interval:  "day",
				Series: []any{
					map[string]any{"type": "event", "id": "A", "name": "session_start", "segment": "event"},
				},
			},
			{
				ID:        "rep-overview-aov",
				Name:      "Average Order Value (AOV)",
				ChartType: "metric",
				Metric:    "average",
				Range:     "30d",
				Series: []any{
					map[string]any{"type": "event", "id": "A", "name": "purchase", "segment": "revenue"},
				},
			},
		},
	},
	{
		ID:          DashboardFunnelID,
		Name:        "Purchase & Conversion Funnel",
		Description: "Step-by-step conversion analysis from product discovery to purchase completion",
		IsDefault:   false,
		Reports: []BuiltinReport{
			{
				ID:        "rep-funnel-main",
				Name:      "Full Checkout Funnel",
				ChartType: "funnel",
				Range:     "30d",
				Series: []any{
					map[string]any{"type": "event", "id": "A", "name": "page_view"},
					map[string]any{"type": "event", "id": "B", "name": "view_item"},
					map[string]any{"type": "event", "id": "C", "name": "add_to_cart"},
					map[string]any{"type": "event", "id": "D", "name": "purchase"},
				},
			},
			{
				ID:        "rep-funnel-cart-trend",
				Name:      "Cart Additions vs Purchases",
				ChartType: "linear",
				Range:     "30d",
				Interval:  "day",
				Series: []any{
					map[string]any{"type": "event", "id": "A", "name": "add_to_cart", "segment": "event"},
					map[string]any{"type": "event", "id": "B", "name": "purchase", "segment": "event"},
				},
			},
		},
	},
	{
		ID:          DashboardMLIntentID,
		Name:        "Behavioral ML & Purchase Intent",
		Description: "Real-time AI purchase propensity, cart abandonment risk, and intent scoring",
		IsDefault:   false,
		Reports: []BuiltinReport{
			{
				ID:        "rep-ml-cart-adds",
				Name:      "High-Propensity Cart Intent Events",
				ChartType: "linear",
				Range:     "30d",
				Interval:  "day",
				Series: []any{
					map[string]any{"type": "event", "id": "A", "name": "add_to_cart", "segment": "event"},
				},
			},
			{
				ID:        "rep-ml-active-intent",
				Name:      "Active Intent Signals",
				ChartType: "metric",
				Range:     "30d",
				Series: []any{
					map[string]any{"type": "event", "id": "A", "name": "add_to_cart", "segment": "event"},
				},
			},
		},
	},
	{
		ID:          DashboardProductsID,
		Name:        "Merchandise & Products",
		Description: "Top revenue products, most viewed items, and cart abandonment SKUs",
		IsDefault:   false,
		Reports: []BuiltinReport{
			{
				ID:         "rep-products-top-revenue",
				Name:       "Top Products by Revenue",
				ChartType:  "bar",
				Range:      "30d",
				Breakdowns: []string{"properties.product_id"},
				Series: []any{
					map[string]any{"type": "event", "id": "A", "name": "purchase", "segment": "revenue"},
				},
			},
			{
				ID:         "rep-products-most-viewed",
				Name:       "Most Viewed Products",
				ChartType:  "bar",
				Range:      "30d",
				Breakdowns: []string{"properties.product_id"},
				Series: []any{
					map[string]any{"type": "event", "id": "A", "name": "view_item", "segment": "event"},
				},
			},
		},
	},
	{
		ID:          DashboardAcquisitionID,
		Name:        "Acquisition & Marketing Channels",
		Description: "Traffic sources, UTM campaigns, device breakdown, and channel conversion",
		IsDefault:   false,
		Reports: []BuiltinReport{
			{
				ID:         "rep-acq-sources",
				Name:       "Sessions by Traffic Source",
				ChartType:  "bar",
				Range:      "30d",
				Breakdowns: []string{"referrer_name"},
				Series: []any{
					map[string]any{"type": "event", "id": "A", "name": "session_start", "segment": "event"},
				},
			},
			{
				ID:         "rep-acq-campaigns",
				Name:       "Campaign Revenue Attribution",
				ChartType:  "bar",
				Range:      "30d",
				Breakdowns: []string{"properties.utm_campaign"},
				Series: []any{
					map[string]any{"type": "event", "id": "A", "name": "purchase", "segment": "revenue"},
				},
			},
		},
	},
}

// GetBuiltinDashboards returns the curated dashboard list formatted for a tenant/shop.
func GetBuiltinDashboards(tenantID, shopID uuid.UUID) []map[string]any {
	now := time.Now().UTC().Format(time.RFC3339)
	res := make([]map[string]any, 0, len(defaultDashboards))
	for _, d := range defaultDashboards {
		reps := make([]map[string]any, 0, len(d.Reports))
		for _, r := range d.Reports {
			reps = append(reps, map[string]any{
				"id":        r.ID,
				"name":      r.Name,
				"chartType": r.ChartType,
			})
		}
		res = append(res, map[string]any{
			"id":             d.ID,
			"name":           d.Name,
			"description":    d.Description,
			"projectId":      shopID.String(),
			"organizationId": tenantID.String(),
			"isDefault":      d.IsDefault,
			"createdAt":      now,
			"updatedAt":      now,
			"reports":        reps,
		})
	}
	return res
}

// GetBuiltinDashboardByID finds a built-in dashboard by ID or name alias.
func GetBuiltinDashboardByID(tenantID, shopID uuid.UUID, id string) map[string]any {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, d := range defaultDashboards {
		if d.ID == id || id == "overview" && d.ID == DashboardOverviewID || id == "funnel" && d.ID == DashboardFunnelID || id == "ml_intent" && d.ID == DashboardMLIntentID || id == "products" && d.ID == DashboardProductsID || id == "acquisition" && d.ID == DashboardAcquisitionID {
			return map[string]any{
				"id":             d.ID,
				"name":           d.Name,
				"description":    d.Description,
				"projectId":      shopID.String(),
				"organizationId": tenantID.String(),
				"isDefault":      d.IsDefault,
				"createdAt":      now,
				"updatedAt":      now,
			}
		}
	}
	// Fallback to first default dashboard
	d := defaultDashboards[0]
	return map[string]any{
		"id":             d.ID,
		"name":           d.Name,
		"description":    d.Description,
		"projectId":      shopID.String(),
		"organizationId": tenantID.String(),
		"isDefault":      true,
		"createdAt":      now,
		"updatedAt":      now,
	}
}

// GetBuiltinReportsByDashboardID returns report definitions for a dashboard.
func GetBuiltinReportsByDashboardID(tenantID, shopID uuid.UUID, dashboardID string) []map[string]any {
	var targetReports []BuiltinReport
	for _, d := range defaultDashboards {
		if d.ID == dashboardID || dashboardID == "overview" && d.ID == DashboardOverviewID || dashboardID == "funnel" && d.ID == DashboardFunnelID || dashboardID == "ml_intent" && d.ID == DashboardMLIntentID || dashboardID == "products" && d.ID == DashboardProductsID || dashboardID == "acquisition" && d.ID == DashboardAcquisitionID {
			targetReports = d.Reports
			break
		}
	}
	if len(targetReports) == 0 {
		targetReports = defaultDashboards[0].Reports
	}

	res := make([]map[string]any, 0, len(targetReports))
	for _, r := range targetReports {
		chartType := r.ChartType
		if chartType == "line" {
			chartType = "linear"
		}
		item := map[string]any{
			"id":          r.ID,
			"name":        r.Name,
			"chartType":   chartType,
			"dashboardId": dashboardID,
			"projectId":   shopID.String(),
			"range":       r.Range,
			"interval":    r.Interval,
			"series":      r.Series,
			"events":      r.Series,
		}
		if r.Metric != "" {
			item["metric"] = r.Metric
		}
		if len(r.Breakdowns) > 0 {
			item["breakdowns"] = r.Breakdowns
		}
		res = append(res, item)
	}
	return res
}

// GetBuiltinReportByID finds a built-in report by its ID across all default dashboards.
func GetBuiltinReportByID(tenantID, shopID uuid.UUID, id string) map[string]any {
	for _, d := range defaultDashboards {
		for _, r := range d.Reports {
			if r.ID == id {
				chartType := r.ChartType
				if chartType == "line" {
					chartType = "linear"
				}
				item := map[string]any{
					"id":          r.ID,
					"name":        r.Name,
					"chartType":   chartType,
					"dashboardId": d.ID,
					"projectId":   shopID.String(),
					"range":       r.Range,
					"interval":    r.Interval,
					"series":      r.Series,
					"events":      r.Series,
				}
				if r.Metric != "" {
					item["metric"] = r.Metric
				}
				if len(r.Breakdowns) > 0 {
					item["breakdowns"] = r.Breakdowns
				}
				return item
			}
		}
	}
	return nil
}

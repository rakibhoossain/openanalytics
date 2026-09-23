package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Dashboard represents a merchant's custom visualization dashboard.
type Dashboard struct {
	ID          uuid.UUID       `json:"id"`
	TenantID    uuid.UUID       `json:"tenant_id"`
	ShopID      uuid.UUID       `json:"shop_id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	IsDefault   bool            `json:"is_default"`
	LayoutGrid  json.RawMessage `json:"layout_grid"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ChartWidget represents an individual visualization widget placed on a dashboard.
type ChartWidget struct {
	ID          uuid.UUID       `json:"id"`
	DashboardID uuid.UUID       `json:"dashboard_id"`
	TenantID    uuid.UUID       `json:"tenant_id"`
	ShopID      uuid.UUID       `json:"shop_id"`
	Title       string          `json:"title"`
	ChartType   string          `json:"chart_type"`  // 'timeseries', 'funnel', 'kpi', 'bar', 'pie'
	MetricType  string          `json:"metric_type"` // 'page_views', 'cart_adds', 'conversion_rate', 'revenue'
	TimeRange   string          `json:"time_range"`  // 'today', '24h', '7d', '30d', 'custom'
	GroupBy     string          `json:"group_by,omitempty"`
	FilterRules json.RawMessage `json:"filter_rules"`
	CustomSQL   string          `json:"custom_sql,omitempty"`
	PositionX   int             `json:"position_x"`
	PositionY   int             `json:"position_y"`
	Width       int             `json:"width"`
	Height      int             `json:"height"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// SavedReport represents an automated scheduled report.
type SavedReport struct {
	ID           uuid.UUID       `json:"id"`
	TenantID     uuid.UUID       `json:"tenant_id"`
	ShopID       uuid.UUID       `json:"shop_id"`
	Name         string          `json:"name"`
	ReportType   string          `json:"report_type"` // 'weekly_executive', 'abandoned_cart', 'traffic_source'
	ScheduleCron string          `json:"schedule_cron,omitempty"`
	Recipients   []string        `json:"recipients"`
	Filters      json.RawMessage `json:"filters"`
	IsActive     bool            `json:"is_active"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// AlertRule represents real-time anomaly detection rules on commerce metrics.
type AlertRule struct {
	ID                  uuid.UUID  `json:"id"`
	TenantID            uuid.UUID  `json:"tenant_id"`
	ShopID              uuid.UUID  `json:"shop_id"`
	Name                string     `json:"name"`
	Metric              string     `json:"metric"` // 'error_rate', 'cart_abandonment_spike'
	ConditionOperator   string     `json:"condition_operator"` // '>', '<', '>='
	ThresholdValue      float64    `json:"threshold_value"`
	WindowMinutes       int        `json:"window_minutes"`
	NotificationChannel string     `json:"notification_channel"` // 'webhook', 'email', 'slack'
	ChannelTarget       string     `json:"channel_target"`
	IsEnabled           bool       `json:"is_enabled"`
	LastTriggeredAt     *time.Time `json:"last_triggered_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

// --- Analytical Query Contract Types ---

// TrendDataPoint represents a single aggregated bucket in a time series.
type TrendDataPoint struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
	Label     string  `json:"label,omitempty"`
}

// FunnelStepResult represents conversion statistics for a single step in a multi-stage funnel.
type FunnelStepResult struct {
	StepIndex      int     `json:"step_index"`
	Name           string  `json:"name"`
	Count          int64   `json:"count"`
	DropoffCount   int64   `json:"dropoff_count"`
	ConversionRate float64 `json:"conversion_rate_percent"` // Conversion from previous step
}

// FunnelResult represents the complete funnel analysis response.
type FunnelResult struct {
	Steps                 []FunnelStepResult `json:"steps"`
	TotalVisitors         int64              `json:"total_visitors"`
	OverallConversionRate float64            `json:"overall_conversion_rate_percent"`
}

// RetentionCohort represents return rates of users across cohorts.
type RetentionCohort struct {
	CohortDate string    `json:"cohort_date"`
	TotalUsers int64     `json:"total_users"`
	DayOffsets []float64 `json:"day_offsets"` // e.g. Day 0 (100%), Day 1, Day 7, Day 14, Day 30
}

// PathCount captures high-traffic URLs.
type PathCount struct {
	Path  string `json:"path"`
	Count int64  `json:"count"`
}

// LiveVisitorsResult represents real-time activity metrics.
type LiveVisitorsResult struct {
	ActiveShoppers int64            `json:"active_shoppers"`
	WindowMinutes  int              `json:"window_minutes"`
	Devices        map[string]int64 `json:"devices"`
	Countries      map[string]int64 `json:"countries"`
	TopPaths       []PathCount      `json:"top_paths"`
}

// ShopperJourneyResult contains the full chronological history of a customer/device.
type ShopperJourneyResult struct {
	DeviceID   string     `json:"device_id"`
	CustomerID *uuid.UUID `json:"customer_id,omitempty"`
	Events     []Event    `json:"events"`
	Sessions   []Session  `json:"sessions"`
}

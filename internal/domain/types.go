package domain

import (
	"time"

	"github.com/google/uuid"
)

// Event represents a single analytical/telemetry event in the platform.
type Event struct {
	ID         uuid.UUID `json:"id"`                    // UUIDv7
	TenantID   uuid.UUID `json:"tenant_id"`             // UUIDv7
	ShopID     uuid.UUID `json:"shop_id"`               // UUIDv7
	Name       string    `json:"name"`                  // e.g. "view_product", "add_to_cart", "checkout_step"
	DeviceID   string    `json:"device_id"`             // SHA-256 fingerprint or UUIDv7
	CustomerID *uuid.UUID `json:"customer_id,omitempty"` // UUIDv7 (nil for guest shoppers)
	SessionID  uuid.UUID `json:"session_id"`            // UUIDv7

	// E-Commerce Attributes (Revenue stored in exact integer cents, e.g. $1.78 = 178 cents)
	Revenue   *int64     `json:"revenue,omitempty"`
	Currency  string     `json:"currency,omitempty"`
	ProductID *uuid.UUID `json:"product_id,omitempty"`
	CartID    *uuid.UUID `json:"cart_id,omitempty"`
	OrderID   *uuid.UUID `json:"order_id,omitempty"`

	// Web & Device Context
	Path         string `json:"path,omitempty"`
	Origin       string `json:"origin,omitempty"`
	Referrer     string `json:"referrer,omitempty"`
	ReferrerName string `json:"referrer_name,omitempty"`
	ReferrerType string `json:"referrer_type,omitempty"`

	OS        string    `json:"os,omitempty"`
	Browser   string    `json:"browser,omitempty"`
	Device    string    `json:"device,omitempty"`
	Country   string    `json:"country,omitempty"`
	City      string    `json:"city,omitempty"`
	Latitude  *float32  `json:"latitude,omitempty"`
	Longitude *float32  `json:"longitude,omitempty"`

	Properties map[string]string `json:"properties,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

// Session represents an aggregated visitor session window.
type Session struct {
	ID         uuid.UUID  `json:"id"`                    // UUIDv7
	TenantID   uuid.UUID  `json:"tenant_id"`             // UUIDv7
	ShopID     uuid.UUID  `json:"shop_id"`               // UUIDv7
	DeviceID   string     `json:"device_id"`
	CustomerID *uuid.UUID `json:"customer_id,omitempty"` // UUIDv7

	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Duration  uint32    `json:"duration_seconds"`

	EntryPath    string `json:"entry_path"`
	ExitPath     string `json:"exit_path"`
	Referrer     string `json:"referrer,omitempty"`
	ReferrerName string `json:"referrer_name,omitempty"`
	ReferrerType string `json:"referrer_type,omitempty"`

	EventsCount  uint32 `json:"events_count"`
	HasCartAdd   bool   `json:"has_cart_add"`
	HasPurchase  bool   `json:"has_purchase"`
	TotalRevenue int64  `json:"total_revenue"` // Stored in integer cents
}

// OverviewMetrics holds current and previous period comparison metric values.
type OverviewMetrics struct {
	BounceRate             float64  `json:"bounce_rate"`
	UniqueVisitors         int64    `json:"unique_visitors"`
	TotalSessions          int64    `json:"total_sessions"`
	AvgSessionDuration     float64  `json:"avg_session_duration"`
	TotalScreenViews       int64    `json:"total_screen_views"`
	ViewsPerSession        float64  `json:"views_per_session"`
	TotalRevenue           int64    `json:"total_revenue"` // In cents
	PrevBounceRate         *float64 `json:"prev_bounce_rate,omitempty"`
	PrevUniqueVisitors     *int64   `json:"prev_unique_visitors,omitempty"`
	PrevTotalSessions      *int64   `json:"prev_total_sessions,omitempty"`
	PrevAvgSessionDuration *float64 `json:"prev_avg_session_duration,omitempty"`
	PrevTotalScreenViews   *int64   `json:"prev_total_screen_views,omitempty"`
	PrevViewsPerSession    *float64 `json:"prev_views_per_session,omitempty"`
	PrevTotalRevenue       *int64   `json:"prev_total_revenue,omitempty"`
}

// OverviewSeriesPoint represents a single interval bucket in an overview timeline.
type OverviewSeriesPoint struct {
	Date                   string   `json:"date"`
	BounceRate             float64  `json:"bounce_rate"`
	UniqueVisitors         int64    `json:"unique_visitors"`
	TotalSessions          int64    `json:"total_sessions"`
	AvgSessionDuration     float64  `json:"avg_session_duration"`
	TotalScreenViews       int64    `json:"total_screen_views"`
	ViewsPerSession        float64  `json:"views_per_session"`
	TotalRevenue           int64    `json:"total_revenue"` // In cents
	PrevBounceRate         *float64 `json:"prev_bounce_rate,omitempty"`
	PrevUniqueVisitors     *int64   `json:"prev_unique_visitors,omitempty"`
	PrevTotalSessions      *int64   `json:"prev_total_sessions,omitempty"`
	PrevAvgSessionDuration *float64 `json:"prev_avg_session_duration,omitempty"`
	PrevTotalScreenViews   *int64   `json:"prev_total_screen_views,omitempty"`
	PrevViewsPerSession    *float64 `json:"prev_views_per_session,omitempty"`
	PrevTotalRevenue       *int64   `json:"prev_total_revenue,omitempty"`
}

// OverviewStatsResult packages both current/previous totals and time series.
type OverviewStatsResult struct {
	Metrics OverviewMetrics       `json:"metrics"`
	Series  []OverviewSeriesPoint `json:"series"`
}

// TopItem represents an aggregated dimension item (e.g. source, page, device).
type TopItem struct {
	Name      string `json:"name"`
	Sessions  int64  `json:"sessions"`
	Pageviews int64  `json:"pageviews"`
	Revenue   int64  `json:"revenue"` // In cents
}

// TopGenericSeriesResult represents top items with their breakdown series.
type TopGenericSeriesResult struct {
	Items  []TopItem                `json:"items"`
	Series []map[string]interface{} `json:"series"`
}

// TopEventItem represents an aggregated event count and unique user reach.
type TopEventItem struct {
	Name           string `json:"name"`
	Count          int64  `json:"count"`
	UniqueVisitors int64  `json:"unique_visitors"`
}

// MinuteCount represents visitor and session activity in a single minute.
type MinuteCount struct {
	Minute       string               `json:"minute"`
	SessionCount int64                `json:"sessionCount"`
	VisitorCount int64                `json:"visitorCount"`
	Timestamp    int64                `json:"timestamp"`
	Time         string               `json:"time"`
	Referrers    []ReferrerMinuteItem `json:"referrers"`
}

// ReferrerMinuteItem tracks referrer activity within a minute bucket.
type ReferrerMinuteItem struct {
	Referrer string `json:"referrer"`
	Count    int64  `json:"count"`
}

// OverviewLiveDataResult encapsulates 30-minute real-time activity for the overview histogram.
type OverviewLiveDataResult struct {
	TotalSessions int64                `json:"totalSessions"`
	MinuteCounts  []MinuteCount        `json:"minuteCounts"`
	Referrers     []ReferrerMinuteItem `json:"referrers"`
}

// ShopperFeature represents rolling behavioral features and ML intent score.
type ShopperFeature struct {
	TenantID          uuid.UUID `json:"tenant_id"`
	ShopID            uuid.UUID `json:"shop_id"`
	DeviceID          string    `json:"device_id"`
	SessionID         uuid.UUID `json:"session_id"`
	ViewsCount        uint32    `json:"views_count"`
	CartAddsCount     uint32    `json:"cart_adds_count"`
	DistinctProducts  uint32    `json:"distinct_products"`
	TotalDwellSeconds uint32    `json:"total_dwell_seconds"`
	HasPurchase       uint8     `json:"has_purchase"`
	CartIntentScore   float32   `json:"cart_intent_score"`
	LastEventAt       time.Time `json:"last_event_at"`
}

// SankeyNode represents a stage in the User Journey Sankey diagram.
type SankeyNode struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	NodeColor  string  `json:"nodeColor"`
	Percentage float64 `json:"percentage"`
	Value      int64   `json:"value"`
	Step       int     `json:"step"`
}

// SankeyLink represents a directed transition between two journey steps.
type SankeyLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Value  int64  `json:"value"`
}

// UserJourneyResult encapsulates the full Sankey graph payload.
type UserJourneyResult struct {
	Nodes []SankeyNode `json:"nodes"`
	Links []SankeyLink `json:"links"`
}



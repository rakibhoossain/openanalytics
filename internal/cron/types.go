package cron

import (
	"time"

	"github.com/google/uuid"
)

// InsightCard represents a computed anomaly or trend insight for UI display.
// Matches OpenPanel's project_insights table and UI RenderedCard format.
type InsightCard struct {
	ID           uuid.UUID `json:"id"`
	ShopID       uuid.UUID `json:"shop_id"`
	TenantID     uuid.UUID `json:"tenant_id"`
	ModuleKey    string    `json:"module_key"`    // "referrers", "entry_pages", "devices", "cart_trends"
	DimensionKey string    `json:"dimension_key"` // e.g. "referrer:Google", "device:mobile"
	WindowKind   string    `json:"window_kind"`   // "rolling_24h", "rolling_7d"
	Title        string    `json:"title"`
	Summary      string    `json:"summary"`
	Direction    string    `json:"direction"` // "up", "down", "flat"
	ChangePct    float64   `json:"change_pct"`
	CurrentVal   float64   `json:"current_val"`
	CompareVal   float64   `json:"compare_val"`
	ImpactScore  float64   `json:"impact_score"`
	CreatedAt    time.Time `json:"created_at"`
}

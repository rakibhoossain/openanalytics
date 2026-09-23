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

	// E-Commerce Attributes
	Revenue   *float64 `json:"revenue,omitempty"`
	Currency  string   `json:"currency,omitempty"`
	ProductID string   `json:"product_id,omitempty"`
	CartID    string   `json:"cart_id,omitempty"`
	OrderID   string   `json:"order_id,omitempty"`

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

	EventsCount  uint32  `json:"events_count"`
	HasCartAdd   bool    `json:"has_cart_add"`
	HasPurchase  bool    `json:"has_purchase"`
	TotalRevenue float64 `json:"total_revenue"`
}

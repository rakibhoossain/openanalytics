package ingest

// TrackRequest represents an incoming telemetry tracking payload.
type TrackRequest struct {
	TenantID   string            `json:"tenant_id,omitempty"`
	ShopID     string            `json:"shop_id,omitempty"`
	Name       string            `json:"name"`
	DeviceID   string            `json:"device_id,omitempty"`
	SessionID  string            `json:"session_id,omitempty"`
	CustomerID string            `json:"customer_id,omitempty"`

	// E-Commerce telemetry
	Revenue   *float64 `json:"revenue,omitempty"`
	Currency  string   `json:"currency,omitempty"`
	ProductID string   `json:"product_id,omitempty"`
	CartID    string   `json:"cart_id,omitempty"`
	OrderID   string   `json:"order_id,omitempty"`

	// Context
	Path       string            `json:"path,omitempty"`
	Referrer   string            `json:"referrer,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
	Timestamp  *int64            `json:"timestamp,omitempty"` // Epoch milliseconds
}

// BatchTrackRequest represents an array of events sent in a single HTTP request.
type BatchTrackRequest struct {
	Events []TrackRequest `json:"events"`
}

// TrackResponse is returned on successful event ingestion.
type TrackResponse struct {
	EventID   string `json:"event_id"`
	DeviceID  string `json:"device_id"`
	SessionID string `json:"session_id,omitempty"`
	Status    string `json:"status"`
}

// DeviceIDResponse is returned by GET /track/device-id.
type DeviceIDResponse struct {
	DeviceID  string `json:"device_id"`
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message,omitempty"`
}

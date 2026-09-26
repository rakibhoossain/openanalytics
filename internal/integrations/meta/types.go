package meta

import (
	"time"

	"github.com/google/uuid"
)

// Provider name identifier.
const ProviderMetaCAPI = "meta_capi"

// MetaCredentials stores the merchant's Pixel and API tokens.
type MetaCredentials struct {
	PixelID       string `json:"pixel_id"`
	AccessToken   string `json:"access_token"`
	TestEventCode string `json:"test_event_code,omitempty"`
}

// ShopIntegration holds the configuration for a shop's 3rd-party integration.
type ShopIntegration struct {
	ShopID          uuid.UUID       `json:"shop_id"`
	TenantID        uuid.UUID       `json:"tenant_id"`
	Provider        string          `json:"provider"`
	Enabled         bool            `json:"enabled"`
	Credentials     MetaCredentials `json:"credentials"`
	EventsWhitelist []string        `json:"events_whitelist"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// CAPIUserData represents Meta Conversions API customer information parameters.
// PII fields (EM, PH, FN, LN, CT, ST, ZP, Country, ExternalID) must be SHA-256 hashed.
type CAPIUserData struct {
	EM              []string `json:"em,omitempty"`
	PH              []string `json:"ph,omitempty"`
	FN              []string `json:"fn,omitempty"`
	LN              []string `json:"ln,omitempty"`
	CT              []string `json:"ct,omitempty"`
	ST              []string `json:"st,omitempty"`
	ZP              []string `json:"zp,omitempty"`
	Country         []string `json:"country,omitempty"`
	ExternalID      []string `json:"external_id,omitempty"`
	ClientIPAddress string   `json:"client_ip_address,omitempty"` // Raw end-user IP (unhashed)
	ClientUserAgent string   `json:"client_user_agent,omitempty"` // Raw end-user UA (unhashed)
	Fbp             string   `json:"fbp,omitempty"`               // Raw _fbp cookie (unhashed)
	Fbc             string   `json:"fbc,omitempty"`               // Raw _fbc cookie (unhashed)
}

// CAPIContentItem represents a single item in custom_data.contents.
type CAPIContentItem struct {
	ID        string  `json:"id"`
	Quantity  int64   `json:"quantity,omitempty"`
	ItemPrice float64 `json:"item_price,omitempty"`
	Title     string  `json:"title,omitempty"`
	Category  string  `json:"category,omitempty"`
	Brand     string  `json:"brand,omitempty"`
}

// CAPICustomData holds e-commerce conversion attributes.
type CAPICustomData struct {
	Value           float64           `json:"value,omitempty"`
	Currency        string            `json:"currency,omitempty"`
	ContentType     string            `json:"content_type,omitempty"`
	ContentIDs      []string          `json:"content_ids,omitempty"`
	Contents        []CAPIContentItem `json:"contents,omitempty"`
	OrderID         string            `json:"order_id,omitempty"`
	ContentCategory string            `json:"content_category,omitempty"`
	NumItems        int               `json:"num_items,omitempty"`
}

// CAPIEvent represents a single Meta Conversions API event payload.
type CAPIEvent struct {
	EventName       string          `json:"event_name"`
	EventTime       int64           `json:"event_time"` // Unix epoch in seconds
	EventID         string          `json:"event_id,omitempty"`
	EventSourceURL  string          `json:"event_source_url,omitempty"`
	ActionSource    string          `json:"action_source"` // "website"
	OptOut          bool            `json:"opt_out,omitempty"`
	UserData        CAPIUserData    `json:"user_data"`
	CustomData      *CAPICustomData `json:"custom_data,omitempty"`
}

// CAPIPayload is the top-level body sent to https://graph.facebook.com/v19.0/{pixel_id}/events.
type CAPIPayload struct {
	Data          []CAPIEvent `json:"data"`
	TestEventCode string      `json:"test_event_code,omitempty"`
}

// CAPIResponse is returned by Meta Graph API.
type CAPIResponse struct {
	EventsReceived int        `json:"events_received"`
	Messages       []string   `json:"messages,omitempty"`
	FBTraceID      string     `json:"fbtrace_id,omitempty"`
	Error          *CAPIError `json:"error,omitempty"`
}

// CAPIError details any rejection from Meta Graph API.
type CAPIError struct {
	Message      string `json:"message"`
	Type         string `json:"type"`
	Code         int    `json:"code"`
	ErrorSubcode int    `json:"error_subcode,omitempty"`
	FBTraceID    string `json:"fbtrace_id,omitempty"`
}

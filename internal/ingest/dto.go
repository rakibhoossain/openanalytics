package ingest

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// FlexibleTimestamp supports ISO 8601 strings, RFC3339, and numeric milliseconds/seconds.
type FlexibleTimestamp time.Time

func (ft *FlexibleTimestamp) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == `""` || s == "" {
		return nil
	}

	// Numeric milliseconds or seconds
	if num, err := strconv.ParseInt(s, 10, 64); err == nil {
		if num > 1e11 {
			*ft = FlexibleTimestamp(time.UnixMilli(num).UTC())
		} else {
			*ft = FlexibleTimestamp(time.Unix(num, 0).UTC())
		}
		return nil
	}
	if fnum, err := strconv.ParseFloat(s, 64); err == nil {
		if fnum > 1e11 {
			*ft = FlexibleTimestamp(time.UnixMilli(int64(fnum)).UTC())
		} else {
			*ft = FlexibleTimestamp(time.Unix(int64(fnum), 0).UTC())
		}
		return nil
	}

	// Quoted string (RFC3339, ISO8601, etc.)
	var str string
	if err := json.Unmarshal(b, &str); err == nil {
		str = strings.TrimSpace(str)
		if num, err := strconv.ParseInt(str, 10, 64); err == nil {
			if num > 1e11 {
				*ft = FlexibleTimestamp(time.UnixMilli(num).UTC())
			} else {
				*ft = FlexibleTimestamp(time.Unix(num, 0).UTC())
			}
			return nil
		}
		formats := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05.000Z",
			"2006-01-02T15:04:05Z",
			"2006-01-02 15:04:05.000",
			"2006-01-02 15:04:05",
			"2006-01-02",
		}
		for _, layout := range formats {
			if t, err := time.Parse(layout, str); err == nil {
				*ft = FlexibleTimestamp(t.UTC())
				return nil
			}
		}
	}
	return nil
}

func (ft *FlexibleTimestamp) Time() time.Time {
	if ft == nil {
		return time.Time{}
	}
	return time.Time(*ft)
}

// TrackRequest represents an incoming telemetry tracking payload.
type TrackRequest struct {
	TenantID   string `json:"tenant_id,omitempty"`
	ShopID     string `json:"shop_id,omitempty"`
	Name       string `json:"name"`
	DeviceID   string `json:"device_id,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	CustomerID string `json:"customer_id,omitempty"`

	// E-Commerce telemetry (Revenue in integer cents, e.g. 100 = $1.00, 178 = $1.78, 14999 = $149.99)
	Revenue   *int64 `json:"revenue,omitempty"`
	Currency  string   `json:"currency,omitempty"`
	ProductID string   `json:"product_id,omitempty"`
	CartID    string   `json:"cart_id,omitempty"`
	OrderID   string   `json:"order_id,omitempty"`

	// Context
	Path       string                 `json:"path,omitempty"`
	Referrer   string                 `json:"referrer,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Timestamp  *FlexibleTimestamp     `json:"timestamp,omitempty"`

	// Testing / Simulation overrides (optional)
	IP        string `json:"ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
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

// ReplayChunkPayload represents an incoming rrweb session recording chunk.
type ReplayChunkPayload struct {
	TenantID       string `json:"tenant_id,omitempty"`
	AltTenantID    string `json:"tenantId,omitempty"`
	ShopID         string `json:"shop_id,omitempty"`
	AltShopID      string `json:"shopId,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	AltSessionID   string `json:"sessionId,omitempty"`
	ChunkIndex     uint16 `json:"chunk_index"`
	AltChunkIndex  uint16 `json:"chunkIndex"`
	EventsCount    uint16 `json:"events_count"`
	AltEventsCount uint16 `json:"eventsCount"`
	IsFullSnapshot bool   `json:"is_full_snapshot"`
	AltSnapshot    bool   `json:"isFullSnapshot"`
	StartedAt      string `json:"started_at"`
	AltStartedAt   string `json:"startedAt"`
	EndedAt        string `json:"ended_at"`
	AltEndedAt     string `json:"endedAt"`
	Payload        string `json:"payload"`
}

// ReplayResponse is returned on successful replay chunk ingestion.
type ReplayResponse struct {
	Status     string `json:"status"`
	ChunkIndex uint16 `json:"chunk_index"`
	SessionID  string `json:"session_id"`
}

// FlattenProperties recursively converts nested objects, booleans, and numbers into
// a flat dot-notation Map(String, String) matching OpenPanel's toDots.
func FlattenProperties(raw map[string]interface{}) map[string]string {
	if raw == nil {
		return make(map[string]string)
	}
	out := make(map[string]string)
	var flatten func(prefix string, val interface{})
	flatten = func(prefix string, val interface{}) {
		switch v := val.(type) {
		case map[string]interface{}:
			for k, child := range v {
				nextPrefix := k
				if prefix != "" {
					nextPrefix = prefix + "." + k
				}
				flatten(nextPrefix, child)
			}
		case []interface{}:
			bytes, _ := json.Marshal(v)
			out[prefix] = string(bytes)
		case string:
			out[prefix] = v
		case float64:
			if v == float64(int64(v)) {
				out[prefix] = strconv.FormatInt(int64(v), 10)
			} else {
				out[prefix] = strconv.FormatFloat(v, 'f', -1, 64)
			}
		case int:
			out[prefix] = strconv.Itoa(v)
		case int64:
			out[prefix] = strconv.FormatInt(v, 10)
		case bool:
			out[prefix] = strconv.FormatBool(v)
		case nil:
			// omit nil
		default:
			out[prefix] = fmt.Sprintf("%v", v)
		}
	}
	for k, v := range raw {
		flatten(k, v)
	}
	return out
}

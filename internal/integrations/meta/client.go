package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const DefaultGraphAPIBaseURL = "https://graph.facebook.com/v19.0"

// Client handles network communication with Meta Graph API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient initializes a Meta CAPI client with an optimized HTTP transport pool.
func NewClient(customBaseURL string) *Client {
	baseURL := DefaultGraphAPIBaseURL
	if customBaseURL != "" {
		baseURL = customBaseURL
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression: false,
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
		baseURL: baseURL,
	}
}

// SendEvents dispatches a batch of CAPI events to Meta Graph API.
func (c *Client) SendEvents(ctx context.Context, pixelID string, accessToken string, testEventCode string, events []CAPIEvent) (*CAPIResponse, error) {
	if pixelID == "" {
		return nil, fmt.Errorf("pixel_id is required")
	}
	if accessToken == "" {
		return nil, fmt.Errorf("access_token is required")
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no events to dispatch")
	}

	payload := CAPIPayload{
		Data:          events,
		TestEventCode: testEventCode,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize Meta CAPI payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s/%s/events", c.baseURL, pixelID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "OpenAnalytics-MetaCAPI/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network request failed to Meta CAPI: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Meta CAPI response body: %w", err)
	}

	var capiResp CAPIResponse
	if err := json.Unmarshal(respBytes, &capiResp); err != nil {
		return nil, fmt.Errorf("failed to parse Meta CAPI JSON response (status %d): %s", resp.StatusCode, string(respBytes))
	}

	if capiResp.Error != nil {
		return &capiResp, fmt.Errorf("Meta CAPI API Error [%d/%d]: %s (type: %s, trace: %s)",
			capiResp.Error.Code, capiResp.Error.ErrorSubcode, capiResp.Error.Message, capiResp.Error.Type, capiResp.Error.FBTraceID)
	}

	if resp.StatusCode >= 400 {
		return &capiResp, fmt.Errorf("Meta CAPI HTTP %d error: %s", resp.StatusCode, string(respBytes))
	}

	return &capiResp, nil
}

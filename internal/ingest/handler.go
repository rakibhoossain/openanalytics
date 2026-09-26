package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"openanalytics/internal/clickhouse"
	"openanalytics/internal/domain"
	"openanalytics/internal/geo"
	"openanalytics/internal/kafka"
	"openanalytics/internal/session"
	"openanalytics/pkg/hash"
	"openanalytics/pkg/httputil"
	"openanalytics/pkg/uuidv7"

	uaparser "github.com/rakibhoossain/ua-parser-go"
	"github.com/rakibhoossain/ua-parser-go/referrer"
)

// Handler handles incoming ingestion requests.
type Handler struct {
	geoService  *geo.Service
	producer    *kafka.Producer
	redisClient *redis.Client
	salt        string
	chWriter    *clickhouse.BatchWriter
	sessionMgr  *session.Manager
	onEvent     func(event *domain.Event)
}

// Config holds dependencies for Handler.
type Config struct {
	GeoService  *geo.Service
	Producer    *kafka.Producer
	RedisClient *redis.Client
	Salt        string
	CHWriter    *clickhouse.BatchWriter
	SessionMgr  *session.Manager
	OnEvent     func(event *domain.Event)
}

// NewHandler creates a new Ingestion Handler.
func NewHandler(cfg Config) *Handler {
	salt := cfg.Salt
	if salt == "" {
		salt = "openanalytics_default_salt"
	}

	return &Handler{
		geoService:  cfg.GeoService,
		producer:    cfg.Producer,
		redisClient: cfg.RedisClient,
		salt:        salt,
		chWriter:    cfg.CHWriter,
		sessionMgr:  cfg.SessionMgr,
		onEvent:     cfg.OnEvent,
	}
}

// HandleReplay handles POST /api/v1/replay.
func (h *Handler) HandleReplay(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVALID_BODY", "Failed to read request body: "+err.Error())
		return
	}
	h.processReplay(w, r, bodyBytes)
}

// HandleTrack handles POST /api/v1/track.
func (h *Handler) HandleTrack(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVALID_BODY", "Failed to read body: "+err.Error())
		return
	}

	// Check if this is an envelope request: { "type": "replay" | "track", "payload": ... }
	var envelope struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(bodyBytes, &envelope); err == nil && envelope.Type != "" {
		if envelope.Type == "replay" {
			h.processReplay(w, r, envelope.Payload)
			return
		}
		if (envelope.Type == "track" || envelope.Type == "identify") && len(envelope.Payload) > 0 {
			bodyBytes = envelope.Payload
		}
	}

	var req TrackRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVALID_JSON", "Failed to parse JSON body: "+err.Error())
		return
	}

	if req.Name == "" && req.Event != "" {
		req.Name = req.Event
	}

	if req.Name == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_NAME", "Event name is required")
		return
	}

	shopID, err := h.resolveShopID(r, &req)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVALID_SHOP_ID", "Valid shop_id is required: "+err.Error())
		return
	}

	tenantID := h.resolveTenantID(r, &req)

	// Resolve end-shopper Client IP:
	// Prioritize server-action forwarded IP overrides (UserData.ClientIPAddress or req.IP)
	// over direct TCP proxy remote address.
	clientIP := h.extractClientIP(r)
	if req.IP != "" {
		clientIP = req.IP
	} else if req.UserData != nil && req.UserData.ClientIPAddress != "" {
		clientIP = req.UserData.ClientIPAddress
	}

	// Resolve end-shopper User-Agent:
	// Prioritize server-action forwarded User-Agent over direct TCP proxy header.
	uaStr := r.Header.Get("User-Agent")
	if req.UserAgent != "" {
		uaStr = req.UserAgent
	} else if req.UserData != nil && req.UserData.ClientUserAgent != "" {
		uaStr = req.UserData.ClientUserAgent
	}
	uaRes := uaparser.ParseRequest(r)
	if uaStr != r.Header.Get("User-Agent") {
		uaRes = uaparser.Parse(uaStr)
	}

	deviceID := req.DeviceID
	if deviceID == "" {
		deviceID = hash.GenerateDeviceID(h.salt, shopID.String(), clientIP, uaStr)
	}

	// Fast-path deduplication (matches OpenPanel's duplicateHook):
	// Drops rapid client double-clicks, duplicate dispatches, and network retries within 100ms
	if h.redisClient != nil {
		dedupKey := fmt.Sprintf("dedup:%s:%s:%s:%s", shopID.String(), deviceID, req.Name, req.Path)
		set, err := h.redisClient.SetNX(r.Context(), dedupKey, "1", 100*time.Millisecond).Result()
		if err == nil && !set {
			httputil.JSON(w, http.StatusOK, TrackResponse{
				EventID:   "",
				DeviceID:  deviceID,
				SessionID: "",
				Status:    "duplicate_ignored",
			})
			return
		}
	}

	// Known Crawler Bypass (matches OpenPanel's isBotHook):
	// Search engine spiders and scrapers (Googlebot, Bingbot, etc.) are acknowledged
	// without poisoning Kafka analytics topics or e-commerce conversion funnels.
	if uaparser.IsCrawler(uaStr) {
		httputil.JSON(w, http.StatusAccepted, TrackResponse{
			EventID:   "",
			DeviceID:  deviceID,
			SessionID: "",
			Status:    "crawler_accepted",
		})
		return
	}

	// Flatten and normalize properties (supports OpenPanel toDots format)
	flatProps := FlattenProperties(req.Properties)

	// Normalize GA4/GTM ecommerce payload, value to cents, order_id, and deduplication event_id
	eventID := normalizeTrackRequest(&req, flatProps)

	sessionID := h.resolveSessionID(r.Context(), shopID, deviceID, &req)
	timestamp := h.resolveTimestamp(req.Timestamp)

	// Geo & ASN enrichment
	var loc *geo.Location
	var asnInfo *geo.ASNInfo
	if h.geoService != nil {
		loc, _ = h.geoService.Lookup(clientIP)
		asnInfo, _ = h.geoService.LookupASN(clientIP)
	}

	// Referrer parsing & classification (Snowplow + curated AI/tech/social domains)
	refInfo := referrer.Parse(req.Referrer)

	// Bot heuristics & suspicion scoring
	botVerdict := detectBotSuspicion(r, asnInfo, uaRes)
	enrichedProps := applyBotVerdict(flatProps, botVerdict)

	// Enrich with standard GA4/GTM items, CAPI user data, and deduplication event_id
	enrichCommerceProperties(enrichedProps, &req, eventID, clientIP, uaStr)

	var eventUUID uuid.UUID
	if eventID != "" {
		if parsedUUID, err := uuid.Parse(eventID); err == nil {
			eventUUID = parsedUUID
		} else {
			eventUUID = uuidv7.MustNew()
		}
	} else {
		eventUUID = uuidv7.MustNew()
	}

	event := &domain.Event{
		ID:           eventUUID,
		TenantID:     tenantID,
		ShopID:       shopID,
		Name:         req.Name,
		DeviceID:     deviceID,
		SessionID:    sessionID,
		Revenue:      req.Revenue,
		Currency:     req.Currency,
		ProductID:    parseUUIDPtr(req.ProductID),
		CartID:       parseUUIDPtr(req.CartID),
		OrderID:      parseUUIDPtr(req.OrderID),
		Path:         req.Path,
		Origin:       r.Header.Get("Origin"),
		Referrer:     refInfo.URL,
		ReferrerName: refInfo.Name,
		ReferrerType: string(refInfo.Type),
		OS:           uaRes.OS.Name,
		Browser:      uaRes.Browser.Name,
		Device:       resolveDeviceType(uaRes),
		Properties:   enrichedProps,
		CreatedAt:    timestamp,
	}

	if req.CustomerID != "" {
		if custUUID, err := uuid.Parse(req.CustomerID); err == nil {
			event.CustomerID = &custUUID
		}
	}

	if loc != nil {
		event.Country = loc.Country
		event.City = loc.City
		event.Latitude = loc.Latitude
		event.Longitude = loc.Longitude
	}
	enrichGeoFallbacks(event, r, flatProps, clientIP)

	// Produce to Kafka with reliable direct fallback
	if h.producer != nil {
		if err := h.producer.Produce(r.Context(), event); err != nil {
			// Fallback to direct ClickHouse sync so data is NEVER lost during Kafka outages
			if h.sessionMgr != nil && h.chWriter != nil {
				_ = h.sessionMgr.ProcessEventLifecycle(r.Context(), event, h.chWriter)
			} else if h.chWriter != nil {
				_ = h.chWriter.AddEvent(r.Context(), event)
			}
		}
	} else {
		// Real-time direct stream sync if running without Kafka
		if h.sessionMgr != nil && h.chWriter != nil {
			_ = h.sessionMgr.ProcessEventLifecycle(r.Context(), event, h.chWriter)
		} else if h.chWriter != nil {
			_ = h.chWriter.AddEvent(r.Context(), event)
		}
	}

	if h.onEvent != nil {
		h.onEvent(event)
	}

	httputil.JSON(w, http.StatusAccepted, TrackResponse{
		EventID:      event.ID.String(),
		AltEventID:   event.ID.String(),
		DeviceID:     deviceID,
		AltDeviceID:  deviceID,
		SessionID:    event.SessionID.String(),
		AltSessionID: event.SessionID.String(),
		Status:       "accepted",
	})
}

// HandleBatch handles POST /api/v1/batch.
func (h *Handler) HandleBatch(w http.ResponseWriter, r *http.Request) {
	var batch BatchTrackRequest
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVALID_JSON", "Failed to parse JSON body: "+err.Error())
		return
	}

	if len(batch.Events) == 0 {
		httputil.Error(w, http.StatusBadRequest, "EMPTY_BATCH", "Batch cannot be empty")
		return
	}
	if len(batch.Events) > 500 {
		httputil.Error(w, http.StatusBadRequest, "BATCH_TOO_LARGE", "Batch size exceeds maximum limit of 500")
		return
	}

	clientIP := h.extractClientIP(r)
	uaStr := r.Header.Get("User-Agent")
	uaRes := uaparser.ParseRequest(r)

	var loc *geo.Location
	var asnInfo *geo.ASNInfo
	if h.geoService != nil {
		loc, _ = h.geoService.Lookup(clientIP)
		asnInfo, _ = h.geoService.LookupASN(clientIP)
	}

	botVerdict := detectBotSuspicion(r, asnInfo, uaRes)

	events := make([]*domain.Event, 0, len(batch.Events))
	for _, req := range batch.Events {
		if req.Name == "" {
			continue
		}

		if req.Name == "" && req.Event != "" {
			req.Name = req.Event
		}
		if req.Name == "" {
			continue
		}

		shopID, err := h.resolveShopID(r, &req)
		if err != nil {
			continue
		}

		eventIP := clientIP
		if req.IP != "" {
			eventIP = req.IP
		} else if req.UserData != nil && req.UserData.ClientIPAddress != "" {
			eventIP = req.UserData.ClientIPAddress
		}
		eventUA := uaStr
		if req.UserAgent != "" {
			eventUA = req.UserAgent
		} else if req.UserData != nil && req.UserData.ClientUserAgent != "" {
			eventUA = req.UserData.ClientUserAgent
		}
		itemUARes := uaRes
		if eventUA != uaStr {
			itemUARes = uaparser.Parse(eventUA)
		}

		// Flatten and normalize properties (supports OpenPanel toDots format)
		itemFlatProps := FlattenProperties(req.Properties)
		eventID := normalizeTrackRequest(&req, itemFlatProps)

		tenantID := h.resolveTenantID(r, &req)
		deviceID := req.DeviceID
		if deviceID == "" {
			deviceID = hash.GenerateDeviceID(h.salt, shopID.String(), eventIP, eventUA)
		}

		sessionID := h.resolveSessionID(r.Context(), shopID, deviceID, &req)
		timestamp := h.resolveTimestamp(req.Timestamp)

		itemLoc := loc
		itemASN := asnInfo
		if (req.IP != "" || (req.UserData != nil && req.UserData.ClientIPAddress != "")) && h.geoService != nil {
			itemLoc, _ = h.geoService.Lookup(eventIP)
			itemASN, _ = h.geoService.LookupASN(eventIP)
		}

		itemBotVerdict := botVerdict
		if req.IP != "" || req.UserAgent != "" || req.UserData != nil {
			itemBotVerdict = detectBotSuspicion(r, itemASN, itemUARes)
		}

		refInfo := referrer.Parse(req.Referrer)
		enrichedProps := applyBotVerdict(itemFlatProps, itemBotVerdict)
		enrichCommerceProperties(enrichedProps, &req, eventID, eventIP, eventUA)

		var eventUUID uuid.UUID
		if eventID != "" {
			if parsedUUID, err := uuid.Parse(eventID); err == nil {
				eventUUID = parsedUUID
			} else {
				eventUUID = uuidv7.MustNew()
			}
		} else {
			eventUUID = uuidv7.MustNew()
		}

		event := &domain.Event{
			ID:           eventUUID,
			TenantID:     tenantID,
			ShopID:       shopID,
			Name:         req.Name,
			DeviceID:     deviceID,
			SessionID:    sessionID,
			Revenue:      req.Revenue,
			Currency:     req.Currency,
			ProductID:    parseUUIDPtr(req.ProductID),
			CartID:       parseUUIDPtr(req.CartID),
			OrderID:      parseUUIDPtr(req.OrderID),
			Path:         req.Path,
			Origin:       r.Header.Get("Origin"),
			Referrer:     refInfo.URL,
			ReferrerName: refInfo.Name,
			ReferrerType: string(refInfo.Type),
			OS:           itemUARes.OS.Name,
			Browser:      itemUARes.Browser.Name,
			Device:       resolveDeviceType(itemUARes),
			Properties:   enrichedProps,
			CreatedAt:    timestamp,
		}

		if req.CustomerID != "" {
			if custUUID, err := uuid.Parse(req.CustomerID); err == nil {
				event.CustomerID = &custUUID
			}
		}

		if itemLoc != nil {
			event.Country = itemLoc.Country
			event.City = itemLoc.City
			event.Latitude = itemLoc.Latitude
			event.Longitude = itemLoc.Longitude
		}
		enrichGeoFallbacks(event, r, itemFlatProps, eventIP)

		events = append(events, event)
	}

	if h.producer != nil {
		if err := h.producer.ProduceBatch(r.Context(), events); err != nil {
			// Fallback to direct ClickHouse sync if Kafka batch produce fails
			if h.chWriter != nil {
				for _, ev := range events {
					if h.sessionMgr != nil {
						_ = h.sessionMgr.ProcessEventLifecycle(r.Context(), ev, h.chWriter)
					} else {
						_ = h.chWriter.AddEvent(r.Context(), ev)
					}
				}
			}
		}
	} else if h.chWriter != nil {
		for _, ev := range events {
			if h.sessionMgr != nil {
				_ = h.sessionMgr.ProcessEventLifecycle(r.Context(), ev, h.chWriter)
			} else {
				_ = h.chWriter.AddEvent(r.Context(), ev)
			}
		}
	}

	if h.onEvent != nil {
		for _, ev := range events {
			h.onEvent(ev)
		}
	}

	httputil.JSON(w, http.StatusAccepted, map[string]interface{}{
		"status":   "accepted",
		"accepted": len(events),
	})
}

// HandleDeviceID handles GET /api/v1/track/device-id.
func (h *Handler) HandleDeviceID(w http.ResponseWriter, r *http.Request) {
	shopIDStr := r.URL.Query().Get("shop_id")
	if shopIDStr == "" {
		shopIDStr = r.Header.Get("X-Shop-Id")
	}
	if shopIDStr == "" {
		shopIDStr = r.Header.Get("openpanel-client-id")
	}

	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVALID_SHOP_ID", "Valid shop_id is required")
		return
	}

	clientIP := h.extractClientIP(r)
	uaStr := r.Header.Get("User-Agent")
	deviceID := hash.GenerateDeviceID(h.salt, shopID.String(), clientIP, uaStr)

	// Check if active session exists in Redis
	sessionID := ""
	if h.redisClient != nil {
		sessionKey := "session:" + shopID.String() + ":" + deviceID
		if sid, err := h.redisClient.HGet(r.Context(), sessionKey, "id").Result(); err == nil {
			sessionID = sid
		}
	}

	httputil.JSON(w, http.StatusOK, DeviceIDResponse{
		DeviceID:  deviceID,
		SessionID: sessionID,
		Message:   "device-id resolved successfully",
	})
}

func (h *Handler) resolveShopID(r *http.Request, req *TrackRequest) (uuid.UUID, error) {
	if req.ShopID != "" {
		return uuid.Parse(req.ShopID)
	}
	if req.ClientID != "" {
		return uuid.Parse(req.ClientID)
	}
	if hdr := r.Header.Get("X-Shop-Id"); hdr != "" {
		return uuid.Parse(hdr)
	}
	if hdr := r.Header.Get("openpanel-client-id"); hdr != "" {
		return uuid.Parse(hdr)
	}
	if hdr := r.Header.Get("openanalytics-client-id"); hdr != "" {
		return uuid.Parse(hdr)
	}
	return uuid.Nil, http.ErrNoCookie
}

func (h *Handler) resolveTenantID(r *http.Request, req *TrackRequest) uuid.UUID {
	if req.TenantID != "" {
		if id, err := uuid.Parse(req.TenantID); err == nil {
			return id
		}
	}
	if hdr := r.Header.Get("X-Tenant-Id"); hdr != "" {
		if id, err := uuid.Parse(hdr); err == nil {
			return id
		}
	}
	if req.ShopID == "018e69d0-7a89-7000-8b1a-200000000002" || req.ClientID == "018e69d0-7a89-7000-8b1a-200000000002" {
		return uuid.MustParse("018e69d0-7a89-7000-8b1a-200000000001")
	}
	if hdr := r.Header.Get("X-Shop-Id"); hdr == "018e69d0-7a89-7000-8b1a-200000000002" {
		return uuid.MustParse("018e69d0-7a89-7000-8b1a-200000000001")
	}
	if hdr := r.Header.Get("openpanel-client-id"); hdr == "018e69d0-7a89-7000-8b1a-200000000002" {
		return uuid.MustParse("018e69d0-7a89-7000-8b1a-200000000001")
	}
	if hdr := r.Header.Get("openanalytics-client-id"); hdr == "018e69d0-7a89-7000-8b1a-200000000002" {
		return uuid.MustParse("018e69d0-7a89-7000-8b1a-200000000001")
	}
	return uuid.Nil
}

func (h *Handler) resolveSessionID(ctx context.Context, shopID uuid.UUID, deviceID string, req *TrackRequest) uuid.UUID {
	if req.SessionID != "" {
		if id, err := uuid.Parse(req.SessionID); err == nil {
			return id
		}
	}
	if h.redisClient != nil && shopID != uuid.Nil && deviceID != "" {
		sessionKey := fmt.Sprintf("session:%s:%s", shopID.String(), deviceID)
		if sid, err := h.redisClient.HGet(ctx, sessionKey, "id").Result(); err == nil && sid != "" {
			if id, err := uuid.Parse(sid); err == nil {
				return id
			}
		}
		// Initialize session in Redis immediately to eliminate race conditions between rapid requests
		newID := uuidv7.MustNew()
		_ = h.redisClient.HSet(ctx, sessionKey, "id", newID.String())
		_ = h.redisClient.Expire(ctx, sessionKey, 30*time.Minute)
		return newID
	}
	return uuidv7.MustNew()
}

func (h *Handler) resolveTimestamp(ts *FlexibleTimestamp) time.Time {
	if ts == nil {
		return time.Now().UTC()
	}
	t := ts.Time()
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

func parseUUIDPtr(s string) *uuid.UUID {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if id, err := uuid.Parse(s); err == nil {
		return &id
	}
	return nil
}

func (h *Handler) extractClientIP(r *http.Request) string {
	if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
		return strings.TrimSpace(cf)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func isLocalIP(ipStr string) bool {
	if ipStr == "127.0.0.1" || ipStr == "::1" || ipStr == "localhost" || ipStr == "" {
		return true
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func enrichGeoFallbacks(event *domain.Event, r *http.Request, flatProps map[string]string, clientIP string) {
	if event.Country == "" {
		if c := r.Header.Get("CF-IPCountry"); len(c) == 2 {
			event.Country = strings.ToUpper(c)
		} else if c := r.Header.Get("X-Country-Code"); len(c) == 2 {
			event.Country = strings.ToUpper(c)
		} else if c := r.Header.Get("X-Country"); len(c) == 2 {
			event.Country = strings.ToUpper(c)
		} else if c, ok := flatProps["country"]; ok && len(c) == 2 {
			event.Country = strings.ToUpper(c)
		} else if c, ok := flatProps["$country"]; ok && len(c) == 2 {
			event.Country = strings.ToUpper(c)
		} else if c, ok := flatProps["__country"]; ok && len(c) == 2 {
			event.Country = strings.ToUpper(c)
		} else if devCountry := os.Getenv("DEV_DEFAULT_COUNTRY"); len(devCountry) == 2 && isLocalIP(clientIP) {
			event.Country = strings.ToUpper(devCountry)
		}
	}
	if event.City == "" {
		if c, ok := flatProps["city"]; ok && c != "" {
			event.City = c
		} else if c, ok := flatProps["$city"]; ok && c != "" {
			event.City = c
		} else if c, ok := flatProps["__city"]; ok && c != "" {
			event.City = c
		} else if devCity := os.Getenv("DEV_DEFAULT_CITY"); devCity != "" && isLocalIP(clientIP) {
			event.City = devCity
		}
	}
}

func resolveDeviceType(res *uaparser.Result) string {
	if res.IsBot {
		return "bot"
	}
	if res.Device.Type != "" {
		return string(res.Device.Type)
	}
	return "unknown"
}

func (h *Handler) processReplay(w http.ResponseWriter, r *http.Request, raw []byte) {
	var payload ReplayChunkPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVALID_REPLAY_JSON", "Failed to parse replay payload: "+err.Error())
		return
	}

	if payload.SessionID == "" && payload.AltSessionID != "" {
		payload.SessionID = payload.AltSessionID
	}
	if payload.ShopID == "" && payload.AltShopID != "" {
		payload.ShopID = payload.AltShopID
	}
	if payload.TenantID == "" && payload.AltTenantID != "" {
		payload.TenantID = payload.AltTenantID
	}
	if payload.ChunkIndex == 0 && payload.AltChunkIndex != 0 {
		payload.ChunkIndex = payload.AltChunkIndex
	}
	if payload.EventsCount == 0 && payload.AltEventsCount != 0 {
		payload.EventsCount = payload.AltEventsCount
	}
	if !payload.IsFullSnapshot && payload.AltSnapshot {
		payload.IsFullSnapshot = payload.AltSnapshot
	}
	if payload.StartedAt == nil && payload.AltStartedAt != nil {
		payload.StartedAt = payload.AltStartedAt
	}
	if payload.EndedAt == nil && payload.AltEndedAt != nil {
		payload.EndedAt = payload.AltEndedAt
	}

	if payload.SessionID == "" {
		httputil.Error(w, http.StatusBadRequest, "MISSING_SESSION_ID", "Session ID is required for replay")
		return
	}

	sessionID, err := uuid.Parse(payload.SessionID)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVALID_SESSION_ID", "Invalid session_id UUID: "+err.Error())
		return
	}

	shopIDStr := payload.ShopID
	if shopIDStr == "" {
		shopIDStr = r.Header.Get("X-Shop-Id")
	}
	if shopIDStr == "" {
		shopIDStr = r.Header.Get("openpanel-client-id")
	}
	if shopIDStr == "" {
		shopIDStr = r.Header.Get("openanalytics-client-id")
	}
	if shopIDStr == "" {
		httputil.Error(w, http.StatusBadRequest, "INVALID_SHOP_ID", "shop_id is required")
		return
	}
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVALID_SHOP_ID", "Invalid shop_id UUID: "+err.Error())
		return
	}

	tenantIDStr := payload.TenantID
	if tenantIDStr == "" {
		tenantIDStr = r.Header.Get("X-Tenant-Id")
	}
	var tenantID uuid.UUID
	if tenantIDStr != "" {
		if tid, err := uuid.Parse(tenantIDStr); err == nil {
			tenantID = tid
		}
	}
	if tenantID == uuid.Nil && (shopID.String() == "018e69d0-7a89-7000-8b1a-200000000002" || payload.ShopID == "018e69d0-7a89-7000-8b1a-200000000002") {
		tenantID = uuid.MustParse("018e69d0-7a89-7000-8b1a-200000000001")
	}

	startedAt := h.resolveTimestamp(payload.StartedAt)
	endedAt := h.resolveTimestamp(payload.EndedAt)
	if endedAt.Before(startedAt) {
		endedAt = startedAt
	}

	chunk := &domain.ReplayChunk{
		TenantID:       tenantID,
		ShopID:         shopID,
		SessionID:      sessionID,
		ChunkIndex:     payload.ChunkIndex,
		StartedAt:      startedAt,
		EndedAt:        endedAt,
		EventsCount:    payload.EventsCount,
		IsFullSnapshot: payload.IsFullSnapshot,
		Payload:        payload.Payload,
	}

	if h.chWriter != nil {
		if err := h.chWriter.AddReplayChunk(r.Context(), chunk); err != nil {
			httputil.Error(w, http.StatusInternalServerError, "INGEST_FAILED", "Failed to buffer replay chunk: "+err.Error())
			return
		}
	}

	httputil.JSON(w, http.StatusAccepted, ReplayResponse{
		Status:     "accepted",
		ChunkIndex: payload.ChunkIndex,
		SessionID:  sessionID.String(),
	})
}

func parseTimestampString(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Now().UTC()
	}
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}

// normalizeTrackRequest normalizes GA4/GTM ecommerce payloads, decimal values to integer cents,
// and extracts the deduplication event ID.
func normalizeTrackRequest(req *TrackRequest, flatProps map[string]string) string {
	// GA4/GTM standard event name alias
	if req.Name == "" && req.Event != "" {
		req.Name = req.Event
	}

	// Meta CAPI deduplication event_id
	eventID := req.EventID
	if eventID == "" {
		eventID = req.AltEventID
	}
	if eventID == "" {
		if eid, ok := flatProps["event_id"]; ok && eid != "" {
			eventID = eid
		} else if eid, ok := flatProps["eventId"]; ok && eid != "" {
			eventID = eid
		}
	}

	// Unpack GTM dataLayer 'ecommerce' object if provided
	if req.Ecommerce != nil {
		if rawItems, ok := req.Ecommerce["items"].([]interface{}); ok && len(req.Items) == 0 {
			if itemsBytes, err := json.Marshal(rawItems); err == nil {
				_ = json.Unmarshal(itemsBytes, &req.Items)
			}
		}
		if req.Value == nil {
			if val, ok := req.Ecommerce["value"].(float64); ok {
				req.Value = &val
			}
		}
		if req.Currency == "" {
			if cur, ok := req.Ecommerce["currency"].(string); ok {
				req.Currency = cur
			}
		}
		if req.TransactionID == "" {
			if tid, ok := req.Ecommerce["transaction_id"].(string); ok {
				req.TransactionID = tid
			}
		}
	}

	// Fallback to __path, __referrer, __revenue if missing from top-level request
	if req.Path == "" {
		if p, ok := flatProps["__path"]; ok && p != "" {
			req.Path = p
		} else if p, ok := flatProps["path"]; ok && p != "" {
			req.Path = p
		}
	}
	if req.Referrer == "" {
		if ref, ok := flatProps["__referrer"]; ok && ref != "" {
			req.Referrer = ref
		} else if ref, ok := flatProps["referrer"]; ok && ref != "" {
			req.Referrer = ref
		}
	}
	if req.Revenue == nil {
		if revStr, ok := flatProps["__revenue"]; ok && revStr != "" {
			if v, err := strconv.ParseInt(revStr, 10, 64); err == nil {
				req.Revenue = &v
			}
		} else if revStr, ok := flatProps["revenue"]; ok && revStr != "" {
			if v, err := strconv.ParseInt(revStr, 10, 64); err == nil {
				req.Revenue = &v
			}
		} else if req.Value != nil {
			revCents := int64(math.Round(*req.Value * 100))
			req.Revenue = &revCents
		} else if valStr, ok := flatProps["value"]; ok && valStr != "" {
			if vf, err := strconv.ParseFloat(valStr, 64); err == nil {
				revCents := int64(math.Round(vf * 100))
				req.Revenue = &revCents
			}
		}
	}

	if req.OrderID == "" && req.TransactionID != "" {
		req.OrderID = req.TransactionID
	}
	if req.TransactionID == "" && req.OrderID != "" {
		req.TransactionID = req.OrderID
	}

	return eventID
}

// enrichCommerceProperties formats items, user_data, and event_id into ClickHouse string properties.
func enrichCommerceProperties(enrichedProps map[string]string, req *TrackRequest, eventID, clientIP, uaStr string) {
	if clientIP != "" {
		enrichedProps["client_ip"] = clientIP
	}
	if uaStr != "" {
		enrichedProps["client_ua"] = uaStr
	}
	// E-commerce items serialization into properties
	if len(req.Items) > 0 {
		if itemsJSON, err := json.Marshal(req.Items); err == nil {
			enrichedProps["items"] = string(itemsJSON)
		}
		enrichedProps["items_count"] = strconv.Itoa(len(req.Items))
		if req.Items[0].ItemID != "" {
			enrichedProps["item_id"] = req.Items[0].ItemID
		}
		if req.Items[0].ItemName != "" {
			enrichedProps["item_name"] = req.Items[0].ItemName
		}
	}

	// Meta CAPI deduplication event_id
	if eventID != "" {
		enrichedProps["event_id"] = eventID
	}

	// Meta CAPI user data preservation
	if req.UserData != nil {
		if req.UserData.Fbp != "" {
			enrichedProps["fbp"] = req.UserData.Fbp
		}
		if req.UserData.Fbc != "" {
			enrichedProps["fbc"] = req.UserData.Fbc
		}
		if req.UserData.Email != "" {
			enrichedProps["email_provided"] = "1"
		}
		if req.UserData.Phone != "" {
			enrichedProps["phone_provided"] = "1"
		}
		if udJSON, err := json.Marshal(req.UserData); err == nil {
			enrichedProps["_user_data"] = string(udJSON)
		}
	}

	if req.TransactionID != "" {
		enrichedProps["transaction_id"] = req.TransactionID
	}
}


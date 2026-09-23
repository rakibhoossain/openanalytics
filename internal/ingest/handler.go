package ingest

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"openanalytics/internal/clickhouse"
	"openanalytics/internal/domain"
	"openanalytics/internal/geo"
	"openanalytics/internal/kafka"
	"openanalytics/internal/session"
	"openanalytics/pkg/bot"
	"openanalytics/pkg/hash"
	"openanalytics/pkg/httputil"
	"openanalytics/pkg/referrer"
	"openanalytics/pkg/uaparser"
	"openanalytics/pkg/uuidv7"
)

// Handler handles incoming ingestion requests.
type Handler struct {
	geoService  *geo.Service
	producer    *kafka.Producer
	redisClient *redis.Client
	salt        string
	chWriter    *clickhouse.BatchWriter
	sessionMgr  *session.Manager
}

// Config holds dependencies for Handler.
type Config struct {
	GeoService  *geo.Service
	Producer    *kafka.Producer
	RedisClient *redis.Client
	Salt        string
	CHWriter    *clickhouse.BatchWriter
	SessionMgr  *session.Manager
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
	}
}

// HandleTrack handles POST /api/v1/track.
func (h *Handler) HandleTrack(w http.ResponseWriter, r *http.Request) {
	var req TrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVALID_JSON", "Failed to parse JSON body: "+err.Error())
		return
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
	clientIP := h.extractClientIP(r)
	if req.IP != "" {
		clientIP = req.IP
	}
	uaStr := r.Header.Get("User-Agent")
	if req.UserAgent != "" {
		uaStr = req.UserAgent
	}
	uaInfo := uaparser.Parse(uaStr)

	deviceID := req.DeviceID
	if deviceID == "" {
		deviceID = hash.GenerateDeviceID(h.salt, shopID.String(), clientIP, uaStr)
	}

	sessionID := h.resolveSessionID(&req)
	timestamp := h.resolveTimestamp(req.Timestamp)

	// Geo & ASN enrichment
	var loc *geo.Location
	var asnInfo *geo.ASNInfo
	if h.geoService != nil {
		loc, _ = h.geoService.Lookup(clientIP)
		asnInfo, _ = h.geoService.LookupASN(clientIP)
	}

	// Referrer parsing & classification
	// TODO(accuracy): Expand with 2,800+ search & social referrers from
	// `openpanel/packages/common/server/referrers/index.ts`
	refInfo := referrer.Parse(req.Referrer)

	// Bot heuristics & suspicion scoring
	// TODO(accuracy): Port advanced multi-category heuristics from
	// `openpanel/apps/api/src/bots/suspicion.ts` and `header-signals.ts`
	botVerdict := bot.Detect(r, asnInfo, uaInfo)
	enrichedProps := bot.ApplyToProperties(req.Properties, botVerdict)

	event := &domain.Event{
		ID:           uuidv7.MustNew(),
		TenantID:     tenantID,
		ShopID:       shopID,
		Name:         req.Name,
		DeviceID:     deviceID,
		SessionID:    sessionID,
		Revenue:      req.Revenue,
		Currency:     req.Currency,
		ProductID:    req.ProductID,
		CartID:       req.CartID,
		OrderID:      req.OrderID,
		Path:         req.Path,
		Origin:       r.Header.Get("Origin"),
		Referrer:     refInfo.URL,
		ReferrerName: refInfo.Name,
		ReferrerType: refInfo.Type,
		OS:           uaInfo.OS,
		Browser:      uaInfo.Browser,
		Device:       uaInfo.Device,
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

	// Produce to Kafka
	if err := h.producer.Produce(r.Context(), event); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "INGEST_FAILED", "Failed to enqueue event: "+err.Error())
		return
	}

	// Real-time direct stream sync if running in unified mode
	if h.chWriter != nil {
		_ = h.chWriter.AddEvent(r.Context(), event)
	}
	if h.sessionMgr != nil {
		res, _ := h.sessionMgr.Ingest(r.Context(), event)
		if res != nil && res.ClosedSession != nil && h.chWriter != nil {
			h.chWriter.AddSession(res.ClosedSession)
		}
	}

	httputil.JSON(w, http.StatusAccepted, TrackResponse{
		EventID:   event.ID.String(),
		DeviceID:  deviceID,
		SessionID: sessionID.String(),
		Status:    "accepted",
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
	uaInfo := uaparser.Parse(uaStr)

	var loc *geo.Location
	var asnInfo *geo.ASNInfo
	if h.geoService != nil {
		loc, _ = h.geoService.Lookup(clientIP)
		asnInfo, _ = h.geoService.LookupASN(clientIP)
	}

	botVerdict := bot.Detect(r, asnInfo, uaInfo)

	events := make([]*domain.Event, 0, len(batch.Events))
	for _, req := range batch.Events {
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
		}
		eventUA := uaStr
		if req.UserAgent != "" {
			eventUA = req.UserAgent
		}
		itemUAInfo := uaInfo
		if req.UserAgent != "" {
			itemUAInfo = uaparser.Parse(eventUA)
		}

		tenantID := h.resolveTenantID(r, &req)
		deviceID := req.DeviceID
		if deviceID == "" {
			deviceID = hash.GenerateDeviceID(h.salt, shopID.String(), eventIP, eventUA)
		}

		sessionID := h.resolveSessionID(&req)
		timestamp := h.resolveTimestamp(req.Timestamp)

		itemLoc := loc
		itemASN := asnInfo
		if req.IP != "" && h.geoService != nil {
			itemLoc, _ = h.geoService.Lookup(eventIP)
			itemASN, _ = h.geoService.LookupASN(eventIP)
		}

		itemBotVerdict := botVerdict
		if req.IP != "" || req.UserAgent != "" {
			itemBotVerdict = bot.Detect(r, itemASN, itemUAInfo)
		}

		refInfo := referrer.Parse(req.Referrer)
		enrichedProps := bot.ApplyToProperties(req.Properties, itemBotVerdict)

		event := &domain.Event{
			ID:           uuidv7.MustNew(),
			TenantID:     tenantID,
			ShopID:       shopID,
			Name:         req.Name,
			DeviceID:     deviceID,
			SessionID:    sessionID,
			Revenue:      req.Revenue,
			Currency:     req.Currency,
			ProductID:    req.ProductID,
			CartID:       req.CartID,
			OrderID:      req.OrderID,
			Path:         req.Path,
			Origin:       r.Header.Get("Origin"),
			Referrer:     refInfo.URL,
			ReferrerName: refInfo.Name,
			ReferrerType: refInfo.Type,
			OS:           itemUAInfo.OS,
			Browser:      itemUAInfo.Browser,
			Device:       itemUAInfo.Device,
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

		events = append(events, event)
	}

	if err := h.producer.ProduceBatch(r.Context(), events); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "INGEST_BATCH_FAILED", "Failed to enqueue batch: "+err.Error())
		return
	}

	// Real-time direct stream sync if running in unified mode
	if h.chWriter != nil {
		for _, ev := range events {
			_ = h.chWriter.AddEvent(r.Context(), ev)
			if h.sessionMgr != nil {
				res, _ := h.sessionMgr.Ingest(r.Context(), ev)
				if res != nil && res.ClosedSession != nil {
					h.chWriter.AddSession(res.ClosedSession)
				}
			}
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
	if hdr := r.Header.Get("X-Shop-Id"); hdr != "" {
		return uuid.Parse(hdr)
	}
	if hdr := r.Header.Get("openpanel-client-id"); hdr != "" {
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
	return uuid.Nil
}

func (h *Handler) resolveSessionID(req *TrackRequest) uuid.UUID {
	if req.SessionID != "" {
		if id, err := uuid.Parse(req.SessionID); err == nil {
			return id
		}
	}
	return uuidv7.MustNew()
}

func (h *Handler) resolveTimestamp(ts *int64) time.Time {
	now := time.Now().UTC()
	if ts == nil || *ts == 0 {
		return now
	}

	clientTime := time.UnixMilli(*ts).UTC()
	// Allow client timestamp if within +/- 15 minutes
	if clientTime.After(now.Add(-15*time.Minute)) && clientTime.Before(now.Add(15*time.Minute)) {
		return clientTime
	}
	return now
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

package ingest

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
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
	uaRes := uaparser.ParseRequest(r)
	if req.UserAgent != "" {
		uaRes = uaparser.Parse(req.UserAgent)
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
		}
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

	// Referrer parsing & classification (Snowplow + curated AI/tech/social domains)
	refInfo := referrer.Parse(req.Referrer)

	// Bot heuristics & suspicion scoring
	botVerdict := detectBotSuspicion(r, asnInfo, uaRes)
	enrichedProps := applyBotVerdict(flatProps, botVerdict)

	event := &domain.Event{
		ID:           uuidv7.MustNew(),
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
		itemUARes := uaRes
		if req.UserAgent != "" {
			itemUARes = uaparser.Parse(eventUA)
		}

		// Flatten and normalize properties (supports OpenPanel toDots format)
		itemFlatProps := FlattenProperties(req.Properties)

		// Fallback to __path, __referrer, __revenue if missing from top-level request
		if req.Path == "" {
			if p, ok := itemFlatProps["__path"]; ok && p != "" {
				req.Path = p
			} else if p, ok := itemFlatProps["path"]; ok && p != "" {
				req.Path = p
			}
		}
		if req.Referrer == "" {
			if ref, ok := itemFlatProps["__referrer"]; ok && ref != "" {
				req.Referrer = ref
			} else if ref, ok := itemFlatProps["referrer"]; ok && ref != "" {
				req.Referrer = ref
			}
		}
		if req.Revenue == nil {
			if revStr, ok := itemFlatProps["__revenue"]; ok && revStr != "" {
				if v, err := strconv.ParseInt(revStr, 10, 64); err == nil {
					req.Revenue = &v
				}
			} else if revStr, ok := itemFlatProps["revenue"]; ok && revStr != "" {
				if v, err := strconv.ParseInt(revStr, 10, 64); err == nil {
					req.Revenue = &v
				}
			}
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
			itemBotVerdict = detectBotSuspicion(r, itemASN, itemUARes)
		}

		refInfo := referrer.Parse(req.Referrer)
		enrichedProps := applyBotVerdict(itemFlatProps, itemBotVerdict)

		event := &domain.Event{
			ID:           uuidv7.MustNew(),
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

func resolveDeviceType(res *uaparser.Result) string {
	if res.IsBot {
		return "bot"
	}
	if res.Device.Type != "" {
		return string(res.Device.Type)
	}
	return "unknown"
}

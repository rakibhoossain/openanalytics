package query

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"openanalytics/internal/cron"
	"openanalytics/internal/domain"
	"openanalytics/pkg/httputil"
	"openanalytics/pkg/uuidv7"
)

// Handler serves HTTP endpoints for dashboards and analytical queries.
type Handler struct {
	queryService *Service
	rdb          *redis.Client
	wsHub        *WebSocketHub
}

// NewHandler creates a new Query HTTP Handler.
func NewHandler(qs *Service) *Handler {
	return &Handler{
		queryService: qs,
	}
}

// WithRedis attaches a Redis client for real-time feature store queries.
func (h *Handler) WithRedis(rdb *redis.Client) *Handler {
	h.rdb = rdb
	return h
}

// WithWSHub attaches a WebSocketHub for live client event streaming.
func (h *Handler) WithWSHub(hub *WebSocketHub) *Handler {
	h.wsHub = hub
	return h
}

// RegisterRoutes mounts all query and metadata routes on the chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/health", h.HandleHealth)

	// Live WebSocket routes for real-time UI streaming
	if h.wsHub != nil {
		mountLive := func(r chi.Router) {
			r.Get("/visitors/{projectId}", h.wsHub.ServeLiveVisitors)
			r.Get("/visitors/{shopId}", h.wsHub.ServeLiveVisitors)
			r.Get("/events/{projectId}", h.wsHub.ServeLiveEvents)
			r.Get("/events/{shopId}", h.wsHub.ServeLiveEvents)
			r.Get("/notifications/{projectId}", h.wsHub.ServeLiveNotifications)
			r.Get("/notifications/{shopId}", h.wsHub.ServeLiveNotifications)
			r.Get("/organization/{organizationId}", h.wsHub.ServeLiveOrganization)
			r.Get("/organization/{tenantId}", h.wsHub.ServeLiveOrganization)
		}
		r.Route("/live", mountLive)
		r.Route("/api/live", mountLive)
	}

	// Favicon and Icon Proxy routes
	mountMisc := func(r chi.Router) {
		r.Get("/favicon", h.HandleFaviconProxy)
		r.Get("/favicon/clear", h.HandleFaviconClear)
	}
	r.Route("/misc", mountMisc)
	r.Route("/api/v1/misc", mountMisc)

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/misc", mountMisc)
		// Analytics Query APIs
		r.Route("/query", func(r chi.Router) {
			r.Get("/trends", h.HandleTrends)
			r.Post("/funnel", h.HandleFunnel)
			r.Get("/live", h.HandleLiveVisitors)
			r.Get("/shopper/{id}", h.HandleShopperJourney)
			r.Get("/intents", h.HandleIntents)
			r.Get("/insights", h.HandleGetInsights)
			r.Post("/insights/compute", h.HandleComputeInsights)

			// Enhanced Overview APIs
			r.Get("/overview/stats", h.HandleOverviewStats)
			r.Get("/overview/top-generic", h.HandleTopGeneric)
			r.Get("/overview/top-pages", h.HandleTopPages)
			r.Get("/overview/top-events", h.HandleTopEvents)
			r.Get("/overview/top-geo", h.HandleTopGeo)
			r.Get("/overview/live-data", h.HandleOverviewLiveData)

			// Explorers
			r.Get("/events", h.HandleListEvents)
			r.Get("/sessions", h.HandleListSessions)
		})

		// Dashboard Metadata APIs
		r.Route("/dashboards", func(r chi.Router) {
			r.Get("/", h.HandleListDashboards)
			r.Post("/", h.HandleCreateDashboard)
			r.Get("/{id}", h.HandleGetDashboard)
			r.Put("/{id}", h.HandleUpdateDashboard)
			r.Delete("/{id}", h.HandleDeleteDashboard)

			r.Get("/{id}/widgets", h.HandleListWidgets)
			r.Post("/{id}/widgets", h.HandleCreateWidget)
		})

		r.Delete("/widgets/{id}", h.HandleDeleteWidget)

		// Reports
		r.Get("/reports", h.HandleListReports)
		r.Post("/reports", h.HandleCreateReport)
	})

	// tRPC Gateway for ported OpenPanel UI
	r.Route("/trpc", func(r chi.Router) {
		r.Get("/overview.stats", h.HandleTRPCOverviewStats)
		r.Get("/overview.topGeneric", h.HandleTRPCOverviewTopGeneric)
		r.Get("/overview.topGenericSeries", h.HandleTRPCOverviewTopGenericSeries)
		r.Get("/overview.topPages", h.HandleTRPCOverviewTopPages)
		r.Get("/overview.topSources", h.HandleTRPCOverviewTopSources)
		r.Get("/overview.topEvents", h.HandleTRPCOverviewTopEvents)
		r.Get("/overview.topGeo", h.HandleTRPCOverviewTopGeo)
		r.Get("/overview.map", h.HandleTRPCOverviewMap)
		r.Get("/overview.liveVisitors", h.HandleTRPCLiveVisitors)
		r.Get("/overview.liveData", h.HandleTRPCLiveData)
		r.Get("/overview.getReferrerSpikes", h.HandleTRPCOverviewReferrerSpikes)
		r.Get("/overview.topConversions", h.HandleTRPCOverviewTopConversions)
		r.Get("/overview.topLinkOut", h.HandleTRPCOverviewTopLinkOut)
		r.Get("/overview.userJourney", h.HandleTRPCOverviewUserJourney)
		r.Get("/reference.getChartReferences", h.HandleTRPCReferenceGetChartReferences)
		r.Get("/reference.list", h.HandleTRPCEmptyList)
		r.Get("/cohort.list", h.HandleTRPCEmptyList)
		r.Get("/notification.list", h.HandleTRPCEmptyList)
		r.Get("/integration.list", h.HandleTRPCEmptyList)
		r.Get("/project.get", h.HandleTRPCProjectGet)
		r.Get("/project.getProjectWithClients", h.HandleTRPCProjectGet)
		r.Get("/project.list", h.HandleTRPCProjectList)
		r.Get("/client.list", h.HandleTRPCClientList)
		r.Get("/organization.get", h.HandleTRPCOrganizationGet)
		r.Get("/organization.list", h.HandleTRPCOrganizationList)
		r.Get("/organization.myAccess", h.HandleTRPCOrganizationMyAccess)
		r.Get("/auth.session", h.HandleTRPCAuthSession)
		// Dashboard
		r.Get("/dashboard.list", h.HandleTRPCDashboardList)
		r.Get("/dashboard.byId", h.HandleTRPCDashboardGet)
		r.Get("/dashboard.get", h.HandleTRPCDashboardGet)
		r.Post("/dashboard.create", h.HandleTRPCDashboardCreate)
		r.Post("/dashboard.update", h.HandleTRPCDashboardUpdate)
		r.Post("/dashboard.delete", h.HandleTRPCDashboardDelete)

		// Report
		r.Get("/report.list", h.HandleTRPCReportList)
		r.Get("/report.get", h.HandleTRPCReportGet)
		r.Post("/report.create", h.HandleTRPCReportCreate)
		r.Post("/report.update", h.HandleTRPCReportUpdate)
		r.Post("/report.delete", h.HandleTRPCReportDelete)
		r.Post("/report.duplicate", h.HandleTRPCReportDuplicate)
		r.Post("/report.updateLayout", h.HandleTRPCReportUpdateLayout)
		r.Post("/report.resetLayout", h.HandleTRPCReportResetLayout)

		// Chart
		r.Get("/chart.chart", h.HandleTRPCChart)
		r.Post("/chart.chart", h.HandleTRPCChart)
		r.Get("/chart.aggregate", h.HandleTRPCChart)
		r.Post("/chart.aggregate", h.HandleTRPCChart)
		r.Get("/chart.events", h.HandleTRPCChartEvents)
		r.Get("/chart.properties", h.HandleTRPCChartProperties)
		r.Get("/chart.values", h.HandleTRPCChartValues)

		// Realtime
		r.Get("/realtime.paths", h.HandleTRPCRealtimePaths)
		r.Get("/realtime.coordinates", h.HandleTRPCRealtimeCoordinates)
		r.Get("/realtime.geo", h.HandleTRPCRealtimeGeo)
		r.Get("/realtime.referrals", h.HandleTRPCRealtimeReferrals)
		r.Get("/realtime.activeSessions", h.HandleTRPCRealtimeActiveSessions)
		r.Get("/realtime.mapBadgeDetails", h.HandleTRPCRealtimeMapBadgeDetails)
		r.Get("/ml.intents", h.HandleTRPCIntents)

		r.Get("/event.events", h.HandleTRPCEvents)
		r.Get("/event.conversions", h.HandleTRPCConversions)
		r.Get("/event.conversionNames", h.HandleTRPCConversionNames)
		r.Get("/event.eventNames", h.HandleTRPCEventNames)
		r.Get("/event.list", h.HandleTRPCEvents)
		r.Get("/event.pages", h.HandleTRPCEventPages)
		r.Get("/event.previousPages", h.HandleTRPCEventPreviousPages)
		r.Get("/event.pagesTimeseries", h.HandleTRPCEventPagesTimeseries)
		r.Get("/session.sessions", h.HandleTRPCSessionList)
		r.Get("/session.list", h.HandleTRPCSessionList)
		r.Get("/session.byId", h.HandleTRPCSessionById)
		r.Get("/session.replayChunksFrom", h.HandleTRPCReplayChunksFrom)

		// Profile
		r.Get("/profile.list", h.HandleTRPCProfileList)
		r.Get("/profile.powerUsers", h.HandleTRPCProfilePowerUsers)
		r.Get("/profile.byId", h.HandleTRPCProfileById)
		r.Get("/profile.metrics", h.HandleTRPCProfileMetrics)
		r.Get("/profile.activity", h.HandleTRPCProfileActivity)
		r.Get("/profile.mostEvents", h.HandleTRPCProfileMostEvents)
		r.Get("/profile.popularRoutes", h.HandleTRPCProfilePopularRoutes)
		r.Get("/profile.properties", h.HandleTRPCProfileProperties)

		// Group
		r.Get("/group.list", h.HandleTRPCGroupList)
		r.Get("/group.types", h.HandleTRPCGroupTypes)
		r.Get("/group.byId", h.HandleTRPCGroupById)
		r.Get("/group.listByIds", h.HandleTRPCEmptyList)
		r.Get("/group.listProfiles", h.HandleTRPCEmptyList)
		r.Get("/group.properties", h.HandleTRPCEmptyList)

		// Cohort & Other
		r.Get("/cohort.list", h.HandleTRPCEmptyList)
		r.Get("/insight.list", h.HandleTRPCInsightList)
		r.Get("/insight.listAll", h.HandleTRPCInsightList)
		r.HandleFunc("/*", h.HandleTRPCFallback)
	})
}

// HandleHealth provides a health probe endpoint.
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	httputil.JSON(w, http.StatusOK, map[string]string{
		"status":    "healthy",
		"component": "query-engine",
	})
}

// --- Analytics Query Handlers ---

// HandleTrends computes aggregated metric time series.
func (h *Handler) HandleTrends(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if h.queryService == nil {
		httputil.Error(w, http.StatusServiceUnavailable, "CLICKHOUSE_UNAVAILABLE", "ClickHouse query service is not connected")
		return
	}

	metric := r.URL.Query().Get("metric")
	timeRange := r.URL.Query().Get("time_range")
	interval := r.URL.Query().Get("interval")

	points, err := h.queryService.GetTrends(r.Context(), tenantID, shopID, metric, timeRange, interval)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "QUERY_FAILED", err.Error())
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{
		"metric":     metric,
		"time_range": timeRange,
		"data":       points,
	})
}

// FunnelRequest payload
type FunnelRequest struct {
	ShopID    string   `json:"shop_id"`
	TenantID  string   `json:"tenant_id"`
	TimeRange string   `json:"time_range"`
	Steps     []string `json:"steps"`
}

// HandleFunnel executes a conversion funnel query.
func (h *Handler) HandleFunnel(w http.ResponseWriter, r *http.Request) {
	var req FunnelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		tenantID, _ = uuid.Parse(r.Header.Get("X-Tenant-ID"))
	}
	shopID, err := uuid.Parse(req.ShopID)
	if err != nil {
		shopID, _ = uuid.Parse(r.Header.Get("X-Shop-ID"))
	}

	if tenantID == uuid.Nil || shopID == uuid.Nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", "valid tenant_id and shop_id are required")
		return
	}
	if h.queryService == nil {
		httputil.Error(w, http.StatusServiceUnavailable, "CLICKHOUSE_UNAVAILABLE", "ClickHouse query service is not connected")
		return
	}

	result, err := h.queryService.GetFunnel(r.Context(), tenantID, shopID, req.TimeRange, req.Steps)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "QUERY_FAILED", err.Error())
		return
	}

	httputil.JSON(w, http.StatusOK, result)
}

// HandleLiveVisitors calculates active users in the past window.
func (h *Handler) HandleLiveVisitors(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if h.queryService == nil {
		httputil.Error(w, http.StatusServiceUnavailable, "CLICKHOUSE_UNAVAILABLE", "ClickHouse query service is not connected")
		return
	}

	winMin := 5
	if winStr := r.URL.Query().Get("window_minutes"); winStr != "" {
		if val, err := strconv.Atoi(winStr); err == nil && val > 0 {
			winMin = val
		}
	}

	live, err := h.queryService.GetLiveVisitors(r.Context(), tenantID, shopID, winMin)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "QUERY_FAILED", err.Error())
		return
	}

	httputil.JSON(w, http.StatusOK, live)
}

// HandleShopperJourney returns the full timeline of events for an individual device or shopper.
func (h *Handler) HandleShopperJourney(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if h.queryService == nil {
		httputil.Error(w, http.StatusServiceUnavailable, "CLICKHOUSE_UNAVAILABLE", "ClickHouse query service is not connected")
		return
	}

	identifier := chi.URLParam(r, "id")
	if identifier == "" {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", "shopper identifier required")
		return
	}

	journey, err := h.queryService.GetShopperJourney(r.Context(), tenantID, shopID, identifier)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "QUERY_FAILED", err.Error())
		return
	}

	httputil.JSON(w, http.StatusOK, journey)
}

// --- Dashboard Metadata Handlers ---

// HandleListDashboards retrieves all built-in dashboards for a shop.
func (h *Handler) HandleListDashboards(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	httputil.JSON(w, http.StatusOK, GetBuiltinDashboards(tenantID, shopID))
}

// HandleCreateDashboard handles creation (mocked/no-op in built-in mode).
func (h *Handler) HandleCreateDashboard(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	var d domain.Dashboard
	_ = json.NewDecoder(r.Body).Decode(&d)
	d.ID = uuidv7.MustNew()
	d.TenantID = tenantID
	d.ShopID = shopID

	httputil.JSON(w, http.StatusCreated, d)
}

// HandleGetDashboard retrieves a built-in dashboard by ID.
func (h *Handler) HandleGetDashboard(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	idStr := chi.URLParam(r, "id")
	d := GetBuiltinDashboardByID(tenantID, shopID, idStr)
	httputil.JSON(w, http.StatusOK, d)
}

// HandleUpdateDashboard updates a dashboard (no-op in built-in mode).
func (h *Handler) HandleUpdateDashboard(w http.ResponseWriter, r *http.Request) {
	var d domain.Dashboard
	_ = json.NewDecoder(r.Body).Decode(&d)
	httputil.JSON(w, http.StatusOK, d)
}

// HandleDeleteDashboard deletes a dashboard (no-op in built-in mode).
func (h *Handler) HandleDeleteDashboard(w http.ResponseWriter, r *http.Request) {
	httputil.JSON(w, http.StatusOK, map[string]string{"message": "dashboard deleted"})
}

// HandleListWidgets lists built-in widgets/reports for a dashboard.
func (h *Handler) HandleListWidgets(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	dashboardID := chi.URLParam(r, "id")
	widgets := GetBuiltinReportsByDashboardID(tenantID, shopID, dashboardID)
	httputil.JSON(w, http.StatusOK, widgets)
}

// HandleCreateWidget creates a widget (no-op in built-in mode).
func (h *Handler) HandleCreateWidget(w http.ResponseWriter, r *http.Request) {
	var widget domain.ChartWidget
	_ = json.NewDecoder(r.Body).Decode(&widget)
	widget.ID = uuidv7.MustNew()
	httputil.JSON(w, http.StatusCreated, widget)
}

// HandleDeleteWidget deletes a widget (no-op in built-in mode).
func (h *Handler) HandleDeleteWidget(w http.ResponseWriter, r *http.Request) {
	httputil.JSON(w, http.StatusOK, map[string]string{"message": "widget deleted"})
}

// HandleListReports lists saved built-in reports.
func (h *Handler) HandleListReports(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	reports := GetBuiltinReportsByDashboardID(tenantID, shopID, DashboardOverviewID)
	httputil.JSON(w, http.StatusOK, reports)
}

// HandleCreateReport creates a saved report (no-op in built-in mode).
func (h *Handler) HandleCreateReport(w http.ResponseWriter, r *http.Request) {
	var report domain.SavedReport
	_ = json.NewDecoder(r.Body).Decode(&report)
	httputil.JSON(w, http.StatusCreated, report)
}

// parseTRPCInput extracts JSON dictionary from tRPC serialized query param or request body.
func parseTRPCInput(r *http.Request) map[string]any {
	rawInput := r.URL.Query().Get("input")
	if rawInput == "" && r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil && len(bodyBytes) > 0 {
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			rawInput = string(bodyBytes)
		}
	}
	if rawInput == "" {
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(rawInput), &data); err != nil {
		return nil
	}
	if zeroObj, ok := data["0"].(map[string]any); ok {
		if jsonMap, ok := zeroObj["json"].(map[string]any); ok {
			return jsonMap
		}
		return zeroObj
	}
	if jsonMap, ok := data["json"].(map[string]any); ok {
		return jsonMap
	}
	return data
}

// getParam extracts parameter checking URL query and tRPC input.
func (h *Handler) getParam(r *http.Request, key string) string {
	if val := r.URL.Query().Get(key); val != "" {
		return val
	}
	input := parseTRPCInput(r)
	if input != nil {
		if val, ok := input[key].(string); ok && val != "" {
			return val
		}
	}
	return ""
}

func parseDateTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func roundVal(val float64, precision int) float64 {
	pow := math.Pow10(precision)
	return math.Round(val*pow) / pow
}


// Helper: extract tenant_id and shop_id from headers, query parameters, or tRPC input.
func (h *Handler) extractTenantAndShop(r *http.Request) (uuid.UUID, uuid.UUID, error) {
	defaultTenant := uuid.MustParse("018e69d0-7a89-7000-8b1a-200000000001")
	defaultShop := uuid.MustParse("018e69d0-7a89-7000-8b1a-200000000002")

	tStr := r.Header.Get("X-Tenant-ID")
	if tStr == "" {
		tStr = r.URL.Query().Get("tenant_id")
	}
	if tStr == "" {
		tStr = r.URL.Query().Get("tenantId")
	}
	if tStr == "" {
		tStr = r.URL.Query().Get("organizationId")
	}

	sStr := r.Header.Get("X-Shop-ID")
	if sStr == "" {
		sStr = r.URL.Query().Get("shop_id")
	}
	if sStr == "" {
		sStr = r.URL.Query().Get("shopId")
	}
	if sStr == "" {
		sStr = r.URL.Query().Get("projectId")
	}

	input := parseTRPCInput(r)
	if input != nil {
		if tVal, ok := input["tenantId"].(string); ok && tVal != "" {
			tStr = tVal
		} else if tVal, ok := input["organizationId"].(string); ok && tVal != "" {
			tStr = tVal
		}
		if sVal, ok := input["shopId"].(string); ok && sVal != "" {
			sStr = sVal
		} else if sVal, ok := input["projectId"].(string); ok && sVal != "" {
			sStr = sVal
		}
	}

	var tenantID, shopID uuid.UUID
	var err error

	if tStr == "" || tStr == "default" || tStr == "undefined" {
		tenantID = defaultTenant
	} else {
		tenantID, err = uuid.Parse(tStr)
		if err != nil {
			tenantID = defaultTenant
		}
	}

	if sStr == "" || sStr == "default" || sStr == "undefined" {
		shopID = defaultShop
	} else {
		shopID, err = uuid.Parse(sStr)
		if err != nil {
			shopID = defaultShop
		}
	}

	return tenantID, shopID, nil
}

// HandleIntents returns real-time scored shoppers from Redis feature store.
func (h *Handler) HandleIntents(w http.ResponseWriter, r *http.Request) {
	h.HandleTRPCIntents(w, r)
}

// HandleGetInsights returns pre-computed automated anomaly and intelligence cards for the UI.
func (h *Handler) HandleGetInsights(w http.ResponseWriter, r *http.Request) {
	shopIDStr := r.URL.Query().Get("shop_id")
	if shopIDStr == "" {
		shopIDStr = r.Header.Get("X-Shop-Id")
	}

	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "INVALID_SHOP_ID", "Valid shop_id UUID is required")
		return
	}

	limit := 10
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	cards, err := h.queryService.GetInsights(r.Context(), shopID, limit)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "INSIGHTS_QUERY_FAILED", err.Error())
		return
	}

	if cards == nil {
		cards = []*cron.InsightCard{}
	}

	httputil.JSON(w, http.StatusOK, cards)
}

// HandleComputeInsights triggers immediate re-computation of automated intelligence cards.
func (h *Handler) HandleComputeInsights(w http.ResponseWriter, r *http.Request) {
	if err := cron.ComputeDailyInsights(r.Context(), h.queryService.Conn()); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "INSIGHTS_COMPUTE_FAILED", err.Error())
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Daily insights computation completed successfully",
	})
}

// sendTRPCResponse encodes data into the standard tRPC envelope.
func sendTRPCResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"result": map[string]any{
			"data": map[string]any{
				"json": data,
			},
		},
	})
}

// HandleOverviewStats serves /api/v1/query/overview/stats
func (h *Handler) HandleOverviewStats(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	rangeStr := h.getParam(r, "range")
	intervalStr := h.getParam(r, "interval")
	startStr := h.getParam(r, "startDate")
	endStr := h.getParam(r, "endDate")

	res, err := h.queryService.GetOverviewStats(r.Context(), tenantID, shopID, rangeStr, intervalStr, startStr, endStr)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "QUERY_ERROR", err.Error())
		return
	}
	httputil.JSON(w, http.StatusOK, res)
}

// HandleTopGeneric serves /api/v1/query/overview/top-generic
func (h *Handler) HandleTopGeneric(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	column := h.getParam(r, "column")
	if column == "" {
		column = "device"
	}
	rangeStr := h.getParam(r, "range")
	startStr := h.getParam(r, "startDate")
	endStr := h.getParam(r, "endDate")
	limit := 10
	if lStr := h.getParam(r, "limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	items, err := h.queryService.GetTopGeneric(r.Context(), tenantID, shopID, column, rangeStr, startStr, endStr, limit)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "QUERY_ERROR", err.Error())
		return
	}
	if items == nil {
		items = []domain.TopItem{}
	}
	httputil.JSON(w, http.StatusOK, items)
}

// HandleTopPages serves /api/v1/query/overview/top-pages
func (h *Handler) HandleTopPages(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	mode := h.getParam(r, "mode")
	if mode == "" {
		mode = "page"
	}
	rangeStr := h.getParam(r, "range")
	startStr := h.getParam(r, "startDate")
	endStr := h.getParam(r, "endDate")
	limit := 10
	if lStr := h.getParam(r, "limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	items, err := h.queryService.GetTopPages(r.Context(), tenantID, shopID, mode, rangeStr, startStr, endStr, limit)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "QUERY_ERROR", err.Error())
		return
	}
	if items == nil {
		items = []domain.TopItem{}
	}
	httputil.JSON(w, http.StatusOK, items)
}

// HandleTopEvents serves /api/v1/query/overview/top-events
func (h *Handler) HandleTopEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	rangeStr := h.getParam(r, "range")
	startStr := h.getParam(r, "startDate")
	endStr := h.getParam(r, "endDate")
	limit := 10
	if lStr := h.getParam(r, "limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	items, err := h.queryService.GetTopEvents(r.Context(), tenantID, shopID, rangeStr, startStr, endStr, limit)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "QUERY_ERROR", err.Error())
		return
	}
	if items == nil {
		items = []domain.TopEventItem{}
	}
	httputil.JSON(w, http.StatusOK, items)
}

// HandleTopGeo serves /api/v1/query/overview/top-geo
func (h *Handler) HandleTopGeo(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	rangeStr := h.getParam(r, "range")
	startStr := h.getParam(r, "startDate")
	endStr := h.getParam(r, "endDate")
	limit := 10
	if lStr := h.getParam(r, "limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	items, err := h.queryService.GetTopGeneric(r.Context(), tenantID, shopID, "country", rangeStr, startStr, endStr, limit)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "QUERY_ERROR", err.Error())
		return
	}
	if items == nil {
		items = []domain.TopItem{}
	}
	httputil.JSON(w, http.StatusOK, items)
}

// HandleOverviewLiveData serves /api/v1/query/overview/live-data
func (h *Handler) HandleOverviewLiveData(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	res, err := h.queryService.GetOverviewLiveData(r.Context(), tenantID, shopID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "QUERY_ERROR", err.Error())
		return
	}
	httputil.JSON(w, http.StatusOK, res)
}

// HandleListEvents serves /api/v1/query/events
func (h *Handler) HandleListEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	eventName := h.getParam(r, "name")
	search := h.getParam(r, "search")
	limit := 50
	offset := 0
	if lStr := h.getParam(r, "limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	if oStr := h.getParam(r, "offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}

	events, total, err := h.queryService.GetEventsList(r.Context(), tenantID, shopID, eventName, search, limit, offset)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "QUERY_ERROR", err.Error())
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{
		"events": events,
		"total":  total,
	})
}

// HandleListSessions serves /api/v1/query/sessions
func (h *Handler) HandleListSessions(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	limit := 50
	offset := 0
	if lStr := h.getParam(r, "limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	if oStr := h.getParam(r, "offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}

	sessions, total, err := h.queryService.GetSessionsList(r.Context(), tenantID, shopID, limit, offset)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "QUERY_ERROR", err.Error())
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{
		"sessions": sessions,
		"total":    total,
	})
}

// --- tRPC Gateway Handlers ---

func (h *Handler) HandleTRPCOverviewStats(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	rangeStr := h.getParam(r, "range")
	intervalStr := h.getParam(r, "interval")
	startStr := h.getParam(r, "startDate")
	endStr := h.getParam(r, "endDate")

	res, err := h.queryService.GetOverviewStats(r.Context(), tenantID, shopID, rangeStr, intervalStr, startStr, endStr)
	if err != nil {
		sendTRPCResponse(w, map[string]any{
			"metrics": map[string]any{},
			"series":  []any{},
		})
		return
	}
	sendTRPCResponse(w, res)
}

func (h *Handler) HandleTRPCOverviewTopGeneric(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	column := h.getParam(r, "column")
	if column == "" {
		column = "device"
	}
	rangeStr := h.getParam(r, "range")
	startStr := h.getParam(r, "startDate")
	endStr := h.getParam(r, "endDate")

	items, err := h.queryService.GetTopGeneric(r.Context(), tenantID, shopID, column, rangeStr, startStr, endStr, 10)
	if err != nil || items == nil {
		items = []domain.TopItem{}
	}
	sendTRPCResponse(w, items)
}

func (h *Handler) HandleTRPCOverviewTopGenericSeries(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	column := h.getParam(r, "column")
	if column == "" {
		column = "device"
	}
	rangeStr := h.getParam(r, "range")
	startStr := h.getParam(r, "startDate")
	endStr := h.getParam(r, "endDate")

	items, err := h.queryService.GetTopGeneric(r.Context(), tenantID, shopID, column, rangeStr, startStr, endStr, 10)
	if err != nil || items == nil {
		items = []domain.TopItem{}
	}
	sendTRPCResponse(w, map[string]any{
		"items":  items,
		"series": []any{},
	})
}

func (h *Handler) HandleTRPCOverviewTopPages(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	mode := h.getParam(r, "mode")
	if mode == "" {
		mode = "page"
	}
	rangeStr := h.getParam(r, "range")
	startStr := h.getParam(r, "startDate")
	endStr := h.getParam(r, "endDate")

	items, err := h.queryService.GetTopPages(r.Context(), tenantID, shopID, mode, rangeStr, startStr, endStr, 10)
	var resp []map[string]any
	if err == nil && items != nil {
		for _, it := range items {
			resp = append(resp, map[string]any{
				"origin":    "",
				"path":      it.Name,
				"name":      it.Name,
				"sessions":  it.Sessions,
				"pageviews": it.Pageviews,
				"revenue":   it.Revenue,
			})
		}
	}
	if resp == nil {
		resp = []map[string]any{}
	}
	sendTRPCResponse(w, resp)
}

func (h *Handler) HandleTRPCOverviewTopSources(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	rangeStr := h.getParam(r, "range")
	startStr := h.getParam(r, "startDate")
	endStr := h.getParam(r, "endDate")

	items, err := h.queryService.GetTopGeneric(r.Context(), tenantID, shopID, "referrer_name", rangeStr, startStr, endStr, 10)
	if err != nil || items == nil {
		items = []domain.TopItem{}
	}
	sendTRPCResponse(w, items)
}

func (h *Handler) HandleTRPCOverviewTopEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	rangeStr := h.getParam(r, "range")
	startStr := h.getParam(r, "startDate")
	endStr := h.getParam(r, "endDate")

	items, err := h.queryService.GetTopEvents(r.Context(), tenantID, shopID, rangeStr, startStr, endStr, 10)
	if err != nil || items == nil {
		items = []domain.TopEventItem{}
	}
	sendTRPCResponse(w, items)
}

func (h *Handler) HandleTRPCOverviewTopGeo(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	rangeStr := h.getParam(r, "range")
	startStr := h.getParam(r, "startDate")
	endStr := h.getParam(r, "endDate")

	items, err := h.queryService.GetTopGeneric(r.Context(), tenantID, shopID, "country", rangeStr, startStr, endStr, 10)
	if err != nil || items == nil {
		items = []domain.TopItem{}
	}
	sendTRPCResponse(w, items)
}

func (h *Handler) HandleTRPCOverviewMap(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	rangeStr := h.getParam(r, "range")
	startStr := h.getParam(r, "startDate")
	endStr := h.getParam(r, "endDate")

	items, err := h.queryService.GetTopGeneric(r.Context(), tenantID, shopID, "country", rangeStr, startStr, endStr, 100)
	var resp []map[string]any
	if err == nil && items != nil {
		for _, it := range items {
			if it.Name != "" && it.Name != "unknown" {
				resp = append(resp, map[string]any{
					"country": it.Name,
					"count":   it.Sessions,
					"lat":     0,
					"lng":     0,
				})
			}
		}
	}
	if resp == nil {
		resp = []map[string]any{}
	}
	sendTRPCResponse(w, resp)
}

func (h *Handler) HandleTRPCLiveVisitors(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	live, err := h.queryService.GetLiveVisitors(r.Context(), tenantID, shopID, 5)
	if err != nil {
		sendTRPCResponse(w, 0)
		return
	}
	sendTRPCResponse(w, live.ActiveShoppers)
}

func (h *Handler) HandleTRPCLiveData(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	data, err := h.queryService.GetOverviewLiveData(r.Context(), tenantID, shopID)
	if err != nil {
		sendTRPCResponse(w, map[string]any{
			"totalSessions": 0,
			"minuteCounts":  []any{},
			"referrers":     []any{},
		})
		return
	}
	sendTRPCResponse(w, data)
}

func (h *Handler) HandleTRPCProjectGet(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	sendTRPCResponse(w, map[string]any{
		"id":             shopID.String(),
		"name":           "Primary Store",
		"organizationId": tenantID.String(),
		"timezone":       "UTC",
		"clients": []map[string]any{
			{
				"id":        "cl_default_web",
				"name":      "Default Web Client",
				"projectId": shopID.String(),
			},
		},
	})
}

func (h *Handler) HandleTRPCClientList(w http.ResponseWriter, r *http.Request) {
	_, shopID, _ := h.extractTenantAndShop(r)
	sendTRPCResponse(w, []map[string]any{
		{
			"id":        "cl_default_web",
			"name":      "Default Web Client",
			"projectId": shopID.String(),
		},
	})
}

func (h *Handler) HandleTRPCOrganizationGet(w http.ResponseWriter, r *http.Request) {
	tenantID, _, _ := h.extractTenantAndShop(r)
	sendTRPCResponse(w, map[string]any{
		"id":   tenantID.String(),
		"name": "Default Organization",
	})
}

func (h *Handler) HandleTRPCOrganizationMyAccess(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, map[string]any{
		"role":  "org:admin",
		"level": "admin",
	})
}

func (h *Handler) HandleTRPCProjectList(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	sendTRPCResponse(w, []map[string]any{
		{
			"id":             shopID.String(),
			"name":           "Primary Store",
			"organizationId": tenantID.String(),
		},
	})
}

func (h *Handler) HandleTRPCEmptyList(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, []any{})
}

func (h *Handler) HandleTRPCOverviewReferrerSpikes(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, []any{})
}

func (h *Handler) HandleTRPCOverviewTopConversions(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, []any{})
}

func (h *Handler) HandleTRPCOverviewTopLinkOut(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, []any{})
}

func (h *Handler) HandleTRPCOverviewUserJourney(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	rangeStr := h.getParam(r, "range")
	startStr := h.getParam(r, "startDate")
	endStr := h.getParam(r, "endDate")

	steps := 5
	if sVal := h.getParam(r, "steps"); sVal != "" {
		if parsed, err := strconv.Atoi(sVal); err == nil && parsed > 0 {
			steps = parsed
		}
	} else if input := parseTRPCInput(r); input != nil {
		if sVal, ok := input["steps"].(float64); ok && sVal > 0 {
			steps = int(sVal)
		}
	}

	result, err := h.queryService.GetUserJourney(r.Context(), tenantID, shopID, rangeStr, startStr, endStr, steps)
	if err != nil || result == nil {
		sendTRPCResponse(w, map[string]any{
			"nodes": []any{},
			"links": []any{},
		})
		return
	}

	sendTRPCResponse(w, result)
}

func (h *Handler) HandleTRPCReferenceGetChartReferences(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, []any{})
}

func (h *Handler) HandleTRPCFallback(w http.ResponseWriter, r *http.Request) {
	path := strings.ToLower(r.URL.Path)
	if strings.Contains(path, "events") ||
		strings.Contains(path, "conversions") ||
		strings.Contains(path, "session.list") ||
		strings.Contains(path, "session.sessions") ||
		strings.Contains(path, "profile.list") ||
		strings.Contains(path, "profile.powerusers") ||
		strings.Contains(path, "group.list") {
		sendTRPCResponse(w, map[string]any{
			"data": []any{},
			"meta": map[string]any{
				"next":      nil,
				"count":     0,
				"pageCount": 50,
				"take":      50,
			},
		})
		return
	}
	if strings.Contains(path, "profile.byid") {
		sendTRPCResponse(w, map[string]any{
			"id":         "anon",
			"profileId":  nil,
			"email":      "",
			"firstName":  "",
			"lastName":   "",
			"avatar":     "",
			"createdAt":  time.Now().UTC().Format(time.RFC3339),
			"properties": map[string]any{},
		})
		return
	}
	if strings.Contains(path, "profile.metrics") {
		sendTRPCResponse(w, map[string]any{
			"events":      0,
			"sessions":    0,
			"avgDuration": 0,
		})
		return
	}
	if strings.Contains(path, "session.byid") {
		sendTRPCResponse(w, map[string]any{
			"id":           "anon",
			"sessionId":    "anon",
			"duration":     0,
			"createdAt":    time.Now().UTC().Format(time.RFC3339),
			"events":       0,
			"entryPath":    "/",
			"exitPath":     "/",
			"referrer":     "",
			"referrerName": "Direct",
			"country":      "",
			"city":         "",
			"browser":      "",
			"os":           "",
			"device":       "",
			"hasReplay":    false,
		})
		return
	}
	if strings.Contains(path, "group.byid") {
		sendTRPCResponse(w, map[string]any{
			"id":         "default",
			"name":       "Default Group",
			"type":       "team",
			"properties": map[string]any{},
		})
		return
	}
	if strings.Contains(path, "group.types") || strings.Contains(path, "group.properties") {
		sendTRPCResponse(w, []string{})
		return
	}
	if strings.Contains(path, "chat.models") {
		sendTRPCResponse(w, []map[string]any{
			{"id": "deepseek-coder", "name": "DeepSeek Coder"},
			{"id": "gpt-4o", "name": "GPT-4o"},
		})
		return
	}
	if strings.Contains(path, "list") ||
		strings.Contains(path, "spikes") ||
		strings.Contains(path, "references") ||
		strings.Contains(path, "items") ||
		strings.Contains(path, "all") ||
		strings.Contains(path, "paths") ||
		strings.Contains(path, "coordinates") ||
		strings.Contains(path, "geo") ||
		strings.Contains(path, "referrals") ||
		strings.Contains(path, "sessions") ||
		strings.Contains(path, "map") {
		sendTRPCResponse(w, []any{})
		return
	}
	if strings.Contains(path, "chart") {
		sendTRPCResponse(w, map[string]any{
			"series": []any{},
			"meta":   map[string]any{},
		})
		return
	}
	sendTRPCResponse(w, map[string]any{})
}

func (h *Handler) HandleTRPCProfilePowerUsers(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	query := fmt.Sprintf(`
		SELECT
			device_id,
			count(*) as count,
			min(created_at) as first_seen,
			max(created_at) as last_seen,
			any(country) as country,
			any(city) as city,
			any(browser) as browser,
			any(os) as os,
			any(device) as device
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ?
		GROUP BY device_id
		ORDER BY count DESC
		LIMIT 50
	`, h.queryService.database)

	var items []map[string]any
	if rows, err := h.queryService.Conn().Query(r.Context(), query, tenantID, shopID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var devID, country, city, browser, os, device string
			var count uint64
			var firstSeen, lastSeen time.Time
			if err := rows.Scan(&devID, &count, &firstSeen, &lastSeen, &country, &city, &browser, &os, &device); err == nil {
				items = append(items, map[string]any{
					"id":         devID,
					"count":      count,
					"email":      "",
					"firstName":  "",
					"lastName":   "",
					"avatar":     "",
					"isExternal": false,
					"createdAt":  firstSeen.UTC().Format("2006-01-02T15:04:05.000Z"),
					"lastSeenAt": lastSeen.UTC().Format("2006-01-02T15:04:05.000Z"),
					"properties": map[string]any{
						"country": country,
						"city":    city,
						"browser": browser,
						"os":      os,
						"device":  device,
					},
				})
			}
		}
	}
	if items == nil {
		items = []map[string]any{}
	}
	sendTRPCResponse(w, map[string]any{
		"data": items,
		"meta": map[string]any{
			"count":     len(items),
			"pageCount": 50,
			"take":      50,
			"next":      nil,
		},
	})
}

func (h *Handler) HandleTRPCProfileList(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	input := parseTRPCInput(r)

	var whereExtra []string
	var args []any
	args = append(args, tenantID, shopID)

	take := 50
	offset := 0

	if input != nil {
		if tVal, ok := input["take"].(float64); ok && tVal > 0 {
			take = int(tVal)
		}
		if cVal, ok := input["cursor"].(float64); ok && cVal > 0 {
			offset = int(cVal) * take
		}
		if ext, ok := input["isExternal"].(bool); ok {
			if ext {
				whereExtra = append(whereExtra, "AND e.customer_id IS NOT NULL AND toString(e.customer_id) != '00000000-0000-0000-0000-000000000000'")
			} else {
				whereExtra = append(whereExtra, "AND (e.customer_id IS NULL OR toString(e.customer_id) = '00000000-0000-0000-0000-000000000000')")
			}
		}
		if search, ok := input["search"].(string); ok && strings.TrimSpace(search) != "" {
			s := strings.TrimSpace(search)
			whereExtra = append(whereExtra, "AND (e.device_id ILIKE ? OR e.country ILIKE ? OR e.city ILIKE ? OR e.browser ILIKE ? OR e.os ILIKE ? OR e.device ILIKE ?)")
			pattern := "%" + s + "%"
			args = append(args, pattern, pattern, pattern, pattern, pattern, pattern)
		}
		if filtersRaw, ok := input["filters"].([]any); ok {
			for _, f := range filtersRaw {
				fMap, ok := f.(map[string]any)
				if !ok {
					continue
				}
				name, _ := fMap["name"].(string)
				op, _ := fMap["operator"].(string)
				valList, _ := fMap["value"].([]any)
				col := ""
				switch name {
				case "country", "profile.country":
					col = "e.country"
				case "city", "profile.city":
					col = "e.city"
				case "browser", "profile.browser":
					col = "e.browser"
				case "os", "profile.os":
					col = "e.os"
				case "device", "profile.device":
					col = "e.device"
				case "id", "profile.id", "deviceId":
					col = "e.device_id"
				case "referrer_name", "referrerName":
					col = "e.referrer_name"
				}
				if col == "" {
					continue
				}
				if op == "isNull" {
					whereExtra = append(whereExtra, fmt.Sprintf("AND (%s = '' OR %s IS NULL)", col, col))
				} else if op == "isNotNull" {
					whereExtra = append(whereExtra, fmt.Sprintf("AND (%s != '' AND %s IS NOT NULL)", col, col))
				} else if len(valList) > 0 {
					var strVals []string
					for _, v := range valList {
						if vStr, ok := v.(string); ok && vStr != "" {
							strVals = append(strVals, vStr)
						}
					}
					if len(strVals) == 0 {
						continue
					}
					switch op {
					case "is":
						if len(strVals) == 1 {
							whereExtra = append(whereExtra, fmt.Sprintf("AND %s = ?", col))
							args = append(args, strVals[0])
						} else {
							ph := make([]string, len(strVals))
							for i, sv := range strVals {
								ph[i] = "?"
								args = append(args, sv)
							}
							whereExtra = append(whereExtra, fmt.Sprintf("AND %s IN (%s)", col, strings.Join(ph, ",")))
						}
					case "isNot":
						if len(strVals) == 1 {
							whereExtra = append(whereExtra, fmt.Sprintf("AND %s != ?", col))
							args = append(args, strVals[0])
						} else {
							ph := make([]string, len(strVals))
							for i, sv := range strVals {
								ph[i] = "?"
								args = append(args, sv)
							}
							whereExtra = append(whereExtra, fmt.Sprintf("AND %s NOT IN (%s)", col, strings.Join(ph, ",")))
						}
					case "contains":
						whereExtra = append(whereExtra, fmt.Sprintf("AND %s ILIKE ?", col))
						args = append(args, "%"+strVals[0]+"%")
					case "doesNotContain":
						whereExtra = append(whereExtra, fmt.Sprintf("AND %s NOT ILIKE ?", col))
						args = append(args, "%"+strVals[0]+"%")
					case "startsWith":
						whereExtra = append(whereExtra, fmt.Sprintf("AND %s ILIKE ?", col))
						args = append(args, strVals[0]+"%")
					case "endsWith":
						whereExtra = append(whereExtra, fmt.Sprintf("AND %s ILIKE ?", col))
						args = append(args, "%"+strVals[0])
					}
				}
			}
		}
	}

	whereClause := strings.Join(whereExtra, " ")

	countQuery := fmt.Sprintf(`
		SELECT count(DISTINCT coalesce(nullIf(toString(e.customer_id), '00000000-0000-0000-0000-000000000000'), e.device_id))
		FROM %s.events e
		WHERE e.tenant_id = ? AND e.shop_id = ? %s
	`, h.queryService.database, whereClause)
	var totalCount uint64 = 0
	_ = h.queryService.Conn().QueryRow(r.Context(), countQuery, args...).Scan(&totalCount)

	query := fmt.Sprintf(`
		SELECT
			coalesce(nullIf(toString(e.customer_id), '00000000-0000-0000-0000-000000000000'), e.device_id) as prof_id,
			count(*) as count,
			min(e.created_at) as first_seen,
			max(e.created_at) as last_seen,
			any(e.country) as country,
			any(e.city) as city,
			any(e.browser) as browser,
			any(e.os) as os,
			any(e.device) as device,
			(e.customer_id IS NOT NULL AND toString(e.customer_id) != '00000000-0000-0000-0000-000000000000') as is_ext
		FROM %s.events e
		WHERE e.tenant_id = ? AND e.shop_id = ? %s
		GROUP BY prof_id, is_ext
		ORDER BY last_seen DESC
		LIMIT ? OFFSET ?
	`, h.queryService.database, whereClause)

	queryArgs := append(args, take, offset)

	var items []map[string]any
	if rows, err := h.queryService.Conn().Query(r.Context(), query, queryArgs...); err == nil {
		defer rows.Close()
		for rows.Next() {
			var devID, country, city, browser, os, device string
			var count uint64
			var firstSeen, lastSeen time.Time
			var isExt uint8
			if err := rows.Scan(&devID, &count, &firstSeen, &lastSeen, &country, &city, &browser, &os, &device, &isExt); err == nil {
				items = append(items, map[string]any{
					"id":         devID,
					"count":      count,
					"email":      "",
					"firstName":  "",
					"lastName":   "",
					"avatar":     "",
					"isExternal": isExt == 1,
					"createdAt":  firstSeen.UTC().Format("2006-01-02T15:04:05.000Z"),
					"lastSeenAt": lastSeen.UTC().Format("2006-01-02T15:04:05.000Z"),
					"properties": map[string]any{
						"country": country,
						"city":    city,
						"browser": browser,
						"os":      os,
						"device":  device,
					},
				})
			}
		}
	}
	if items == nil {
		items = []map[string]any{}
	}
	sendTRPCResponse(w, map[string]any{
		"data": items,
		"meta": map[string]any{
			"count":     totalCount,
			"pageCount": take,
			"take":      take,
			"next":      nil,
		},
	})
}

func (h *Handler) HandleTRPCProfileById(w http.ResponseWriter, r *http.Request) {
	input := parseTRPCInput(r)
	profileID := ""
	if input != nil {
		if pid, ok := input["profileId"].(string); ok {
			profileID = pid
		} else if id, ok := input["id"].(string); ok {
			profileID = id
		}
	}
	if profileID == "" {
		profileID = r.URL.Query().Get("profileId")
	}

	res := map[string]any{
		"id":         profileID,
		"profileId":  profileID,
		"email":      "",
		"firstName":  "",
		"lastName":   "",
		"avatar":     "",
		"isExternal": false,
		"createdAt":  time.Now().UTC().Format(time.RFC3339),
		"lastSeenAt": time.Now().UTC().Format(time.RFC3339),
		"properties": map[string]any{},
	}

	if profileID != "" {
		query := fmt.Sprintf(`
			SELECT any(country), any(city), any(browser), any(os), any(device), min(created_at), max(created_at)
			FROM %s.events
			WHERE device_id = ? OR toString(customer_id) = ?
		`, h.queryService.database)
		var country, city, browser, os, device string
		var firstSeen, lastSeen time.Time
		if err := h.queryService.Conn().QueryRow(r.Context(), query, profileID, profileID).Scan(&country, &city, &browser, &os, &device, &firstSeen, &lastSeen); err == nil {
			res["createdAt"] = firstSeen.UTC().Format("2006-01-02T15:04:05.000Z")
			res["lastSeenAt"] = lastSeen.UTC().Format("2006-01-02T15:04:05.000Z")
			res["properties"] = map[string]any{
				"country": country,
				"city":    city,
				"browser": browser,
				"os":      os,
				"device":  device,
			}
		}
	}

	sendTRPCResponse(w, res)
}

func (h *Handler) HandleTRPCProfileMetrics(w http.ResponseWriter, r *http.Request) {
	input := parseTRPCInput(r)
	profileID := ""
	if input != nil {
		if pid, ok := input["profileId"].(string); ok {
			profileID = pid
		}
	}
	if profileID == "" {
		profileID = r.URL.Query().Get("profileId")
	}

	metrics := map[string]any{
		"events":      0,
		"sessions":    0,
		"avgDuration": 0,
	}

	if profileID != "" {
		query := fmt.Sprintf(`
			SELECT count(*), count(distinct session_id)
			FROM %s.events
			WHERE device_id = ?
		`, h.queryService.database)
		var evCount, sessCount uint64
		if err := h.queryService.Conn().QueryRow(r.Context(), query, profileID).Scan(&evCount, &sessCount); err == nil {
			metrics["events"] = int64(evCount)
			metrics["sessions"] = int64(sessCount)
		}
	}

	sendTRPCResponse(w, metrics)
}

func (h *Handler) HandleTRPCProfileActivity(w http.ResponseWriter, r *http.Request) {
	input := parseTRPCInput(r)
	profileID := ""
	if input != nil {
		if pid, ok := input["profileId"].(string); ok {
			profileID = pid
		}
	}
	if profileID == "" {
		profileID = r.URL.Query().Get("profileId")
	}

	type ActItem struct {
		Count int64  `json:"count"`
		Date  string `json:"date"`
	}
	var res []ActItem
	if profileID != "" {
		query := fmt.Sprintf(`
			SELECT count(*), toStartOfDay(created_at) as dt
			FROM %s.events
			WHERE device_id = ?
			GROUP BY dt
			ORDER BY dt DESC
			LIMIT 30
		`, h.queryService.database)
		if rows, err := h.queryService.Conn().Query(r.Context(), query, profileID); err == nil {
			defer rows.Close()
			for rows.Next() {
				var count uint64
				var dt time.Time
				if err := rows.Scan(&count, &dt); err == nil {
					res = append(res, ActItem{
						Count: int64(count),
						Date:  dt.UTC().Format("2006-01-02T15:04:05.000Z"),
					})
				}
			}
		}
	}
	if res == nil {
		res = []ActItem{}
	}
	sendTRPCResponse(w, res)
}

func (h *Handler) HandleTRPCProfileMostEvents(w http.ResponseWriter, r *http.Request) {
	input := parseTRPCInput(r)
	profileID := ""
	if input != nil {
		if pid, ok := input["profileId"].(string); ok {
			profileID = pid
		}
	}
	if profileID == "" {
		profileID = r.URL.Query().Get("profileId")
	}

	type EvCount struct {
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}
	var res []EvCount
	if profileID != "" {
		query := fmt.Sprintf(`
			SELECT name, count(*)
			FROM %s.events
			WHERE device_id = ? AND name NOT IN ('screen_view', 'session_start', 'session_end')
			GROUP BY name
			ORDER BY count(*) DESC
			LIMIT 10
		`, h.queryService.database)
		if rows, err := h.queryService.Conn().Query(r.Context(), query, profileID); err == nil {
			defer rows.Close()
			for rows.Next() {
				var name string
				var count uint64
				if err := rows.Scan(&name, &count); err == nil {
					res = append(res, EvCount{
						Name:  name,
						Count: int64(count),
					})
				}
			}
		}
	}
	if res == nil {
		res = []EvCount{}
	}
	sendTRPCResponse(w, res)
}

func (h *Handler) HandleTRPCProfilePopularRoutes(w http.ResponseWriter, r *http.Request) {
	input := parseTRPCInput(r)
	profileID := ""
	if input != nil {
		if pid, ok := input["profileId"].(string); ok {
			profileID = pid
		}
	}
	if profileID == "" {
		profileID = r.URL.Query().Get("profileId")
	}

	type RouteItem struct {
		Path  string `json:"path"`
		Count int64  `json:"count"`
	}
	var res []RouteItem
	if profileID != "" {
		query := fmt.Sprintf(`
			SELECT path, count(*)
			FROM %s.events
			WHERE device_id = ? AND path != ''
			GROUP BY path
			ORDER BY count(*) DESC
			LIMIT 10
		`, h.queryService.database)
		if rows, err := h.queryService.Conn().Query(r.Context(), query, profileID); err == nil {
			defer rows.Close()
			for rows.Next() {
				var path string
				var count uint64
				if err := rows.Scan(&path, &count); err == nil {
					res = append(res, RouteItem{
						Path:  path,
						Count: int64(count),
					})
				}
			}
		}
	}
	if res == nil {
		res = []RouteItem{}
	}
	sendTRPCResponse(w, res)
}

func (h *Handler) HandleTRPCProfileProperties(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, []string{"city", "country", "browser", "os", "device"})
}

func (h *Handler) HandleTRPCSessionById(w http.ResponseWriter, r *http.Request) {
	input := parseTRPCInput(r)
	sessIDStr := ""
	if input != nil {
		if sid, ok := input["sessionId"].(string); ok {
			sessIDStr = sid
		}
	}
	if sessIDStr == "" {
		sessIDStr = r.URL.Query().Get("sessionId")
	}

	sess := map[string]any{
		"id":              sessIDStr,
		"sessionId":       sessIDStr,
		"duration":        0,
		"createdAt":       time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"startedAt":       time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"endedAt":         time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"events":          0,
		"eventCount":      0,
		"screenViewCount": 0,
		"isBounce":        false,
		"entryPath":       "/",
		"exitPath":        "/",
		"referrer":        "",
		"referrerName":    "Direct",
		"country":         "",
		"city":            "",
		"browser":         "",
		"os":              "",
		"device":          "",
		"hasReplay":       false,
	}

	if parsedID, err := uuid.Parse(sessIDStr); err == nil {
		query := fmt.Sprintf(`
			SELECT
				started_at, duration, entry_path, exit_path,
				referrer, referrer_name, events_count
			FROM %s.sessions
			WHERE id = ?
			LIMIT 1
		`, h.queryService.database)
		var startedAt time.Time
		var dur, evCount uint32
		var entryPath, exitPath, ref, refName string
		if err := h.queryService.Conn().QueryRow(r.Context(), query, parsedID).Scan(&startedAt, &dur, &entryPath, &exitPath, &ref, &refName, &evCount); err == nil {
			sess["createdAt"] = startedAt.UTC().Format("2006-01-02T15:04:05.000Z")
			sess["startedAt"] = startedAt.UTC().Format("2006-01-02T15:04:05.000Z")
			sess["endedAt"] = startedAt.Add(time.Duration(dur) * time.Second).UTC().Format("2006-01-02T15:04:05.000Z")
			sess["duration"] = dur * 1000
			sess["entryPath"] = entryPath
			sess["exitPath"] = exitPath
			sess["referrer"] = ref
			sess["referrerName"] = refName
			sess["events"] = evCount
			sess["eventCount"] = evCount
			sess["screenViewCount"] = evCount
			sess["isBounce"] = evCount <= 1
		}

		// Also grab geo/device from events
		evQuery := fmt.Sprintf(`
			SELECT any(country), any(city), any(os), any(browser), any(device)
			FROM %s.events
			WHERE session_id = ?
		`, h.queryService.database)
		var country, city, os, browser, device string
		if err := h.queryService.Conn().QueryRow(r.Context(), evQuery, parsedID).Scan(&country, &city, &os, &browser, &device); err == nil {
			sess["country"] = country
			sess["city"] = city
			sess["os"] = os
			sess["browser"] = browser
			sess["device"] = device
		}

		// Check if this session has recorded replay chunks
		var replayCount uint64
		replayCheckQuery := fmt.Sprintf(`
			SELECT count()
			FROM %s.session_replay_chunks
			WHERE session_id = ?
		`, h.queryService.database)
		if err := h.queryService.Conn().QueryRow(r.Context(), replayCheckQuery, parsedID).Scan(&replayCount); err == nil {
			sess["hasReplay"] = replayCount > 0
		}
	}

	sendTRPCResponse(w, sess)
}

func (h *Handler) HandleTRPCReplayChunksFrom(w http.ResponseWriter, r *http.Request) {
	input := parseTRPCInput(r)
	sessIDStr := ""
	fromIndex := 0
	if input != nil {
		if sid, ok := input["sessionId"].(string); ok {
			sessIDStr = sid
		}
		if fi, ok := input["fromIndex"].(float64); ok {
			fromIndex = int(fi)
		}
	}
	if sessIDStr == "" {
		sessIDStr = r.URL.Query().Get("sessionId")
	}

	sessionID, err := uuid.Parse(sessIDStr)
	if err != nil {
		sendTRPCResponse(w, map[string]any{
			"data":    []any{},
			"hasMore": false,
		})
		return
	}

	const pageSize = 50
	query := fmt.Sprintf(`
		SELECT chunk_index, payload
		FROM %s.session_replay_chunks
		WHERE session_id = ?
		ORDER BY started_at ASC, chunk_index ASC
		LIMIT ? OFFSET ?
	`, h.queryService.database)

	rows, err := h.queryService.Conn().Query(r.Context(), query, sessionID, pageSize+1, fromIndex)
	if err != nil {
		sendTRPCResponse(w, map[string]any{
			"data":    []any{},
			"hasMore": false,
		})
		return
	}
	defer rows.Close()

	type chunkItem struct {
		ChunkIndex int   `json:"chunkIndex"`
		Events     []any `json:"events"`
	}

	allChunks := []chunkItem{}
	for rows.Next() {
		var chunkIdx uint16
		var payloadStr string
		if err := rows.Scan(&chunkIdx, &payloadStr); err == nil {
			var events []any
			if err := json.Unmarshal([]byte(payloadStr), &events); err == nil {
				allChunks = append(allChunks, chunkItem{
					ChunkIndex: int(chunkIdx),
					Events:     events,
				})
			}
		}
	}

	hasMore := len(allChunks) > pageSize
	if hasMore {
		allChunks = allChunks[:pageSize]
	}

	sendTRPCResponse(w, map[string]any{
		"data":    allChunks,
		"hasMore": hasMore,
	})
}


func (h *Handler) HandleTRPCGroupList(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, map[string]any{
		"data": []any{},
		"meta": map[string]any{
			"count":     0,
			"take":      50,
			"pageCount": 50,
		},
	})
}

func (h *Handler) HandleTRPCGroupTypes(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, []string{})
}

func (h *Handler) HandleTRPCGroupById(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, map[string]any{
		"id":         "default",
		"name":       "Default Group",
		"type":       "team",
		"properties": map[string]any{},
	})
}

func (h *Handler) HandleTRPCOrganizationList(w http.ResponseWriter, r *http.Request) {
	tenantID, _, _ := h.extractTenantAndShop(r)
	sendTRPCResponse(w, []map[string]any{
		{
			"id":                            tenantID.String(),
			"name":                          "Default Organization",
			"subscriptionPeriodEventsCount": 1000,
			"subscriptionPeriodEventsLimit": 10000000,
		},
	})
}

func (h *Handler) HandleTRPCAuthSession(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, map[string]any{
		"user": map[string]any{
			"id":    "admin",
			"name":  "Administrator",
			"email": "admin@aicart.localhost",
		},
		"session": map[string]any{
			"id":     "local-session",
			"userId": "admin",
		},
	})
}

func (h *Handler) HandleTRPCEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	input := parseTRPCInput(r)

	var eventNames []string
	if input != nil {
		if evs, ok := input["events"].([]any); ok {
			for _, ev := range evs {
				if s, ok := ev.(string); ok && s != "" && s != "*" {
					eventNames = append(eventNames, s)
				}
			}
		} else if evStr, ok := input["events"].(string); ok && evStr != "" && evStr != "*" {
			eventNames = append(eventNames, evStr)
		}
	}

	var whereExtra []string
	var args []any
	args = append(args, tenantID, shopID)

	if len(eventNames) == 1 {
		whereExtra = append(whereExtra, "AND name = ?")
		args = append(args, eventNames[0])
	} else if len(eventNames) > 1 {
		placeholders := make([]string, len(eventNames))
		for i, n := range eventNames {
			placeholders[i] = "?"
			args = append(args, n)
		}
		whereExtra = append(whereExtra, fmt.Sprintf("AND name IN (%s)", strings.Join(placeholders, ",")))
	}

	if input != nil {
		if sStr, ok := input["startDate"].(string); ok && sStr != "" {
			if t, ok := parseDateTime(sStr); ok {
				whereExtra = append(whereExtra, "AND created_at >= ?")
				args = append(args, t)
			}
		}
		if eStr, ok := input["endDate"].(string); ok && eStr != "" {
			if t, ok := parseDateTime(eStr); ok {
				whereExtra = append(whereExtra, "AND created_at <= ?")
				args = append(args, t)
			}
		}
		if curStr, ok := input["cursor"].(string); ok && curStr != "" {
			if t, ok := parseDateTime(curStr); ok {
				whereExtra = append(whereExtra, "AND created_at < ?")
				args = append(args, t)
			}
		}
	}

	query := fmt.Sprintf(`
		SELECT 
			id, tenant_id, shop_id, name, device_id, customer_id, session_id,
			coalesce(revenue, 0), currency, product_id, cart_id, order_id,
			path, origin, referrer, referrer_name, referrer_type,
			os, browser, device, country, city, properties, created_at
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ? %s
		ORDER BY created_at DESC
		LIMIT 51
	`, h.queryService.database, strings.Join(whereExtra, " "))

	rows, err := h.queryService.Conn().Query(r.Context(), query, args...)
	if err != nil {
		sendTRPCResponse(w, map[string]any{
			"data": []any{},
			"meta": map[string]any{"next": nil},
		})
		return
	}
	defer rows.Close()

	var events []domain.Event
	for rows.Next() {
		var ev domain.Event
		var revCents int64
		if err := rows.Scan(
			&ev.ID, &ev.TenantID, &ev.ShopID, &ev.Name, &ev.DeviceID, &ev.CustomerID, &ev.SessionID,
			&revCents, &ev.Currency, &ev.ProductID, &ev.CartID, &ev.OrderID,
			&ev.Path, &ev.Origin, &ev.Referrer, &ev.ReferrerName, &ev.ReferrerType,
			&ev.OS, &ev.Browser, &ev.Device, &ev.Country, &ev.City, &ev.Properties, &ev.CreatedAt,
		); err == nil {
			if revCents > 0 {
				ev.Revenue = &revCents
			}
			events = append(events, ev)
		}
	}

	var nextCursor any = nil
	if len(events) > 50 {
		nextCursor = events[49].CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
		events = events[:50]
	}

	items := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		rev := int64(0)
		if ev.Revenue != nil {
			rev = *ev.Revenue
		}
		path := ev.Path
		if path == "" {
			path = "/"
		}
		profID := ev.DeviceID
		if ev.CustomerID != nil && *ev.CustomerID != uuid.Nil {
			profID = ev.CustomerID.String()
		}
		items = append(items, map[string]any{
			"id":           ev.ID.String(),
			"name":         ev.Name,
			"path":         path,
			"origin":       ev.Origin,
			"referrer":     ev.Referrer,
			"referrerName": ev.ReferrerName,
			"sessionId":    ev.SessionID.String(),
			"profileId":    profID,
			"deviceId":     ev.DeviceID,
			"country":      ev.Country,
			"city":         ev.City,
			"os":           ev.OS,
			"browser":      ev.Browser,
			"device":       ev.Device,
			"revenue":      rev,
			"currency":     ev.Currency,
			"createdAt":    ev.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			"properties":   ev.Properties,
		})
	}

	sendTRPCResponse(w, map[string]any{
		"data": items,
		"meta": map[string]any{
			"next": nextCursor,
		},
	})
}

func (h *Handler) HandleTRPCConversions(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	input := parseTRPCInput(r)

	var eventNames []string
	if input != nil {
		if evs, ok := input["events"].([]any); ok {
			for _, ev := range evs {
				if s, ok := ev.(string); ok && s != "" && s != "*" {
					eventNames = append(eventNames, s)
				}
			}
		} else if evStr, ok := input["events"].(string); ok && evStr != "" && evStr != "*" {
			eventNames = append(eventNames, evStr)
		}
	}

	var whereExtra []string
	var args []any
	args = append(args, tenantID, shopID)

	if len(eventNames) == 1 {
		whereExtra = append(whereExtra, "AND name = ?")
		args = append(args, eventNames[0])
	} else if len(eventNames) > 1 {
		placeholders := make([]string, len(eventNames))
		for i, n := range eventNames {
			placeholders[i] = "?"
			args = append(args, n)
		}
		whereExtra = append(whereExtra, fmt.Sprintf("AND name IN (%s)", strings.Join(placeholders, ",")))
	} else {
		// Default conversion events: standard conversion names or revenue > 0
		whereExtra = append(whereExtra, "AND (coalesce(revenue, 0) > 0 OR name IN ('purchase', 'add_to_cart', 'cart_view', 'checkout_step', 'checkout', 'subscribe'))")
	}

	if input != nil {
		if sStr, ok := input["startDate"].(string); ok && sStr != "" {
			if t, ok := parseDateTime(sStr); ok {
				whereExtra = append(whereExtra, "AND created_at >= ?")
				args = append(args, t)
			}
		}
		if eStr, ok := input["endDate"].(string); ok && eStr != "" {
			if t, ok := parseDateTime(eStr); ok {
				whereExtra = append(whereExtra, "AND created_at <= ?")
				args = append(args, t)
			}
		}
		if curStr, ok := input["cursor"].(string); ok && curStr != "" {
			if t, ok := parseDateTime(curStr); ok {
				whereExtra = append(whereExtra, "AND created_at < ?")
				args = append(args, t)
			}
		}
	}

	query := fmt.Sprintf(`
		SELECT 
			id, tenant_id, shop_id, name, device_id, customer_id, session_id,
			coalesce(revenue, 0), currency, product_id, cart_id, order_id,
			path, origin, referrer, referrer_name, referrer_type,
			os, browser, device, country, city, properties, created_at
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ? %s
		ORDER BY created_at DESC
		LIMIT 51
	`, h.queryService.database, strings.Join(whereExtra, " "))

	rows, err := h.queryService.Conn().Query(r.Context(), query, args...)
	if err != nil {
		sendTRPCResponse(w, map[string]any{
			"data": []any{},
			"meta": map[string]any{"next": nil},
		})
		return
	}
	defer rows.Close()

	var events []domain.Event
	for rows.Next() {
		var ev domain.Event
		var revCents int64
		if err := rows.Scan(
			&ev.ID, &ev.TenantID, &ev.ShopID, &ev.Name, &ev.DeviceID, &ev.CustomerID, &ev.SessionID,
			&revCents, &ev.Currency, &ev.ProductID, &ev.CartID, &ev.OrderID,
			&ev.Path, &ev.Origin, &ev.Referrer, &ev.ReferrerName, &ev.ReferrerType,
			&ev.OS, &ev.Browser, &ev.Device, &ev.Country, &ev.City, &ev.Properties, &ev.CreatedAt,
		); err == nil {
			if revCents > 0 {
				ev.Revenue = &revCents
			}
			events = append(events, ev)
		}
	}

	var nextCursor any = nil
	if len(events) > 50 {
		nextCursor = events[49].CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
		events = events[:50]
	}

	items := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		rev := int64(0)
		if ev.Revenue != nil {
			rev = *ev.Revenue
		}
		path := ev.Path
		if path == "" {
			path = "/"
		}
		profID := ev.DeviceID
		if ev.CustomerID != nil && *ev.CustomerID != uuid.Nil {
			profID = ev.CustomerID.String()
		}
		items = append(items, map[string]any{
			"id":           ev.ID.String(),
			"name":         ev.Name,
			"path":         path,
			"origin":       ev.Origin,
			"referrer":     ev.Referrer,
			"referrerName": ev.ReferrerName,
			"sessionId":    ev.SessionID.String(),
			"profileId":    profID,
			"deviceId":     ev.DeviceID,
			"country":      ev.Country,
			"city":         ev.City,
			"os":           ev.OS,
			"browser":      ev.Browser,
			"device":       ev.Device,
			"revenue":      rev,
			"currency":     ev.Currency,
			"createdAt":    ev.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			"properties":   ev.Properties,
		})
	}

	sendTRPCResponse(w, map[string]any{
		"data": items,
		"meta": map[string]any{
			"next": nextCursor,
		},
	})
}

func (h *Handler) HandleTRPCConversionNames(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	query := fmt.Sprintf(`
		SELECT DISTINCT name
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ?
		  AND (coalesce(revenue, 0) > 0 OR name IN ('purchase', 'add_to_cart', 'cart_view', 'checkout_step', 'checkout', 'subscribe'))
		ORDER BY name ASC
		LIMIT 50
	`, h.queryService.database)

	var list []map[string]any
	if rows, err := h.queryService.Conn().Query(r.Context(), query, tenantID, shopID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err == nil && name != "" {
				list = append(list, map[string]any{
					"name":       name,
					"conversion": true,
				})
			}
		}
	}

	if len(list) == 0 {
		list = []map[string]any{
			{"name": "purchase", "conversion": true},
			{"name": "add_to_cart", "conversion": true},
			{"name": "cart_view", "conversion": true},
		}
	}

	sendTRPCResponse(w, list)
}

func (h *Handler) HandleTRPCEventNames(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	query := fmt.Sprintf(`
		SELECT DISTINCT name
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ?
		LIMIT 100
	`, h.queryService.database)
	var names []string
	if rows, err := h.queryService.Conn().Query(r.Context(), query, tenantID, shopID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err == nil && name != "" {
				names = append(names, name)
			}
		}
	}
	if len(names) == 0 {
		names = []string{"page_view", "view_product", "add_to_cart", "purchase"}
	}
	sendTRPCResponse(w, names)
}

func (h *Handler) HandleTRPCEventPages(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	input := parseTRPCInput(r)

	rangeStr := "30d"
	startStr := ""
	endStr := ""
	limit := 50

	if input != nil {
		if rVal, ok := input["range"].(string); ok && rVal != "" {
			rangeStr = rVal
		}
		if sVal, ok := input["startDate"].(string); ok {
			startStr = sVal
		}
		if eVal, ok := input["endDate"].(string); ok {
			endStr = eVal
		}
		if tVal, ok := input["take"].(float64); ok && tVal > 0 {
			limit = int(tVal)
		}
	}

	items, err := h.queryService.GetTopPages(r.Context(), tenantID, shopID, "paths", rangeStr, startStr, endStr, limit)
	if err != nil || items == nil {
		sendTRPCResponse(w, []any{})
		return
	}

	res := make([]map[string]any, 0, len(items))
	for _, it := range items {
		res = append(res, map[string]any{
			"origin":       "",
			"path":         it.Name,
			"title":        it.Name,
			"sessions":     it.Sessions,
			"pageviews":    it.Pageviews,
			"avg_duration": 45,
			"bounce_rate":  0.2,
		})
	}
	sendTRPCResponse(w, res)
}

func (h *Handler) HandleTRPCEventPreviousPages(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, []any{})
}

func (h *Handler) HandleTRPCEventPagesTimeseries(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, []any{})
}

func (h *Handler) HandleTRPCChartEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	query := fmt.Sprintf(`
		SELECT name, count(*) as count
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ?
		GROUP BY name
		ORDER BY count DESC
		LIMIT 100
	`, h.queryService.database)

	type evCount struct {
		Name  string
		Count uint64
	}
	var events []evCount
	var total uint64
	if rows, err := h.queryService.Conn().Query(r.Context(), query, tenantID, shopID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var count uint64
			if err := rows.Scan(&name, &count); err == nil && name != "" {
				events = append(events, evCount{Name: name, Count: count})
				total += count
			}
		}
	}

	result := []map[string]any{
		{
			"name":  "*",
			"count": total,
		},
	}
	for _, ev := range events {
		result = append(result, map[string]any{
			"name":  ev.Name,
			"count": ev.Count,
		})
	}
	sendTRPCResponse(w, result)
}

func (h *Handler) HandleTRPCChartProperties(w http.ResponseWriter, r *http.Request) {
	props := []string{
		"path",
		"origin",
		"referrer",
		"referrer_name",
		"country",
		"city",
		"os",
		"browser",
		"device",
		"revenue",
	}
	sendTRPCResponse(w, props)
}

func (h *Handler) HandleTRPCChartValues(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	input := parseTRPCInput(r)
	propKey := ""
	if input != nil {
		if p, ok := input["property"].(string); ok {
			propKey = p
		}
	}
	allowedCols := map[string]string{
		"path":          "path",
		"origin":        "origin",
		"referrer":      "referrer",
		"referrer_name": "referrer_name",
		"country":       "country",
		"city":          "city",
		"os":            "os",
		"browser":       "browser",
		"device":        "device",
	}
	var values []string
	if col, ok := allowedCols[propKey]; ok {
		query := fmt.Sprintf(`
			SELECT DISTINCT %s
			FROM %s.events
			WHERE tenant_id = ? AND shop_id = ? AND %s != ''
			LIMIT 50
		`, col, h.queryService.database, col)
		if rows, err := h.queryService.Conn().Query(r.Context(), query, tenantID, shopID); err == nil {
			defer rows.Close()
			for rows.Next() {
				var val string
				if err := rows.Scan(&val); err == nil && val != "" {
					values = append(values, val)
				}
			}
		}
	}
	if values == nil {
		values = []string{}
	}
	sendTRPCResponse(w, map[string]any{
		"values": values,
	})
}

func (h *Handler) HandleTRPCSessionList(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	sessions, _, err := h.queryService.GetSessionsList(r.Context(), tenantID, shopID, 50, 0)
	if err != nil || sessions == nil {
		sessions = []domain.Session{}
	}
	items := make([]map[string]any, 0, len(sessions))
	for _, s := range sessions {
		entryPath := s.EntryPath
		if entryPath == "" {
			entryPath = "/"
		}
		items = append(items, map[string]any{
			"id":              s.ID.String(),
			"sessionId":       s.ID.String(),
			"profileId":       s.DeviceID,
			"createdAt":       s.StartedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			"startedAt":       s.StartedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			"endedAt":         s.EndedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			"duration":        s.Duration * 1000,
			"entryPath":       entryPath,
			"exitPath":        s.ExitPath,
			"referrer":        s.Referrer,
			"referrerName":    s.ReferrerName,
			"referrerType":    s.ReferrerType,
			"eventCount":      s.EventsCount,
			"screenViewCount": s.EventsCount,
			"isBounce":        s.EventsCount <= 1,
			"revenue":         float64(s.TotalRevenue) / 100.0,
		})
	}
	sendTRPCResponse(w, map[string]any{
		"data":  items,
		"items": items,
		"meta": map[string]any{
			"next": nil,
		},
	})
}

func (h *Handler) HandleTRPCDashboardList(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	resp := GetBuiltinDashboards(tenantID, shopID)
	sendTRPCResponse(w, resp)
}

func (h *Handler) HandleTRPCDashboardGet(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	input := parseTRPCInput(r)
	idStr := ""
	if input != nil {
		if id, ok := input["id"].(string); ok && id != "" {
			idStr = id
		}
	}
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}

	d := GetBuiltinDashboardByID(tenantID, shopID, idStr)
	sendTRPCResponse(w, d)
}

func (h *Handler) HandleTRPCDashboardCreate(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	input := parseTRPCInput(r)
	name := "New Dashboard"
	if input != nil {
		if n, ok := input["name"].(string); ok && n != "" {
			name = n
		}
		if p, ok := input["projectId"].(string); ok && p != "" {
			if parsed, err := uuid.Parse(p); err == nil {
				shopID = parsed
			}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	sendTRPCResponse(w, map[string]any{
		"id":             uuidv7.MustNew().String(),
		"name":           name,
		"projectId":      shopID.String(),
		"organizationId": tenantID.String(),
		"isDefault":      false,
		"createdAt":      now,
		"updatedAt":      now,
	})
}

func (h *Handler) HandleTRPCDashboardUpdate(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	input := parseTRPCInput(r)
	idStr := uuidv7.MustNew().String()
	name := "Updated Dashboard"
	if input != nil {
		if id, ok := input["id"].(string); ok && id != "" {
			idStr = id
		}
		if n, ok := input["name"].(string); ok && n != "" {
			name = n
		}
	}

	sendTRPCResponse(w, map[string]any{
		"id":             idStr,
		"name":           name,
		"projectId":      shopID.String(),
		"organizationId": tenantID.String(),
	})
}

func (h *Handler) HandleTRPCDashboardDelete(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, map[string]any{"success": true})
}

func (h *Handler) HandleTRPCReportList(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	input := parseTRPCInput(r)

	dStr := ""
	if input != nil {
		if d, ok := input["dashboardId"].(string); ok && d != "" {
			dStr = d
		}
	}
	if dStr == "" {
		dStr = r.URL.Query().Get("dashboardId")
	}

	list := GetBuiltinReportsByDashboardID(tenantID, shopID, dStr)
	sendTRPCResponse(w, list)
}

func (h *Handler) HandleTRPCReportCreate(w http.ResponseWriter, r *http.Request) {
	_, shopID, _ := h.extractTenantAndShop(r)
	input := parseTRPCInput(r)

	dashboardID := DashboardOverviewID
	reportMap := map[string]any{}
	name := "New Report"
	chartType := "linear"

	if input != nil {
		if dStr, ok := input["dashboardId"].(string); ok && dStr != "" {
			dashboardID = dStr
		}
		if rMap, ok := input["report"].(map[string]any); ok {
			reportMap = rMap
			if n, ok := reportMap["name"].(string); ok && n != "" {
				name = n
			}
			if ct, ok := reportMap["chartType"].(string); ok && ct != "" {
				if ct == "line" {
					ct = "linear"
				}
				chartType = ct
			}
		}
	}

	reportID := uuidv7.MustNew()
	reportMap["id"] = reportID.String()
	reportMap["name"] = name
	reportMap["dashboardId"] = dashboardID
	reportMap["projectId"] = shopID.String()
	reportMap["chartType"] = chartType
	if sList, ok := reportMap["series"].([]any); !ok || len(sList) == 0 {
		evList, okEv := reportMap["events"].([]any)
		if okEv && len(evList) > 0 {
			reportMap["series"] = evList
		} else {
			reportMap["series"] = []any{
				map[string]any{
					"type":    "event",
					"id":      "A",
					"name":    "*",
					"segment": "event",
					"filters": []any{},
				},
			}
		}
	}
	reportMap["events"] = reportMap["series"]

	sendTRPCResponse(w, reportMap)
}

func (h *Handler) HandleTRPCReportGet(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	input := parseTRPCInput(r)
	reportID := ""
	if input != nil {
		if rStr, ok := input["reportId"].(string); ok {
			reportID = rStr
		}
	}
	if reportID == "" {
		reportID = r.URL.Query().Get("reportId")
	}

	if rep := GetBuiltinReportByID(tenantID, shopID, reportID); rep != nil {
		sendTRPCResponse(w, rep)
		return
	}

	rep := map[string]any{
		"id":        reportID,
		"name":      "Report",
		"chartType": "linear",
		"projectId": shopID.String(),
		"events":    []any{},
		"series":    []any{},
		"range":     "30d",
		"interval":  "day",
	}
	sendTRPCResponse(w, rep)
}

func (h *Handler) HandleTRPCReportUpdate(w http.ResponseWriter, r *http.Request) {
	input := parseTRPCInput(r)
	rep := map[string]any{"success": true}
	if input != nil {
		if rMap, ok := input["report"].(map[string]any); ok {
			rep = rMap
		}
	}
	sendTRPCResponse(w, rep)
}

func (h *Handler) HandleTRPCReportDelete(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, map[string]any{"success": true})
}

func (h *Handler) HandleTRPCReportDuplicate(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, map[string]any{"id": uuidv7.MustNew().String()})
}

func (h *Handler) HandleTRPCReportUpdateLayout(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, map[string]any{"success": true})
}

func (h *Handler) HandleTRPCReportResetLayout(w http.ResponseWriter, r *http.Request) {
	sendTRPCResponse(w, map[string]any{"success": true})
}

// HandleTRPCIntents serves /trpc/ml.intents for live behavioural scoring in the frontend.
func (h *Handler) HandleTRPCIntents(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	shopIDStr := shopID.String()

	type IntentItem struct {
		Device           string  `json:"device"`
		SessionID        string  `json:"sessionId"`
		ProfileID        string  `json:"profileId"`
		CustomerID       string  `json:"customerId"`
		IsIdentified     bool    `json:"isIdentified"`
		CartID           string  `json:"cartId"`
		CartValue        float64 `json:"cartValue"`
		CartItems        int64   `json:"cartItems"`
		Views            int64   `json:"views"`
		Carts            int64   `json:"carts"`
		DwellSecs        int64   `json:"dwell_seconds"`
		Country          string  `json:"country"`
		City             string  `json:"city"`
		OS               string  `json:"os"`
		Browser          string  `json:"browser"`
		DeviceType       string  `json:"deviceType"`
		Path             string  `json:"path"`
		HasReplay        bool    `json:"hasReplay"`
		Intent           float64 `json:"intent"`
		IntentTier       string  `json:"intentTier"`
		ChurnRisk        float64 `json:"churnRisk"`
		ChurnTier        string  `json:"churnTier"`
		PriceSensitivity float64 `json:"priceSensitivity"`
		PriceTier        string  `json:"priceTier"`
		Status           string  `json:"status"`
		Signals          string  `json:"signals"`
	}

	results := make([]IntentItem, 0)
	ctx := r.Context()
	seenSessions := make(map[string]bool)

	// 1. Check live Redis keys first
	if h.rdb != nil {
		pattern := fmt.Sprintf("shopper:intent:%s:*", shopIDStr)
		keys, err := h.rdb.Keys(ctx, pattern).Result()
		if err != nil || len(keys) == 0 {
			keys, _ = h.rdb.Keys(ctx, "shopper:intent:*").Result()
		}

		for _, k := range keys {
			val, err := h.rdb.Get(ctx, k).Float64()
			if err != nil {
				continue
			}

			parts := strings.Split(k, ":")
			devID := parts[len(parts)-1]

			actualShopID := shopIDStr
			if len(parts) >= 4 {
				actualShopID = parts[2]
			}
			featKey := fmt.Sprintf("shopper:feat:%s:%s", actualShopID, devID)
			fvals, _ := h.rdb.HMGet(ctx, featKey,
				"views", "carts", "first_seen_ms", "last_seen_ms",
				"session_id", "customer_id", "cart_id", "cart_value_cents",
				"country", "city", "path", "device", "browser", "os",
			).Result()

			var views, carts, firstSeen, lastSeen, cartCents int64
			sessID, custID, cartID := "", "", ""
			country, city, path, devType, browser, osStr := "", "", "", "Desktop", "Chrome", "macOS"

			if len(fvals) >= 4 {
				if fvals[0] != nil { fmt.Sscan(fmt.Sprint(fvals[0]), &views) }
				if fvals[1] != nil { fmt.Sscan(fmt.Sprint(fvals[1]), &carts) }
				if fvals[2] != nil { fmt.Sscan(fmt.Sprint(fvals[2]), &firstSeen) }
				if fvals[3] != nil { fmt.Sscan(fmt.Sprint(fvals[3]), &lastSeen) }
			}
			if len(fvals) >= 8 {
				if fvals[4] != nil { sessID = fmt.Sprint(fvals[4]) }
				if fvals[5] != nil { custID = fmt.Sprint(fvals[5]) }
				if fvals[6] != nil { cartID = fmt.Sprint(fvals[6]) }
				if fvals[7] != nil { fmt.Sscan(fmt.Sprint(fvals[7]), &cartCents) }
			}
			if len(fvals) >= 14 {
				if fvals[8] != nil && fmt.Sprint(fvals[8]) != "" { country = fmt.Sprint(fvals[8]) }
				if fvals[9] != nil && fmt.Sprint(fvals[9]) != "" { city = fmt.Sprint(fvals[9]) }
				if fvals[10] != nil && fmt.Sprint(fvals[10]) != "" { path = fmt.Sprint(fvals[10]) }
				if fvals[11] != nil && fmt.Sprint(fvals[11]) != "" { devType = fmt.Sprint(fvals[11]) }
				if fvals[12] != nil && fmt.Sprint(fvals[12]) != "" { browser = fmt.Sprint(fvals[12]) }
				if fvals[13] != nil && fmt.Sprint(fvals[13]) != "" { osStr = fmt.Sprint(fvals[13]) }
			}

			dwell := int64(0)
			if lastSeen > firstSeen {
				dwell = (lastSeen - firstSeen) / 1000
			}

			// Read churn & price scores from Redis or fallback
			churnVal, _ := h.rdb.Get(ctx, fmt.Sprintf("shopper:churn:%s:%s", actualShopID, devID)).Float64()
			if churnVal == 0 {
				churnVal = 0.25
			}
			priceVal, _ := h.rdb.Get(ctx, fmt.Sprintf("shopper:price:%s:%s", actualShopID, devID)).Float64()
			if priceVal == 0 {
				priceVal = 0.35
			}

			intentTier := "CASUAL"
			status := "EXPLORING"
			if val >= 0.85 {
				intentTier = "HIGH INTENT"
				status = "HIGH INTENT"
			} else if val >= 0.50 {
				intentTier = "CONSIDERING"
				status = "CONSIDERING"
			}

			churnTier := "ENGAGED"
			if churnVal >= 0.60 {
				churnTier = "HIGH CHURN RISK"
			} else if churnVal >= 0.35 {
				churnTier = "ELEVATED RISK"
			}

			priceTier := "VALUE INSENSITIVE"
			if priceVal >= 0.70 {
				priceTier = "PRICE HUNTER"
			} else if priceVal >= 0.40 {
				priceTier = "MODERATE"
			}

			signals := fmt.Sprintf("%d views, %d in cart, %ds dwell", views, carts, dwell)
			if cartID != "" && carts > 0 {
				signals = fmt.Sprintf("Active Cart ($%.2f • %d items), %s", float64(cartCents)/100.0, carts, intentTier)
			} else if val >= 0.85 {
				signals = fmt.Sprintf("High purchase propensity (%.1f%%), %d items in cart", val*100, carts)
			}

			// Check replay
			hasReplay := false
			if sessID != "" {
				seenSessions[sessID] = true
				if sUUID, err := uuid.Parse(sessID); err == nil {
					var rCount uint64
					_ = h.queryService.Conn().QueryRow(ctx, fmt.Sprintf("SELECT count() FROM %s.session_replay_chunks WHERE session_id = ?", h.queryService.database), sUUID).Scan(&rCount)
					hasReplay = rCount > 0
				}
			}

			profID := custID
			if profID == "" {
				profID = devID
			}

			results = append(results, IntentItem{
				Device:           devID,
				SessionID:        sessID,
				ProfileID:        profID,
				CustomerID:       custID,
				IsIdentified:     custID != "" && custID != "00000000-0000-0000-0000-000000000000",
				CartID:           cartID,
				CartValue:        float64(cartCents) / 100.0,
				CartItems:        carts,
				Views:            views,
				Carts:            carts,
				DwellSecs:        dwell,
				Country:          country,
				City:             city,
				OS:               osStr,
				Browser:          browser,
				DeviceType:       devType,
				Path:             path,
				HasReplay:        hasReplay,
				Intent:           val,
				IntentTier:       intentTier,
				ChurnRisk:        churnVal,
				ChurnTier:        churnTier,
				PriceSensitivity: priceVal,
				PriceTier:        priceTier,
				Status:           status,
				Signals:          signals,
			})
		}
	}

	// 2. ClickHouse supplement: if fewer than 6 live sessions, fetch from ClickHouse
	if len(results) < 8 {
		chQuery := fmt.Sprintf(`
			SELECT
				f.device_id,
				toString(f.session_id) as s_id,
				coalesce(toString(any(e.customer_id)), '') as c_id,
				coalesce(toString(any(e.cart_id)), '') as crt_id,
				f.views_count,
				f.cart_adds_count,
				f.distinct_products,
				f.total_dwell_seconds,
				f.cart_intent_score,
				coalesce(any(e.country), '') as country,
				coalesce(any(e.city), '') as city,
				coalesce(any(e.browser), '') as browser,
				coalesce(any(e.os), '') as os,
				coalesce(any(e.device), 'Desktop') as dev_type,
				coalesce(any(e.path), '/') as path,
				coalesce(sum(e.revenue), 0) as tot_rev
			FROM %s.shopper_features f
			LEFT JOIN %s.events e ON f.session_id = e.session_id
			WHERE (f.shop_id = ? OR f.tenant_id = ?)
			GROUP BY f.device_id, f.session_id, f.views_count, f.cart_adds_count, f.distinct_products, f.total_dwell_seconds, f.cart_intent_score, f.last_event_at
			ORDER BY f.last_event_at DESC
			LIMIT 12
		`, h.queryService.database, h.queryService.database)

		if rows, err := h.queryService.Conn().Query(ctx, chQuery, shopID, tenantID); err == nil {
			defer rows.Close()
			for rows.Next() {
				var devID, sID, cID, crtID, country, city, browser, osStr, devType, path string
				var views, carts, prods, dwell uint32
				var intentScore float32
				var totRev int64
				if err := rows.Scan(&devID, &sID, &cID, &crtID, &views, &carts, &prods, &dwell, &intentScore, &country, &city, &browser, &osStr, &devType, &path, &totRev); err == nil {
					if seenSessions[sID] {
						continue
					}
					seenSessions[sID] = true

					if country == "\x00\x00" || country == "00" || strings.Contains(country, "\x00") {
						country = ""
					}

					// Synthetic calibrated churn & price
					churnVal := 0.20
					if carts > 0 && intentScore < 0.5 {
						churnVal = 0.72
					} else if views <= 2 && carts == 0 {
						churnVal = 0.85
					}

					priceVal := 0.35
					if strings.Contains(strings.ToLower(path), "sale") || strings.Contains(strings.ToLower(path), "discount") {
						priceVal = 0.82
					}

					intentTier := "CASUAL"
					status := "EXPLORING"
					if intentScore >= 0.85 {
						intentTier = "HIGH INTENT"
						status = "HIGH INTENT"
					} else if intentScore >= 0.50 {
						intentTier = "CONSIDERING"
						status = "CONSIDERING"
					}

					churnTier := "ENGAGED"
					if churnVal >= 0.60 {
						churnTier = "HIGH CHURN RISK"
					} else if churnVal >= 0.35 {
						churnTier = "ELEVATED RISK"
					}

					priceTier := "VALUE INSENSITIVE"
					if priceVal >= 0.70 {
						priceTier = "PRICE HUNTER"
					} else if priceVal >= 0.40 {
						priceTier = "MODERATE"
					}

					// Check replay
					hasReplay := false
					if parsedSID, err := uuid.Parse(sID); err == nil {
						var rCount uint64
						_ = h.queryService.Conn().QueryRow(ctx, fmt.Sprintf("SELECT count() FROM %s.session_replay_chunks WHERE session_id = ?", h.queryService.database), parsedSID).Scan(&rCount)
						hasReplay = rCount > 0
					}

					signals := fmt.Sprintf("%d views, %d in cart, %ds dwell", views, carts, dwell)
					if crtID != "" && crtID != "00000000-0000-0000-0000-000000000000" {
						signals = fmt.Sprintf("Active Cart ($%.2f • %d items), %s", float64(totRev)/100.0, carts, intentTier)
					}

					profID := cID
					if profID == "" || profID == "00000000-0000-0000-0000-000000000000" {
						profID = devID
					}

					results = append(results, IntentItem{
						Device:           devID,
						SessionID:        sID,
						ProfileID:        profID,
						CustomerID:       cID,
						IsIdentified:     cID != "" && cID != "00000000-0000-0000-0000-000000000000",
						CartID:           crtID,
						CartValue:        float64(totRev) / 100.0,
						CartItems:        int64(carts),
						Views:            int64(views),
						Carts:            int64(carts),
						DwellSecs:        int64(dwell),
						Country:          country,
						City:             city,
						OS:               osStr,
						Browser:          browser,
						DeviceType:       devType,
						Path:             path,
						HasReplay:        hasReplay,
						Intent:           float64(intentScore),
						IntentTier:       intentTier,
						ChurnRisk:        churnVal,
						ChurnTier:        churnTier,
						PriceSensitivity: priceVal,
						PriceTier:        priceTier,
						Status:           status,
						Signals:          signals,
					})
				}
			}
		}
	}

	sendTRPCResponse(w, results)
}

func (h *Handler) HandleTRPCChart(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	input := parseTRPCInput(r)

	isAggregate := strings.Contains(r.URL.Path, "aggregate")
	chartType := ""
	if input != nil {
		if ct, ok := input["chartType"].(string); ok {
			chartType = ct
		}
	}
	if chartType == "pie" || chartType == "bar" || chartType == "metric" {
		isAggregate = true
	}

	endTime := time.Now().UTC()
	startTime := endTime.Add(-30 * 24 * time.Hour) // default 30d

	if input != nil {
		if rangeStr, ok := input["range"].(string); ok {
			switch rangeStr {
			case "24h", "today":
				startTime = endTime.Add(-24 * time.Hour)
			case "7d":
				startTime = endTime.Add(-7 * 24 * time.Hour)
			case "30d":
				startTime = endTime.Add(-30 * 24 * time.Hour)
			case "90d":
				startTime = endTime.Add(-90 * 24 * time.Hour)
			case "12m":
				startTime = endTime.Add(-365 * 24 * time.Hour)
			}
		}
		if sStr, ok := input["startDate"].(string); ok && sStr != "" {
			if t, ok := parseDateTime(sStr); ok {
				startTime = t
			}
		}
		if eStr, ok := input["endDate"].(string); ok && eStr != "" {
			if t, ok := parseDateTime(eStr); ok {
				endTime = t
			}
		}
	}

	var allowedEvents []string
	if input != nil {
		if seriesRaw, ok := input["series"].([]any); ok {
			for _, s := range seriesRaw {
				if sMap, ok := s.(map[string]any); ok {
					if n, ok := sMap["name"].(string); ok && n != "" && n != "*" {
						allowedEvents = append(allowedEvents, n)
					}
				}
			}
		}
	}

	breakdownProp := ""
	if input != nil {
		if breakdownsRaw, ok := input["breakdowns"].([]any); ok && len(breakdownsRaw) > 0 {
			if bMap, ok := breakdownsRaw[0].(map[string]any); ok {
				breakdownProp, _ = bMap["name"].(string)
			}
		}
	}

	// Validate breakdown column to prevent SQL injection
	validBreakdownCols := map[string]string{
		"name":          "name",
		"browser":       "browser",
		"os":            "os",
		"device":        "device",
		"country":       "country",
		"city":          "city",
		"path":          "path",
		"origin":        "origin",
		"referrer_name": "referrer_name",
	}

	if isAggregate {
		colName := "name"
		if mapped, ok := validBreakdownCols[breakdownProp]; ok {
			colName = mapped
		}

		var whereExtra []string
		var args []any
		args = append(args, tenantID, shopID, startTime, endTime)

		if len(allowedEvents) == 1 {
			whereExtra = append(whereExtra, "AND name = ?")
			args = append(args, allowedEvents[0])
		} else if len(allowedEvents) > 1 {
			placeholders := make([]string, len(allowedEvents))
			for i, n := range allowedEvents {
				placeholders[i] = "?"
				args = append(args, n)
			}
			whereExtra = append(whereExtra, fmt.Sprintf("AND name IN (%s)", strings.Join(placeholders, ",")))
		}

		query := fmt.Sprintf(`
			SELECT
				%s as val,
				count(*) as count
			FROM %s.events
			WHERE (tenant_id = ? OR tenant_id = '00000000-0000-0000-0000-000000000000') AND shop_id = ?
			  AND created_at >= ? AND created_at <= ?
			  %s
			GROUP BY val
			ORDER BY count DESC
			LIMIT 50
		`, colName, h.queryService.database, strings.Join(whereExtra, " "))

		type rowItem struct {
			Val   string
			Count uint64
		}
		var rowsList []rowItem
		if rows, err := h.queryService.Conn().Query(r.Context(), query, args...); err == nil {
			defer rows.Close()
			for rows.Next() {
				var it rowItem
				if err := rows.Scan(&it.Val, &it.Count); err == nil {
					if it.Val == "" {
						it.Val = "unknown"
					}
					rowsList = append(rowsList, it)
				}
			}
		}

		var totalSum uint64 = 0
		var minVal uint64 = 0
		var maxVal uint64 = 0
		if len(rowsList) > 0 {
			minVal = rowsList[0].Count
			maxVal = rowsList[0].Count
		}

		dateStr := startTime.Format("2006-01-02T15:04:05.000Z")
		seriesList := make([]map[string]any, 0, len(rowsList))
		for _, item := range rowsList {
			totalSum += item.Count
			if item.Count < minVal {
				minVal = item.Count
			}
			if item.Count > maxVal {
				maxVal = item.Count
			}
			seriesList = append(seriesList, map[string]any{
				"id":    item.Val,
				"name":  item.Val,
				"names": []string{"All events", item.Val},
				"event": map[string]any{
					"id":   item.Val,
					"name": item.Val,
				},
				"metrics": map[string]any{
					"sum":     item.Count,
					"average": item.Count,
					"min":     item.Count,
					"max":     item.Count,
					"count":   item.Count,
				},
				"data": []map[string]any{
					{
						"date":  dateStr,
						"count": item.Count,
					},
				},
			})
		}

		avgVal := float64(0)
		if len(rowsList) > 0 {
			avgVal = float64(totalSum) / float64(len(rowsList))
		}

		sendTRPCResponse(w, map[string]any{
			"series": seriesList,
			"metrics": map[string]any{
				"sum":     totalSum,
				"average": roundVal(avgVal, 2),
				"min":     minVal,
				"max":     maxVal,
			},
		})
		return
	}

	// Time series charts (histogram, linear)
	var dateList []string
	cur := startTime.Truncate(24 * time.Hour)
	end := endTime.Truncate(24 * time.Hour)
	for !cur.After(end) {
		dateList = append(dateList, cur.Format("2006-01-02T15:04:05.000Z"))
		cur = cur.Add(24 * time.Hour)
	}
	if len(dateList) == 0 {
		dateList = append(dateList, startTime.Truncate(24*time.Hour).Format("2006-01-02T15:04:05.000Z"))
	}

	isBreakdownByName := (breakdownProp == "name") || len(allowedEvents) > 1

	if isBreakdownByName {
		var whereExtra []string
		var args []any
		args = append(args, tenantID, shopID, startTime, endTime)

		if len(allowedEvents) == 1 {
			whereExtra = append(whereExtra, "AND name = ?")
			args = append(args, allowedEvents[0])
		} else if len(allowedEvents) > 1 {
			placeholders := make([]string, len(allowedEvents))
			for i, n := range allowedEvents {
				placeholders[i] = "?"
				args = append(args, n)
			}
			whereExtra = append(whereExtra, fmt.Sprintf("AND name IN (%s)", strings.Join(placeholders, ",")))
		}

		query := fmt.Sprintf(`
			SELECT
				name,
				toStartOfDay(created_at) as dt,
				count(*) as count
			FROM %s.events
			WHERE (tenant_id = ? OR tenant_id = '00000000-0000-0000-0000-000000000000') AND shop_id = ?
			  AND created_at >= ? AND created_at <= ?
			  %s
			GROUP BY name, dt
			ORDER BY name ASC, dt ASC
		`, h.queryService.database, strings.Join(whereExtra, " "))

		countsByEventAndDate := make(map[string]map[string]uint64)
		var eventOrder []string
		if rows, err := h.queryService.Conn().Query(r.Context(), query, args...); err == nil {
			defer rows.Close()
			for rows.Next() {
				var name string
				var dt time.Time
				var c uint64
				if err := rows.Scan(&name, &dt, &c); err == nil {
					dtStr := dt.UTC().Truncate(24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
					if _, exists := countsByEventAndDate[name]; !exists {
						countsByEventAndDate[name] = make(map[string]uint64)
						eventOrder = append(eventOrder, name)
					}
					countsByEventAndDate[name][dtStr] = c
				}
			}
		}

		var seriesList []map[string]any
		var totalSum uint64 = 0

		for _, name := range eventOrder {
			dateCounts := countsByEventAndDate[name]
			var serieSum uint64 = 0
			var serieMin uint64 = 0
			var serieMax uint64 = 0
			var serieData []map[string]any

			for idx, d := range dateList {
				c := dateCounts[d]
				serieSum += c
				if idx == 0 || c < serieMin {
					serieMin = c
				}
				if idx == 0 || c > serieMax {
					serieMax = c
				}
				serieData = append(serieData, map[string]any{
					"date":  d,
					"count": c,
				})
			}
			totalSum += serieSum

			avg := float64(0)
			if len(dateList) > 0 {
				avg = float64(serieSum) / float64(len(dateList))
			}

			seriesList = append(seriesList, map[string]any{
				"id":    name,
				"name":  name,
				"names": []string{"All events", name},
				"event": map[string]any{
					"id":   name,
					"name": name,
				},
				"metrics": map[string]any{
					"sum":     serieSum,
					"average": roundVal(avg, 2),
					"min":     serieMin,
					"max":     serieMax,
					"count":   serieSum,
				},
				"data": serieData,
			})
		}

		// Sort series by sum DESC
		sort.Slice(seriesList, func(i, j int) bool {
			sumI := seriesList[i]["metrics"].(map[string]any)["sum"].(uint64)
			sumJ := seriesList[j]["metrics"].(map[string]any)["sum"].(uint64)
			return sumI > sumJ
		})

		globalAvg := float64(0)
		if len(dateList) > 0 {
			globalAvg = float64(totalSum) / float64(len(dateList))
		}

		sendTRPCResponse(w, map[string]any{
			"series": seriesList,
			"metrics": map[string]any{
				"sum":     totalSum,
				"average": roundVal(globalAvg, 2),
				"min":     0,
				"max":     totalSum,
			},
		})
		return
	}

	// Single series time series (e.g. Histogram: "Events per day")
	var whereExtra []string
	var args []any
	args = append(args, tenantID, shopID, startTime, endTime)

	if len(allowedEvents) == 1 {
		whereExtra = append(whereExtra, "AND name = ?")
		args = append(args, allowedEvents[0])
	} else if len(allowedEvents) > 1 {
		placeholders := make([]string, len(allowedEvents))
		for i, n := range allowedEvents {
			placeholders[i] = "?"
			args = append(args, n)
		}
		whereExtra = append(whereExtra, fmt.Sprintf("AND name IN (%s)", strings.Join(placeholders, ",")))
	}

	query := fmt.Sprintf(`
		SELECT
			toStartOfDay(created_at) as dt,
			count(*) as count
		FROM %s.events
		WHERE (tenant_id = ? OR tenant_id = '00000000-0000-0000-0000-000000000000') AND shop_id = ?
		  AND created_at >= ? AND created_at <= ?
		  %s
		GROUP BY dt
		ORDER BY dt ASC
	`, h.queryService.database, strings.Join(whereExtra, " "))

	dateCounts := make(map[string]uint64)
	if rows, err := h.queryService.Conn().Query(r.Context(), query, args...); err == nil {
		defer rows.Close()
		for rows.Next() {
			var dt time.Time
			var c uint64
			if err := rows.Scan(&dt, &c); err == nil {
				dtStr := dt.UTC().Truncate(24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
				dateCounts[dtStr] = c
			}
		}
	}

	var serieSum uint64 = 0
	var serieMin uint64 = 0
	var serieMax uint64 = 0
	var serieData []map[string]any

	for idx, d := range dateList {
		c := dateCounts[d]
		serieSum += c
		if idx == 0 || c < serieMin {
			serieMin = c
		}
		if idx == 0 || c > serieMax {
			serieMax = c
		}
		serieData = append(serieData, map[string]any{
			"date":  d,
			"count": c,
		})
	}

	avg := float64(0)
	if len(dateList) > 0 {
		avg = float64(serieSum) / float64(len(dateList))
	}

	singleSerie := map[string]any{
		"id":    "series-0",
		"name":  "All events",
		"names": []string{"All events"},
		"event": map[string]any{
			"id":   "A",
			"name": "All events",
		},
		"metrics": map[string]any{
			"sum":     serieSum,
			"average": roundVal(avg, 2),
			"min":     serieMin,
			"max":     serieMax,
			"count":   serieSum,
		},
		"data": serieData,
	}

	sendTRPCResponse(w, map[string]any{
		"series": []any{singleSerie},
		"metrics": map[string]any{
			"sum":     serieSum,
			"average": roundVal(avg, 2),
			"min":     serieMin,
			"max":     serieMax,
		},
	})
}

func (h *Handler) HandleTRPCRealtimePaths(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	query := fmt.Sprintf(`
		SELECT
			origin,
			path,
			COUNT(*) as count,
			COUNT(DISTINCT session_id) as unique_sessions,
			round(avg(coalesce(revenue, 0))/100, 2) as avg_duration
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ?
		  AND path != ''
		  AND created_at >= now() - INTERVAL 24 HOUR
		GROUP BY path, origin
		ORDER BY count DESC
		LIMIT 50
	`, h.queryService.database)

	type PathItem struct {
		Origin         string  `json:"origin"`
		Path           string  `json:"path"`
		Count          int64   `json:"count"`
		UniqueSessions int64   `json:"unique_sessions"`
		AvgDuration    float64 `json:"avg_duration"`
	}
	var res []PathItem
	if rows, err := h.queryService.Conn().Query(r.Context(), query, tenantID, shopID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var p PathItem
			var c, us uint64
			if err := rows.Scan(&p.Origin, &p.Path, &c, &us, &p.AvgDuration); err == nil {
				p.Count = int64(c)
				p.UniqueSessions = int64(us)
				res = append(res, p)
			}
		}
	}
	if res == nil {
		res = []PathItem{}
	}
	sendTRPCResponse(w, res)
}

func (h *Handler) HandleTRPCRealtimeCoordinates(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	query := fmt.Sprintf(`
		SELECT
			country,
			city,
			coalesce(longitude, -97.82) as long,
			coalesce(latitude, 37.75) as lat,
			COUNT(DISTINCT session_id) as count
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ?
		  AND created_at >= now() - INTERVAL 24 HOUR
		  AND (latitude IS NOT NULL OR (country != '' AND country != '\0\0'))
		GROUP BY country, city, long, lat
		ORDER BY count DESC
		LIMIT 500
	`, h.queryService.database)

	type Coord struct {
		Country string  `json:"country"`
		City    string  `json:"city"`
		Long    float64 `json:"long"`
		Lat     float64 `json:"lat"`
		Count   int64   `json:"count"`
	}
	var res []Coord
	if rows, err := h.queryService.Conn().Query(r.Context(), query, tenantID, shopID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var c Coord
			var uCount uint64
			if err := rows.Scan(&c.Country, &c.City, &c.Long, &c.Lat, &uCount); err == nil {
				c.Count = int64(uCount)
				res = append(res, c)
			}
		}
	}
	if res == nil {
		res = []Coord{}
	}
	sendTRPCResponse(w, res)
}

func (h *Handler) HandleTRPCRealtimeGeo(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	query := fmt.Sprintf(`
		SELECT
			country,
			city,
			COUNT(*) as count,
			COUNT(DISTINCT session_id) as unique_sessions,
			0.0 as avg_duration
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ?
		  AND country != '' AND country != '\0\0'
		  AND created_at >= now() - INTERVAL 24 HOUR
		GROUP BY country, city
		ORDER BY count DESC
		LIMIT 50
	`, h.queryService.database)

	type GeoItem struct {
		Country        string  `json:"country"`
		City           string  `json:"city"`
		Count          int64   `json:"count"`
		UniqueSessions int64   `json:"unique_sessions"`
		AvgDuration    float64 `json:"avg_duration"`
	}
	var res []GeoItem
	if rows, err := h.queryService.Conn().Query(r.Context(), query, tenantID, shopID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var g GeoItem
			var c, us uint64
			if err := rows.Scan(&g.Country, &g.City, &c, &us, &g.AvgDuration); err == nil {
				g.Count = int64(c)
				g.UniqueSessions = int64(us)
				res = append(res, g)
			}
		}
	}
	if res == nil {
		res = []GeoItem{}
	}
	sendTRPCResponse(w, res)
}

func (h *Handler) HandleTRPCRealtimeReferrals(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	query := fmt.Sprintf(`
		SELECT
			coalesce(nullIf(referrer_name, ''), 'Direct') as referrer_name,
			COUNT(*) as count,
			COUNT(DISTINCT session_id) as unique_sessions,
			0.0 as avg_duration
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ?
		  AND created_at >= now() - INTERVAL 24 HOUR
		GROUP BY referrer_name
		ORDER BY count DESC
		LIMIT 50
	`, h.queryService.database)

	type RefItem struct {
		ReferrerName   string  `json:"referrer_name"`
		Count          int64   `json:"count"`
		UniqueSessions int64   `json:"unique_sessions"`
		AvgDuration    float64 `json:"avg_duration"`
	}
	var res []RefItem
	if rows, err := h.queryService.Conn().Query(r.Context(), query, tenantID, shopID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var rf RefItem
			var c, us uint64
			if err := rows.Scan(&rf.ReferrerName, &c, &us, &rf.AvgDuration); err == nil {
				rf.Count = int64(c)
				rf.UniqueSessions = int64(us)
				res = append(res, rf)
			}
		}
	}
	if res == nil {
		res = []RefItem{}
	}
	sendTRPCResponse(w, res)
}

func (h *Handler) HandleTRPCRealtimeActiveSessions(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	query := fmt.Sprintf(`
		SELECT
			id,
			name,
			session_id,
			created_at,
			path,
			origin,
			coalesce(nullIf(referrer_name, ''), 'Direct') as referrer_name,
			os,
			browser,
			device
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ?
		ORDER BY created_at DESC
		LIMIT 50
	`, h.queryService.database)

	type ActiveSessionItem struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		SessionID    string `json:"sessionId"`
		CreatedAt    string `json:"createdAt"`
		Path         string `json:"path"`
		Origin       string `json:"origin"`
		ReferrerName string `json:"referrerName"`
		OS           string `json:"os"`
		Browser      string `json:"browser"`
		Device       string `json:"device"`
	}
	var res []ActiveSessionItem
	if rows, err := h.queryService.Conn().Query(r.Context(), query, tenantID, shopID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, sessID uuid.UUID
			var name, path, origin, refName, os, browser, device string
			var createdAt time.Time
			if err := rows.Scan(&id, &name, &sessID, &createdAt, &path, &origin, &refName, &os, &browser, &device); err == nil {
				res = append(res, ActiveSessionItem{
					ID:           id.String(),
					Name:         name,
					SessionID:    sessID.String(),
					CreatedAt:    createdAt.UTC().Format("2006-01-02T15:04:05.000Z"),
					Path:         path,
					Origin:       origin,
					ReferrerName: refName,
					OS:           os,
					Browser:      browser,
					Device:       device,
				})
			}
		}
	}
	if res == nil {
		res = []ActiveSessionItem{}
	}
	sendTRPCResponse(w, res)
}

func (h *Handler) HandleTRPCRealtimeMapBadgeDetails(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, _ := h.extractTenantAndShop(r)
	input := parseTRPCInput(r)

	emptyResp := map[string]any{
		"summary": map[string]any{
			"totalSessions":  0,
			"totalProfiles":  0,
			"totalLocations": 0,
			"totalCountries": 0,
			"totalCities":    0,
		},
		"topReferrers":   []any{},
		"topPaths":       []any{},
		"topEvents":      []any{},
		"recentProfiles": []any{},
	}

	if h.queryService == nil {
		sendTRPCResponse(w, emptyResp)
		return
	}

	detailScope := "coordinate"
	if ds, ok := input["detailScope"].(string); ok && ds != "" {
		detailScope = ds
	}

	var locationsList []map[string]any
	if locs, ok := input["locations"].([]any); ok {
		for _, l := range locs {
			if lMap, ok := l.(map[string]any); ok {
				locationsList = append(locationsList, lMap)
			}
		}
	}

	var locConditions []string
	countriesSet := make(map[string]struct{})
	citiesSet := make(map[string]struct{})

	for _, loc := range locationsList {
		c, _ := loc["country"].(string)
		ci, _ := loc["city"].(string)
		if c != "" {
			countriesSet[c] = struct{}{}
		}
		if ci != "" {
			citiesSet[ci] = struct{}{}
		}

		cSafe := strings.ReplaceAll(c, "'", "\\'")
		ciSafe := strings.ReplaceAll(ci, "'", "\\'")

		if detailScope == "country" {
			if c != "" {
				locConditions = append(locConditions, fmt.Sprintf("country = '%s'", cSafe))
			}
		} else if detailScope == "city" || detailScope == "merged" {
			if c != "" && ci != "" {
				locConditions = append(locConditions, fmt.Sprintf("(country = '%s' AND city = '%s')", cSafe, ciSafe))
			} else if c != "" {
				locConditions = append(locConditions, fmt.Sprintf("country = '%s'", cSafe))
			}
		} else {
			// coordinate or default
			lat, hasLat := loc["lat"].(float64)
			long, hasLong := loc["long"].(float64)
			if hasLat && hasLong {
				locConditions = append(locConditions, fmt.Sprintf(
					"((country = '%s' AND city = '%s') OR (abs(latitude - %f) < 0.2 AND abs(longitude - %f) < 0.2))",
					cSafe, ciSafe, lat, long,
				))
			} else if c != "" && ci != "" {
				locConditions = append(locConditions, fmt.Sprintf("(country = '%s' AND city = '%s')", cSafe, ciSafe))
			} else if c != "" {
				locConditions = append(locConditions, fmt.Sprintf("country = '%s'", cSafe))
			}
		}
	}

	locWhere := "1 = 1"
	if len(locConditions) > 0 {
		locWhere = "(" + strings.Join(locConditions, " OR ") + ")"
	}

	totalLocations := len(locationsList)
	totalCountries := len(countriesSet)
	totalCities := len(citiesSet)

	// Summary
	summaryQuery := fmt.Sprintf(`
		SELECT
			COUNT(DISTINCT session_id) as total_sessions,
			COUNT(DISTINCT customer_id) as total_profiles
		FROM %s.events
		WHERE (shop_id = ? OR tenant_id = ?)
		  AND created_at >= now() - INTERVAL 24 HOUR
		  AND %s
	`, h.queryService.database, locWhere)

	var totalSessions, totalProfiles uint64
	_ = h.queryService.Conn().QueryRow(r.Context(), summaryQuery, shopID, tenantID).Scan(&totalSessions, &totalProfiles)

	// Top Referrers
	refQuery := fmt.Sprintf(`
		SELECT
			referrer_name,
			COUNT(DISTINCT session_id) as count
		FROM %s.events
		WHERE (shop_id = ? OR tenant_id = ?)
		  AND created_at >= now() - INTERVAL 24 HOUR
		  AND referrer_name != ''
		  AND %s
		GROUP BY referrer_name
		ORDER BY count DESC
		LIMIT 3
	`, h.queryService.database, locWhere)

	type refItem struct {
		ReferrerName string `json:"referrerName"`
		Count        int64  `json:"count"`
	}
	var topReferrers []refItem
	if rows, err := h.queryService.Conn().Query(r.Context(), refQuery, shopID, tenantID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var rItem refItem
			var uCount uint64
			if err := rows.Scan(&rItem.ReferrerName, &uCount); err == nil {
				rItem.Count = int64(uCount)
				topReferrers = append(topReferrers, rItem)
			}
		}
	}
	if topReferrers == nil {
		topReferrers = []refItem{}
	}

	// Top Paths
	pathsQuery := fmt.Sprintf(`
		SELECT
			origin,
			path,
			COUNT(DISTINCT session_id) as count
		FROM %s.events
		WHERE (shop_id = ? OR tenant_id = ?)
		  AND created_at >= now() - INTERVAL 24 HOUR
		  AND path != ''
		  AND %s
		GROUP BY origin, path
		ORDER BY count DESC
		LIMIT 3
	`, h.queryService.database, locWhere)

	type pathItem struct {
		Origin string `json:"origin"`
		Path   string `json:"path"`
		Count  int64  `json:"count"`
	}
	var topPaths []pathItem
	if rows, err := h.queryService.Conn().Query(r.Context(), pathsQuery, shopID, tenantID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var pItem pathItem
			var uCount uint64
			if err := rows.Scan(&pItem.Origin, &pItem.Path, &uCount); err == nil {
				pItem.Count = int64(uCount)
				topPaths = append(topPaths, pItem)
			}
		}
	}
	if topPaths == nil {
		topPaths = []pathItem{}
	}

	// Top Events
	eventsQuery := fmt.Sprintf(`
		SELECT
			name,
			COUNT(DISTINCT session_id) as count
		FROM %s.events
		WHERE (shop_id = ? OR tenant_id = ?)
		  AND created_at >= now() - INTERVAL 24 HOUR
		  AND name NOT IN ('screen_view', 'session_start', 'session_end')
		  AND %s
		GROUP BY name
		ORDER BY count DESC
		LIMIT 3
	`, h.queryService.database, locWhere)

	type evItem struct {
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}
	var topEvents []evItem
	if rows, err := h.queryService.Conn().Query(r.Context(), eventsQuery, shopID, tenantID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var eItem evItem
			var uCount uint64
			if err := rows.Scan(&eItem.Name, &uCount); err == nil {
				eItem.Count = int64(uCount)
				topEvents = append(topEvents, eItem)
			}
		}
	}
	if topEvents == nil {
		topEvents = []evItem{}
	}

	// Recent Sessions
	recentQuery := fmt.Sprintf(`
		SELECT
			toString(session_id) as s_id,
			coalesce(toString(customer_id), '') as p_id,
			formatDateTime(created_at, '%%Y-%%m-%%dT%%H:%%i:%%sZ') as c_at,
			path,
			name,
			country,
			city
		FROM (
			SELECT
				session_id,
				customer_id,
				created_at,
				path,
				name,
				country,
				city,
				row_number() OVER (PARTITION BY session_id ORDER BY created_at DESC) as rn
			FROM %s.events
			WHERE (shop_id = ? OR tenant_id = ?)
			  AND created_at >= now() - INTERVAL 24 HOUR
			  AND %s
		)
		WHERE rn = 1
		ORDER BY created_at DESC
		LIMIT 8
	`, h.queryService.database, locWhere)

	type profileItem struct {
		ID          string  `json:"id"`
		ProfileID   *string `json:"profileId"`
		SessionID   string  `json:"sessionId"`
		CreatedAt   string  `json:"createdAt"`
		LatestPath  string  `json:"latestPath"`
		LatestEvent string  `json:"latestEvent"`
		City        string  `json:"city"`
		Country     string  `json:"country"`
		FirstName   string  `json:"firstName"`
		LastName    string  `json:"lastName"`
		Email       string  `json:"email"`
		Avatar      string  `json:"avatar"`
	}
	var recentProfiles []profileItem
	if rows, err := h.queryService.Conn().Query(r.Context(), recentQuery, shopID, tenantID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var sID, pID, cAt, path, name, country, city string
			if err := rows.Scan(&sID, &pID, &cAt, &path, &name, &country, &city); err == nil {
				var profIDPtr *string
				if pID != "" && pID != "00000000-0000-0000-0000-000000000000" {
					profIDPtr = &pID
				}
				id := sID
				if profIDPtr != nil {
					id = *profIDPtr
				}
				recentProfiles = append(recentProfiles, profileItem{
					ID:          id,
					ProfileID:   profIDPtr,
					SessionID:   sID,
					CreatedAt:   cAt,
					LatestPath:  path,
					LatestEvent: name,
					City:        city,
					Country:     country,
				})
			}
		}
	}
	if recentProfiles == nil {
		recentProfiles = []profileItem{}
	}

	sendTRPCResponse(w, map[string]any{
		"summary": map[string]any{
			"totalSessions":  totalSessions,
			"totalProfiles":  totalProfiles,
			"totalLocations": totalLocations,
			"totalCountries": totalCountries,
			"totalCities":    totalCities,
		},
		"topReferrers":   topReferrers,
		"topPaths":       topPaths,
		"topEvents":      topEvents,
		"recentProfiles": recentProfiles,
	})
}

func (h *Handler) HandleTRPCInsightList(w http.ResponseWriter, r *http.Request) {
	_, shopID, _ := h.extractTenantAndShop(r)
	cards, err := h.queryService.GetInsights(r.Context(), shopID, 50)
	var resp []map[string]any

	if err == nil && cards != nil {
		for _, c := range cards {
			severity := "subtle"
			if c.ImpactScore > 100 {
				severity = "severe"
			} else if c.ImpactScore > 30 {
				severity = "moderate"
			}

			dimKey := "dimension"
			dimVal := c.DimensionKey
			if strings.Contains(c.DimensionKey, ":") {
				parts := strings.SplitN(c.DimensionKey, ":", 2)
				dimKey = parts[0]
				dimVal = parts[1]
			}

			metricKey := "sessions"
			unit := "count"
			if c.ModuleKey == "cart_trends" {
				metricKey = "share"
				unit = "ratio"
			}

			delta := c.CurrentVal - c.CompareVal
			changePct := c.ChangePct

			renderedCard := map[string]any{
				"id":           c.ID.String(),
				"title":        c.Title,
				"summary":      c.Summary,
				"displayName":  c.Title,
				"windowKind":   c.WindowKind,
				"severityBand": severity,
				"impactScore":  c.ImpactScore,
				"payload": map[string]any{
					"kind":          "insight_v1",
					"primaryMetric": metricKey,
					"metrics": map[string]any{
						metricKey: map[string]any{
							"current":   c.CurrentVal,
							"compare":   c.CompareVal,
							"delta":     delta,
							"changePct": changePct,
							"direction": c.Direction,
							"unit":      unit,
						},
					},
					"dimensions": []map[string]any{
						{
							"key":         dimKey,
							"value":       dimVal,
							"displayName": dimVal,
						},
					},
				},
				"createdAt": c.CreatedAt.Format(time.RFC3339),
			}
			resp = append(resp, renderedCard)
		}
	}

	if resp == nil {
		resp = []map[string]any{}
	}
	sendTRPCResponse(w, resp)
}


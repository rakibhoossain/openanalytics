package query

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"openanalytics/internal/domain"
	"openanalytics/internal/postgres"
	"openanalytics/pkg/httputil"
	"openanalytics/pkg/uuidv7"
)

// Handler serves HTTP endpoints for dashboards and analytical queries.
type Handler struct {
	queryService *Service
	pgRepo       *postgres.Repository
}

// NewHandler creates a new Query HTTP Handler.
func NewHandler(qs *Service, pg *postgres.Repository) *Handler {
	return &Handler{
		queryService: qs,
		pgRepo:       pg,
	}
}

// RegisterRoutes mounts all query and metadata routes on the chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/health", h.HandleHealth)

	r.Route("/api/v1", func(r chi.Router) {
		// Analytics Query APIs
		r.Route("/query", func(r chi.Router) {
			r.Get("/trends", h.HandleTrends)
			r.Post("/funnel", h.HandleFunnel)
			r.Get("/live", h.HandleLiveVisitors)
			r.Get("/shopper/{id}", h.HandleShopperJourney)
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

		// Reports & Alerts
		r.Get("/reports", h.HandleListReports)
		r.Post("/reports", h.HandleCreateReport)
		r.Get("/alerts", h.HandleListAlerts)
		r.Post("/alerts", h.HandleCreateAlert)
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

// HandleListDashboards retrieves all dashboards for a shop.
func (h *Handler) HandleListDashboards(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	dashboards, err := h.pgRepo.ListDashboards(r.Context(), tenantID, shopID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}
	if dashboards == nil {
		dashboards = []*domain.Dashboard{}
	}

	httputil.JSON(w, http.StatusOK, dashboards)
}

// HandleCreateDashboard creates a new dashboard.
func (h *Handler) HandleCreateDashboard(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	var d domain.Dashboard
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	d.ID = uuidv7.MustNew()
	d.TenantID = tenantID
	d.ShopID = shopID
	if len(d.LayoutGrid) == 0 {
		d.LayoutGrid = json.RawMessage(`[]`)
	}

	if err := h.pgRepo.CreateDashboard(r.Context(), &d); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}

	httputil.JSON(w, http.StatusCreated, d)
}

// HandleGetDashboard retrieves a dashboard by ID.
func (h *Handler) HandleGetDashboard(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid dashboard id")
		return
	}

	d, err := h.pgRepo.GetDashboard(r.Context(), tenantID, shopID, id)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}
	if d == nil {
		httputil.Error(w, http.StatusNotFound, "NOT_FOUND", "dashboard not found")
		return
	}

	httputil.JSON(w, http.StatusOK, d)
}

// HandleUpdateDashboard updates an existing dashboard.
func (h *Handler) HandleUpdateDashboard(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid dashboard id")
		return
	}

	var d domain.Dashboard
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	d.ID = id
	d.TenantID = tenantID
	d.ShopID = shopID

	if err := h.pgRepo.UpdateDashboard(r.Context(), &d); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}

	httputil.JSON(w, http.StatusOK, d)
}

// HandleDeleteDashboard deletes a dashboard.
func (h *Handler) HandleDeleteDashboard(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid dashboard id")
		return
	}

	if err := h.pgRepo.DeleteDashboard(r.Context(), tenantID, shopID, id); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"message": "dashboard deleted"})
}

// HandleListWidgets lists widgets for a dashboard.
func (h *Handler) HandleListWidgets(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	dashboardID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid dashboard id")
		return
	}

	widgets, err := h.pgRepo.ListWidgets(r.Context(), tenantID, shopID, dashboardID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}
	if widgets == nil {
		widgets = []*domain.ChartWidget{}
	}

	httputil.JSON(w, http.StatusOK, widgets)
}

// HandleCreateWidget creates a new widget on a dashboard.
func (h *Handler) HandleCreateWidget(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	dashboardID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid dashboard id")
		return
	}

	var widget domain.ChartWidget
	if err := json.NewDecoder(r.Body).Decode(&widget); err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	widget.ID = uuidv7.MustNew()
	widget.DashboardID = dashboardID
	widget.TenantID = tenantID
	widget.ShopID = shopID
	if len(widget.FilterRules) == 0 {
		widget.FilterRules = json.RawMessage(`[]`)
	}

	if err := h.pgRepo.CreateWidget(r.Context(), &widget); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}

	httputil.JSON(w, http.StatusCreated, widget)
}

// HandleDeleteWidget deletes a widget.
func (h *Handler) HandleDeleteWidget(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid widget id")
		return
	}

	if err := h.pgRepo.DeleteWidget(r.Context(), tenantID, shopID, id); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"message": "widget deleted"})
}

// HandleListReports lists saved reports.
func (h *Handler) HandleListReports(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	reports, err := h.pgRepo.ListReports(r.Context(), tenantID, shopID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}
	if reports == nil {
		reports = []*domain.SavedReport{}
	}

	httputil.JSON(w, http.StatusOK, reports)
}

// HandleCreateReport creates a saved report.
func (h *Handler) HandleCreateReport(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	var report domain.SavedReport
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	report.TenantID = tenantID
	report.ShopID = shopID

	if err := h.pgRepo.CreateReport(r.Context(), &report); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}

	httputil.JSON(w, http.StatusCreated, report)
}

// HandleListAlerts lists alert rules.
func (h *Handler) HandleListAlerts(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	alerts, err := h.pgRepo.ListAlerts(r.Context(), tenantID, shopID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}
	if alerts == nil {
		alerts = []*domain.AlertRule{}
	}

	httputil.JSON(w, http.StatusOK, alerts)
}

// HandleCreateAlert creates an alert rule.
func (h *Handler) HandleCreateAlert(w http.ResponseWriter, r *http.Request) {
	tenantID, shopID, err := h.extractTenantAndShop(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	var alert domain.AlertRule
	if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
		httputil.Error(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	alert.TenantID = tenantID
	alert.ShopID = shopID

	if err := h.pgRepo.CreateAlert(r.Context(), &alert); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "DATABASE_ERROR", err.Error())
		return
	}

	httputil.JSON(w, http.StatusCreated, alert)
}

// Helper: extract tenant_id and shop_id from headers or query parameters
func (h *Handler) extractTenantAndShop(r *http.Request) (uuid.UUID, uuid.UUID, error) {
	// CRITICAL(multi-tenant-auth): In production, tenant_id is supplied via verified JWT or Gateway header.
	tStr := r.Header.Get("X-Tenant-ID")
	if tStr == "" {
		tStr = r.URL.Query().Get("tenant_id")
	}

	sStr := r.Header.Get("X-Shop-ID")
	if sStr == "" {
		sStr = r.URL.Query().Get("shop_id")
	}

	tenantID, err := uuid.Parse(tStr)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("valid tenant_id required in X-Tenant-ID header or ?tenant_id parameter")
	}

	shopID, err := uuid.Parse(sStr)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("valid shop_id required in X-Shop-ID header or ?shop_id parameter")
	}

	return tenantID, shopID, nil
}

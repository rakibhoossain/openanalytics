package query

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

	"openanalytics/internal/cron"
	"openanalytics/internal/domain"
)

// Service provides analytical query execution across ClickHouse tables.
type Service struct {
	conn     driver.Conn
	database string
}

// Config holds options for the analytics query service.
type Config struct {
	Addr     string
	Database string
	Username string
	Password string
}

// NewService connects to ClickHouse via native TCP for querying.
func NewService(ctx context.Context, cfg Config) (*Service, error) {
	if cfg.Database == "" {
		cfg.Database = "openpanel"
	}

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 30,
		},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open clickhouse connection: %w", err)
	}

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping clickhouse at %s: %w", cfg.Addr, err)
	}

	return &Service{
		conn:     conn,
		database: cfg.Database,
	}, nil
}

// Close closes ClickHouse connection.
func (s *Service) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// --- Query Implementations ---

// GetTrends computes time-series aggregated metrics for a merchant shop.
func (s *Service) GetTrends(ctx context.Context, tenantID, shopID uuid.UUID, metric, timeRange, interval string) ([]domain.TrendDataPoint, error) {
	// CRITICAL(tenant-isolation): Enforce tenant_id and shop_id on every ClickHouse query.
	// ClickHouse ORDER BY (tenant_id, shop_id, ...) allows primary index skipping for ultra-fast scans.
	now := time.Now().UTC()
	var startTime time.Time

	switch timeRange {
	case "today":
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case "24h":
		startTime = now.Add(-24 * time.Hour)
	case "30d":
		startTime = now.Add(-30 * 24 * time.Hour)
	case "7d":
		fallthrough
	default:
		startTime = now.Add(-7 * 24 * time.Hour)
	}

	dateTrunc := "toStartOfHour"
	if interval == "day" || timeRange == "30d" {
		dateTrunc = "toStartOfDay"
	}

	var metricExpr string
	var whereClause string

	switch metric {
	case "unique_visitors":
		metricExpr = "uniq(device_id)"
	case "cart_adds":
		metricExpr = "count()"
		whereClause = "AND name = 'add_to_cart'"
	case "orders":
		metricExpr = "count()"
		whereClause = "AND name IN ('purchase', 'order_completed')"
	case "revenue":
		metricExpr = "coalesce(sum(revenue), 0)"
		whereClause = "AND revenue IS NOT NULL"
	case "page_views":
		fallthrough
	default:
		metricExpr = "count()"
		whereClause = "AND name = 'page_view'"
	}

	query := fmt.Sprintf(`
		SELECT 
			toString(%s(created_at)) AS bucket,
			toFloat64(%s) AS value
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ? AND created_at >= ? %s
		GROUP BY bucket
		ORDER BY bucket ASC
	`, dateTrunc, metricExpr, s.database, whereClause)

	rows, err := s.conn.Query(ctx, query, tenantID, shopID, startTime)
	if err != nil {
		return nil, fmt.Errorf("clickhouse trends query error: %w", err)
	}
	defer rows.Close()

	var points []domain.TrendDataPoint
	for rows.Next() {
		var pt domain.TrendDataPoint
		if err := rows.Scan(&pt.Timestamp, &pt.Value); err != nil {
			return nil, err
		}
		points = append(points, pt)
	}

	return points, rows.Err()
}

// GetFunnel calculates conversion rates through a sequence of steps.
func (s *Service) GetFunnel(ctx context.Context, tenantID, shopID uuid.UUID, timeRange string, steps []string) (*domain.FunnelResult, error) {
	if len(steps) == 0 {
		steps = []string{"page_view", "add_to_cart", "checkout_step", "purchase"}
	}

	// CRITICAL(funnel-step-limit): Limit steps to maximum 10 to avoid combinatorial explosion in windowFunnel.
	if len(steps) > 10 {
		steps = steps[:10]
	}

	now := time.Now().UTC()
	var startTime time.Time
	switch timeRange {
	case "24h":
		startTime = now.Add(-24 * time.Hour)
	case "30d":
		startTime = now.Add(-30 * 24 * time.Hour)
	case "7d":
		fallthrough
	default:
		startTime = now.Add(-7 * 24 * time.Hour)
	}

	// Build windowFunnel conditions: windowFunnel(86400)(toDateTime(created_at), name = 's1', name = 's2', ...)
	conds := make([]string, len(steps))
	for i, step := range steps {
		escaped := strings.ReplaceAll(step, "'", "\\'")
		conds[i] = fmt.Sprintf("name = '%s'", escaped)
	}

	// Window: 24 hours (86400 seconds) for completion
	query := fmt.Sprintf(`
		SELECT 
			level,
			count() as users
		FROM (
			SELECT 
				device_id,
				windowFunnel(86400)(toDateTime(created_at), %s) as level
			FROM %s.events
			WHERE tenant_id = ? AND shop_id = ? AND created_at >= ?
			GROUP BY device_id
		)
		WHERE level > 0
		GROUP BY level
		ORDER BY level ASC
	`, strings.Join(conds, ", "), s.database)

	rows, err := s.conn.Query(ctx, query, tenantID, shopID, startTime)
	if err != nil {
		return nil, fmt.Errorf("clickhouse funnel query error: %w", err)
	}
	defer rows.Close()

	// Level counts: level X reached means user reached at least level X
	// In ClickHouse windowFunnel, level represents maximum stage reached.
	maxLevels := make(map[int]int64)
	for rows.Next() {
		var level uint8 // CRITICAL(clickhouse-uint8): ClickHouse windowFunnel returns UInt8
		var users uint64 // CRITICAL(clickhouse-uint64): ClickHouse count() returns UInt64
		if err := rows.Scan(&level, &users); err != nil {
			return nil, err
		}
		maxLevels[int(level)] = int64(users)
	}

	// Calculate cumulative reaches: reach(i) = sum(users where level >= i)
	cumulative := make([]int64, len(steps))
	for i := range steps {
		stepLevel := i + 1
		var sum int64
		for l, cnt := range maxLevels {
			if l >= stepLevel {
				sum += cnt
			}
		}
		cumulative[i] = sum
	}

	var totalVisitors int64
	if len(cumulative) > 0 {
		totalVisitors = cumulative[0]
	}

	funnelSteps := make([]domain.FunnelStepResult, len(steps))
	for i, name := range steps {
		cnt := cumulative[i]
		var dropoff int64
		var convRate float64

		if i == 0 {
			convRate = 100.0
			if len(steps) > 1 {
				dropoff = totalVisitors - cumulative[1]
			}
		} else {
			prev := cumulative[i-1]
			if prev > 0 {
				convRate = (float64(cnt) / float64(prev)) * 100.0
				dropoff = prev - cnt
			}
		}

		funnelSteps[i] = domain.FunnelStepResult{
			StepIndex:      i + 1,
			Name:           name,
			Count:          cnt,
			DropoffCount:   dropoff,
			ConversionRate: convRate,
		}
	}

	var overallRate float64
	if totalVisitors > 0 && len(cumulative) > 0 {
		overallRate = (float64(cumulative[len(cumulative)-1]) / float64(totalVisitors)) * 100.0
	}

	return &domain.FunnelResult{
		Steps:                 funnelSteps,
		TotalVisitors:         totalVisitors,
		OverallConversionRate: overallRate,
	}, nil
}

// GetLiveVisitors calculates real-time active shopper counts, devices, and countries.
func (s *Service) GetLiveVisitors(ctx context.Context, tenantID, shopID uuid.UUID, windowMinutes int) (*domain.LiveVisitorsResult, error) {
	if windowMinutes <= 0 || windowMinutes > 60 {
		windowMinutes = 5
	}
	cutoff := time.Now().UTC().Add(-time.Duration(windowMinutes) * time.Minute)

	// 1. Total distinct active shoppers
	// CRITICAL(clickhouse-uint64): ClickHouse uniq() and count() return UInt64
	totalQuery := fmt.Sprintf(`
		SELECT uniq(device_id) 
		FROM %s.events 
		WHERE tenant_id = ? AND shop_id = ? AND created_at >= ?
	`, s.database)

	var totalActive uint64
	if err := s.conn.QueryRow(ctx, totalQuery, tenantID, shopID, cutoff).Scan(&totalActive); err != nil {
		return nil, fmt.Errorf("failed to query active shoppers: %w", err)
	}

	// 2. Device breakdown
	devices := make(map[string]int64)
	deviceQuery := fmt.Sprintf(`
		SELECT device, count() 
		FROM %s.events 
		WHERE tenant_id = ? AND shop_id = ? AND created_at >= ? AND device != ''
		GROUP BY device
	`, s.database)
	if devRows, err := s.conn.Query(ctx, deviceQuery, tenantID, shopID, cutoff); err == nil {
		defer devRows.Close()
		for devRows.Next() {
			var dev string
			var cnt uint64
			if err := devRows.Scan(&dev, &cnt); err == nil {
				devices[dev] = int64(cnt)
			}
		}
	}

	// 3. Country breakdown
	countries := make(map[string]int64)
	countryQuery := fmt.Sprintf(`
		SELECT country, count() 
		FROM %s.events 
		WHERE tenant_id = ? AND shop_id = ? AND created_at >= ? AND country != ''
		GROUP BY country
		ORDER BY count() DESC
		LIMIT 10
	`, s.database)
	if cRows, err := s.conn.Query(ctx, countryQuery, tenantID, shopID, cutoff); err == nil {
		defer cRows.Close()
		for cRows.Next() {
			var c string
			var cnt uint64
			if err := cRows.Scan(&c, &cnt); err == nil {
				countries[c] = int64(cnt)
			}
		}
	}

	// 4. Top active paths
	var topPaths []domain.PathCount
	pathQuery := fmt.Sprintf(`
		SELECT path, count() 
		FROM %s.events 
		WHERE tenant_id = ? AND shop_id = ? AND created_at >= ? AND path != ''
		GROUP BY path
		ORDER BY count() DESC
		LIMIT 5
	`, s.database)
	if pRows, err := s.conn.Query(ctx, pathQuery, tenantID, shopID, cutoff); err == nil {
		defer pRows.Close()
		for pRows.Next() {
			var p string
			var cnt uint64
			if err := pRows.Scan(&p, &cnt); err == nil {
				topPaths = append(topPaths, domain.PathCount{Path: p, Count: int64(cnt)})
			}
		}
	}

	return &domain.LiveVisitorsResult{
		ActiveShoppers: int64(totalActive),
		WindowMinutes:  windowMinutes,
		Devices:        devices,
		Countries:      countries,
		TopPaths:       topPaths,
	}, nil
}

// GetShopperJourney returns the complete chronological event stream and sessions for a shopper.
func (s *Service) GetShopperJourney(ctx context.Context, tenantID, shopID uuid.UUID, identifier string) (*domain.ShopperJourneyResult, error) {
	// WEAK_POINT(identifier-resolution): Supports either device_id or customer_id UUID string.
	custID, parseErr := uuid.Parse(identifier)

	var whereClause string
	var args []any
	if parseErr == nil {
		whereClause = "(customer_id = ? OR device_id = ?)"
		args = []any{tenantID, shopID, custID, identifier}
	} else {
		whereClause = "device_id = ?"
		args = []any{tenantID, shopID, identifier}
	}

	query := fmt.Sprintf(`
		SELECT 
			id, tenant_id, shop_id, name, device_id, customer_id, session_id,
			coalesce(revenue, 0), currency, product_id, cart_id, order_id,
			path, origin, referrer, referrer_name, referrer_type,
			os, browser, device, country, city, created_at
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ? AND %s
		ORDER BY created_at ASC
		LIMIT 200
	`, s.database, whereClause)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query shopper events: %w", err)
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
			&ev.OS, &ev.Browser, &ev.Device, &ev.Country, &ev.City, &ev.CreatedAt,
		); err != nil {
			return nil, err
		}
		if revCents > 0 {
			ev.Revenue = &revCents
		}
		events = append(events, ev)
	}

	var resCustomerID *uuid.UUID
	if parseErr == nil {
		resCustomerID = &custID
	}

	return &domain.ShopperJourneyResult{
		DeviceID:   identifier,
		CustomerID: resCustomerID,
		Events:     events,
		Sessions:   nil, // Populated via sessions query if needed
	}, nil
}

// Conn returns the underlying ClickHouse native driver connection.
func (s *Service) Conn() driver.Conn {
	return s.conn
}

// GetInsights retrieves the latest automated anomaly and intelligence cards for a shop.
func (s *Service) GetInsights(ctx context.Context, shopID uuid.UUID, limit int) ([]*cron.InsightCard, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	query := fmt.Sprintf(`
		SELECT
			id, shop_id, tenant_id, module_key, dimension_key, window_kind,
			title, summary, direction, change_pct, current_val, compare_val,
			impact_score, created_at
		FROM %s.project_insights
		WHERE shop_id = ?
		ORDER BY impact_score DESC, created_at DESC
		LIMIT ?;
	`, s.database)

	rows, err := s.conn.Query(ctx, query, shopID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query project insights: %w", err)
	}
	defer rows.Close()

	var cards []*cron.InsightCard
	for rows.Next() {
		var c cron.InsightCard
		err := rows.Scan(
			&c.ID,
			&c.ShopID,
			&c.TenantID,
			&c.ModuleKey,
			&c.DimensionKey,
			&c.WindowKind,
			&c.Title,
			&c.Summary,
			&c.Direction,
			&c.ChangePct,
			&c.CurrentVal,
			&c.CompareVal,
			&c.ImpactScore,
			&c.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan insight card: %w", err)
		}
		cards = append(cards, &c)
	}

	return cards, nil
}

// resolveTimeRange calculates the UTC start and end bounds for the current and previous comparison periods.
func resolveTimeRange(timeRange, startDateStr, endDateStr string) (time.Time, time.Time, time.Time, time.Time) {
	now := time.Now().UTC()
	var curStart, curEnd time.Time

	if startDateStr != "" && endDateStr != "" {
		s, err1 := time.Parse(time.RFC3339, startDateStr)
		e, err2 := time.Parse(time.RFC3339, endDateStr)
		if err1 == nil && err2 == nil {
			curStart = s.UTC()
			curEnd = e.UTC()
		}
	}

	if curStart.IsZero() {
		switch timeRange {
		case "30min":
			curStart = now.Add(-30 * time.Minute)
			curEnd = now
		case "lastHour":
			curStart = now.Add(-1 * time.Hour)
			curEnd = now
		case "last24h", "24h":
			curStart = now.Add(-24 * time.Hour)
			curEnd = now
		case "today":
			curStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			curEnd = now
		case "30d":
			curStart = now.Add(-30 * 24 * time.Hour)
			curEnd = now
		case "7d":
			fallthrough
		default:
			curStart = now.Add(-7 * 24 * time.Hour)
			curEnd = now
		}
	}

	duration := curEnd.Sub(curStart)
	prevStart := curStart.Add(-duration)
	prevEnd := curStart

	return curStart, curEnd, prevStart, prevEnd
}

// resolveIntervalFunc maps interval strings to ClickHouse date grouping expressions.
func resolveIntervalFunc(interval, timeRange string) string {
	switch interval {
	case "minute":
		return "toStartOfMinute"
	case "hour":
		return "toStartOfHour"
	case "day":
		return "toStartOfDay"
	case "week":
		return "toStartOfWeek"
	case "month":
		return "toStartOfMonth"
	default:
		if timeRange == "30min" || timeRange == "lastHour" {
			return "toStartOfMinute"
		} else if timeRange == "today" || timeRange == "last24h" || timeRange == "24h" {
			return "toStartOfHour"
		}
		return "toStartOfDay"
	}
}

// GetOverviewStats retrieves multi-metric overview KPIs and time-series for current & previous periods.
func (s *Service) GetOverviewStats(ctx context.Context, tenantID, shopID uuid.UUID, timeRange, interval, startDateStr, endDateStr string) (*domain.OverviewStatsResult, error) {
	curStart, curEnd, prevStart, prevEnd := resolveTimeRange(timeRange, startDateStr, endDateStr)
	dateTrunc := resolveIntervalFunc(interval, timeRange)

	// 1. Current period aggregated metrics
	queryCurMetrics := fmt.Sprintf(`
		SELECT 
			uniq(device_id) AS unique_visitors,
			count() AS total_sessions,
			coalesce(round(countIf(events_count <= 1 OR duration = 0) * 100.0 / nullIf(count(), 0), 2), 0.0) AS bounce_rate,
			coalesce(round(avgIf(duration, duration > 0), 2), 0.0) AS avg_session_duration,
			coalesce(sum(total_revenue), toInt64(0)) AS total_revenue
		FROM %s.sessions
		WHERE tenant_id = ? AND shop_id = ? AND started_at BETWEEN ? AND ?
	`, s.database)

	var curUniqueVisitors, curTotalSessions uint64
	var curTotalRevenue int64
	var curBounceRate, curAvgDuration float64
	_ = s.conn.QueryRow(ctx, queryCurMetrics, tenantID, shopID, curStart, curEnd).Scan(
		&curUniqueVisitors, &curTotalSessions, &curBounceRate, &curAvgDuration, &curTotalRevenue,
	)

	// Current screen views
	queryCurViews := fmt.Sprintf(`
		SELECT count()
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ? AND created_at BETWEEN ? AND ?
		  AND (name = 'page_view' OR name = 'screen_view')
	`, s.database)
	var curScreenViews uint64
	_ = s.conn.QueryRow(ctx, queryCurViews, tenantID, shopID, curStart, curEnd).Scan(&curScreenViews)

	var curViewsPerSession float64
	if curTotalSessions > 0 {
		curViewsPerSession = float64(curScreenViews) / float64(curTotalSessions)
	}

	// 2. Previous period aggregated metrics
	var prevUniqueVisitors, prevTotalSessions uint64
	var prevTotalRevenue int64
	var prevBounceRate, prevAvgDuration float64
	_ = s.conn.QueryRow(ctx, queryCurMetrics, tenantID, shopID, prevStart, prevEnd).Scan(
		&prevUniqueVisitors, &prevTotalSessions, &prevBounceRate, &prevAvgDuration, &prevTotalRevenue,
	)

	var prevScreenViews uint64
	_ = s.conn.QueryRow(ctx, queryCurViews, tenantID, shopID, prevStart, prevEnd).Scan(&prevScreenViews)

	var prevViewsPerSession float64
	if prevTotalSessions > 0 {
		prevViewsPerSession = float64(prevScreenViews) / float64(prevTotalSessions)
	}

	curBounceRate = sanitizeFloat(curBounceRate)
	curAvgDuration = sanitizeFloat(curAvgDuration)
	curViewsPerSession = sanitizeFloat(curViewsPerSession)
	prevBounceRate = sanitizeFloat(prevBounceRate)
	prevAvgDuration = sanitizeFloat(prevAvgDuration)
	prevViewsPerSession = sanitizeFloat(prevViewsPerSession)

	// 3. Current period time-series from events & sessions
	queryCurSeries := fmt.Sprintf(`
		SELECT 
			toString(%s(created_at)) AS bucket,
			uniq(device_id) AS unique_visitors,
			uniq(session_id) AS total_sessions,
			countIf(name = 'page_view' OR name = 'screen_view') AS screen_views,
			coalesce(sum(revenue), toInt64(0)) AS revenue
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ? AND created_at BETWEEN ? AND ?
		GROUP BY bucket
		ORDER BY bucket ASC
	`, dateTrunc, s.database)

	rowsCur, err := s.conn.Query(ctx, queryCurSeries, tenantID, shopID, curStart, curEnd)
	if err != nil {
		return nil, fmt.Errorf("overview series query failed: %w", err)
	}
	defer rowsCur.Close()

	seriesMap := make(map[string]*domain.OverviewSeriesPoint)
	var seriesOrder []string

	for rowsCur.Next() {
		var bucket string
		var uv, sess, views uint64
		var rev int64
		if err := rowsCur.Scan(&bucket, &uv, &sess, &views, &rev); err == nil {
			var vps float64
			if sess > 0 {
				vps = float64(views) / float64(sess)
			}
			pt := &domain.OverviewSeriesPoint{
				Date:             bucket,
				UniqueVisitors:   int64(uv),
				TotalSessions:    int64(sess),
				TotalScreenViews: int64(views),
				ViewsPerSession:  sanitizeFloat(vps),
				TotalRevenue:     rev,
			}
			seriesMap[bucket] = pt
			seriesOrder = append(seriesOrder, bucket)
		}
	}

	// Query previous period series
	queryPrevSeries := fmt.Sprintf(`
		SELECT 
			toString(%s(created_at)) AS bucket,
			uniq(device_id) AS unique_visitors,
			uniq(session_id) AS total_sessions,
			countIf(name = 'page_view' OR name = 'screen_view') AS screen_views,
			coalesce(sum(revenue), toInt64(0)) AS revenue
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ? AND created_at BETWEEN ? AND ?
		GROUP BY bucket
		ORDER BY bucket ASC
	`, dateTrunc, s.database)

	var prevPoints []*domain.OverviewSeriesPoint
	if rowsPrev, err := s.conn.Query(ctx, queryPrevSeries, tenantID, shopID, prevStart, prevEnd); err == nil {
		defer rowsPrev.Close()
		for rowsPrev.Next() {
			var bucket string
			var uv, sess, views uint64
			var rev int64
			if err := rowsPrev.Scan(&bucket, &uv, &sess, &views, &rev); err == nil {
				var vps float64
				if sess > 0 {
					vps = float64(views) / float64(sess)
				}
				prevPoints = append(prevPoints, &domain.OverviewSeriesPoint{
					Date:             bucket,
					UniqueVisitors:   int64(uv),
					TotalSessions:    int64(sess),
					TotalScreenViews: int64(views),
					ViewsPerSession:  sanitizeFloat(vps),
					TotalRevenue:     rev,
				})
			}
		}
	}

	// Correlate current points with aligned previous points
	finalSeries := make([]domain.OverviewSeriesPoint, 0)
	for i, bucket := range seriesOrder {
		pt := seriesMap[bucket]
		if i < len(prevPoints) {
			prevPt := prevPoints[i]
			pt.PrevUniqueVisitors = &prevPt.UniqueVisitors
			pt.PrevTotalSessions = &prevPt.TotalSessions
			pt.PrevTotalScreenViews = &prevPt.TotalScreenViews
			pt.PrevViewsPerSession = &prevPt.ViewsPerSession
			pt.PrevTotalRevenue = &prevPt.TotalRevenue
			pt.PrevBounceRate = &prevPt.BounceRate
			pt.PrevAvgSessionDuration = &prevPt.AvgSessionDuration
		}
		finalSeries = append(finalSeries, *pt)
	}

	pUV := int64(prevUniqueVisitors)
	pSess := int64(prevTotalSessions)
	pViews := int64(prevScreenViews)

	return &domain.OverviewStatsResult{
		Metrics: domain.OverviewMetrics{
			BounceRate:             curBounceRate,
			UniqueVisitors:         int64(curUniqueVisitors),
			TotalSessions:          int64(curTotalSessions),
			AvgSessionDuration:     curAvgDuration,
			TotalScreenViews:       int64(curScreenViews),
			ViewsPerSession:        curViewsPerSession,
			TotalRevenue:           curTotalRevenue,
			PrevBounceRate:         &prevBounceRate,
			PrevUniqueVisitors:     &pUV,
			PrevTotalSessions:      &pSess,
			PrevAvgSessionDuration: &prevAvgDuration,
			PrevTotalScreenViews:   &pViews,
			PrevViewsPerSession:    &prevViewsPerSession,
			PrevTotalRevenue:       &prevTotalRevenue,
		},
		Series: finalSeries,
	}, nil
}

// sanitizeFloat replaces NaN and Inf with 0.0.
func sanitizeFloat(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0.0
	}
	return f
}

// GetTopGeneric aggregates dimensions like device, browser, os, or referrer_name.
func (s *Service) GetTopGeneric(ctx context.Context, tenantID, shopID uuid.UUID, column, timeRange, startDateStr, endDateStr string, limit int) ([]domain.TopItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	// Whitelist column to prevent SQL injection
	allowed := map[string]bool{
		"device":        true,
		"browser":       true,
		"os":            true,
		"referrer_name": true,
		"country":       true,
		"city":          true,
	}
	if !allowed[column] {
		column = "device"
	}

	curStart, curEnd, _, _ := resolveTimeRange(timeRange, startDateStr, endDateStr)

	query := fmt.Sprintf(`
		SELECT 
			coalesce(nullIf(%s, ''), 'Unknown') AS name,
			uniq(session_id) AS sessions,
			countIf(name = 'page_view' OR name = 'screen_view') AS pageviews,
			coalesce(sum(revenue), toInt64(0)) AS revenue
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ? AND created_at BETWEEN ? AND ?
		GROUP BY name
		ORDER BY sessions DESC
		LIMIT ?
	`, column, s.database)

	rows, err := s.conn.Query(ctx, query, tenantID, shopID, curStart, curEnd, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top %s: %w", column, err)
	}
	defer rows.Close()

	var items []domain.TopItem
	for rows.Next() {
		var name string
		var sess, pvs uint64
		var rev int64
		if err := rows.Scan(&name, &sess, &pvs, &rev); err == nil {
			items = append(items, domain.TopItem{
				Name:      name,
				Sessions:  int64(sess),
				Pageviews: int64(pvs),
				Revenue:   rev,
			})
		}
	}

	return items, nil
}

// GetTopPages aggregates top visited paths, entry paths, or exit paths.
func (s *Service) GetTopPages(ctx context.Context, tenantID, shopID uuid.UUID, mode, timeRange, startDateStr, endDateStr string, limit int) ([]domain.TopItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	curStart, curEnd, _, _ := resolveTimeRange(timeRange, startDateStr, endDateStr)

	var query string
	if mode == "entry" {
		query = fmt.Sprintf(`
			SELECT 
				coalesce(nullIf(entry_path, ''), '/') AS name,
				count() AS sessions,
				sum(events_count) AS pageviews,
				coalesce(sum(total_revenue), toInt64(0)) AS revenue
			FROM %s.sessions
			WHERE tenant_id = ? AND shop_id = ? AND started_at BETWEEN ? AND ?
			  AND entry_path != ''
			GROUP BY name
			ORDER BY sessions DESC
			LIMIT ?
		`, s.database)
	} else if mode == "exit" {
		query = fmt.Sprintf(`
			SELECT 
				coalesce(nullIf(exit_path, ''), '/') AS name,
				count() AS sessions,
				sum(events_count) AS pageviews,
				coalesce(sum(total_revenue), toInt64(0)) AS revenue
			FROM %s.sessions
			WHERE tenant_id = ? AND shop_id = ? AND ended_at BETWEEN ? AND ?
			  AND exit_path != ''
			GROUP BY name
			ORDER BY sessions DESC
			LIMIT ?
		`, s.database)
	} else {
		// mode == "page"
		query = fmt.Sprintf(`
			SELECT 
				coalesce(nullIf(path, ''), '/') AS name,
				uniq(session_id) AS sessions,
				count() AS pageviews,
				coalesce(sum(revenue), toInt64(0)) AS revenue
			FROM %s.events
			WHERE tenant_id = ? AND shop_id = ? AND created_at BETWEEN ? AND ?
			  AND (name = 'page_view' OR name = 'screen_view' OR path != '')
			GROUP BY name
			ORDER BY pageviews DESC
			LIMIT ?
		`, s.database)
	}

	rows, err := s.conn.Query(ctx, query, tenantID, shopID, curStart, curEnd, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top pages (%s): %w", mode, err)
	}
	defer rows.Close()

	var items []domain.TopItem
	for rows.Next() {
		var name string
		var sess, pvs uint64
		var rev int64
		if err := rows.Scan(&name, &sess, &pvs, &rev); err == nil {
			items = append(items, domain.TopItem{
				Name:      name,
				Sessions:  int64(sess),
				Pageviews: int64(pvs),
				Revenue:   rev,
			})
		}
	}

	return items, nil
}

// GetTopEvents aggregates event occurrences and unique user reach.
func (s *Service) GetTopEvents(ctx context.Context, tenantID, shopID uuid.UUID, timeRange, startDateStr, endDateStr string, limit int) ([]domain.TopEventItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	curStart, curEnd, _, _ := resolveTimeRange(timeRange, startDateStr, endDateStr)

	query := fmt.Sprintf(`
		SELECT 
			name,
			count() AS count,
			uniq(device_id) AS unique_visitors
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ? AND created_at BETWEEN ? AND ?
		GROUP BY name
		ORDER BY count DESC
		LIMIT ?
	`, s.database)

	rows, err := s.conn.Query(ctx, query, tenantID, shopID, curStart, curEnd, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top events: %w", err)
	}
	defer rows.Close()

	var items []domain.TopEventItem
	for rows.Next() {
		var name string
		var cnt, uv uint64
		if err := rows.Scan(&name, &cnt, &uv); err == nil {
			items = append(items, domain.TopEventItem{
				Name:           name,
				Count:          int64(cnt),
				UniqueVisitors: int64(uv),
			})
		}
	}

	return items, nil
}

// GetOverviewLiveData returns real-time minute-by-minute shopper counts and top referrers in the last 30 minutes.
func (s *Service) GetOverviewLiveData(ctx context.Context, tenantID, shopID uuid.UUID) (*domain.OverviewLiveDataResult, error) {
	cutoff := time.Now().UTC().Add(-30 * time.Minute)

	// 1. Total sessions in last 30 min
	totalQuery := fmt.Sprintf(`
		SELECT uniq(session_id)
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ? AND created_at >= ?
	`, s.database)
	var totalSessions uint64
	_ = s.conn.QueryRow(ctx, totalQuery, tenantID, shopID, cutoff).Scan(&totalSessions)

	// 2. Minute counts
	minuteQuery := fmt.Sprintf(`
		SELECT 
			toString(toStartOfMinute(created_at)) AS minute,
			uniq(session_id) AS session_count,
			uniq(device_id) AS visitor_count
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ? AND created_at >= ?
		GROUP BY minute
		ORDER BY minute ASC
	`, s.database)

	minuteCounts := []domain.MinuteCount{}
	if mRows, err := s.conn.Query(ctx, minuteQuery, tenantID, shopID, cutoff); err == nil {
		defer mRows.Close()
		for mRows.Next() {
			var m domain.MinuteCount
			var uSess, uVis uint64
			if err := mRows.Scan(&m.Minute, &uSess, &uVis); err == nil {
				m.SessionCount = int64(uSess)
				m.VisitorCount = int64(uVis)
				if t, parseErr := time.Parse("2006-01-02 15:04:05", m.Minute); parseErr == nil {
					m.Timestamp = t.UnixMilli()
					m.Time = t.Format("15:04")
				}
				minuteCounts = append(minuteCounts, m)
			}
		}
	}

	// 3. Referrers
	refQuery := fmt.Sprintf(`
		SELECT 
			coalesce(nullIf(referrer_name, ''), 'Direct') AS referrer,
			uniq(session_id) AS count
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ? AND created_at >= ?
		GROUP BY referrer
		ORDER BY count DESC
		LIMIT 10
	`, s.database)

	referrers := []domain.ReferrerMinuteItem{}
	if rRows, err := s.conn.Query(ctx, refQuery, tenantID, shopID, cutoff); err == nil {
		defer rRows.Close()
		for rRows.Next() {
			var r domain.ReferrerMinuteItem
			var c uint64
			if err := rRows.Scan(&r.Referrer, &c); err == nil {
				r.Count = int64(c)
				referrers = append(referrers, r)
			}
		}
	}

	return &domain.OverviewLiveDataResult{
		TotalSessions: int64(totalSessions),
		MinuteCounts:  minuteCounts,
		Referrers:     referrers,
	}, nil
}

// GetEventsList returns paginated raw analytical events.
func (s *Service) GetEventsList(ctx context.Context, tenantID, shopID uuid.UUID, eventName, search string, limit, offset int) ([]domain.Event, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var whereExtra []string
	var args []any
	args = append(args, tenantID, shopID)

	if eventName != "" {
		whereExtra = append(whereExtra, "AND name = ?")
		args = append(args, eventName)
	}
	if search != "" {
		whereExtra = append(whereExtra, "AND (path LIKE ? OR device_id LIKE ? OR city LIKE ?)")
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern, pattern)
	}

	countQuery := fmt.Sprintf(`
		SELECT count()
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ? %s
	`, s.database, strings.Join(whereExtra, " "))

	var total uint64
	_ = s.conn.QueryRow(ctx, countQuery, args...).Scan(&total)

	queryArgs := append(args, limit, offset)
	dataQuery := fmt.Sprintf(`
		SELECT 
			id, tenant_id, shop_id, name, device_id, customer_id, session_id,
			coalesce(revenue, 0), currency, product_id, cart_id, order_id,
			path, origin, referrer, referrer_name, referrer_type,
			os, browser, device, country, city, properties, created_at
		FROM %s.events
		WHERE tenant_id = ? AND shop_id = ? %s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, s.database, strings.Join(whereExtra, " "))

	rows, err := s.conn.Query(ctx, dataQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list events: %w", err)
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

	return events, int64(total), nil
}

// GetSessionsList returns paginated visitor sessions from ClickHouse.
func (s *Service) GetSessionsList(ctx context.Context, tenantID, shopID uuid.UUID, limit, offset int) ([]domain.Session, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	countQuery := fmt.Sprintf(`
		SELECT count()
		FROM %s.sessions
		WHERE tenant_id = ? AND shop_id = ?
	`, s.database)

	var total uint64
	_ = s.conn.QueryRow(ctx, countQuery, tenantID, shopID).Scan(&total)

	dataQuery := fmt.Sprintf(`
		SELECT 
			id, tenant_id, shop_id, device_id, customer_id,
			started_at, ended_at, duration,
			entry_path, exit_path, referrer, referrer_name, referrer_type,
			events_count, has_cart_add, has_purchase, total_revenue
		FROM %s.sessions
		WHERE tenant_id = ? AND shop_id = ?
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?
	`, s.database)

	rows, err := s.conn.Query(ctx, dataQuery, tenantID, shopID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []domain.Session
	for rows.Next() {
		var sess domain.Session
		var cartAdd, purch uint8
		if err := rows.Scan(
			&sess.ID, &sess.TenantID, &sess.ShopID, &sess.DeviceID, &sess.CustomerID,
			&sess.StartedAt, &sess.EndedAt, &sess.Duration,
			&sess.EntryPath, &sess.ExitPath, &sess.Referrer, &sess.ReferrerName, &sess.ReferrerType,
			&sess.EventsCount, &cartAdd, &purch, &sess.TotalRevenue,
		); err == nil {
			sess.HasCartAdd = cartAdd > 0
			sess.HasPurchase = purch > 0
			sessions = append(sessions, sess)
		}
	}

	return sessions, int64(total), nil
}

var journeyColors = []string{
	"#3b82f6", "#10b981", "#f59e0b", "#ef4444", "#8b5cf6",
	"#ec4899", "#06b6d4", "#84cc16", "#f97316", "#6366f1",
}

// GetUserJourney constructs a multi-step user navigation graph for the Sankey diagram.
func (s *Service) GetUserJourney(ctx context.Context, tenantID, shopID uuid.UUID, timeRange, startDateStr, endDateStr string, steps int) (*domain.UserJourneyResult, error) {
	if steps < 2 {
		steps = 2
	}
	if steps > 10 {
		steps = 10
	}

	curStart, curEnd, _, _ := resolveTimeRange(timeRange, startDateStr, endDateStr)

	// Step 1: Query top 3 entry pages
	topEntriesQuery := fmt.Sprintf(`
		WITH 
		ordered_events AS (
			SELECT 
				toString(session_id) as session_id,
				coalesce(nullIf(path, ''), '/') as path,
				created_at
			FROM %s.events
			WHERE (tenant_id = ? OR tenant_id = toUUID('00000000-0000-0000-0000-000000000000')) 
			  AND shop_id = ? 
			  AND created_at BETWEEN ? AND ?
			  AND (name = 'screen_view' OR name = 'page_view' OR path != '')
			  AND path != ''
			ORDER BY session_id ASC, created_at ASC
		),
		paths_deduped_cte AS (
			SELECT 
				session_id,
				arraySlice(
					arrayFilter(
						(x, i) -> i = 1 OR x != paths_raw[i - 1],
						groupArray(path) as paths_raw,
						arrayEnumerate(paths_raw)
					),
					1, %d
				) as paths_deduped
			FROM ordered_events
			GROUP BY session_id
		),
		session_paths AS (
			SELECT 
				session_id,
				if(
					arrayFirstIndex(x -> x > 1, arrayEnumerateUniq(paths_deduped)) = 0,
					paths_deduped,
					arraySlice(
						paths_deduped,
						1,
						arrayFirstIndex(x -> x > 1, arrayEnumerateUniq(paths_deduped)) - 1
					)
				) as paths,
				paths[1] as entry_page
			FROM paths_deduped_cte
			HAVING length(paths) >= 2
		)
		SELECT 
			entry_page, 
			count() as count
		FROM session_paths
		GROUP BY entry_page
		ORDER BY count DESC
		LIMIT 3
	`, s.database, steps)

	type entryRow struct {
		EntryPage string `ch:"entry_page"`
		Count     uint64 `ch:"count"`
	}

	var topEntries []entryRow
	if err := s.conn.Select(ctx, &topEntries, topEntriesQuery, tenantID, shopID, curStart, curEnd); err != nil {
		return &domain.UserJourneyResult{Nodes: []domain.SankeyNode{}, Links: []domain.SankeyLink{}}, err
	}

	if len(topEntries) == 0 {
		return &domain.UserJourneyResult{Nodes: []domain.SankeyNode{}, Links: []domain.SankeyLink{}}, nil
	}

	var topEntryPages []string
	var totalSessions int64
	for _, e := range topEntries {
		topEntryPages = append(topEntryPages, e.EntryPage)
		totalSessions += int64(e.Count)
	}

	// Step 2: Query transitions between steps
	transitionsQuery := fmt.Sprintf(`
		WITH 
		ordered_events AS (
			SELECT 
				toString(session_id) as session_id,
				coalesce(nullIf(path, ''), '/') as path,
				created_at
			FROM %s.events
			WHERE (tenant_id = ? OR tenant_id = toUUID('00000000-0000-0000-0000-000000000000')) 
			  AND shop_id = ? 
			  AND created_at BETWEEN ? AND ?
			  AND (name = 'screen_view' OR name = 'page_view' OR path != '')
			  AND path != ''
			ORDER BY session_id ASC, created_at ASC
		),
		paths_deduped_cte AS (
			SELECT 
				session_id,
				arraySlice(
					arrayFilter(
						(x, i) -> i = 1 OR x != paths_raw[i - 1],
						groupArray(path) as paths_raw,
						arrayEnumerate(paths_raw)
					),
					1, %d
				) as paths_deduped
			FROM ordered_events
			GROUP BY session_id
		),
		session_paths AS (
			SELECT 
				session_id,
				if(
					arrayFirstIndex(x -> x > 1, arrayEnumerateUniq(paths_deduped)) = 0,
					paths_deduped,
					arraySlice(
						paths_deduped,
						1,
						arrayFirstIndex(x -> x > 1, arrayEnumerateUniq(paths_deduped)) - 1
					)
				) as paths
			FROM paths_deduped_cte
			HAVING length(paths) >= 2 AND has(?, paths[1])
		)
		SELECT 
			pair.1 as source,
			pair.2 as target,
			pair.3 as step,
			count() as value
		FROM (
			SELECT arrayJoin(
				arrayMap(i -> (paths[i], paths[i + 1], i), range(1, length(paths)))
			) as pair 
			FROM session_paths
		)
		GROUP BY source, target, step
		ORDER BY step ASC, value DESC
	`, s.database, steps)

	type transitionRow struct {
		Source string `ch:"source"`
		Target string `ch:"target"`
		Step   uint64 `ch:"step"`
		Value  uint64 `ch:"value"`
	}

	var transitions []transitionRow
	if err := s.conn.Select(ctx, &transitions, transitionsQuery, tenantID, shopID, curStart, curEnd, topEntryPages); err != nil {
		return &domain.UserJourneyResult{Nodes: []domain.SankeyNode{}, Links: []domain.SankeyLink{}}, err
	}

	if len(transitions) == 0 {
		return &domain.UserJourneyResult{Nodes: []domain.SankeyNode{}, Links: []domain.SankeyLink{}}, nil
	}

	// Step 3: Progressive Graph Construction
	type nodeInfo struct {
		path  string
		value int64
		step  int
		color string
	}

	nodes := make(map[string]*nodeInfo)
	var rawLinks []domain.SankeyLink

	getNodeID := func(path string, step int) string {
		return fmt.Sprintf("%s::step%d", path, step)
	}

	transitionsByStep := make(map[int][]transitionRow)
	for _, t := range transitions {
		sIdx := int(t.Step)
		transitionsByStep[sIdx] = append(transitionsByStep[sIdx], t)
	}

	activeNodes := make(map[string]string)
	for idx, entry := range topEntries {
		nodeID := getNodeID(entry.EntryPage, 1)
		color := journeyColors[idx%len(journeyColors)]
		nodes[nodeID] = &nodeInfo{
			path:  entry.EntryPage,
			value: int64(entry.Count),
			step:  1,
			color: color,
		}
		activeNodes[entry.EntryPage] = nodeID
	}

	for step := 1; step < steps; step++ {
		stepTransitions := transitionsByStep[step]
		nextActiveNodes := make(map[string]string)

		for sourcePath, sourceNodeID := range activeNodes {
			var fromSource []transitionRow
			for _, t := range stepTransitions {
				if t.Source == sourcePath {
					fromSource = append(fromSource, t)
				}
			}

			// Sort by value DESC, take top 3 destinations per node
			sort.Slice(fromSource, func(i, j int) bool {
				return fromSource[i].Value > fromSource[j].Value
			})
			if len(fromSource) > 3 {
				fromSource = fromSource[:3]
			}

			for _, t := range fromSource {
				if t.Source == t.Target {
					continue
				}

				targetNodeID := getNodeID(t.Target, step+1)
				val := int64(t.Value)

				rawLinks = append(rawLinks, domain.SankeyLink{
					Source: sourceNodeID,
					Target: targetNodeID,
					Value:  val,
				})

				existing, ok := nodes[targetNodeID]
				if ok {
					existing.value += val
				} else {
					sourceData := nodes[sourceNodeID]
					color := journeyColors[len(nodes)%len(journeyColors)]
					if sourceData != nil && sourceData.color != "" {
						color = sourceData.color
					}
					nodes[targetNodeID] = &nodeInfo{
						path:  t.Target,
						value: val,
						step:  step + 1,
						color: color,
					}
				}
				nextActiveNodes[t.Target] = targetNodeID
			}
		}

		activeNodes = nextActiveNodes
		if len(activeNodes) == 0 {
			break
		}
	}

	// Step 4: Filter links by threshold (0.25% of total sessions)
	minLinkValue := int64(math.Ceil(float64(totalSessions) * 0.0025))
	if minLinkValue < 1 {
		minLinkValue = 1
	}

	var filteredLinks []domain.SankeyLink
	for _, l := range rawLinks {
		if l.Value >= minLinkValue {
			filteredLinks = append(filteredLinks, l)
		}
	}

	// Step 5: Prune nodes and compute final node values
	referencedNodeIDs := make(map[string]bool)
	nodeValuesFromLinks := make(map[string]int64)

	for _, l := range filteredLinks {
		referencedNodeIDs[l.Source] = true
		referencedNodeIDs[l.Target] = true
		nodeValuesFromLinks[l.Target] += l.Value
	}

	// Remove entry nodes with no outgoing links
	for id, n := range nodes {
		if n.step == 1 {
			hasOutgoing := false
			for _, l := range filteredLinks {
				if l.Source == id {
					hasOutgoing = true
					break
				}
			}
			if !hasOutgoing {
				delete(referencedNodeIDs, id)
			}
		}
	}

	var finalNodes []domain.SankeyNode
	for id, data := range nodes {
		if !referencedNodeIDs[id] {
			continue
		}
		val := data.value
		if data.step != 1 {
			if linkVal, exists := nodeValuesFromLinks[id]; exists {
				val = linkVal
			}
		}
		pct := 0.0
		if totalSessions > 0 {
			pct = (float64(val) / float64(totalSessions)) * 100
		}
		finalNodes = append(finalNodes, domain.SankeyNode{
			ID:         id,
			Label:      data.path,
			NodeColor:  data.color,
			Percentage: math.Round(pct*100) / 100,
			Value:      val,
			Step:       data.step,
		})
	}

	// Sort nodes by step ASC, value DESC
	sort.Slice(finalNodes, func(i, j int) bool {
		if finalNodes[i].Step != finalNodes[j].Step {
			return finalNodes[i].Step < finalNodes[j].Step
		}
		return finalNodes[i].Value > finalNodes[j].Value
	})

	// Final link validation: ensure all endpoints exist in finalNodes
	finalNodeIDSet := make(map[string]bool)
	for _, n := range finalNodes {
		finalNodeIDSet[n.ID] = true
	}

	var validLinks []domain.SankeyLink
	for _, l := range filteredLinks {
		if finalNodeIDSet[l.Source] && finalNodeIDSet[l.Target] {
			validLinks = append(validLinks, l)
		}
	}

	if finalNodes == nil {
		finalNodes = []domain.SankeyNode{}
	}
	if validLinks == nil {
		validLinks = []domain.SankeyLink{}
	}

	return &domain.UserJourneyResult{
		Nodes: finalNodes,
		Links: validLinks,
	}, nil
}


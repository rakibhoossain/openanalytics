package query

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

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
		var level int
		var users uint64 // CRITICAL(clickhouse-uint64): ClickHouse count() returns UInt64
		if err := rows.Scan(&level, &users); err != nil {
			return nil, err
		}
		maxLevels[level] = int64(users)
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
		var rev float64
		if err := rows.Scan(
			&ev.ID, &ev.TenantID, &ev.ShopID, &ev.Name, &ev.DeviceID, &ev.CustomerID, &ev.SessionID,
			&rev, &ev.Currency, &ev.ProductID, &ev.CartID, &ev.OrderID,
			&ev.Path, &ev.Origin, &ev.Referrer, &ev.ReferrerName, &ev.ReferrerType,
			&ev.OS, &ev.Browser, &ev.Device, &ev.Country, &ev.City, &ev.CreatedAt,
		); err != nil {
			return nil, err
		}
		if rev > 0 {
			ev.Revenue = &rev
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

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"openanalytics/internal/domain"
	"openanalytics/pkg/uuidv7"
)

// Repository provides PostgreSQL metadata storage for dashboards, widgets, and reports.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository initializes a PostgreSQL connection pool.
func NewRepository(ctx context.Context, dsn string) (*Repository, error) {
	// WEAK_POINT(pool-exhaustion): Configure max connections to avoid exhausting PostgreSQL connections
	// when multiple instances of the query service are deployed.
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid postgres dsn: %w", err)
	}

	cfg.MaxConns = 25
	cfg.MinConns = 5
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping postgres at %s: %w", cfg.ConnConfig.Host, err)
	}

	return &Repository{pool: pool}, nil
}

// Close closes the underlying pool.
func (r *Repository) Close() {
	if r.pool != nil {
		r.pool.Close()
	}
}

// --- Dashboard Operations ---

// CreateDashboard stores a new dashboard configuration.
func (r *Repository) CreateDashboard(ctx context.Context, d *domain.Dashboard) error {
	if d.ID == uuid.Nil {
		d.ID = uuidv7.MustNew()
	}
	now := time.Now().UTC()
	d.CreatedAt = now
	d.UpdatedAt = now

	// CRITICAL(tenant-isolation): Always bind tenant_id and shop_id on every insert.
	query := `
		INSERT INTO dashboards (id, tenant_id, shop_id, name, description, is_default, layout_grid, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.pool.Exec(ctx, query,
		d.ID, d.TenantID, d.ShopID, d.Name, d.Description, d.IsDefault, d.LayoutGrid, d.CreatedAt, d.UpdatedAt,
	)
	return err
}

// GetDashboard retrieves a single dashboard scoped by tenant and shop.
func (r *Repository) GetDashboard(ctx context.Context, tenantID, shopID, id uuid.UUID) (*domain.Dashboard, error) {
	// CRITICAL(tenant-isolation): Enforce tenant_id and shop_id to prevent cross-tenant enumeration.
	query := `
		SELECT id, tenant_id, shop_id, name, description, is_default, layout_grid, created_at, updated_at
		FROM dashboards
		WHERE id = $1 AND tenant_id = $2 AND shop_id = $3
	`
	var d domain.Dashboard
	var desc *string
	err := r.pool.QueryRow(ctx, query, id, tenantID, shopID).Scan(
		&d.ID, &d.TenantID, &d.ShopID, &d.Name, &desc, &d.IsDefault, &d.LayoutGrid, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if desc != nil {
		d.Description = *desc
	}
	return &d, nil
}

// ListDashboards returns all dashboards for a specific merchant shop.
func (r *Repository) ListDashboards(ctx context.Context, tenantID, shopID uuid.UUID) ([]*domain.Dashboard, error) {
	query := `
		SELECT id, tenant_id, shop_id, name, description, is_default, layout_grid, created_at, updated_at
		FROM dashboards
		WHERE tenant_id = $1 AND shop_id = $2
		ORDER BY is_default DESC, created_at ASC
	`
	rows, err := r.pool.Query(ctx, query, tenantID, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*domain.Dashboard
	for rows.Next() {
		var d domain.Dashboard
		var desc *string
		if err := rows.Scan(
			&d.ID, &d.TenantID, &d.ShopID, &d.Name, &desc, &d.IsDefault, &d.LayoutGrid, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if desc != nil {
			d.Description = *desc
		}
		result = append(result, &d)
	}
	return result, rows.Err()
}

// UpdateDashboard updates an existing dashboard.
func (r *Repository) UpdateDashboard(ctx context.Context, d *domain.Dashboard) error {
	d.UpdatedAt = time.Now().UTC()
	// CRITICAL(tenant-isolation): Enforce tenant_id and shop_id on updates.
	query := `
		UPDATE dashboards
		SET name = $1, description = $2, is_default = $3, layout_grid = $4, updated_at = $5
		WHERE id = $6 AND tenant_id = $7 AND shop_id = $8
	`
	tag, err := r.pool.Exec(ctx, query,
		d.Name, d.Description, d.IsDefault, d.LayoutGrid, d.UpdatedAt, d.ID, d.TenantID, d.ShopID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("dashboard not found or unauthorized")
	}
	return nil
}

// DeleteDashboard deletes a dashboard and cascades to its widgets.
func (r *Repository) DeleteDashboard(ctx context.Context, tenantID, shopID, id uuid.UUID) error {
	query := `DELETE FROM dashboards WHERE id = $1 AND tenant_id = $2 AND shop_id = $3`
	tag, err := r.pool.Exec(ctx, query, id, tenantID, shopID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("dashboard not found or unauthorized")
	}
	return nil
}

// --- Chart Widget Operations ---

// CreateWidget adds a chart widget to a dashboard.
func (r *Repository) CreateWidget(ctx context.Context, w *domain.ChartWidget) error {
	if w.ID == uuid.Nil {
		w.ID = uuidv7.MustNew()
	}
	now := time.Now().UTC()
	w.CreatedAt = now
	w.UpdatedAt = now

	// CRITICAL(tenant-isolation): Ensure widget is constrained to the tenant and shop.
	query := `
		INSERT INTO chart_widgets (
			id, dashboard_id, tenant_id, shop_id, title, chart_type, metric_type,
			time_range, group_by, filter_rules, custom_sql, position_x, position_y, width, height,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`
	_, err := r.pool.Exec(ctx, query,
		w.ID, w.DashboardID, w.TenantID, w.ShopID, w.Title, w.ChartType, w.MetricType,
		w.TimeRange, w.GroupBy, w.FilterRules, w.CustomSQL, w.PositionX, w.PositionY, w.Width, w.Height,
		w.CreatedAt, w.UpdatedAt,
	)
	return err
}

// ListWidgets retrieves all widgets belonging to a specific dashboard.
func (r *Repository) ListWidgets(ctx context.Context, tenantID, shopID, dashboardID uuid.UUID) ([]*domain.ChartWidget, error) {
	query := `
		SELECT id, dashboard_id, tenant_id, shop_id, title, chart_type, metric_type,
		       time_range, group_by, filter_rules, custom_sql, position_x, position_y, width, height,
		       created_at, updated_at
		FROM chart_widgets
		WHERE dashboard_id = $1 AND tenant_id = $2 AND shop_id = $3
		ORDER BY position_y ASC, position_x ASC
	`
	rows, err := r.pool.Query(ctx, query, dashboardID, tenantID, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var widgets []*domain.ChartWidget
	for rows.Next() {
		var w domain.ChartWidget
		var groupBy, customSQL *string
		if err := rows.Scan(
			&w.ID, &w.DashboardID, &w.TenantID, &w.ShopID, &w.Title, &w.ChartType, &w.MetricType,
			&w.TimeRange, &groupBy, &w.FilterRules, &customSQL, &w.PositionX, &w.PositionY, &w.Width, &w.Height,
			&w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if groupBy != nil {
			w.GroupBy = *groupBy
		}
		if customSQL != nil {
			w.CustomSQL = *customSQL
		}
		widgets = append(widgets, &w)
	}
	return widgets, rows.Err()
}

// DeleteWidget deletes a chart widget.
func (r *Repository) DeleteWidget(ctx context.Context, tenantID, shopID, id uuid.UUID) error {
	query := `DELETE FROM chart_widgets WHERE id = $1 AND tenant_id = $2 AND shop_id = $3`
	tag, err := r.pool.Exec(ctx, query, id, tenantID, shopID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("widget not found or unauthorized")
	}
	return nil
}

// --- Saved Reports Operations ---

// CreateReport creates a scheduled or saved report.
func (r *Repository) CreateReport(ctx context.Context, report *domain.SavedReport) error {
	if report.ID == uuid.Nil {
		report.ID = uuidv7.MustNew()
	}
	now := time.Now().UTC()
	report.CreatedAt = now
	report.UpdatedAt = now

	query := `
		INSERT INTO saved_reports (
			id, tenant_id, shop_id, name, report_type, schedule_cron, recipients, filters, is_active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := r.pool.Exec(ctx, query,
		report.ID, report.TenantID, report.ShopID, report.Name, report.ReportType,
		report.ScheduleCron, report.Recipients, report.Filters, report.IsActive,
		report.CreatedAt, report.UpdatedAt,
	)
	return err
}

// ListReports retrieves all saved reports for a shop.
func (r *Repository) ListReports(ctx context.Context, tenantID, shopID uuid.UUID) ([]*domain.SavedReport, error) {
	query := `
		SELECT id, tenant_id, shop_id, name, report_type, schedule_cron, recipients, filters, is_active, created_at, updated_at
		FROM saved_reports
		WHERE tenant_id = $1 AND shop_id = $2
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, tenantID, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []*domain.SavedReport
	for rows.Next() {
		var sr domain.SavedReport
		var cron *string
		if err := rows.Scan(
			&sr.ID, &sr.TenantID, &sr.ShopID, &sr.Name, &sr.ReportType, &cron,
			&sr.Recipients, &sr.Filters, &sr.IsActive, &sr.CreatedAt, &sr.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if cron != nil {
			sr.ScheduleCron = *cron
		}
		reports = append(reports, &sr)
	}
	return reports, rows.Err()
}

// --- Alert Rules Operations ---

// CreateAlert creates an anomaly detection alert rule.
func (r *Repository) CreateAlert(ctx context.Context, alert *domain.AlertRule) error {
	if alert.ID == uuid.Nil {
		alert.ID = uuidv7.MustNew()
	}
	alert.CreatedAt = time.Now().UTC()

	query := `
		INSERT INTO alert_rules (
			id, tenant_id, shop_id, name, metric, condition_operator, threshold_value,
			window_minutes, notification_channel, channel_target, is_enabled, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := r.pool.Exec(ctx, query,
		alert.ID, alert.TenantID, alert.ShopID, alert.Name, alert.Metric, alert.ConditionOperator,
		alert.ThresholdValue, alert.WindowMinutes, alert.NotificationChannel, alert.ChannelTarget,
		alert.IsEnabled, alert.CreatedAt,
	)
	return err
}

// ListAlerts lists alert rules for a merchant shop.
func (r *Repository) ListAlerts(ctx context.Context, tenantID, shopID uuid.UUID) ([]*domain.AlertRule, error) {
	query := `
		SELECT id, tenant_id, shop_id, name, metric, condition_operator, threshold_value,
		       window_minutes, notification_channel, channel_target, is_enabled, last_triggered_at, created_at
		FROM alert_rules
		WHERE tenant_id = $1 AND shop_id = $2
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, tenantID, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []*domain.AlertRule
	for rows.Next() {
		var a domain.AlertRule
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.ShopID, &a.Name, &a.Metric, &a.ConditionOperator,
			&a.ThresholdValue, &a.WindowMinutes, &a.NotificationChannel, &a.ChannelTarget,
			&a.IsEnabled, &a.LastTriggeredAt, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		alerts = append(alerts, &a)
	}
	return alerts, rows.Err()
}

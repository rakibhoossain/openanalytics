-- Extension for UUID generation
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Dashboards (Custom views per merchant shop)
CREATE TABLE IF NOT EXISTS dashboards (
    id UUID PRIMARY KEY,                     -- UUIDv7
    tenant_id UUID NOT NULL,                 -- UUIDv7
    shop_id UUID NOT NULL,                   -- UUIDv7
    name VARCHAR(255) NOT NULL,
    description TEXT,
    is_default BOOLEAN DEFAULT FALSE,
    layout_grid JSONB NOT NULL DEFAULT '[]', -- Grid layout positions for widgets
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dashboards_tenant_shop ON dashboards (tenant_id, shop_id);

-- Chart Widgets (Individual cards on a dashboard: Funnel, Line, Bar, KPI)
CREATE TABLE IF NOT EXISTS chart_widgets (
    id UUID PRIMARY KEY,                     -- UUIDv7
    dashboard_id UUID NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL,
    shop_id UUID NOT NULL,
    title VARCHAR(255) NOT NULL,
    chart_type VARCHAR(50) NOT NULL,         -- 'timeseries', 'funnel', 'kpi', 'bar', 'pie'
    metric_type VARCHAR(100) NOT NULL,       -- 'page_views', 'cart_adds', 'conversion_rate', 'revenue'
    time_range VARCHAR(50) NOT NULL,         -- 'today', '24h', '7d', '30d', 'custom'
    group_by VARCHAR(50),                    -- 'day', 'hour', 'browser', 'country', 'utm_source'
    filter_rules JSONB NOT NULL DEFAULT '[]',-- Array of filters [{field, op, value}]
    custom_sql TEXT,                         -- Optional custom ClickHouse SQL snippet
    position_x INT NOT NULL DEFAULT 0,
    position_y INT NOT NULL DEFAULT 0,
    width INT NOT NULL DEFAULT 6,
    height INT NOT NULL DEFAULT 4,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chart_widgets_dashboard ON chart_widgets (dashboard_id);

-- Saved Reports (Scheduled exports, PDF/CSV triggers)
CREATE TABLE IF NOT EXISTS saved_reports (
    id UUID PRIMARY KEY,                     -- UUIDv7
    tenant_id UUID NOT NULL,
    shop_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    report_type VARCHAR(50) NOT NULL,        -- 'weekly_executive', 'abandoned_cart', 'traffic_source'
    schedule_cron VARCHAR(50),               -- e.g. '0 9 * * 1'
    recipients TEXT[] NOT NULL DEFAULT '{}',
    filters JSONB NOT NULL DEFAULT '{}',
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Alert Rules (Real-time monitoring on conversion or traffic anomalies)
CREATE TABLE IF NOT EXISTS alert_rules (
    id UUID PRIMARY KEY,                     -- UUIDv7
    tenant_id UUID NOT NULL,
    shop_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    metric VARCHAR(100) NOT NULL,            -- 'error_rate', 'cart_abandonment_spike'
    condition_operator VARCHAR(10) NOT NULL, -- '>', '<', '>='
    threshold_value DOUBLE PRECISION NOT NULL,
    window_minutes INT NOT NULL DEFAULT 15,
    notification_channel VARCHAR(50) NOT NULL, -- 'webhook', 'email', 'slack'
    channel_target TEXT NOT NULL,            -- URL or email address
    is_enabled BOOLEAN DEFAULT TRUE,
    last_triggered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_tenant_shop ON alert_rules (tenant_id, shop_id);

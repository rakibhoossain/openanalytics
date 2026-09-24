CREATE DATABASE IF NOT EXISTS openpanel;

-- Events Table
CREATE TABLE IF NOT EXISTS openpanel.events (
    id UUID,                                 -- UUIDv7
    tenant_id UUID,                          -- UUIDv7
    shop_id UUID,                            -- UUIDv7
    name LowCardinality(String),             -- 'view_product', 'add_to_cart', 'checkout_step'
    device_id String CODEC(ZSTD(3)),
    customer_id Nullable(UUID),              -- UUIDv7 (null for guest shoppers)
    session_id UUID,                         -- UUIDv7
    
    -- E-Commerce Telemetry (Revenue stored in exact integer cents, e.g. $1.78 = 178 cents)
    revenue Nullable(Int64) CODEC(T64, ZSTD(3)),
    currency LowCardinality(String),
    product_id Nullable(UUID),               -- UUIDv7
    cart_id Nullable(UUID),                  -- UUIDv7
    order_id Nullable(UUID),                 -- UUIDv7
    
    -- Web & Device Context
    path String CODEC(ZSTD(3)),
    origin String CODEC(ZSTD(3)),
    referrer String CODEC(ZSTD(3)),
    referrer_name String CODEC(ZSTD(3)),
    referrer_type LowCardinality(String),
    
    os LowCardinality(String),
    browser LowCardinality(String),
    device LowCardinality(String),
    country LowCardinality(FixedString(2)),
    city String,
    latitude Nullable(Float64),
    longitude Nullable(Float64),
    
    properties Map(String, String) CODEC(ZSTD(3)),
    created_at DateTime64(3, 'UTC') CODEC(DoubleDelta, ZSTD(3))
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(created_at)
ORDER BY (tenant_id, shop_id, toDate(created_at), session_id, name, created_at)
SETTINGS index_granularity = 8192;

-- Sessions Table
CREATE TABLE IF NOT EXISTS openpanel.sessions (
    id UUID,                                 -- UUIDv7
    tenant_id UUID,                          -- UUIDv7
    shop_id UUID,                            -- UUIDv7
    device_id String CODEC(ZSTD(3)),
    customer_id Nullable(UUID),              -- UUIDv7
    
    started_at DateTime64(3, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
    ended_at DateTime64(3, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
    duration UInt32 CODEC(Delta(4), LZ4),
    
    entry_path String CODEC(ZSTD(3)),
    exit_path String CODEC(ZSTD(3)),
    referrer String CODEC(ZSTD(3)),
    referrer_name String CODEC(ZSTD(3)),
    referrer_type LowCardinality(String),
    
    events_count UInt32,
    has_cart_add UInt8,
    has_purchase UInt8,
    total_revenue Int64 DEFAULT 0            -- Stored in integer cents
)
ENGINE = ReplacingMergeTree(ended_at)
PARTITION BY toYYYYMM(started_at)
ORDER BY (tenant_id, shop_id, toDate(started_at), device_id, id)
SETTINGS index_granularity = 8192;

-- Hourly Metrics Rollup Table
CREATE TABLE IF NOT EXISTS openpanel.hourly_metrics (
    shop_id UUID,
    tenant_id UUID,
    hour DateTime('UTC') CODEC(DoubleDelta, ZSTD(3)),
    unique_visitors UInt32,
    sessions_count UInt32,
    pageviews UInt32,
    cart_adds UInt32,
    purchases UInt32,
    gross_revenue Int64 DEFAULT 0            -- Stored in integer cents
) ENGINE = ReplacingMergeTree()
PARTITION BY toYYYYMM(hour)
ORDER BY (tenant_id, shop_id, hour)
SETTINGS index_granularity = 8192;

-- Automated Commerce Insights & Anomaly Detection Feed
CREATE TABLE IF NOT EXISTS openpanel.project_insights (
    id UUID,
    shop_id UUID,
    tenant_id UUID,
    module_key LowCardinality(String),
    dimension_key String,
    window_kind LowCardinality(String),
    title String,
    summary String,
    direction LowCardinality(String),
    change_pct Float64,
    current_val Float64,
    compare_val Float64,
    impact_score Float64,
    created_at DateTime64(3, 'UTC')
) ENGINE = ReplacingMergeTree(created_at)
PARTITION BY toYYYYMM(created_at)
ORDER BY (tenant_id, shop_id, module_key, window_kind, dimension_key)
SETTINGS index_granularity = 8192;

-- OpenAnalytics Behavioral ML Feature Store
CREATE TABLE IF NOT EXISTS openpanel.shopper_features (
    tenant_id UUID,
    shop_id UUID,
    device_id String CODEC(ZSTD(3)),
    session_id UUID,
    
    views_count UInt32,
    cart_adds_count UInt32,
    distinct_products UInt32,
    total_dwell_seconds UInt32,
    has_purchase UInt8,
    cart_intent_score Float32 DEFAULT 0.0,
    
    last_event_at DateTime64(3, 'UTC') CODEC(DoubleDelta, ZSTD(3))
)
ENGINE = ReplacingMergeTree(last_event_at)
PARTITION BY toYYYYMM(last_event_at)
ORDER BY (tenant_id, shop_id, device_id, session_id)
SETTINGS index_granularity = 8192;

-- OpenAnalytics Session Replay Chunks Store
CREATE TABLE IF NOT EXISTS openpanel.session_replay_chunks (
    tenant_id UUID,
    shop_id UUID,
    session_id UUID,
    chunk_index UInt16,
    started_at DateTime64(3, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
    ended_at DateTime64(3, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
    events_count UInt16,
    is_full_snapshot UInt8,
    payload String CODEC(ZSTD(6))
) ENGINE = ReplacingMergeTree()
PARTITION BY toYYYYMM(started_at)
ORDER BY (tenant_id, shop_id, session_id, started_at, chunk_index)
TTL toDateTime(started_at) + toIntervalDay(30)
SETTINGS index_granularity = 8192;

-- Default Database Views (ensures DBeaver, IDEs, and tools querying 'default' DB see all data)
CREATE VIEW IF NOT EXISTS default.events AS SELECT * FROM openpanel.events;
CREATE VIEW IF NOT EXISTS default.sessions AS SELECT * FROM openpanel.sessions;
CREATE VIEW IF NOT EXISTS default.hourly_metrics AS SELECT * FROM openpanel.hourly_metrics;
CREATE VIEW IF NOT EXISTS default.project_insights AS SELECT * FROM openpanel.project_insights;
CREATE VIEW IF NOT EXISTS default.shopper_features AS SELECT * FROM openpanel.shopper_features;
CREATE VIEW IF NOT EXISTS default.session_replay_chunks AS SELECT * FROM openpanel.session_replay_chunks;

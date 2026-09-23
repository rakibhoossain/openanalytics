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
    
    -- E-Commerce Telemetry
    revenue Nullable(Float64) CODEC(Gorilla, LZ4),
    currency LowCardinality(String),
    product_id String CODEC(ZSTD(3)),
    cart_id String CODEC(ZSTD(3)),
    order_id String CODEC(ZSTD(3)),
    
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
    created_at DateTime64(3) CODEC(DoubleDelta, ZSTD(3))
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
    
    started_at DateTime64(3) CODEC(DoubleDelta, ZSTD(3)),
    ended_at DateTime64(3) CODEC(DoubleDelta, ZSTD(3)),
    duration UInt32 CODEC(Delta(4), LZ4),
    
    entry_path String CODEC(ZSTD(3)),
    exit_path String CODEC(ZSTD(3)),
    referrer String CODEC(ZSTD(3)),
    referrer_name String CODEC(ZSTD(3)),
    referrer_type LowCardinality(String),
    
    events_count UInt32,
    has_cart_add UInt8,
    has_purchase UInt8,
    total_revenue Float64 DEFAULT 0
)
ENGINE = ReplacingMergeTree(ended_at)
PARTITION BY toYYYYMM(started_at)
ORDER BY (tenant_id, shop_id, toDate(started_at), device_id, id)
SETTINGS index_granularity = 8192;

-- Default Database Views (ensures DBeaver, IDEs, and tools querying 'default' DB see all data)
CREATE VIEW IF NOT EXISTS default.events AS SELECT * FROM openpanel.events;
CREATE VIEW IF NOT EXISTS default.sessions AS SELECT * FROM openpanel.sessions;

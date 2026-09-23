-- OpenAnalytics Behavioral ML Feature Store
-- Aggregates rolling shopper signals to predict real-time cart abandonment and purchase propensity.

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
    
    last_event_at DateTime64(3) CODEC(DoubleDelta, ZSTD(3))
)
ENGINE = ReplacingMergeTree(last_event_at)
PARTITION BY toYYYYMM(last_event_at)
ORDER BY (tenant_id, shop_id, device_id, session_id)
SETTINGS index_granularity = 8192;

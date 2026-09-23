# OpenAnalytics Architecture & Technical Blueprint
**High-Throughput Multi-Tenant Analytics & Behavioral Intelligence for `ai-cart`**

---

## 1. Executive Summary & Design Goals

`OpenAnalytics` is an enterprise multi-tenant analytics and behavioral machine-learning platform built specifically for the `ai-cart` SaaS e-commerce platform.

It decouples the system into:
1. **High-Performance Golang Data Plane (built on Go-Chi `github.com/go-chi/chi/v5`)**: Ingestion API and stream workers handling 50,000+ events/sec per node with sub-5ms $p99$ response times.
2. **Streaming Pipeline (Kafka)**: Pinned partitioning (`shop_id:device_id`) connecting ingestion with horizontally scalable worker instances and ML scoring.
3. **Multi-Instance Worker Scalability**: Distributed workers running across multiple container replicas with Kafka Consumer Group coordination, Redis distributed session state, and atomic cron execution.
4. **Hybrid State & Storage Architecture**:
   - **ClickHouse (OLAP)**: High-speed columnar analytics, funnel analysis, cohort retention, and ML feature store.
   - **PostgreSQL (Metadata OLTP)**: Saved reports, chart definitions, custom dashboards, alert rules, and merchant visual configurations.
   - **Redis (Real-time Cache & Distributed State)**: Active session state across worker instances (30m sliding window), live visitor HyperLogLog counters, deduplication, and sub-millisecond ML purchase-intent cache.
5. **Behavioral Machine Learning (LightGBM / ONNX)**: Real-time purchase intent and cart-abandonment prediction embedded directly into the Go stream processor.

---

## 2. Infrastructure & Connected Topology

All core infrastructure services are verified and operational:

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│ INFRASTRUCTURE TOPOLOGY                                                                │
│                                                                                        │
│  • Web Framework:      Go-Chi (github.com/go-chi/chi/v5)                               │
│  • Kafka Broker:       91.99.83.171:9092                                               │
│  • ClickHouse Server:  127.0.0.1:9000 (Native TCP) / 127.0.0.1:8123 (HTTP)            │
│  • Redis Server:       127.0.0.1:6379                                                  │
│  • PostgreSQL:         postgresql://openpanel:openpanel@127.0.0.1:5435/openpanel       │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

```
                                  STOREFRONTS & CLIENTS
                  (Shopify-like Storefronts, Mobile Apps, Headless API)
                                            │
                                            ▼
                          ┌───────────────────────────────────┐
                          │   Edge Gateway (Caddy / Envoy)    │
                          │   • SSL Termination               │
                          │   • Public Key / Token Validation │
                          └─────────────────┬─────────────────┘
                                            │
                                            ▼
┌───────────────────────────────────────────────────────────────────────────────────────────┐
│ GOLANG INGESTION SERVICE (`cmd/ingest`) [Go-Chi HTTP Router]                              │
│                                                                                           │
│   • Middleware: chi/middleware (RealIP, Recoverer, Logger, CORS, RateLimiter)             │
│   • Endpoints: POST /track, POST /batch, GET /track/device-id                             │
│   • Zero-alloc JSON parsing (sonic / goccy/go-json)                                       │
│   • Embedded MaxMind GeoIP (oschwald/geoip2-golang, memory-mapped MMDB)                   │
│   • Fast User-Agent Parser & Device Fingerprinting (SHA-256)                              │
│   • Redis Fast Layer: Rate limiting & instant session resolution                          │
│   • Async Partitioned Kafka Producer (Key: shop_id:device_id) ➔ 91.99.83.171:9092         │
└───────────────────────────────────────────┬───────────────────────────────────────────────┘
                                            │
                                            ▼
┌───────────────────────────────────────────────────────────────────────────────────────────┐
│ STREAMING BROKER (Kafka @ 91.99.83.171:9092)                                              │
│                                                                                           │
│   • Topic: `analytics.events.raw` (Partitioned by hash(shop_id:device_id))               │
│   • Topic: `analytics.sessions.boundary` (Session start/end events)                       │
│   • Topic: `analytics.intent.scored` (Real-time shopper predictions)                      │
└─────────────────────┬───────────────────────────────────────────────┬─────────────────────┘
                      │                                               │
                      ▼                                               ▼
┌───────────────────────────────────────────┐   ┌───────────────────────────────────────────┐
│ MULTI-INSTANCE WORKERS (`cmd/worker`)     │   │ REAL-TIME ML SCORERS (`cmd/ml-worker`)    │
│ [Horizontally Scaled: Replica 1 ... N]   │   │ [Horizontally Scaled: Replica 1 ... N]    │
│                                           │   │                                           │
│ • Kafka Consumer Group Auto-Rebalancing   │   │ • Kafka Consumer Group                    │
│ • Redis Distributed Session State         │   │ • Real-time Dwell & Cart Feature Accumulator│
│ • Atomic Session Boundaries (Lua script)  │   │ • Embedded LightGBM / ONNX Runtime        │
│ • Columnar Batching (e.g. 5000 rows/2s)   │   │ • Real-time Cart Abandonment Probability  │
│ • Native TCP to ClickHouse @ 9000         │   │ • Publishes score to Redis (< 0.5ms)      │
└─────────────────────┬─────────────────────┘   └─────────────────────┬─────────────────────┘
                      │                                               │
                      ▼                                               ▼
┌───────────────────────────────────────────────────────────────────────────────────────────┐
│ CLICKHOUSE COLUMNAR CLUSTER (localhost:9000)                                              │
│                                                                                           │
│   • `events`: Raw clickstream (partitioned by toYYYYMM, ordered by tenant_id, shop_id...) │
│   • `sessions`: Session duration, bounce, entry/exit path, UTM referrer attribution       │
│   • `customers`: Shopper profiles, total spend, orders count, RFM metrics                 │
│   • Materialized Views: Rolling feature store for ML model training                       │
└───────────────────────────────────────────▲───────────────────────────────────────────────┘
                                            │
                                            │ Read OLAP Queries
                                            │
┌───────────────────────────────────────────┴───────────────────────────────────────────────┐
│ GOLANG QUERY SERVICE (`cmd/query`) [Go-Chi HTTP Router]                                   │
│                                                                                           │
│   • Middleware: chi/middleware (Auth, TenantContext, Timeout, Compression, Logger)        │
│   • Endpoints: Funnels, Retention, Trends, Realtime Live Shoppers, User Journeys          │
│   • Authenticated via `ai-cart` Tenant JWT (Injected `X-Tenant-Id`, `X-Shop-Id`)          │
│   • Reads OLAP aggregations from ClickHouse (9000)                                        │
│   • Reads & Writes Dashboard / Report / Chart metadata from PostgreSQL (5432)            │
│   • Reads Real-time Live Shopper metrics from Redis (6379)                                │
└───────────────────────────────────────────▲───────────────────────────────────────────────┘
                                            │
                                            ▼
┌───────────────────────────────────────────────────────────────────────────────────────────┐
│ `ai-cart` MERCHANT DASHBOARD (`ui/`)                                                      │
│                                                                                           │
│   • OpenPanel TanStack / React UI adapted into `ai-cart` Design System                    │
│   • Standalone auth removed; multi-tenant context inherited from parent app               │
│   • Custom report builder, interactive chart widgets, and real-time live view             │
└───────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Web Framework: Go-Chi (`github.com/go-chi/chi/v5`)

Both `cmd/ingest` and `cmd/query` are built using **Go-Chi**, providing 100% compatibility with standard library `net/http`, zero dynamic memory allocations during routing, and high-performance middleware chaining:

### Standard Ingestion Middleware Stack
```go
r := chi.NewRouter()

// Global Middleware
r.Use(middleware.RequestID)
r.Use(middleware.RealIP)
r.Use(middleware.Logger)
r.Use(middleware.Recoverer)
r.Use(middleware.Timeout(5 * time.Second))
r.Use(cors.Handler(cors.Options{
    AllowedOrigins:   []string{"*"},
    AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
    AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Shop-Id", "X-Tenant-Id"},
    AllowCredentials: false,
    MaxAge:           86400 * 7,
}))

// Route Registration
r.Route("/api/v1", func(r chi.Router) {
    r.Post("/track", ingestHandler.HandleTrack)
    r.Post("/batch", ingestHandler.HandleBatch)
    r.Get("/track/device-id", ingestHandler.HandleDeviceID)
    r.Get("/healthz", ingestHandler.HandleHealth)
})
```

---

## 4. Multi-Instance Worker Architecture & Distributed Invariants

Running `cmd/worker` across multiple container instances (e.g. in Kubernetes or Docker Swarm) introduces distributed systems challenges: partition ownership, race-free session state, concurrent ClickHouse batch flushing, and distributed crons.

```
                  KAFKA TOPIC: `analytics.events.raw` (e.g., 16 Partitions)
                  [P0] [P1] [P2] [P3] [P4] [P5] [P6] [P7] ... [P15]
                       │         │         │         │
                       ▼         ▼         ▼         ▼
             ┌───────────┐ ┌───────────┐ ┌───────────┐ ┌───────────┐
             │ Worker #1 │ │ Worker #2 │ │ Worker #3 │ │ Worker #4 │
             └─────┬─────┘ └─────┬─────┘ └─────┬─────┘ └─────┬─────┘
                   │             │             │             │
                   ├─────────────┴─────────────┴─────────────┤
                   ▼                                         ▼
         [REDIS DISTRIBUTED STATE]                 [CLICKHOUSE CLUSTER]
     • Atomic Lua Session Transition            • Concurrent Block Inserts
     • Distributed Cron Locks (Redlock)         • Parallel Batch MergeTree
```

### 4.1 Kafka Consumer Groups & Partition Affinity
- All worker replicas join the same consumer group: `KAFKA_CONSUMER_GROUP = "openanalytics-worker-group"`.
- Kafka automatically balances partition assignments among active worker instances.
- **Partition Affinity Rule**: Ingest produces messages with key `shop_id + ":" + device_id`.
  - Guarantees that all events from the same shopper always land on the **same partition**.
  - Guarantees that at any given moment, exactly **one worker instance** processes that shopper's events in strict FIFO order, preventing out-of-order session mutations.

### 4.2 Rebalance Safety & Redis Distributed Session State
What happens if Worker #1 dies and Kafka reassigns Partition #0 to Worker #2?
- Because session state is stored in **Redis** (not in local worker RAM), Worker #2 seamlessly picks up where Worker #1 left off.
- **Atomic Session Transition via Redis Lua**:
  When an event arrives, the worker executes an atomic Lua script:
  ```lua
  -- KEYS[1]: session:<shop_id>:<device_id>
  -- ARGV[1]: current_event_time_ms, ARGV[2]: max_idle_ms (30m), ARGV[3]: new_session_id
  local last_seen = redis.call('HGET', KEYS[1], 'last_seen_at')
  if not last_seen or (tonumber(ARGV[1]) - tonumber(last_seen)) > tonumber(ARGV[2]) then
      -- New Session or Boundary
      redis.call('HMSET', KEYS[1], 'id', ARGV[3], 'started_at', ARGV[1], 'last_seen_at', ARGV[1])
      redis.call('EXPIRE', KEYS[1], 1800)
      return { 'new', ARGV[3] }
  else
      -- Extend Existing Session
      redis.call('HSET', KEYS[1], 'last_seen_at', ARGV[1])
      redis.call('EXPIRE', KEYS[1], 1800)
      local current_id = redis.call('HGET', KEYS[1], 'id')
      return { 'extend', current_id }
  end
  ```
  This guarantees zero race conditions across worker restarts, scale-outs, or partition rebalances.

### 4.3 Concurrent ClickHouse Batch Flushing
- ClickHouse's `MergeTree` engine is designed for concurrent multi-client batch writes.
- Each worker instance buffers up to `5,000 events` (or `2 seconds` timer) in local memory.
- When the threshold triggers, that worker opens a native TCP block stream and flushes its batch to ClickHouse. ClickHouse smoothly merges these parts in the background without locking.

### 4.4 Distributed Cron & Maintenance Execution (Redis Leader Lock)
Periodic jobs (e.g. session vacuuming, hourly rollups, ML feature extraction) must **never** run redundantly on every worker instance:
- Workers coordinate using a distributed Redis lock pattern (`Redlock` or `SETNX`):
  ```go
  acquired, err := rdb.SetNX(ctx, "lock:cron:hourly_rollup", workerID, 10*time.Minute).Result()
  if acquired {
      // Exactly ONE worker instance executes the maintenance task
      runHourlyRollup()
  }
  ```

---

## 5. Multi-Tenant Identity & UUIDv7 Standard

All entity identifiers in `OpenAnalytics` use **UUIDv7** (time-ordered 128-bit identifiers):

### Why UUIDv7?
1. **Index Performance & B-Trees**: Unlike random UUIDv4 which causes severe B-tree page fragmentation in PostgreSQL, UUIDv7 is monotonically increasing based on millisecond timestamp.
2. **ClickHouse Ordering**: Time-locality of UUIDv7 aligns with ClickHouse sorting keys (`ORDER BY`), yielding superior compression and faster range queries.
3. **Decentralized Generation**: IDs can be generated safely on any distributed Go worker or client without database sequence locks.

### Entity Hierarchy

```
┌─────────────────────────────────────────────────────────────────────┐
│ 1. TENANT (`tenant_id`: UUIDv7)                                     │
│    The merchant organization / enterprise account in `ai-cart`.     │
├─────────────────────────────────────────────────────────────────────┤
│ 2. SHOP (`shop_id`: UUIDv7)  ──► (Replaces OpenPanel's `project_id`)│
│    The specific storefront instance (e.g., US Store, EU Store).     │
├─────────────────────────────────────────────────────────────────────┤
│ 3. CUSTOMER / SHOPPER (`customer_id`: UUIDv7)                       │
│    The registered shopper. Optional / Null if guest visitor.        │
├─────────────────────────────────────────────────────────────────────┤
│ 4. DEVICE (`device_id`: UUIDv7 or SHA-256 Hash)                     │
│    Anonymous fingerprint generated from IP + UA + Store Salt.       │
├─────────────────────────────────────────────────────────────────────┤
│ 5. SESSION (`session_id`: UUIDv7)                                   │
│    30-minute rolling visit window tracking entry/exit/referrer.     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 6. Database Schemas

### 6.1 ClickHouse DDL (`openanalytics`)

```sql
CREATE DATABASE IF NOT EXISTS openanalytics;

-- Raw Events Table
CREATE TABLE IF NOT EXISTS openanalytics.events (
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
    
    properties Map(String, String) CODEC(ZSTD(3)),
    created_at DateTime64(3) CODEC(DoubleDelta, ZSTD(3))
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(created_at)
ORDER BY (tenant_id, shop_id, toDate(created_at), session_id, name, created_at)
SETTINGS index_granularity = 8192;

-- Sessions Table (ReplacingMergeTree for deduplication on update)
CREATE TABLE IF NOT EXISTS openanalytics.sessions (
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
```

---

### 6.2 PostgreSQL DDL (`analytics_db`)

PostgreSQL stores tenant dashboards, visual widgets, saved funnels, and alert configurations:

```sql
-- Extension for UUID generation if needed
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
```

---

## 7. Behavioral Analysis & Machine Learning Architecture (LightGBM)

### Key LightGBM Models
1. **Real-time Purchase Propensity Score ($P_{buy} \in [0.0, 1.0]$)**:
   - High score ($>0.75$): Ready to convert. Avoid aggressive discounts (protect margins).
   - Medium score ($0.40 - 0.74$): Hesitant buyer. Show personalized social proof or free shipping badge.
   - Low score with high cart value: High abandonment risk. Trigger exit-intent nudge or timed discount coupon.
2. **Shopper Revisit / Churn Propensity**:
   - Scores registered customers on repeat-purchase likelihood based on RFM features.

### Feature Pipeline & Online Inference

```
┌────────────────────────────────────────────────────────────────────────┐
│ 1. CLICKHOUSE FEATURE STORE (Batch / Hourly)                          │
│                                                                        │
│   Materialized view computes rolling features:                         │
│   • `rolling_dwell_time`: Average time spent per product page.         │
│   • `view_to_cart_ratio`: Product page visits vs cart adds.            │
│   • `category_switch_count`: Browsing across multiple departments.     │
│   • `past_order_count`, `past_aov`: Historical shopper metrics.        │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ Batch Export (Parquet)
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│ 2. PYTHON TRAINING PIPELINE (`ml/training/`)                           │
│                                                                        │
│   • Train `lightgbm.LGBMClassifier` with early stopping and AUC eval.  │
│   • Convert model to ONNX via `onnxmltools` / `skl2onnx`.              │
│   • Output artifact: `ml/models/cart_intent_v1.onnx`.                  │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ Deploy Artifact
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│ 3. REAL-TIME GO STREAM INFERENCE (`cmd/ml-worker`)                     │
│                                                                        │
│   • Reads live event stream from Kafka (91.99.83.171:9092).            │
│   • Accumulates in-session features in Redis (6379) per device.         │
│   • Evaluates ONNX runtime (`yalue/onnxruntime_go`) in < 0.4 ms.       │
│   • Updates Redis: `SET shopper:intent:<device_id> 0.82 EX 1800`       │
│   • Downstream `ai-cart` frontend queries intent to adjust UI dynamic! │
└────────────────────────────────────────────────────────────────────────┘
```

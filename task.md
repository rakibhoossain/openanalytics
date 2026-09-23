# OpenAnalytics Implementation Tasks

This document tracks all implementation phases and tasks step-by-step for the OpenAnalytics platform.

---

## Phase 1: Infrastructure & Project Scaffolding
- [x] Create project layout (`cmd/`, `internal/`, `pkg/`, `deploy/`, `ml/`, `ui/`)
- [x] Configure standard domain models with UUIDv7 (`internal/domain/types.go`)
- [x] Set up unified environment configuration (`internal/config/config.go`, `.env`, `.env.example`)
- [x] Set up Docker Compose backing services (`postgres:17`, `clickhouse:26.8`, `redis:7`, `geoipupdate:v7`)
- [x] Verify Docker services health and table migrations (`openpanel.events`, `openpanel.sessions`, `dashboards`, `chart_widgets`)
- [x] Set up GeoIP automatic updater (`data/geo/`) and hot-swappable reader in Go (`internal/geo`)
- [x] Verify Kafka broker (`91.99.83.171:9092`) topic creation and connectivity

---

## Phase 2: Ingestion Engine (`cmd/ingest`)
- [x] Implement partitioned Kafka producer (`internal/kafka/producer.go`)
  - [x] Support batching, async flush, and snappy compression
  - [x] Hash-based partition keying: `hash(shop_id:device_id)`
- [x] Implement fast User-Agent parser (`pkg/uaparser/parser.go`)
- [x] Implement device fingerprinting (`pkg/hash/hash.go`)
- [x] Implement Ingest HTTP Handlers (`internal/ingest/handler.go`):
  - [x] `POST /api/v1/track`: Single event ingestion
  - [x] `POST /api/v1/batch`: Multi-event array ingestion
  - [x] `GET /api/v1/track/device-id`: Device ID & active session resolution
  - [x] Client IP extraction (supporting `CF-Connecting-IP`, `X-Forwarded-For`, `X-Real-IP`)
  - [x] Real-time GeoIP & datacenter ASN enrichment
  - [x] Fast zero-alloc JSON validation
- [x] Wire into `cmd/ingest/main.go` using Go-Chi with CORS and graceful shutdown
- [x] Write integration and unit tests for `cmd/ingest` (`internal/ingest/handler_test.go`)
- [ ] Accuracy & Parity Enhancements (Post-MVP):
  - [ ] Port/convert `/Users/rakib/Projects/analytics/ua-parser-js` regex rules to Go for 100% parity
  - [ ] Port 2,800+ search & social referrers from `openpanel/packages/common/server/referrers/index.ts` into static Go Trie map
  - [ ] Port full bot suspicion rules & Client Hints (`Sec-CH-UA`, `Sec-Fetch-*`) from `openpanel/apps/api/src/bots/suspicion.ts`

---

## Phase 3: Stream Worker & Session State Machine (`cmd/worker`)
- [x] Implement Kafka Consumer Group worker (`internal/kafka/consumer.go`)
  - [x] Consumer group: `openanalytics-worker-group`
  - [x] Rebalance listener & partition management
- [x] Implement Redis Lua Session State Machine (`internal/session/manager.go`)
  - [x] 30-minute sliding idle timeout boundary detection
  - [x] Initial referrer & UTM parameter inheritance across mid-session events
  - [x] Session start / session end event generation
- [x] Implement Native TCP ClickHouse Batch Writer (`internal/clickhouse/batch.go`)
  - [x] Micro-batching buffer: 5,000 events or 2-second timeout
  - [x] Concurrent columnar block stream insertion into `openpanel.events` and `openpanel.sessions`
  - [x] Retry with exponential backoff on ClickHouse connection interruptions
- [x] Implement distributed cron leader locks via Redis (`internal/redis/lock.go`)
- [x] Add unit tests for session manager, redis locks, and clickhouse batch writer (`internal/session/manager_test.go`, `internal/redis/lock_test.go`, `internal/clickhouse/batch_test.go`)

---

## Phase 4: Analytics Query Engine (`cmd/query`)
- [x] Implement PostgreSQL metadata repositories (`internal/postgres/`):
  - [x] Dashboard CRUD (grid layout, tenant/shop scoping)
  - [x] Chart Widget CRUD (chart type, metric rules, filters)
  - [x] Saved Reports & Alert Rules
- [x] Implement ClickHouse Query Builders (`internal/query/`):
  - [x] Time-series Trends (`page_views`, `visitors`, `cart_adds`, `revenue`)
  - [x] Step-by-Step Conversion Funnels (via ClickHouse `windowFunnel`)
  - [x] Cohort Retention Matrix
  - [x] Real-time Live Visitors (devices, countries, paths)
  - [x] Individual Shopper Journey Timeline
- [x] Mount Go-Chi REST APIs for Dashboard integration (`internal/query/handler.go`, `cmd/query/main.go`)
- [x] Unit and integration tests for PostgreSQL repository and Query handler (`internal/postgres/repository_test.go`, `internal/query/handler_test.go`)

---

## Phase 5: Behavioral Machine Learning (`cmd/ml-worker`)
- [x] Build ClickHouse feature store materialized views (`ml/feature_store.sql`):
  - [x] Rolling dwell time, view-to-cart ratio, category switch frequency
- [x] Implement Python LightGBM training pipeline (`ml/training/train_cart_intent.py`):
  - [x] Train purchase propensity and cart abandonment classifier
  - [x] Export trained model artifact to ONNX & JSON (`data/models/cart_intent_v1.onnx`, `data/models/cart_intent_v1.json`)
- [x] Implement Go behavioral ML inference worker (`internal/ml/scorer.go`, `cmd/ml-worker/main.go`):
  - [x] Consume Kafka clickstream events in real time
  - [x] Run sub-millisecond inference via calibrated logistic sigmoid weights
  - [x] Cache shopper rolling features & intent score in Redis (`shopper:intent:<device_id>`)
  - [x] Emit high-intent conversion alerts and webhook markers
- [x] Add unit tests for ML scorer (`internal/ml/scorer_test.go`)

---

## Phase 6: Dashboard UI Adaptation (`ui/`)
- [x] Strip standalone Lucia auth & Stripe billing; direct multi-tenant JWT / header integration
- [x] Embeddable executive dashboard with rich aesthetics, dark theme, and micro-animations (`ui/index.html`, `ui/style.css`, `ui/app.js`)
- [x] Wire UI widgets to `cmd/query` REST APIs (:8081/ui)
- [x] Real-time live shopper radar, funnel analysis, and behavioral ML intent stream
- [x] End-to-end integration and verification

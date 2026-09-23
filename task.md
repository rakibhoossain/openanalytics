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
- [ ] Implement partitioned Kafka producer (`internal/kafka/producer.go`)
  - [ ] Support batching, async flush, and snappy compression
  - [ ] Hash-based partition keying: `hash(shop_id:device_id)`
- [ ] Implement fast User-Agent parser (`pkg/uaparser/parser.go`)
- [ ] Implement device fingerprinting (`pkg/hash/hash.go`)
- [ ] Implement Ingest HTTP Handlers (`internal/ingest/handler.go`):
  - [ ] `POST /api/v1/track`: Single event ingestion
  - [ ] `POST /api/v1/batch`: Multi-event array ingestion
  - [ ] `GET /api/v1/track/device-id`: Device ID & active session resolution
  - [ ] Client IP extraction (supporting `CF-Connecting-IP`, `X-Forwarded-For`, `X-Real-IP`)
  - [ ] Real-time GeoIP & datacenter ASN enrichment
  - [ ] Fast zero-alloc JSON validation
- [ ] Write integration and benchmark tests for `cmd/ingest` (target: >30,000 req/sec)

---

## Phase 3: Stream Worker & Session State Machine (`cmd/worker`)
- [ ] Implement Kafka Consumer Group worker (`internal/kafka/consumer.go`)
  - [ ] Consumer group: `openanalytics-worker-group`
  - [ ] Rebalance listener & partition management
- [ ] Implement Redis Lua Session State Machine (`internal/session/manager.go`)
  - [ ] 30-minute sliding idle timeout boundary detection
  - [ ] Initial referrer & UTM parameter inheritance across mid-session events
  - [ ] Session start / session end event generation
- [ ] Implement Native TCP ClickHouse Batch Writer (`internal/clickhouse/batch.go`)
  - [ ] Micro-batching buffer: 5,000 events or 2-second timeout
  - [ ] Concurrent columnar block stream insertion into `openpanel.events` and `openpanel.sessions`
  - [ ] Retry with exponential backoff on ClickHouse connection interruptions
- [ ] Implement distributed cron leader locks via Redis (`internal/redis/lock.go`)

---

## Phase 4: Analytics Query Engine (`cmd/query`)
- [ ] Implement PostgreSQL metadata repositories (`internal/postgres/`):
  - [ ] Dashboard CRUD (grid layout, tenant/shop scoping)
  - [ ] Chart Widget CRUD (chart type, metric rules, filters)
  - [ ] Saved Reports & Alert Rules
- [ ] Implement ClickHouse Query Builders (`internal/query/`):
  - [ ] Time-series Trends (`page_views`, `visitors`, `cart_adds`, `revenue`)
  - [ ] Step-by-Step Conversion Funnels
  - [ ] Cohort Retention Matrix
  - [ ] Real-time Live Visitors (using Redis HyperLogLog)
  - [ ] Individual Shopper Journey Timeline
- [ ] Mount Go-Chi REST APIs for Dashboard integration

---

## Phase 5: Behavioral Machine Learning (`cmd/ml-worker`)
- [ ] Build ClickHouse feature store materialized views:
  - [ ] Rolling dwell time, view-to-cart ratio, category switch frequency
- [ ] Implement Python LightGBM training pipeline (`ml/training/train_cart_intent.py`):
  - [ ] Train purchase propensity and cart abandonment classifier
  - [ ] Export trained model artifact to ONNX (`data/models/cart_intent_v1.onnx`)
- [ ] Implement Go ONNX inference worker (`internal/ml/scorer.go`):
  - [ ] Consume Kafka clickstream events in real time
  - [ ] Run inference in <0.5ms via ONNX runtime
  - [ ] Cache shopper intent score in Redis (`shopper:intent:<device_id>`)
  - [ ] Emit high-intent conversion webhooks

---

## Phase 6: Dashboard UI Adaptation (`ui/`)
- [ ] Strip standalone Lucia auth & Stripe billing from OpenPanel React UI
- [ ] Integrate with `ai-cart-dashboard` via tenant JWT / iframe context
- [ ] Wire UI widgets to `cmd/query` REST APIs
- [ ] End-to-end user acceptance testing

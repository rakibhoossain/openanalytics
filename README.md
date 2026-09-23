# OpenAnalytics

High-throughput, multi-tenant analytics and behavioral machine-learning platform built for the **`ai-cart`** e-commerce SaaS ecosystem.

`OpenAnalytics` replaces the heavy Node.js ingestion and worker layers of OpenPanel with a blazing-fast **Golang Data Plane (built on Go-Chi `github.com/go-chi/chi/v5`)**, stream processing via **Kafka / Redpanda**, horizontally scalable multi-instance workers, columnar analytical storage in **ClickHouse**, persistent report/dashboard metadata in **PostgreSQL**, distributed session state in **Redis**, and real-time shopper intent prediction using **LightGBM / ONNX**.

---

## Infrastructure Topology

The system connects to the following infrastructure:

| Service | Address | Purpose |
| :--- | :--- | :--- |
| **HTTP Framework** | `go-chi/chi/v5` | Zero-allocation HTTP routing & middleware |
| **Kafka Broker** | `91.99.83.171:9092` | High-throughput distributed event streaming |
| **ClickHouse** | `127.0.0.1:9000` (Native TCP) | Columnar analytics, funnels, retention, ML feature store |
| **Redis** | `127.0.0.1:6379` | Distributed session state across workers, live counters, intent cache |
| **PostgreSQL** | `localhost:5435/openpanel` (Docker) | Saved reports, custom dashboards, chart widgets, alerts |

---

## Key Features

- **Blazing Fast Ingestion (Go-Chi)**: Ingests 50,000+ events/sec per node with sub-5ms $p99$ response times and low memory footprint (~30MB per container).
- **Multi-Instance Worker Scalability**: Workers scale horizontally across $N$ instances using Kafka Consumer Groups, with distributed session state coordinated through atomic Redis Lua scripts.
- **UUIDv7 Standard**: All entity IDs (`tenant_id`, `shop_id`, `customer_id`, `session_id`, `event_id`) use time-ordered 128-bit UUIDv7 for optimal B-tree indexing and ClickHouse sorting.
- **Stream-First Architecture**: Decoupled ingestion and storage via Kafka partitioned by `shop_id:device_id`.
- **Hybrid Storage Model**:
  - **ClickHouse**: Billions of raw telemetry events, aggregate metrics, and time-series rollups.
  - **PostgreSQL (`analytics_db`)**: Custom merchant dashboards, visual chart widgets, saved reports, and alert triggers.
  - **Redis**: Ephemeral session state across worker instances, real-time live shoppers via HyperLogLog, and sub-millisecond intent cache.
- **Behavioral ML with LightGBM**: Real-time purchase intent and cart abandonment scoring powered by embedded ONNX runtime.
- **Clean SaaS Integration**: No redundant internal auth or billing; delegates directly to `ai-cart`'s API gateway and tenant tokens.

---

## Repository Layout (Standard Enterprise Go Layout)

```
openanalytics/
├── cmd/                               # Application entrypoints
│   ├── ingest/                        # High-throughput Go-Chi HTTP ingestion server (POST /track)
│   │   └── main.go
│   ├── worker/                        # Multi-instance stream consumer & ClickHouse batch flusher
│   │   └── main.go
│   ├── query/                         # Go-Chi analytics query API for merchant dashboard
│   │   └── main.go
│   └── ml-worker/                     # Real-time LightGBM inference stream processor
│       └── main.go
│
├── internal/                          # Private application code (not importable by external apps)
│   ├── config/                        # Unified environment & runtime configuration
│   ├── domain/                        # Core domain models (Event, Session, Customer, Shop, UUIDv7)
│   ├── geo/                           # MaxMind GeoLite2 auto-downloader (7-day ticker) & hot-swap service
│   ├── ingest/                        # Go-Chi HTTP handlers, bot checks, device fingerprinting
│   ├── session/                       # Distributed Redis Lua session state machine (30m window)
│   ├── clickhouse/                    # ClickHouse connection pool, native TCP block flusher
│   ├── postgres/                      # PostgreSQL client, report/dashboard/widget repositories
│   ├── redis/                         # Redis client, distributed lock manager, live counters
│   ├── kafka/                         # Kafka partitioned producer and consumer group workers
│   ├── query/                         # Analytics query builders (Funnels, Retention, Trends, Live)
│   └── ml/                            # Embedded ONNX runtime & feature transformer
│
├── pkg/                               # Public shared libraries (reusable across services)
│   ├── uuidv7/                        # High-performance UUIDv7 generator & parser
│   ├── geoip/                         # Embedded MaxMind GeoIP2 MMDB lookup
│   ├── uaparser/                      # High-speed User-Agent parsing
│   ├── hash/                          # Device ID salting & SHA-256 fingerprinting
│   └── httputil/                      # Middleware, zero-alloc JSON serializers, health checks
│
├── ml/                                # Machine learning offline training pipeline (Python)
│   ├── notebooks/                     # Exploratory analysis & feature validation
│   ├── training/                      # LightGBM training scripts (`train_cart_intent.py`)
│   ├── models/                        # Exported `.onnx` and `.txt` model artifacts
│   └── requirements.txt
│
├── ui/                                # Merchant analytics dashboard (TanStack / React)
│   ├── src/                           # Visualizations: Trends, Funnels, Retention, Live
│   ├── package.json
│   └── vite.config.ts
│
├── deploy/                            # Infrastructure and deployment descriptors
│   ├── docker/                        # Multi-stage Dockerfiles for Go binaries
│   │   ├── Dockerfile.ingest
│   │   ├── Dockerfile.worker
│   │   └── Dockerfile.query
│   ├── clickhouse/                    # ClickHouse table DDL & materialized views
│   ├── postgres/                      # PostgreSQL schema migrations (dashboards, widgets)
│   └── docker-compose.yml             # Local dev environment
│
├── architecture.md                    # In-depth architectural blueprint and technical decisions
├── .env.example                       # Reference environment variables
├── Makefile                           # Development, test, and build targets
└── go.mod                             # Go modules definition
```

---

## Multi-Instance Worker Design

The backend worker (`cmd/worker`) is designed to run as **multiple stateless container instances**:

1. **Kafka Consumer Groups**: Instances join `openanalytics-worker-group`. Kafka automatically balances partitions among active instances.
2. **Partition Affinity**: Events are keyed by `shop_id:device_id`, guaranteeing all events for a given shopper arrive sequentially at the same worker.
3. **Rebalance Safety**: Session state is persisted in **Redis** with atomic Lua scripts. If an instance restarts or Kafka rebalances, the new worker seamlessly continues the session.
4. **Independent ClickHouse Batches**: Each worker accumulates up to 5,000 events locally and streams them directly into ClickHouse via native TCP. ClickHouse merges batches concurrently in the background.
5. **Distributed Cron Locks**: Scheduled maintenance tasks (session vacuuming, hourly rollups) use Redis `SETNX` distributed locks so only one worker instance executes them at a time.

---

## Quick Start (Environment Setup)

### 1. Configure Environment
```bash
cp .env.example .env
```

### 2. Run the Go Ingest Service (Go-Chi)
```bash
go run cmd/ingest/main.go
```

### 3. Run Worker Instances (Horizontally Scalable)
```bash
# Terminal 1 - Worker Instance #1
WORKER_ID=worker-1 go run cmd/worker/main.go

# Terminal 2 - Worker Instance #2
WORKER_ID=worker-2 go run cmd/worker/main.go
```

### 4. Run the Analytics Query API
```bash
go run cmd/query/main.go
```

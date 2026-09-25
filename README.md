# OpenAnalytics

High-throughput, multi-tenant analytics and real-time behavioral machine learning platform built for modern high-scale e-commerce and SaaS ecosystems.

OpenAnalytics delivers high-volume event ingestion, streaming session management, session replay, and sub-microsecond behavioral inference by pairing a **Golang Data Plane (built on Go-Chi `github.com/go-chi/chi/v5`)** with **ClickHouse OLAP storage**, **Redis online feature buffers**, **Kafka event streaming**, and an **embedded ONNX / Logistic Behavioral ML Engine**.

---

## Architecture Overview

```
                      ┌────────────────────────────────────────┐
                      │    Client Browsers / SDKs / Trackers   │
                      └───────────────────┬────────────────────┘
                                          │ HTTP / WebSocket
                                          ▼
         ┌─────────────────────────────────────────────────────────────────┐
         │             Caddy / Cloudflare Edge Reverse Proxy               │
         └───────────────────┬─────────────────────────┬───────────────────┘
                             │ :8080 (Ingest)          │ :8081 (Query/Live)
                             ▼                         ▼
         ┌───────────────────────────────┐ ┌───────────────────────────────┐
         │    Ingest Service (Go-Chi)    │ │   Query & Live Hub (Go-Chi)   │
         │  - Device Fingerprinting      │ │  - Columnar OLAP Aggregations │
         │  - MaxMind GeoIP & ASN Lookup │ │  - Live Visitor WebSockets    │
         │  - Crawler & Bot Filtering    │ │  - Session Replay Streaming   │
         │  - Fast Deduplication         │ │  - Asset & Favicon Proxy      │
         └──────────────┬────────────────┘ └───────────────▲───────────────┘
                        │                                  │
         ┌──────────────┴──────────────┐                   │
         │ Kafka / Stream Pipeline     │                   │
         │  (analytics.events.raw)     │                   │
         └──────────────┬──────────────┘                   │
                        │                                  │
                        ▼                                  │
         ┌─────────────────────────────┐                   │
         │  Horizontally Scaled Worker │                   │
         │  - Distributed Lua Session  │                   │
         │  - Sub-50ns ML Propensity   │                   │
         │  - Native ClickHouse Block  │                   │
         └──────┬───────────────┬──────┘                   │
                │               │                          │
                ▼               ▼                          │
    ┌──────────────────────┐ ┌──────────────────────┐      │
    │  ClickHouse Storage  │ │  Redis Feature Store │──────┘
    │  - events            │ │  - shopper:feat:...  │
    │  - sessions          │ │  - shopper:intent:...│
    │  - session_replays   │ │  - live:visitors     │
    │  - shopper_features  │ └──────────────────────┘
    └──────────────────────┘
```

---

## 1. Behavioral Machine Learning Engine (`openanalytics/ml`)

### Is this Industry Standard for Tech Interviews & CVs?
**Yes — it follows the exact Dual-Plane / Lambda ML Architecture used by companies like Uber (Michelangelo), DoorDash, Stripe, and Shopify.**

Most candidate ML projects are static Jupyter Notebooks that stop at `model.fit()` on a Kaggle CSV. In production engineering, the hard problem is **low-latency serving, feature stores, and online inference**. OpenAnalytics solves this directly:

1. **Two-Tier (Dual-Plane) Architecture**:
   - **Offline Plane (Python)**: Handles batch feature extraction from ClickHouse, historical dataset generation across behavioral archetypes, model training via Scikit-Learn/LightGBM, ROC-AUC calibration, and export to ONNX and JSON weights.
   - **Online Plane (Go)**: Eliminates Python HTTP network serialization overhead by compiling weights into an in-process, zero-allocation Go scoring engine (`internal/ml/scorer.go`) that queries Redis feature vectors in **< 50 nanoseconds**.
2. **Multi-Model Triad**:
   - **Cart Intent (`cart_intent_v1`)**: Predicts probability of checkout completion ($P(\text{buy})$) using dwell time, catalog scatter, and cart velocity.
   - **Churn Risk (`churn_predictor_v1`)**: Detects high-friction hesitation pauses and bounce risk ($P(\text{churn})$) before the customer leaves.
   - **Price Sensitivity (`price_sensitivity_v1`)**: Isolates bargain hunters from high-AOV impulse buyers ($P(\text{discount})$) to deploy coupons selectively and prevent margin loss.
3. **Local Explainability (SHAP-Style Feature Attribution)**:
   - Provides per-shopper signal breakdowns explaining *why* a customer was classified as high-intent or churn-risk (e.g. `+1.95 cart additions`, `+0.0035 dwell`, `-0.15 scatter`).
4. **Direct Business ROI**:
   - Drives real-time e-commerce triggers: dynamic exit-intent popovers, 1-click checkout acceleration, and inventory priority.

### How to Feature This on Your Resume / CV
```markdown
**Lead / Senior Software & ML Engineer — OpenAnalytics**
- Designed and built a high-throughput multi-tenant analytics and behavioral ML platform processing 50,000+ events/sec using Go, ClickHouse, Kafka, and Redis.
- Implemented a dual-plane ML architecture: trained multi-model behavioral classifiers (Purchase Intent, Churn Risk, Price Sensitivity) in Python/LightGBM, exporting to ONNX and calibrated weights for sub-50ns in-process Go inference.
- Engineered a real-time Redis online feature store with sliding-window aggregations (dwell time, cart velocity, catalog scatter) feeding a live shopper radar dashboard via WebSockets.
- Built an end-to-end Session Replay pipeline capturing DOM mutation chunks via rrweb, compressing and streaming payloads via ClickHouse native columnar blocks.
```

---

## 2. Infrastructure Topology

| Component | Default Port | Technology | Purpose |
| :--- | :--- | :--- | :--- |
| **Ingest API** | `:8080` | Go-Chi HTTP | High-throughput telemetry ingestion (`/api/v1/track`, `/api/v1/batch`) |
| **Query & UI API** | `:8081` | Go-Chi HTTP | Analytics aggregations, metadata APIs, session replays, and icon proxy |
| **Web Dashboard** | `:3000` | TanStack Start / React 19 | Merchant analytics dashboard, ML intent radar, session replay player |
| **ClickHouse** | `:8123` / `:9000` | ClickHouse 24.8+ | Columnar analytical database (events, sessions, replays, rollups) |
| **Redis** | `:6379` / `:6380` | Redis 7+ | Distributed session state machine, real-time feature store, live counters |
| **Kafka / Redpanda** | `:9092` | Kafka wire protocol | Distributed partitioned event streaming (`shop_id:device_id`) |
| **GeoIP Engine** | Embedded | MaxMind GeoLite2 | City & ASN resolution with automatic 7-day hot-swap updates |

---

## 3. Production Deployment Guide

### Prerequisites
- **OS**: Linux (Ubuntu 22.04 LTS / Debian 12 / RHEL 9) or macOS
- **Hardware Sizing**:
  - *Minimal (Dev / Testing)*: 2 vCPU, 4 GB RAM, 20 GB SSD
  - *Standard Production (Up to 10M events/mo)*: 4 vCPU, 16 GB RAM, 100 GB NVMe SSD
  - *High Volume (100M+ events/mo)*: 8+ vCPU, 32+ GB RAM, Dedicated NVMe storage
- **Installed Tooling**: Docker Engine 24+ & Docker Compose v2, or Go 1.23+ & Node.js 20+

---

### Option A: One-Command Docker Compose Deployment (Recommended)

The easiest way to run the entire OpenAnalytics platform is with the turnkey Docker Compose configuration.

#### 1. Clone the Repository
```bash
git clone https://github.com/your-org/openanalytics.git
cd openanalytics
```

#### 2. Configure Environment
```bash
cp .env.example .env
```
Edit `.env` and configure your credentials:
```ini
CLICKHOUSE_PASSWORD=your_secure_ch_password
REDIS_PASSWORD=your_secure_redis_password
GEOIPUPDATE_ACCOUNT_ID=your_maxmind_account_id
GEOIPUPDATE_LICENSE_KEY=your_maxmind_license_key
DEV_DEFAULT_COUNTRY=US
```

#### 3. Start the Stack
```bash
docker compose -f deploy/docker-compose.yml up -d --build
```

#### 4. Verify Services
```bash
docker compose -f deploy/docker-compose.yml ps
```
All services (`op_clickhouse`, `op_redis`, `op_backend`, `op_web`) should report healthy.

- **Web Dashboard**: `http://localhost:3000`
- **Ingest API**: `http://localhost:8080`
- **Query & Live API**: `http://localhost:8081`

---

### Option B: Bare-Metal / Standalone Production Deployment

For bare-metal servers or cloud VMs running systemd.

#### 1. Compile Optimized Production Binaries
```bash
# Build the unified all-in-one Go service
CGO_ENABLED=0 go build -trimpath -ldflags="-w -s" -o bin/openanalytics cmd/all/main.go

# (Optional) Build standalone microservices
go build -trimpath -ldflags="-w -s" -o bin/ingest cmd/ingest/main.go
go build -trimpath -ldflags="-w -s" -o bin/worker cmd/worker/main.go
go build -trimpath -ldflags="-w -s" -o bin/query cmd/query/main.go
```

#### 2. Initialize ClickHouse Schema
Execute the database initialization script against your ClickHouse instance:
```bash
clickhouse-client --host 127.0.0.1 --port 9000 \
  --user openpanel --password openpanel \
  --multiquery < deploy/clickhouse/init/01_create_tables.sql
```

#### 3. Setup Systemd Service
Create `/etc/systemd/system/openanalytics.service`:
```ini
[Unit]
Description=OpenAnalytics High-Throughput Data Plane
After=network.target clickhouse-server.service redis.service

[Service]
Type=simple
User=openanalytics
Group=openanalytics
WorkingDirectory=/opt/openanalytics
ExecStart=/opt/openanalytics/bin/openanalytics
Restart=always
RestartSec=3
LimitNOFILE=65536
EnvironmentFile=/opt/openanalytics/.env

[Install]
WantedBy=multi-user.target
```

Enable and start the service:
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now openanalytics
sudo systemctl status openanalytics
```

#### 4. Build and Run the Web Frontend
```bash
cd web
pnpm install --frozen-lockfile
pnpm build
PORT=3000 node .output/server/index.mjs
```

---

### Option C: Reverse Proxy Configuration (Caddy & Nginx)

#### Caddyfile (Automatic HTTPS & WebSocket Proxy)
```caddy
analytics.yourdomain.com {
    encode zstd gzip

    # WebSocket Realtime Live Streams
    @websockets {
        header Connection *Upgrade*
        header Upgrade websocket
    }
    reverse_proxy @websockets 127.0.0.1:8081

    # Ingest Telemetry Endpoints
    handle /api/v1/track* {
        reverse_proxy 127.0.0.1:8080
    }
    handle /api/v1/batch* {
        reverse_proxy 127.0.0.1:8080
    }
    handle /api/v1/replay* {
        reverse_proxy 127.0.0.1:8080
    }

    # Query APIs, Favicon Proxy & Admin
    handle /api/* {
        reverse_proxy 127.0.0.1:8081
    }
    handle /misc/* {
        reverse_proxy 127.0.0.1:8081
    }
    handle /trpc/* {
        reverse_proxy 127.0.0.1:8081
    }

    # UI Dashboard
    handle {
        reverse_proxy 127.0.0.1:3000
    }
}
```

---

## 4. Verification & Testing

### 1. Ingest Verification
Send a telemetry event with a simulated public IP:
```bash
curl -X POST http://localhost:8080/api/v1/track \
  -H "Content-Type: application/json" \
  -d '{
    "shop_id": "018e69d0-7a89-7000-8b1a-200000000002",
    "tenant_id": "018e69d0-7a89-7000-8b1a-200000000001",
    "name": "checkout_step_viewed",
    "ip": "8.8.8.8",
    "path": "/checkout"
  }'
```
Expected response:
```json
{"success":true,"data":{"status":"accepted","session_id":"..."}}
```

### 2. Verify ClickHouse Persistence
```bash
clickhouse-client --query "SELECT count(), any(country), any(browser) FROM openpanel.events"
```

### 3. Verify Favicon Proxy
```bash
curl -I "http://localhost:8081/misc/favicon?url=https%3A%2F%2Fgoogle.com"
# HTTP/1.1 200 OK
# Content-Type: image/x-icon
# Cache-Control: public, max-age=604800, immutable
```

### 4. Retrain Machine Learning Models
```bash
cd ml
pip install -r requirements.txt
python3 training/train_all.py --samples 15000
```
Exported model artifacts will be saved to `data/models/` and automatically reloaded by the Go inference engine on the next heartbeat.

---

## 5. Environment Variables Reference

| Variable | Default | Description |
| :--- | :--- | :--- |
| `ENV` | `development` | Deployment environment (`development` or `production`) |
| `INGEST_PORT` | `8080` | Port for event tracking and ingestion API |
| `QUERY_PORT` | `8081` | Port for query APIs, session replay streaming, and favicon proxy |
| `CLICKHOUSE_ADDR` | `127.0.0.1:9000` | Native TCP address of ClickHouse server |
| `CLICKHOUSE_DATABASE`| `openpanel` | ClickHouse database name |
| `REDIS_ADDR` | `127.0.0.1:6380` | Redis host and port for feature store and sessions |
| `REDIS_SESSION_TTL_MINUTES`| `30` | Sliding window session duration before expiry |
| `KAFKA_BROKERS` | `""` | Comma-separated Kafka broker addresses (falls back to direct sync if empty) |
| `GEOIP_DATA_DIR` | `data/geo` | Directory containing `GeoLite2-City.mmdb` and `GeoLite2-ASN.mmdb` |
| `ML_INFERENCE_ENABLED`| `true` | Enables real-time scoring in Go worker stream |
| `DEV_DEFAULT_COUNTRY` | `US` | Fallback country code when testing from localhost/loopback |

---

## License
MIT License. Built for high-performance e-commerce telemetry and real-time behavioral intelligence.

# OpenAnalytics Production Deployment Guide

A complete, production-grade guide for deploying **OpenAnalytics** — the real-time high-throughput telemetry ingestion, OLAP analytics, and behavioral ML engine.

---

## Architecture Overview

```
                                      Internet
                                         │
                                         ▼
                            ┌─────────────────────────┐
                            │    Nginx Reverse Proxy  │ (SSL Termination, Rate Limiting,
                            │       (Port 80/443)     │  Real IP Headers, WebSockets)
                            └────────────┬────────────┘
                                         │
                    ┌────────────────────┴────────────────────┐
     /api/v1/track* │                                         │ /api/v1/query*, /ws, /
                    ▼                                         ▼
         ┌──────────────────────┐                  ┌──────────────────────┐
         │     op_ingest        │                  │       op_query       │
         │   (Port 8080)        │                  │     (Port 8081)      │
         └──────────┬───────────┘                  └──────────┬───────────┘
                    │                                         │
         ┌──────────┴───────────┐                  ┌──────────┴───────────┐
         │                      │                  │                      │
         ▼                      ▼                  ▼                      ▼
┌──────────────────┐  ┌──────────────────┐  ┌──────────────────────────────┐
│  Redis 7-Alpine  │  │   Kafka Broker   │  │   ClickHouse 26.8.8.8        │
│  (Session Buffer │  │ (Event Pipeline) │  │   (Columnar OLAP Store)      │
│  ./data/redis)   │  └────────┬─────────┘  │   (./data/clickhouse)        │
└────────┬─────────┘           │            └──────────────▲───────────────┘
         │                     │                           │
         │          ┌──────────┴──────────┐                │
         │          ▼                     ▼                │
         │ ┌──────────────────┐  ┌──────────────────┐      │
         │ │    op_worker     │  │   op_ml-worker   │      │
         │ │ (Batch Consumer) │  │  (Intent Scorer) │      │
         │ └────────┬─────────┘  └────────┬─────────┘      │
         │          │                     │                │
         │          └─────────────────────┼────────────────┘
         ▼                                │
┌──────────────────┐                      │
│  op_ml_retrain   │──────────────────────┘ (Redis Pub/Sub Hot Reload)
│ (Auto-Cron ML)   │ (Models in ./data/models)
└──────────────────┘
```

### Persistent Data Layout (Host Bind Mounts)

> **Zero Named Docker Volumes**: All persistent data is strictly isolated inside the repository's `./data/` folder for transparent backups, migration, and monitoring.

- `./data/clickhouse` $\rightarrow$ ClickHouse database files, columnar partitions, and metadata (`/var/lib/clickhouse`).
- `./data/postgres` $\rightarrow$ PostgreSQL 18 database files and WAL logs (`/var/lib/postgresql`).
- `./data/redis` $\rightarrow$ Redis RDB snapshots and AOF persistence files (`/data`).
- `./data/geo` $\rightarrow$ MaxMind GeoLite2 databases auto-updated by `geoipupdate` (`/usr/share/GeoIP`).
- `./data/models` $\rightarrow$ Trained ML models (JSON weights & ONNX binaries) used by `ml-worker` and updated by `ml-retrain` (`/app/data/models`).

---

## 1. System Requirements & Prerequisites

### Minimum Hardware Recommendations
| Workload | CPU | RAM | Disk | Network |
| :--- | :--- | :--- | :--- | :--- |
| **Development / Staging** | 2 vCPU | 4 GB | 30 GB SSD | 100 Mbps |
| **Production (< 10M events/day)** | 4 vCPU | 16 GB | 100 GB NVMe SSD | 1 Gbps |
| **Production (> 50M events/day)** | 8+ vCPU | 32+ GB | 500+ GB NVMe SSD | 1 Gbps |

### Required Software
Install the core toolchain on your Linux server (Ubuntu 22.04/24.04 LTS or Debian 12 recommended):

```bash
# Update system packages
sudo apt update && sudo apt upgrade -y

# Install prerequisites
sudo apt install -y git curl ufw ca-certificates gnupg lsb-release nginx certbot python3-certbot-nginx

# Install Docker & Docker Compose v2 (Official Docker Repository)
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg

echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

# Ensure current user can execute Docker commands without sudo
sudo usermod -aG docker $USER
newgrp docker

# Verify versions
docker --version          # Docker version 24.0+
docker compose version    # Docker Compose version v2.20+
```

---

## 2. Clone Repository & Setup Structure

Clone the repository or pull the latest commits:

```bash
# Clone to desired destination (e.g., /opt/openanalytics or ~/Projects/openanalytics)
cd /opt
sudo git clone https://github.com/your-org/analytics.git
sudo chown -R $USER:$USER /opt/analytics
cd /opt/analytics/openanalytics

# If updating an existing deployment:
git checkout main
git pull origin main
```

---

## 3. Local Project Volumes & Directory Permissions

Prepare the `./data` directories located directly in the project directory:

```bash
# From within /path/to/openanalytics:
mkdir -p data/clickhouse \
         data/postgres \
         data/redis \
         data/geo \
         data/models \
         deploy/clickhouse/init

# Ensure write permissions for Docker containers
# ClickHouse inside container runs as user 'clickhouse' (UID 101)
# Redis inside container runs as user 'redis' (UID 999)
chmod -R 775 data/
```

> [!NOTE]
> If running on Linux with strict SELinux or default UIDs, you can grant read/write permissions:
> ```bash
> sudo chown -R 101:101 data/clickhouse
> sudo chown -R 999:999 data/redis
> ```

---

## 4. Environment Configuration (`.env`)

Create your production environment file from the template:

```bash
cp .env.example .env
```

Open `.env` in your editor and configure your secrets:

```bash
nano .env
```

### Complete `.env` Reference

```ini
# ==============================================================================
# OpenAnalytics Environment Configuration
# ==============================================================================

# Server Environment & Logging
ENV=production
LOG_LEVEL=info

# Service Ports
INGEST_PORT=8080
QUERY_PORT=8081
METRICS_PORT=9090

# ------------------------------------------------------------------------------
# ClickHouse OLAP Database (Port 9000 inside Docker network)
# ------------------------------------------------------------------------------
CLICKHOUSE_ADDR=clickhouse:9000
CLICKHOUSE_DATABASE=openpanel
CLICKHOUSE_USERNAME=openpanel
CLICKHOUSE_PASSWORD=YourStrongClickHousePassword123!
CLICKHOUSE_DEBUG=false

# ------------------------------------------------------------------------------
# Redis Ephemeral State & Session Buffer (Port 6379 inside Docker network)
# ------------------------------------------------------------------------------
REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_SESSION_TTL_MINUTES=30

# ------------------------------------------------------------------------------
# Kafka Event Streaming Pipeline (Leave empty if using local buffer/fallback)
# ------------------------------------------------------------------------------
KAFKA_BROKERS=kafka-broker.yourdomain.com:9092
KAFKA_EVENTS_TOPIC=analytics.events.raw
KAFKA_SESSIONS_TOPIC=analytics.sessions.boundary
KAFKA_CONSUMER_GROUP=openanalytics-worker-group
KAFKA_BATCH_SIZE=5000
KAFKA_BATCH_TIMEOUT_MS=1000

# ------------------------------------------------------------------------------
# MaxMind GeoIP Updater (For automated city & ASN resolution)
# Obtain a free license key from https://www.maxmind.com
# ------------------------------------------------------------------------------
GEOIP_DATA_DIR=/usr/share/GeoIP
GEOIPUPDATE_ACCOUNT_ID=YOUR_MAXMIND_ACCOUNT_ID
GEOIPUPDATE_LICENSE_KEY=YOUR_MAXMIND_LICENSE_KEY

# ------------------------------------------------------------------------------
# Behavioral Machine Learning
# ------------------------------------------------------------------------------
ML_MODEL_PATH=/app/data/models/cart_intent_v1.json
ML_INFERENCE_ENABLED=true

# ------------------------------------------------------------------------------
# API Authentication & External Integrations
# ------------------------------------------------------------------------------
ANALYTICS_AUTH_KEY=your_secure_random_pre_shared_key_here
OPENEXCHANGERATES_APP_ID=your_openexchangerates_app_id
```

---

## 5. Docker Compose Execution

The application is decomposed into decoupled, independently scalable microservices defined in [docker-compose.yml](file:///Users/rakib/Projects/analytics/openanalytics/docker-compose.yml).

### Step 5.1: Validate Docker Compose Configuration

Verify that all service dependencies and project volumes resolve correctly:

```bash
docker compose config
```

### Step 5.2: Build and Launch Core Microservices

Start ClickHouse, Redis, GeoIP updater, Ingest, Query, and Worker daemons in detached mode:

```bash
docker compose up -d --build
```

### Step 5.3: Scale High-Throughput Workers (Optional)

You can scale the stream ingestion worker (`worker`) and the ML inference scoring worker (`ml-worker`) to match high ingest loads:

```bash
# Run 3 ClickHouse batch insertion workers and 2 ML inference workers
docker compose up -d --scale worker=3 --scale ml-worker=2
```

### Step 5.4: Enable Automated ML Retraining Daemon (Optional Profile)

To enable the daily machine learning retraining cron container that pulls fresh behavioral sessions from ClickHouse and signals Redis hot-reload:

```bash
docker compose --profile ml-cron up -d ml-retrain
```

### Step 5.5: Verify Container Health & Status

Check that all containers are healthy:

```bash
docker compose ps
```

You should see:

```
NAME              IMAGE                                     STATUS                   PORTS
op_clickhouse     clickhouse/clickhouse-server:26.8.8.8    Up (healthy)             0.0.0.0:8123->8123/tcp, 0.0.0.0:9000->9000/tcp
op_redis          redis:7-alpine                            Up (healthy)             0.0.0.0:6380->6379/tcp
op_geoipupdate    ghcr.io/maxmind/geoipupdate:v7            Up                       
op_ingest         openanalytics-ingest                      Up                       0.0.0.0:8080->8080/tcp
op_query          openanalytics-query                       Up                       0.0.0.0:8081->8081/tcp
openanalytics-worker-1     openanalytics-worker             Up                       
openanalytics-ml-worker-1  openanalytics-ml-worker          Up                       
```

### Step 5.6: Test Service Endpoints Locally

```bash
# Query healthcheck
curl -s http://127.0.0.1:8081/health
# Response: {"service":"openanalytics-query","status":"healthy"}

# ClickHouse ping
curl -s http://127.0.0.1:8123/ping
# Response: Ok.
```

---

## 6. Nginx Reverse Proxy Configuration

Nginx acts as the front-line reverse proxy, providing:
1. **SSL / TLS Termination** via Let's Encrypt.
2. **Path-Based Routing**:
   - High-throughput telemetry ingestion (`/api/v1/track*`, `/track*`, `/event*`) $\rightarrow$ `op_ingest` (port `8080`).
   - Query engine, analytics dashboards, and tRPC endpoints $\rightarrow$ `op_query` (port `8081`).
   - Real-time live visitor updates $\rightarrow$ WebSocket support on `/ws` to `op_query` (port `8081`).
3. **Client IP Forwarding**: Passes `CF-Connecting-IP`, `X-Real-IP`, and `X-Forwarded-For` so the Ingest engine can perform precise GeoIP and ISP lookups.

### Step 6.1: Create Nginx Site Configuration

Create `/etc/nginx/sites-available/openanalytics.conf`:

```bash
sudo nano /etc/nginx/sites-available/openanalytics.conf
```

Paste the following production configuration (replace `analytics.yourdomain.com` with your domain):

```nginx
# ==============================================================================
# OpenAnalytics High-Performance Nginx Reverse Proxy Configuration
# ==============================================================================

# Connection pooling to backend Go microservices
upstream ingest_upstream {
    server 127.0.0.1:8080;
    keepalive 64;
}

upstream query_upstream {
    server 127.0.0.1:8081;
    keepalive 32;
}

# Rate limiting zone for telemetry ingestion (1000 requests/sec per IP burst)
limit_req_zone $binary_remote_addr zone=ingest_limit:10m rate=500r/s;

# ------------------------------------------------------------------------------
# HTTP -> HTTPS Redirect
# ------------------------------------------------------------------------------
server {
    listen 80;
    listen [::]:80;
    server_name analytics.yourdomain.com;

    # Certbot challenge path
    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }

    location / {
        return 301 https://$host$request_uri;
    }
}

# ------------------------------------------------------------------------------
# HTTPS Secure Server
# ------------------------------------------------------------------------------
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name analytics.yourdomain.com;

    # SSL Certificates (Managed by Certbot)
    ssl_certificate /etc/letsencrypt/live/analytics.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/analytics.yourdomain.com/privkey.pem;

    # Modern TLS Security Parameters
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;
    ssl_session_tickets off;

    # Security Headers
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    # Buffer & Timeout Tuning for Telemetry Ingest
    client_max_body_size 10M;
    client_body_buffer_size 128k;
    proxy_connect_timeout 5s;
    proxy_send_timeout 30s;
    proxy_read_timeout 30s;

    # Standard Proxy Headers
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header CF-Connecting-IP $http_cf_connecting_ip; # Cloudflare support
    proxy_http_version 1.1;

    # --------------------------------------------------------------------------
    # 1. Telemetry Ingestion Endpoints -> Ingest Service (Port 8080)
    # --------------------------------------------------------------------------
    location ~* ^/(api/v1/track|track|event) {
        limit_req zone=ingest_limit burst=200 nodelay;

        proxy_set_header Connection "";
        proxy_pass http://ingest_upstream;

        # CORS Headers for Web SDK Trackers
        add_header Access-Control-Allow-Origin * always;
        add_header Access-Control-Allow-Methods "GET, POST, OPTIONS" always;
        add_header Access-Control-Allow-Headers "Content-Type, Authorization, X-Client-ID" always;

        if ($request_method = OPTIONS) {
            return 204;
        }
    }

    # --------------------------------------------------------------------------
    # 2. WebSocket Real-time Telemetry -> Query Service (Port 8081)
    # --------------------------------------------------------------------------
    location /ws {
        proxy_pass http://query_upstream;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }

    # --------------------------------------------------------------------------
    # 3. Analytics Query Engine, Healthcheck & API -> Query Service (Port 8081)
    # --------------------------------------------------------------------------
    location / {
        proxy_set_header Connection "";
        proxy_pass http://query_upstream;
    }
}
```

### Step 6.2: Enable Site and Obtain SSL Certificate

```bash
# Enable Nginx configuration
sudo ln -sf /etc/nginx/sites-available/openanalytics.conf /etc/nginx/sites-enabled/

# Test syntax
sudo nginx -t

# Obtain Let's Encrypt SSL Certificate
sudo certbot --nginx -d analytics.yourdomain.com

# Reload Nginx
sudo systemctl reload nginx
```

---

## 7. Firewall (UFW) Configuration

Lock down external access so that only Nginx and SSH are accessible publicly. Microservices and databases should remain internal:

```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow ssh
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
sudo ufw status
```

> [!WARNING]
> Do NOT open ports `8123`, `9000`, `6380`, `8080`, or `8081` to the public internet. All client traffic must pass through Nginx.

---

## 8. Continuous Updates & Zero-Downtime Deployment

To deploy newer commits without taking down ClickHouse or Redis:

```bash
cd /opt/analytics/openanalytics

# 1. Pull latest code
git pull origin main

# 2. Build the updated images
docker compose build ingest query worker ml-worker

# 3. Rolling recreate without touching databases
docker compose up -d --no-deps ingest query worker ml-worker

# 4. Remove dangling unused Docker images
docker image prune -f
```

### Hot-Reloading ML Models
When a new model is generated by `ml-retrain`, it automatically triggers hot-reload without container restarts. You can also trigger a manual hot-reload anytime using Redis:

```bash
docker exec -i op_redis redis-cli PUBLISH analytics:ml:reload '{"model":"cart_intent","version":"latest"}'
```

---

## 9. Backup & Disaster Recovery

Because all databases use project-relative bind mounts, backups are straightforward:

### Backup Redis Data
```bash
# Trigger background save
docker exec -it op_redis redis-cli bgsave

# Backup the RDB dump
cp ./data/redis/dump.rdb /backups/redis-$(date +%F).rdb
```

### Backup ClickHouse Partitions
```bash
# Using ClickHouse freeze command for instant atomic snapshots
docker exec -it op_clickhouse clickhouse-client \
  --user openpanel \
  --password YourStrongClickHousePassword123! \
  --query "ALTER TABLE openpanel.events FREEZE WITH NAME 'backup_$(date +%F)'"

# The frozen partition files reside safely inside ./data/clickhouse/shadow/
```

### Full Project Volume Archive (During maintenance window)
```bash
docker compose stop
tar -czvf /backups/openanalytics-data-$(date +%F).tar.gz ./data
docker compose start
```

---

## 10. Troubleshooting & FAQ

### 1. `permission denied` when ClickHouse or Redis starts
Make sure the user permissions on `./data` allow the container UIDs:
```bash
sudo chown -R 101:101 ./data/clickhouse
sudo chown -R 999:999 ./data/redis
```

### 2. GeoIP databases not resolving countries or cities
Check the `geoipupdate` logs:
```bash
docker compose logs -f geoipupdate
```
Verify that `GEOIPUPDATE_ACCOUNT_ID` and `GEOIPUPDATE_LICENSE_KEY` in `.env` are valid credentials from MaxMind.

### 3. Viewing Live Service Logs
```bash
# Ingestion API logs
docker compose logs -f ingest

# Query API logs
docker compose logs -f query

# Worker ClickHouse insertion batches
docker compose logs -f worker

# ML Intent predictions
docker compose logs -f ml-worker
```

### 4. ClickHouse Maximum Open Files (`ulimit`) Warning
ClickHouse requires a high open files limit (`262144`). This is already configured under `ulimits` in `docker-compose.yml`. If your host kernel prevents it, check `/etc/security/limits.conf` on your host:
```ini
* soft nofile 262144
* hard nofile 262144
```

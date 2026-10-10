#!/usr/bin/env bash
# ==============================================================================
# OpenAnalytics Multi-Platform Docker Build & Push Script
# Builds multi-arch images (linux/amd64, linux/arm64) using Docker Buildx
# and pushes them to Docker Hub for zero-compilation server deployments.
# ==============================================================================

set -euo pipefail

# Configuration
DOCKER_REGISTRY="${DOCKER_REGISTRY:-rakibhoossain}"
VERSION="${VERSION:-1.0.0}"
# Server architecture: 91.99.83.171 is linux/amd64.
# (Set PLATFORMS="linux/amd64,linux/arm64" if you want both)
PLATFORMS="${PLATFORMS:-linux/amd64}"
ACTION="${1:---push}" # Default: --push
SERVICE_FILTER="${2:-all}" # Pass service name (e.g. ml-retrain) to build only that service

# Colors for terminal output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo -e "${CYAN}================================================================${NC}"
echo -e "${CYAN}🚀 OpenAnalytics Multi-Platform Build & Release Pipeline${NC}"
echo -e "${CYAN}================================================================${NC}"
echo -e "Registry:   ${GREEN}${DOCKER_REGISTRY}${NC}"
echo -e "Version:    ${GREEN}${VERSION}${NC}"
echo -e "Platforms:  ${GREEN}${PLATFORMS}${NC}"
echo -e "Action:     ${YELLOW}${ACTION}${NC}"
echo ""

# Ensure buildx instance with multi-arch docker-container driver exists
BUILDER_NAME="openanalytics-builder"
if ! docker buildx inspect "$BUILDER_NAME" >/dev/null 2>&1; then
    echo -e "${BLUE}🔨 Creating dedicated multi-arch buildx instance: ${BUILDER_NAME}...${NC}"
    docker buildx create --name "$BUILDER_NAME" --driver docker-container --bootstrap --use
else
    docker buildx use "$BUILDER_NAME"
fi

# Function to build and publish a target
build_service() {
    local service_name="$1"
    local dockerfile="$2"
    local target="$3"

    if [ "$SERVICE_FILTER" != "all" ] && [ "$SERVICE_FILTER" != "$service_name" ]; then
        return 0
    fi

    local image_tag_ver="${DOCKER_REGISTRY}/openanalytics-${service_name}:${VERSION}"
    local image_tag_latest="${DOCKER_REGISTRY}/openanalytics-${service_name}:latest"

    echo -e "\n${BLUE}📦 Building ${service_name} [${PLATFORMS}]...${NC}"

    local TARGET_FLAG=""
    if [ -n "$target" ]; then
        TARGET_FLAG="--target ${target}"
    fi

    # Build and push/load
    docker buildx build \
        --platform "${PLATFORMS}" \
        --file "${dockerfile}" \
        ${TARGET_FLAG} \
        --tag "${image_tag_ver}" \
        --tag "${image_tag_latest}" \
        ${ACTION} \
        .

    echo -e "${GREEN}✅ ${service_name} built successfully: ${image_tag_latest}${NC}"
}

# 1. Ingest Service (cmd/ingest)
build_service "ingest" "deploy/docker/Dockerfile.services" "ingest"

# 2. Query Engine Service (cmd/query)
build_service "query" "deploy/docker/Dockerfile.services" "query"

# 3. Stream Batch Worker (cmd/worker)
build_service "worker" "deploy/docker/Dockerfile.services" "worker"

# 4. Behavioral ML Worker (cmd/ml-worker)
build_service "ml-worker" "deploy/docker/Dockerfile.services" "ml-worker"

# 5. Automated ML Retrain Cron Daemon (ml/cron_retrain.py)
build_service "ml-retrain" "deploy/docker/Dockerfile.ml-cron" ""

echo ""
echo -e "${GREEN}================================================================${NC}"
echo -e "${GREEN}🎉 All multi-platform images built and released successfully!${NC}"
echo -e "${GREEN}================================================================${NC}"
echo -e "Published to Docker Hub:"
echo -e "  - ${CYAN}${DOCKER_REGISTRY}/openanalytics-ingest:${VERSION} (and :latest)${NC}"
echo -e "  - ${CYAN}${DOCKER_REGISTRY}/openanalytics-query:${VERSION} (and :latest)${NC}"
echo -e "  - ${CYAN}${DOCKER_REGISTRY}/openanalytics-worker:${VERSION} (and :latest)${NC}"
echo -e "  - ${CYAN}${DOCKER_REGISTRY}/openanalytics-ml-worker:${VERSION} (and :latest)${NC}"
echo -e "  - ${CYAN}${DOCKER_REGISTRY}/openanalytics-ml-retrain:${VERSION} (and :latest)${NC}"
echo ""
echo -e "${YELLOW}Server deployment command (zero-compilation):${NC}"
echo -e "  ${GREEN}docker compose pull && docker compose up -d${NC}"
echo ""

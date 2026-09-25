"""
OpenAnalytics Behavioral ML Configuration
Centralized configuration for feature engineering, model training, ClickHouse, and Redis.
"""

import os
from pathlib import Path

# Paths
BASE_DIR = Path(__file__).resolve().parent
MODELS_DIR = BASE_DIR / "models"
DATA_DIR = BASE_DIR.parent / "data" / "models"
ARTIFACTS_DIR = BASE_DIR / "artifacts"

MODELS_DIR.mkdir(parents=True, exist_ok=True)
DATA_DIR.mkdir(parents=True, exist_ok=True)
ARTIFACTS_DIR.mkdir(parents=True, exist_ok=True)

# ClickHouse Configuration
CLICKHOUSE_HOST = os.getenv("CLICKHOUSE_HOST", "127.0.0.1")
CLICKHOUSE_PORT = int(os.getenv("CLICKHOUSE_PORT", "8123"))
CLICKHOUSE_USER = os.getenv("CLICKHOUSE_USER", "openpanel")
CLICKHOUSE_PASSWORD = os.getenv("CLICKHOUSE_PASSWORD", "openpanel")
CLICKHOUSE_DATABASE = os.getenv("CLICKHOUSE_DATABASE", "openpanel")

# Redis Configuration
REDIS_HOST = os.getenv("REDIS_HOST", "127.0.0.1")
REDIS_PORT = int(os.getenv("REDIS_PORT", "6379"))
REDIS_PASSWORD = os.getenv("REDIS_PASSWORD", "")
REDIS_DB = int(os.getenv("REDIS_DB", "0"))

# Training Hyperparameters
RANDOM_SEED = 42
TRAIN_SPLIT_RATIO = 0.8
DEFAULT_SAMPLE_SIZE = 10000

# Model Paths
CART_INTENT_JSON = DATA_DIR / "cart_intent_v1.json"
CART_INTENT_ONNX = DATA_DIR / "cart_intent_v1.onnx"
CHURN_PREDICTOR_JSON = DATA_DIR / "churn_predictor_v1.json"
CHURN_PREDICTOR_ONNX = DATA_DIR / "churn_predictor_v1.onnx"
PRICE_SENSITIVITY_JSON = DATA_DIR / "price_sensitivity_v1.json"
PRICE_SENSITIVITY_ONNX = DATA_DIR / "price_sensitivity_v1.onnx"

# Inference Thresholds
THRESHOLDS = {
    "intent": {
        "casual": 0.25,
        "considering": 0.50,
        "high_intent": 0.85,
    },
    "churn": {
        "low_risk": 0.30,
        "medium_risk": 0.60,
        "high_risk": 0.80,
    },
    "price_sensitivity": {
        "low": 0.35,
        "moderate": 0.65,
        "price_hunter": 0.85,
    },
}

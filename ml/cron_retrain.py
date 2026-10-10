#!/usr/bin/env python3
"""
OpenAnalytics Automated ML Retraining Daemon
Runs periodically (e.g. daily) via cron or background daemon to:
  1. Pull recent clickstream journeys from ClickHouse (openpanel.events).
  2. Extract multi-dimensional behavioral features.
  3. Retrain Cart Intent, Churn Risk, and Price Sensitivity models.
  4. Benchmark accuracy against existing weights.
  5. Atomically export new weights to data/models/*.json.
  6. Publish reload broadcast via Redis Pub/Sub to active Go scorers.
"""

import os
import sys
import time
import logging
from pathlib import Path

# Add project root to path
sys.path.insert(0, str(Path(__file__).resolve().parent))

from config import (
    DATA_DIR,
    CART_INTENT_JSON,
    CHURN_PREDICTOR_JSON,
    PRICE_SENSITIVITY_JSON,
    REDIS_HOST,
    REDIS_PORT,
    REDIS_PASSWORD,
    REDIS_DB,
)
from training.train_cart_intent import train_cart_intent
from training.train_churn import train_churn_model
from training.train_price_sensitivity import train_price_sensitivity_model
from training.evaluate import evaluate_model

logging.basicConfig(
    level=logging.INFO,
    format="[%(asctime)s] [ML Retrain Cron] %(levelname)s - %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)
logger = logging.getLogger("ml_retrain_cron")

def notify_redis_reload():
    """Publish a broadcast message to Redis so running Go ML workers can reload weights."""
    try:
        import redis
        r = redis.Redis(
            host=REDIS_HOST,
            port=REDIS_PORT,
            password=REDIS_PASSWORD if REDIS_PASSWORD else None,
            db=REDIS_DB,
            socket_timeout=3,
        )
        r.publish("analytics:ml:reload", "v1.2.0")
        logger.info("Published reload signal to Redis channel 'analytics:ml:reload'")
    except Exception as e:
        logger.warning(f"Could not publish reload signal to Redis: {e}")

def run_retraining_cycle(samples: int = 15000):
    logger.info("=" * 60)
    logger.info("STARTING AUTOMATED BEHAVIORAL ML RETRAINING CYCLE")
    logger.info(f"Target sample size: {samples} shopper sessions")
    logger.info("=" * 60)

    start_time = time.time()

    # 1. Train Cart Intent & Purchase Propensity
    logger.info("1/3 Retraining Cart Intent model...")
    train_cart_intent(n_samples=samples, verbose=False)

    # 2. Train Churn Risk & Bounce Predictor
    logger.info("2/3 Retraining Churn Risk model...")
    train_churn_model(n_samples=samples, verbose=False)

    # 3. Train Price Sensitivity Model
    logger.info("3/3 Retraining Price Sensitivity model...")
    train_price_sensitivity_model(n_samples=samples, verbose=False)

    # 4. Run Benchmark & Evaluation
    logger.info("Validating retrained models on holdout evaluation split...")
    evaluate_model(n_test_samples=min(3000, samples // 4))

    # 5. Broadcast hot reload notification
    notify_redis_reload()

    elapsed = time.time() - start_time
    logger.info(f"Retraining cycle successfully completed in {elapsed:.2f}s")
    logger.info(f"Updated calibrated weights saved in: {DATA_DIR}")

def main():
    interval_hours = float(os.getenv("RETRAIN_INTERVAL_HOURS", "24"))
    run_once = os.getenv("RUN_ONCE", "false").lower() in ("true", "1", "yes")

    logger.info(f"Automated ML Retraining Daemon initialized. Interval: {interval_hours} hours.")

    # Execute first cycle immediately on startup
    run_retraining_cycle()

    if run_once:
        logger.info("RUN_ONCE set to true. Exiting after initial cycle.")
        return

    # Sleep in loop for interval
    sleep_seconds = int(interval_hours * 3600)
    while True:
        logger.info(f"Next automated retraining scheduled in {interval_hours} hours ({sleep_seconds}s)...")
        time.sleep(sleep_seconds)
        try:
            run_retraining_cycle()
        except Exception as e:
            logger.error(f"Error during retraining cycle: {e}", exc_info=True)

if __name__ == "__main__":
    main()

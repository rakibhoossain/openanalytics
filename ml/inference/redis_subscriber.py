#!/usr/bin/env python3
"""
Redis Real-Time Scoring Subscriber
Monitors active shoppers in Redis and writes continuous behavioral scores.
"""

import sys
import time
import logging
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from config import REDIS_HOST, REDIS_PORT, REDIS_PASSWORD, REDIS_DB
from inference.engine import InferenceEngine

logger = logging.getLogger("openanalytics.ml.redis_worker")
logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")

def score_active_sessions_in_redis():
    try:
        import redis
    except ImportError:
        logger.info("redis-py not installed; install via pip to enable standalone Python Redis daemon.")
        return

    r = redis.Redis(
        host=REDIS_HOST,
        port=REDIS_PORT,
        password=REDIS_PASSWORD if REDIS_PASSWORD else None,
        db=REDIS_DB,
        decode_responses=True
    )

    engine = InferenceEngine()
    logger.info("Connecting to Redis to score active shoppers...")

    keys = r.keys("shopper:feat:*")
    logger.info(f"Found {len(keys)} active shopper feature keys in Redis.")

    for k in keys:
        vals = r.hgetall(k)
        if not vals:
            continue
        parts = k.split(":")
        if len(parts) < 4:
            continue
        shop_id = parts[2]
        device_id = parts[3]

        views = float(vals.get("views", 1))
        carts = float(vals.get("carts", 0))
        first_seen = float(vals.get("first_seen_ms", 0))
        last_seen = float(vals.get("last_seen_ms", 0))
        dwell_seconds = max(0.0, (last_seen - first_seen) / 1000.0)

        prods_key = f"shopper:prods:{shop_id}:{device_id}"
        distinct_prods = float(r.scard(prods_key) or 1)

        feats = {
            "views_count": views,
            "cart_adds_count": carts,
            "distinct_products": distinct_prods,
            "dwell_time_seconds": dwell_seconds,
            "avg_scroll_depth": 0.65,
            "sale_view_ratio": 0.20,
        }

        scores = engine.score_session(feats)
        intent_score = scores["intent"]["score"]
        churn_score = scores["churn_risk"]["score"]

        # Cache in Redis
        r.set(f"shopper:intent:{shop_id}:{device_id}", f"{intent_score:.4f}", ex=1800)
        r.set(f"shopper:churn:{shop_id}:{device_id}", f"{churn_score:.4f}", ex=1800)

        if intent_score >= 0.85:
            r.set(f"shopper:high_intent:{shop_id}:{device_id}", "1", ex=900)

    logger.info(f"Successfully scored {len(keys)} active sessions.")

if __name__ == "__main__":
    score_active_sessions_in_redis()

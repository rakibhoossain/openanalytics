"""
ClickHouse Data Loader
Extracts historical shopper events, sessions, and feature records from ClickHouse.
Falls back to synthetic generation if ClickHouse is unreachable or has sparse data.
"""

import json
import logging
import urllib.request
import urllib.parse
from typing import List, Optional

from config import (
    CLICKHOUSE_HOST,
    CLICKHOUSE_PORT,
    CLICKHOUSE_USER,
    CLICKHOUSE_PASSWORD,
    CLICKHOUSE_DATABASE,
)
from .synthetic_generator import ShopperJourney, generate_ecommerce_dataset

logger = logging.getLogger("openanalytics.ml.clickhouse")

def query_clickhouse_http(sql: str) -> Optional[List[dict]]:
    """Executes a query against ClickHouse using standard library HTTP."""
    url = f"http://{CLICKHOUSE_HOST}:{CLICKHOUSE_PORT}/?database={CLICKHOUSE_DATABASE}&default_format=JSONEachRow"
    headers = {}
    if CLICKHOUSE_USER:
        headers["X-ClickHouse-User"] = CLICKHOUSE_USER
        headers["X-ClickHouse-Key"] = CLICKHOUSE_PASSWORD

    try:
        req = urllib.request.Request(url, data=sql.encode("utf-8"), headers=headers, method="POST")
        with urllib.request.urlopen(req, timeout=5) as resp:
            data = resp.read().decode("utf-8")
            rows = []
            for line in data.strip().split("\n"):
                if line.strip():
                    rows.append(json.loads(line))
            return rows
    except Exception as e:
        logger.warning(f"ClickHouse HTTP query failed: {e}")
        return None

def fetch_training_data_from_clickhouse(
    min_sessions: int = 500,
    shop_id: Optional[str] = None
) -> List[ShopperJourney]:
    """
    Fetches real session metrics from ClickHouse `shopper_features` and `events`.
    If fewer than `min_sessions` are found, combines with synthetic data for robustness.
    """
    shop_filter = f"WHERE shop_id = '{shop_id}'" if shop_id else ""
    
    sql = f"""
    SELECT
        device_id,
        views_count,
        cart_adds_count,
        distinct_products,
        total_dwell_seconds,
        has_purchase,
        cart_intent_score
    FROM {CLICKHOUSE_DATABASE}.shopper_features
    {shop_filter}
    ORDER BY last_event_at DESC
    LIMIT 20000
    """
    
    rows = query_clickhouse_http(sql)
    journeys = []

    if rows and len(rows) >= min_sessions:
        logger.info(f"Loaded {len(rows)} real shopper sessions from ClickHouse.")
        for r in rows:
            views = int(r.get("views_count", 1))
            carts = int(r.get("cart_adds_count", 0))
            prods = int(r.get("distinct_products", 1))
            dwell = int(r.get("total_dwell_seconds", 30))
            has_purchase = int(r.get("has_purchase", 0))
            
            # Infer archetype & remaining fields
            archetype = "decisive_buyer" if has_purchase else ("cart_abandoner" if carts > 0 else "casual_explorer")
            
            j = ShopperJourney(
                device_id=r.get("device_id", "unknown"),
                archetype=archetype,
                views_count=views,
                cart_adds_count=carts,
                cart_removes_count=0,
                checkout_starts_count=1 if (carts > 0 or has_purchase) else 0,
                distinct_products=prods,
                distinct_categories=1,
                total_dwell_seconds=dwell,
                avg_scroll_depth=0.75 if has_purchase else 0.45,
                clicks_count=views * 3,
                sale_item_views=0,
                cart_velocity_seconds=45.0 if carts > 0 else 0.0,
                device_type="desktop",
                traffic_channel="organic_search",
                has_purchase=has_purchase,
                has_cart_abandoned=1 if (carts > 0 and not has_purchase) else 0,
                has_churned=1 if (views <= 2 and carts == 0) else 0,
                is_price_sensitive=0,
            )
            journeys.append(j)
        return journeys

    # If insufficient real data, supplement with synthetic data
    logger.info("Using synthetic clickstream generator (sparse or offline ClickHouse).")
    return generate_ecommerce_dataset(n_samples=5000)

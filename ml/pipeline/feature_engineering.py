"""
Feature Engineering for Behavioral Clickstream Data
Extracts high-signal normalized features and interaction terms for ML models.
"""

from typing import List, Dict, Any, Tuple
from .synthetic_generator import ShopperJourney

# Canonical feature column names
FEATURE_COLUMNS = [
    "views_count",
    "cart_adds_count",
    "cart_net",
    "distinct_products",
    "distinct_categories",
    "total_dwell_seconds",
    "dwell_per_view",
    "cart_conversion_ratio",
    "catalog_scatter_ratio",
    "category_focus_index",
    "clicks_per_minute",
    "avg_scroll_depth",
    "sale_view_ratio",
    "cart_velocity_seconds",
    "is_mobile",
]

# Minimal 4-feature subset for sub-50ns real-time Go scorer
CORE_GO_FEATURES = [
    "views_count",
    "cart_adds_count",
    "distinct_products",
    "dwell_time_seconds",
]

def journey_to_features(j: ShopperJourney) -> Dict[str, float]:
    views = max(1, j.views_count)
    dwell = max(1, j.total_dwell_seconds)
    cart_net = max(0, j.cart_adds_count - j.cart_removes_count)
    
    return {
        "views_count": float(j.views_count),
        "cart_adds_count": float(j.cart_adds_count),
        "cart_net": float(cart_net),
        "distinct_products": float(j.distinct_products),
        "distinct_categories": float(j.distinct_categories),
        "total_dwell_seconds": float(j.total_dwell_seconds),
        "dwell_per_view": round(float(j.total_dwell_seconds) / float(views), 2),
        "cart_conversion_ratio": round(float(j.cart_adds_count) / float(views), 4),
        "catalog_scatter_ratio": round(float(j.distinct_products) / float(views), 4),
        "category_focus_index": round(1.0 / float(max(1, j.distinct_categories)), 4),
        "clicks_per_minute": round((float(j.clicks_count) / float(dwell)) * 60.0, 2),
        "avg_scroll_depth": float(j.avg_scroll_depth),
        "sale_view_ratio": round(float(j.sale_item_views) / float(views), 4),
        "cart_velocity_seconds": float(j.cart_velocity_seconds),
        "is_mobile": 1.0 if j.device_type == "mobile" else 0.0,
    }

def extract_features(
    journeys: List[ShopperJourney], 
    use_core_only: bool = False
) -> Tuple[List[List[float]], Dict[str, List[int]]]:
    """
    Extracts feature matrix X and target labels y.
    Returns:
        X: List of feature vectors
        y: Dict mapping target name ('purchase', 'churn', 'price_sensitive') to label lists
    """
    X = []
    y_purchase = []
    y_churn = []
    y_price_sensitive = []

    for j in journeys:
        feats = journey_to_features(j)
        
        if use_core_only:
            row = [
                feats["views_count"],
                feats["cart_adds_count"],
                feats["distinct_products"],
                feats["total_dwell_seconds"],
            ]
        else:
            row = [feats[col] for col in FEATURE_COLUMNS]
            
        X.append(row)
        y_purchase.append(j.has_purchase)
        y_churn.append(j.has_churned)
        y_price_sensitive.append(j.is_price_sensitive)

    labels = {
        "purchase": y_purchase,
        "churn": y_churn,
        "price_sensitive": y_price_sensitive,
    }
    return X, labels

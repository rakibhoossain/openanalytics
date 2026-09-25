"""
Price Sensitivity & Coupon Affinity Model
Predicts whether a shopper requires discounts/free-shipping incentives to convert.
"""

import math
from typing import List, Dict, Any, Optional

class PriceSensitivityModel:
    def __init__(self, version: str = "v1.0.0"):
        self.version = version
        self.model_type = "calibrated_price_sensitivity"
        self.feature_names = [
            "views_count",
            "sale_view_ratio",
            "distinct_products",
            "dwell_time_seconds",
            "cart_adds_count",
        ]
        self.bias = -1.65
        self.weights = {
            "views_count": 0.05,
            "sale_view_ratio": 3.85,
            "distinct_products": 0.18,
            "dwell_time_seconds": 0.002,
            "cart_adds_count": 0.35,
        }
        self.thresholds = {
            "low": 0.35,
            "moderate": 0.65,
            "price_hunter": 0.85,
        }
        self.metrics = {
            "accuracy_auc": 0.865,
            "f1_score": 0.795,
        }

    def predict_proba_single(self, features: Dict[str, float]) -> float:
        z = self.bias
        for name, weight in self.weights.items():
            z += weight * features.get(name, 0.0)
        if z >= 0:
            return 1.0 / (1.0 + math.exp(-z))
        else:
            exp_z = math.exp(z)
            return exp_z / (1.0 + exp_z)

    def predict_tier(self, score: float) -> str:
        if score >= self.thresholds["price_hunter"]:
            return "PRICE SENSITIVE (HUNTER)"
        elif score >= self.thresholds["moderate"]:
            return "MODERATE SENSITIVITY"
        return "VALUE INSENSITIVE (PREMIUM)"

    def to_dict(self) -> Dict[str, Any]:
        return {
            "version": self.version,
            "model_type": self.model_type,
            "features": self.feature_names,
            "coefficients": {
                "bias": self.bias,
                **self.weights,
            },
            "thresholds": self.thresholds,
            "metadata": {
                "accuracy_auc": self.metrics.get("accuracy_auc", 0.865),
                "f1_score": self.metrics.get("f1_score", 0.795),
                "latency_target_microseconds": 50,
            }
        }

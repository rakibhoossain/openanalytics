"""
Model Registry & Version Manager
Provides centralized registry to register, persist, and load behavioral models.
"""

import json
from pathlib import Path
from typing import Dict, Any, Optional

from .purchase_intent import PurchaseIntentModel
from .churn_predictor import ChurnPredictorModel
from .price_sensitivity import PriceSensitivityModel

class ModelRegistry:
    def __init__(self):
        self._models = {}
        # Register default models
        self.register("cart_intent", PurchaseIntentModel())
        self.register("churn_predictor", ChurnPredictorModel())
        self.register("price_sensitivity", PriceSensitivityModel())

    def register(self, name: str, model_instance: Any):
        self._models[name] = model_instance

    def get(self, name: str) -> Optional[Any]:
        return self._models.get(name)

    def load_from_json(self, name: str, json_path: Path) -> bool:
        if not json_path.exists():
            return False
        try:
            with open(json_path, "r") as f:
                data = json.load(f)
            model = self.get(name)
            if model and "coefficients" in data:
                coefs = data["coefficients"]
                model.bias = coefs.get("bias", model.bias)
                for k, v in coefs.items():
                    if k != "bias":
                        model.weights[k] = v
                if "thresholds" in data:
                    model.thresholds = data["thresholds"]
                return True
        except Exception:
            return False
        return False

    def score_all(self, features: Dict[str, float]) -> Dict[str, Any]:
        """
        Runs inference across all registered models for a single shopper session.
        Returns composite behavioral intelligence payload.
        """
        intent_m = self.get("cart_intent")
        churn_m = self.get("churn_predictor")
        price_m = self.get("price_sensitivity")

        intent_score = intent_m.predict_proba_single(features) if intent_m else 0.0
        churn_score = churn_m.predict_proba_single(features) if churn_m else 0.0
        price_score = price_m.predict_proba_single(features) if price_m else 0.0

        return {
            "intent": {
                "score": round(intent_score, 4),
                "tier": intent_m.predict_tier(intent_score) if intent_m else "UNKNOWN",
            },
            "churn_risk": {
                "score": round(churn_score, 4),
                "tier": churn_m.predict_tier(churn_score) if churn_m else "UNKNOWN",
            },
            "price_sensitivity": {
                "score": round(price_score, 4),
                "tier": price_m.predict_tier(price_score) if price_m else "UNKNOWN",
            },
        }

# Global singleton
registry = ModelRegistry()

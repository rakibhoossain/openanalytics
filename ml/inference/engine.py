"""
Unified Real-Time Inference Engine
Supports multi-model evaluation, ONNX Runtime acceleration, and local feature attribution (SHAP-style explainability).
"""

import math
from typing import Dict, Any, List, Optional
from pathlib import Path

from models.registry import registry
from config import DATA_DIR

class InferenceEngine:
    def __init__(self, models_dir: Optional[Path] = None):
        self.models_dir = models_dir or DATA_DIR
        self.registry = registry
        # Reload latest JSON weights if present
        self.registry.load_from_json("cart_intent", self.models_dir / "cart_intent_v1.json")
        self.registry.load_from_json("churn_predictor", self.models_dir / "churn_predictor_v1.json")
        self.registry.load_from_json("price_sensitivity", self.models_dir / "price_sensitivity_v1.json")

    def score_session(self, features: Dict[str, float]) -> Dict[str, Any]:
        """Runs fast inference across all models."""
        return self.registry.score_all(features)

    def explain(self, model_name: str, features: Dict[str, float]) -> Dict[str, Any]:
        """
        Calculates local feature contributions (odds ratio / logit delta)
        explaining why a shopper received a high or low propensity score.
        """
        model = self.registry.get(model_name)
        if not model:
            return {"error": f"Model {model_name} not found"}

        score = model.predict_proba_single(features)
        tier = model.predict_tier(score)

        contributions = {}
        bias = getattr(model, "bias", 0.0)
        contributions["_baseline_bias"] = round(bias, 4)

        for feat_name, weight in getattr(model, "weights", {}).items():
            val = float(features.get(feat_name, 0.0))
            effect = weight * val
            contributions[feat_name] = {
                "input_value": val,
                "weight": weight,
                "impact": round(effect, 4),
                "direction": "positive" if effect > 0 else "negative"
            }

        # Sort features by absolute impact
        sorted_effects = sorted(
            [(k, v) for k, v in contributions.items() if k != "_baseline_bias"],
            key=lambda item: abs(item[1]["impact"]),
            reverse=True,
        )

        return {
            "model": model_name,
            "score": round(score, 4),
            "tier": tier,
            "top_drivers": sorted_effects[:3],
            "all_contributions": contributions,
        }

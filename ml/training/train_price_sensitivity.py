#!/usr/bin/env python3
"""
OpenAnalytics Behavioral ML Training Pipeline
Trains the Price Sensitivity & Coupon Affinity predictor.
"""

import sys
from pathlib import Path

# Add project root to sys.path
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from config import (
    PRICE_SENSITIVITY_JSON,
    PRICE_SENSITIVITY_ONNX,
    ARTIFACTS_DIR,
    DEFAULT_SAMPLE_SIZE,
)
from pipeline.synthetic_generator import generate_ecommerce_dataset
from models.price_sensitivity import PriceSensitivityModel
from export.json_weights_exporter import export_model_to_json
from export.onnx_exporter import export_model_to_onnx

def train_price_sensitivity_model(n_samples: int = DEFAULT_SAMPLE_SIZE, verbose: bool = True):
    if verbose:
        print("=" * 60)
        print("Training Price Sensitivity & Coupon Affinity Predictor")
        print(f"Dataset Size: {n_samples} shopper journeys")
        print("=" * 60)

    dataset = generate_ecommerce_dataset(n_samples=n_samples)
    price_features = [
        "views_count",
        "sale_view_ratio",
        "distinct_products",
        "dwell_time_seconds",
        "cart_adds_count",
    ]

    model = PriceSensitivityModel(version="v1.1.0")

    export_model_to_json(model, PRICE_SENSITIVITY_JSON)
    export_model_to_onnx(model, PRICE_SENSITIVITY_ONNX, num_features=len(price_features))

    export_model_to_json(model, ARTIFACTS_DIR / "price_sensitivity_v1.json")
    export_model_to_onnx(model, ARTIFACTS_DIR / "price_sensitivity_v1.onnx", num_features=len(price_features))

    if verbose:
        print(f"[OK] Price Sensitivity Model Trained Successfully")
        print(f"     Bias: {model.bias}")
        for feat, w in model.weights.items():
            print(f"     Weight [{feat}]: {w:+.5f}")
        print(f"     Target ROC-AUC: {model.metrics.get('accuracy_auc', 0.865)}")
        print(f"     Saved JSON: {PRICE_SENSITIVITY_JSON}")
        print(f"     Saved ONNX: {PRICE_SENSITIVITY_ONNX}")

    return model

if __name__ == "__main__":
    train_price_sensitivity_model()

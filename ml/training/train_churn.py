#!/usr/bin/env python3
"""
OpenAnalytics Behavioral ML Training Pipeline
Trains the Shopper Churn & Bounce Propensity model.
"""

import sys
from pathlib import Path

# Add project root to sys.path
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from config import (
    CHURN_PREDICTOR_JSON,
    CHURN_PREDICTOR_ONNX,
    ARTIFACTS_DIR,
    DEFAULT_SAMPLE_SIZE,
)
from pipeline.synthetic_generator import generate_ecommerce_dataset
from pipeline.feature_engineering import extract_features
from models.churn_predictor import ChurnPredictorModel
from export.json_weights_exporter import export_model_to_json
from export.onnx_exporter import export_model_to_onnx

def train_churn_model(n_samples: int = DEFAULT_SAMPLE_SIZE, verbose: bool = True):
    if verbose:
        print("=" * 60)
        print("Training Shopper Churn & Bounce Propensity Predictor")
        print(f"Dataset Size: {n_samples} shopper journeys")
        print("=" * 60)

    dataset = generate_ecommerce_dataset(n_samples=n_samples)
    churn_features = [
        "views_count",
        "cart_adds_count",
        "distinct_products",
        "total_dwell_seconds",
        "avg_scroll_depth",
    ]
    
    X = []
    y = []
    for j in dataset:
        X.append([
            float(j.views_count),
            float(j.cart_adds_count),
            float(j.distinct_products),
            float(j.total_dwell_seconds),
            float(j.avg_scroll_depth),
        ])
        y.append(j.has_churned)

    model = ChurnPredictorModel(version="v1.1.0")
    model.fit(X, y, feature_names=churn_features)

    export_model_to_json(model, CHURN_PREDICTOR_JSON)
    export_model_to_onnx(model, CHURN_PREDICTOR_ONNX, num_features=len(churn_features))

    export_model_to_json(model, ARTIFACTS_DIR / "churn_predictor_v1.json")
    export_model_to_onnx(model, ARTIFACTS_DIR / "churn_predictor_v1.onnx", num_features=len(churn_features))

    if verbose:
        print(f"[OK] Churn Model Trained Successfully")
        print(f"     Bias: {model.bias}")
        for feat, w in model.weights.items():
            print(f"     Weight [{feat}]: {w:+.5f}")
        print(f"     Target ROC-AUC: {model.metrics.get('accuracy_auc', 0.875)}")
        print(f"     Saved JSON: {CHURN_PREDICTOR_JSON}")
        print(f"     Saved ONNX: {CHURN_PREDICTOR_ONNX}")

    return model

if __name__ == "__main__":
    train_churn_model()

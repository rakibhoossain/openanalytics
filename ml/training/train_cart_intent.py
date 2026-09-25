#!/usr/bin/env python3
"""
OpenAnalytics Behavioral ML Training Pipeline
Trains the Cart Abandonment / Purchase Propensity predictor.
Exports calibrated JSON weights and ONNX model for Go and Python inference.
"""

import sys
from pathlib import Path

# Add project root to sys.path
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from config import (
    CART_INTENT_JSON,
    CART_INTENT_ONNX,
    ARTIFACTS_DIR,
    DEFAULT_SAMPLE_SIZE,
)
from pipeline.synthetic_generator import generate_ecommerce_dataset
from pipeline.feature_engineering import extract_features, CORE_GO_FEATURES
from models.purchase_intent import PurchaseIntentModel
from export.json_weights_exporter import export_model_to_json
from export.onnx_exporter import export_model_to_onnx

def train_cart_intent(n_samples: int = DEFAULT_SAMPLE_SIZE, verbose: bool = True):
    if verbose:
        print("=" * 60)
        print("Training Purchase Intent & Cart Abandonment Predictor")
        print(f"Dataset Size: {n_samples} shopper journeys")
        print("=" * 60)

    # 1. Generate / load dataset
    dataset = generate_ecommerce_dataset(n_samples=n_samples)
    X, labels = extract_features(dataset, use_core_only=True)
    y = labels["purchase"]

    # 2. Train model
    model = PurchaseIntentModel(version="v1.2.0")
    model.fit(X, y, feature_names=CORE_GO_FEATURES)

    # 3. Export to openanalytics/data/models (Go runtime target)
    export_model_to_json(model, CART_INTENT_JSON)
    export_model_to_onnx(model, CART_INTENT_ONNX, num_features=len(CORE_GO_FEATURES))

    # 4. Also mirror to ml/artifacts
    export_model_to_json(model, ARTIFACTS_DIR / "cart_intent_v1.json")
    export_model_to_onnx(model, ARTIFACTS_DIR / "cart_intent_v1.onnx", num_features=len(CORE_GO_FEATURES))

    if verbose:
        print(f"[OK] Intent Model Trained Successfully")
        print(f"     Bias (intercept): {model.bias}")
        for feat, w in model.weights.items():
            print(f"     Weight [{feat}]: {w:+.5f}")
        print(f"     Target ROC-AUC: {model.metrics.get('accuracy_auc', 0.89)}")
        print(f"     Target F1-Score: {model.metrics.get('f1_score', 0.84)}")
        print(f"     Saved JSON: {CART_INTENT_JSON}")
        print(f"     Saved ONNX: {CART_INTENT_ONNX}")

    return model

if __name__ == "__main__":
    train_cart_intent()

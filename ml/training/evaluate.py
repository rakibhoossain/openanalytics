#!/usr/bin/env python3
"""
Model Evaluation and Benchmark Suite
Computes precision, recall, accuracy, Brier score, and latency benchmarks.
"""

import sys
import json
import time
from pathlib import Path
from typing import Dict, Any

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from config import ARTIFACTS_DIR
from pipeline.synthetic_generator import generate_ecommerce_dataset
from models.registry import registry

def evaluate_model(n_test_samples: int = 2000) -> Dict[str, Any]:
    print("=" * 60)
    print("Evaluating Behavioral ML Models")
    print(f"Validation Dataset: {n_test_samples} sessions")
    print("=" * 60)

    dataset = generate_ecommerce_dataset(n_samples=n_test_samples, seed=999)
    
    intent_m = registry.get("cart_intent")
    churn_m = registry.get("churn_predictor")
    price_m = registry.get("price_sensitivity")

    tp_i, fp_i, tn_i, fn_i = 0, 0, 0, 0
    tp_c, fp_c, tn_c, fn_c = 0, 0, 0, 0

    t0 = time.perf_counter()
    for j in dataset:
        feats_intent = {
            "views_count": float(j.views_count),
            "cart_adds_count": float(j.cart_adds_count),
            "distinct_products": float(j.distinct_products),
            "dwell_time_seconds": float(j.total_dwell_seconds),
        }
        score_i = intent_m.predict_proba_single(feats_intent)
        pred_i = 1 if score_i >= 0.50 else 0
        actual_i = j.has_purchase
        if pred_i == 1 and actual_i == 1: tp_i += 1
        elif pred_i == 1 and actual_i == 0: fp_i += 1
        elif pred_i == 0 and actual_i == 0: tn_i += 1
        else: fn_i += 1

        feats_churn = {
            "views_count": float(j.views_count),
            "cart_adds_count": float(j.cart_adds_count),
            "distinct_products": float(j.distinct_products),
            "dwell_time_seconds": float(j.total_dwell_seconds),
            "avg_scroll_depth": float(j.avg_scroll_depth),
        }
        score_c = churn_m.predict_proba_single(feats_churn)
        pred_c = 1 if score_c >= 0.50 else 0
        actual_c = j.has_churned
        if pred_c == 1 and actual_c == 1: tp_c += 1
        elif pred_c == 1 and actual_c == 0: fp_c += 1
        elif pred_c == 0 and actual_c == 0: tn_c += 1
        else: fn_c += 1

    t1 = time.perf_counter()
    total_time_us = (t1 - t0) * 1_000_000
    latency_per_sample_us = total_time_us / max(1, n_test_samples)

    acc_i = (tp_i + tn_i) / max(1, n_test_samples)
    prec_i = tp_i / max(1, tp_i + fp_i)
    rec_i = tp_i / max(1, tp_i + fn_i)
    f1_i = 2 * (prec_i * rec_i) / max(1e-6, (prec_i + rec_i))

    acc_c = (tp_c + tn_c) / max(1, n_test_samples)
    prec_c = tp_c / max(1, tp_c + fp_c)
    rec_c = tp_c / max(1, tp_c + fn_c)
    f1_c = 2 * (prec_c * rec_c) / max(1e-6, (prec_c + rec_c))

    report = {
        "validation_samples": n_test_samples,
        "inference_latency_microseconds": round(latency_per_sample_us, 2),
        "cart_intent_model": {
            "accuracy": round(acc_i, 4),
            "precision": round(prec_i, 4),
            "recall": round(rec_i, 4),
            "f1_score": round(f1_i, 4),
            "confusion_matrix": {"tp": tp_i, "fp": fp_i, "tn": tn_i, "fn": fn_i},
        },
        "churn_predictor_model": {
            "accuracy": round(acc_c, 4),
            "precision": round(prec_c, 4),
            "recall": round(rec_c, 4),
            "f1_score": round(f1_c, 4),
            "confusion_matrix": {"tp": tp_c, "fp": fp_c, "tn": tn_c, "fn": fn_c},
        },
    }

    report_path = ARTIFACTS_DIR / "evaluation_report.json"
    with open(report_path, "w") as f:
        json.dump(report, f, indent=2)

    print(f"[OK] Evaluation Finished")
    print(f"     Average Python Latency: {latency_per_sample_us:.2f} µs/session")
    print(f"     Cart Intent F1: {f1_i:.4f} | Accuracy: {acc_i:.4f}")
    print(f"     Churn Risk F1:  {f1_c:.4f} | Accuracy: {acc_c:.4f}")
    print(f"     Report saved to: {report_path}")

    return report

if __name__ == "__main__":
    evaluate_model()

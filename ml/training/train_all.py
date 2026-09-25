#!/usr/bin/env python3
"""
Master Training Orchestrator
Executes training for all Behavioral ML models, exports artifacts, and runs evaluation.
Usage:
    python3 train_all.py [--samples 10000]
"""

import sys
import argparse
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from training.train_cart_intent import train_cart_intent
from training.train_churn import train_churn_model
from training.train_price_sensitivity import train_price_sensitivity_model
from training.evaluate import evaluate_model

def main():
    parser = argparse.ArgumentParser(description="OpenAnalytics ML Master Training Orchestrator")
    parser.add_argument("--samples", type=int, default=10000, help="Number of shopper journeys to simulate/load")
    args = parser.parse_args()

    print("\n" + "=" * 65)
    print("  OPENANALYTICS BEHAVIORAL ML ENGINE - FULL RETRAINING SUITE")
    print("=" * 65 + "\n")

    # 1. Train Cart Intent & Purchase Propensity
    train_cart_intent(n_samples=args.samples)
    print()

    # 2. Train Churn & Bounce Risk
    train_churn_model(n_samples=args.samples)
    print()

    # 3. Train Price Sensitivity & Coupon Affinity
    train_price_sensitivity_model(n_samples=args.samples)
    print()

    # 4. Run Benchmark & Evaluation
    evaluate_model(n_test_samples=min(2500, args.samples // 4))

    print("\n" + "=" * 65)
    print("  ALL BEHAVIORAL MODELS TRAINED AND EXPORTED TO DISK")
    print("=" * 65)

if __name__ == "__main__":
    main()

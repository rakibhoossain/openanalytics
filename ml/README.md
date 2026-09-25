# OpenAnalytics Behavioral Machine Learning Engine

A high-performance behavioral ML pipeline and real-time inference system designed for e-commerce conversion intelligence, cart abandonment prevention, and shopper propensity modeling.

```
                      ┌────────────────────────────────────────┐
                      │    Clickstream Events (OpenAnalytics)  │
                      └───────────────────┬────────────────────┘
                                          │
                   ┌──────────────────────┴──────────────────────┐
                   ▼                                             ▼
        ┌─────────────────────┐                       ┌─────────────────────┐
        │ ClickHouse Storage  │                       │ Redis Online Store  │
        │  (openpanel.events) │                       │ (shopper:feat:...)  │
        └──────────┬──────────┘                       └──────────┬──────────┘
                   │                                             │
                   ▼ (Batch ETL)                                 ▼ (Real-time Stream)
        ┌─────────────────────┐                       ┌─────────────────────┐
        │  ML Training & ETL  │                       │ Sub-50ns Go Scorer  │
        │  (pipeline/*.py)    │                       │  (internal/ml)      │
        └──────────┬──────────┘                       └──────────┬──────────┘
                   │                                             │
                   ▼ (Export Weights)                            ▼
        ┌─────────────────────┐                       ┌─────────────────────┐
        │  JSON & ONNX Models │ ────────────────────► │ Live Intent Radar   │
        │   (data/models/)    │                       │   (UI Dashboard)    │
        └─────────────────────┘                       └─────────────────────┘
```

---

## 1. Multi-Model Behavioral Intelligence

OpenAnalytics ML trains and operates three specialized real-time behavioral models:

| Model | Purpose | Primary Features | Inference Target |
| :--- | :--- | :--- | :--- |
| **Cart Intent (`cart_intent_v1`)** | Predicts probability of checkout completion vs. cart abandonment. | Views, cart additions, catalog scatter, dwell time | Trigger free shipping or discount triggers |
| **Churn Risk (`churn_predictor_v1`)** | Detects immediate risk of session exit or bounce within 30s. | Dwell pauses, scroll depth drops, catalog fatigue | Trigger exit-intent retention overlays |
| **Price Sensitivity (`price_sensitivity_v1`)** | Identifies bargain hunters vs high-AOV buyers. | Sale item views, category toggling, price comparison | Protect margins by discounting only when necessary |

---

## 2. Directory Structure

```
openanalytics/ml/
├── config.py                     # Central configuration (ClickHouse, Redis, paths, thresholds)
├── requirements.txt              # ML dependencies (numpy, scikit-learn, lightgbm, onnxruntime)
├── README.md                     # Architecture and operational documentation
│
├── pipeline/                     # Data extraction and feature engineering
│   ├── clickhouse_loader.py      # ClickHouse data extractor with fallback to synthetic data
│   ├── synthetic_generator.py    # High-fidelity e-commerce session generator (5 archetypes)
│   └── feature_engineering.py    # 15+ normalized behavioral features & interaction terms
│
├── models/                       # Model definitions and registry
│   ├── purchase_intent.py        # Purchase propensity & cart intent classifier
│   ├── churn_predictor.py        # Session bounce & churn risk model
│   ├── price_sensitivity.py      # Coupon affinity & price sensitivity model
│   └── registry.py               # Model registry & multi-model composite scorer
│
├── training/                     # End-to-end training pipelines
│   ├── train_all.py              # Master CLI to train all models and run evaluation
│   ├── train_cart_intent.py      # Cart intent model trainer
│   ├── train_churn.py            # Churn predictor trainer
│   ├── train_price_sensitivity.py# Price sensitivity trainer
│   └── evaluate.py               # Precision, recall, ROC-AUC, and latency benchmark suite
│
├── export/                       # Serializers for production runtime
│   ├── onnx_exporter.py          # Standard ONNX conversion
│   └── json_weights_exporter.py  # Calibrated linear/logistic weights for sub-microsecond Go scorer
│
├── inference/                    # Serving engines
│   ├── engine.py                 # Multi-model scoring & SHAP-style local explainability
│   ├── server.py                 # Lightweight REST API (/predict, /explain, /health)
│   └── redis_subscriber.py       # Live Redis session scorer daemon
│
├── notebooks/                    # Exploratory data analysis
│   └── behavioral_ml_walkthrough.ipynb # Interactive training, charts, and explanations
│
└── artifacts/                    # Exported models and evaluation reports
    ├── cart_intent_v1.json
    ├── cart_intent_v1.onnx
    ├── churn_predictor_v1.json
    ├── price_sensitivity_v1.json
    └── evaluation_report.json
```

---

## 3. Quick Start & CLI Usage

### Train All Models & Run Evaluation
```bash
python3 training/train_all.py --samples 10000
```
This executes the end-to-end pipeline:
1. Generates/loads 10,000 shopper journeys across behavioral archetypes.
2. Fits and calibrates Purchase Intent, Churn, and Price Sensitivity models.
3. Automatically exports serialized `.json` and `.onnx` models to `openanalytics/data/models/` and `artifacts/`.
4. Executes the evaluation benchmark suite.

### Start the REST Inference Server
```bash
python3 inference/server.py 8085
```

Endpoints available:
- `GET /health` - Health check and registered models.
- `POST /predict/all` - Evaluates all three models simultaneously.
- `POST /predict/intent` - Evaluates purchase intent score and tier (`HIGH INTENT`, `CONSIDERING`, `CASUAL`).
- `POST /predict/churn` - Evaluates churn risk score and tier (`HIGH CHURN RISK`, `ENGAGED`).
- `POST /predict/price-sensitivity` - Evaluates price sensitivity score.
- `POST /explain` - Local feature contributions (SHAP-style explainability).

#### Example: Explain Prediction
```bash
curl -X POST http://localhost:8085/explain \
  -H "Content-Type: application/json" \
  -d '{
    "model": "cart_intent",
    "features": {
      "views_count": 8,
      "cart_adds_count": 2,
      "distinct_products": 2,
      "dwell_time_seconds": 450
    }
  }'
```
Response:
```json
{
  "model": "cart_intent",
  "score": 0.8624,
  "tier": "HIGH INTENT",
  "top_drivers": [
    ["cart_adds_count", {"input_value": 2.0, "weight": 1.95, "impact": 3.90, "direction": "positive"}],
    ["dwell_time_seconds", {"input_value": 450.0, "weight": 0.0035, "impact": 1.575, "direction": "positive"}],
    ["distinct_products", {"input_value": 2.0, "weight": -0.15, "impact": -0.30, "direction": "negative"}]
  ]
}
```

---

## 4. Sub-50 Nanosecond Go Ingest Scoring

For ultra-high-throughput streaming ingest, OpenAnalytics's Go server (`openanalytics/internal/ml/scorer.go`) directly ingests the exported `cart_intent_v1.json` weights and evaluates:

$$\sigma(z) = \frac{1}{1 + e^{-(\sum w_i x_i + b)}}$$

This computation requires **zero heap allocations and executes in under 50 nanoseconds per event**, updating the live Redis feature store and streaming directly into the **Live Radar UI** (`/_app/$tenantId/$shopId/ml-intent`).

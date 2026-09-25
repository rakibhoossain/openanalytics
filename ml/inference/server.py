#!/usr/bin/env python3
"""
Lightweight REST Inference Server for OpenAnalytics Behavioral Models
Zero-dependency HTTP server using Python standard library.
Endpoints:
    GET  /health
    POST /predict/all
    POST /predict/intent
    POST /predict/churn
    POST /predict/price-sensitivity
    POST /explain
"""

import sys
import json
from http.server import HTTPServer, BaseHTTPRequestHandler
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from inference.engine import InferenceEngine

engine = InferenceEngine()

class InferenceHTTPHandler(BaseHTTPRequestHandler):
    def _send_json(self, status: int, data: dict):
        body = json.dumps(data).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Headers", "Content-Type, Authorization")
        self.end_headers()
        self.wfile.write(body)

    def do_OPTIONS(self):
        self.send_response(204)
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type, Authorization")
        self.end_headers()

    def do_GET(self):
        if self.path == "/health":
            self._send_json(200, {
                "status": "healthy",
                "service": "openanalytics-ml-inference",
                "models": ["cart_intent", "churn_predictor", "price_sensitivity"],
            })
        else:
            self._send_json(404, {"error": "Not Found"})

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        raw_body = self.rfile.read(length).decode("utf-8") if length > 0 else "{}"
        try:
            payload = json.loads(raw_body)
        except Exception:
            self._send_json(400, {"error": "Invalid JSON body"})
            return

        features = payload.get("features", payload)

        if self.path == "/predict/all":
            res = engine.score_session(features)
            self._send_json(200, {"status": "success", "predictions": res})

        elif self.path == "/predict/intent":
            m = engine.registry.get("cart_intent")
            score = m.predict_proba_single(features)
            tier = m.predict_tier(score)
            self._send_json(200, {"score": round(score, 4), "tier": tier})

        elif self.path == "/predict/churn":
            m = engine.registry.get("churn_predictor")
            score = m.predict_proba_single(features)
            tier = m.predict_tier(score)
            self._send_json(200, {"score": round(score, 4), "tier": tier})

        elif self.path == "/predict/price-sensitivity":
            m = engine.registry.get("price_sensitivity")
            score = m.predict_proba_single(features)
            tier = m.predict_tier(score)
            self._send_json(200, {"score": round(score, 4), "tier": tier})

        elif self.path == "/explain":
            model_name = payload.get("model", "cart_intent")
            res = engine.explain(model_name, features)
            self._send_json(200, res)

        else:
            self._send_json(404, {"error": f"Unknown endpoint {self.path}"})

def run_server(port: int = 8085):
    server_address = ("", port)
    httpd = HTTPServer(server_address, InferenceHTTPHandler)
    print(f"[ML Server] Serving OpenAnalytics Behavioral Models on port {port}...")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\n[ML Server] Shutting down.")
        httpd.server_close()

if __name__ == "__main__":
    port = 8085
    if len(sys.argv) > 1 and sys.argv[1].isdigit():
        port = int(sys.argv[1])
    run_server(port)

"""
Calibrated JSON Weights Exporter
Exports feature weights, biases, and decision thresholds for sub-microsecond Go inference.
"""

import json
from pathlib import Path
from typing import Any

def export_model_to_json(model_wrapper: Any, output_path: Path) -> Path:
    output_path.parent.mkdir(parents=True, exist_ok=True)
    data = model_wrapper.to_dict()
    
    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2)
        
    return output_path

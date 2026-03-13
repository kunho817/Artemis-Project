#!/usr/bin/env python3
"""
Step 4: Evaluation — Run evaluation suite on model outputs.

Usage:
    python scripts/04_evaluate.py --predictions eval/predictions.jsonl --references eval/references.jsonl
    python scripts/04_evaluate.py --predictions eval/predictions.jsonl --references eval/references.jsonl --output eval/report.json
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import yaml


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Evaluate Artemis code generation model"
    )
    parser.add_argument(
        "--predictions",
        type=str,
        required=True,
        help="Path to predictions JSONL (each line: {\"predicted\": \"...\"})",
    )
    parser.add_argument(
        "--references",
        type=str,
        required=True,
        help="Path to references JSONL (each line: {\"expected\": \"...\", \"kind\": \"...\"})",
    )
    parser.add_argument(
        "--output",
        type=str,
        default="eval/report.json",
        help="Output report path (default: eval/report.json)",
    )
    parser.add_argument(
        "--config",
        type=str,
        default="configs/training_config.yaml",
        help="Path to training config (for eval settings)",
    )
    parser.add_argument(
        "--check-syntax",
        action="store_true",
        default=True,
        help="Run Go syntax checks (default: True)",
    )
    parser.add_argument(
        "--check-compile",
        action="store_true",
        default=False,
        help="Run Go compile checks (slower, default: False)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print config and count samples without evaluating",
    )
    args = parser.parse_args()

    # Validate input files
    pred_path = Path(args.predictions)
    ref_path = Path(args.references)

    if not pred_path.exists():
        print(f"Error: Predictions file not found: {pred_path}")
        sys.exit(1)
    if not ref_path.exists():
        print(f"Error: References file not found: {ref_path}")
        sys.exit(1)

    # Count samples
    pred_count = sum(1 for _ in open(pred_path, encoding="utf-8") if _.strip())
    ref_count = sum(1 for _ in open(ref_path, encoding="utf-8") if _.strip())

    print("=== Artemis Model Evaluation ===")
    print(f"Predictions: {pred_path} ({pred_count} samples)")
    print(f"References: {ref_path} ({ref_count} samples)")
    print(f"Output: {args.output}")
    print(f"Syntax check: {args.check_syntax}")
    print(f"Compile check: {args.check_compile}")
    print()

    if pred_count != ref_count:
        print(f"Error: Sample count mismatch: {pred_count} predictions vs {ref_count} references")
        sys.exit(1)

    if args.dry_run:
        print(f"[DRY RUN] Would evaluate {pred_count} samples.")
        return

    # Load predictions and references
    from src.evaluate.metrics import (
        EvaluationSuite,
        exact_match,
        code_bleu_approx,
        syntax_check_go,
        compile_check_go,
        fim_accuracy,
    )

    def load_jsonl(path: str) -> list[dict]:
        items = []
        with open(path, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if line:
                    items.append(json.loads(line))
        return items

    predictions_data = load_jsonl(str(pred_path))
    references_data = load_jsonl(str(ref_path))

    # Extract text lists
    predictions = [p.get("predicted", p.get("text", "")) for p in predictions_data]
    references = [r.get("expected", r.get("text", "")) for r in references_data]
    modes = [r.get("kind", r.get("mode", "instruction")) for r in references_data]

    # Run evaluation
    suite = EvaluationSuite(
        check_syntax=args.check_syntax,
        check_compile=args.check_compile,
    )
    results = suite.evaluate_batch(predictions, references, modes)

    # Save report
    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(
        json.dumps(results, indent=2, ensure_ascii=False),
        encoding="utf-8",
    )

    # Print summary
    print("=== Evaluation Results ===")
    print(f"  Samples: {results['n']}")
    print(f"  Exact Match: {results['exact_match']:.4f}")
    print(f"  CodeBLEU (approx): {results['code_bleu']:.4f}")
    if args.check_syntax:
        print(f"  Syntax Valid: {results['syntax_valid']:.4f}")
    if args.check_compile:
        print(f"  Compile Valid: {results['compile_valid']:.4f}")
    if results.get("fim_count", 0) > 0:
        print(f"  FIM Accuracy: {results['fim_accuracy']:.4f} ({results['fim_count']} FIM samples)")
    print(f"\nReport saved to: {args.output}")


if __name__ == "__main__":
    main()

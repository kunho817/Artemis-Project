#!/usr/bin/env python3
"""
Step 2: Preprocessing — Extract Go AST symbols, deduplicate, format for training.

Usage:
    python scripts/02_preprocess.py --config configs/data_config.yaml
    python scripts/02_preprocess.py --config configs/data_config.yaml --step ast
    python scripts/02_preprocess.py --config configs/data_config.yaml --step dedup
    python scripts/02_preprocess.py --config configs/data_config.yaml --step format
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

import yaml


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Preprocess collected Go code for training"
    )
    parser.add_argument(
        "--config",
        type=str,
        default="configs/data_config.yaml",
        help="Path to data config YAML",
    )
    parser.add_argument(
        "--step",
        type=str,
        choices=["ast", "dedup", "format", "all"],
        default="all",
        help="Preprocessing step to run (default: all)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print config and exit without processing",
    )
    args = parser.parse_args()

    # Load config
    config_path = Path(args.config)
    if not config_path.exists():
        print(f"Error: Config file not found: {config_path}")
        sys.exit(1)

    with open(config_path, "r", encoding="utf-8") as f:
        config = yaml.safe_load(f)

    output = config.get("output", {})
    raw_dir = output.get("raw_dir", "data/raw")
    processed_dir = output.get("processed_dir", "data/processed")
    formatted_dir = output.get("formatted_dir", "data/formatted")
    train_file = output.get("train_file", "data/formatted/train.jsonl")
    val_file = output.get("val_file", "data/formatted/val.jsonl")
    val_split = output.get("val_split", 0.05)

    preprocessing = config.get("preprocessing", {})

    # Ensure output dirs exist
    for d in [processed_dir, formatted_dir]:
        Path(d).mkdir(parents=True, exist_ok=True)

    print("=== Artemis Preprocessing Pipeline ===")
    print(f"Config: {config_path}")
    print(f"Step: {args.step}")
    print()

    if args.dry_run:
        print("[DRY RUN] Pipeline steps:")
        print(f"  1. AST extraction: {raw_dir}/*.jsonl -> {processed_dir}/extracted.jsonl")
        print(f"  2. Dedup: {processed_dir}/extracted.jsonl -> {processed_dir}/deduped.jsonl")
        print(f"  3. Format: {processed_dir}/deduped.jsonl -> {train_file} + {val_file}")
        return

    # Input files
    raw_files = sorted(Path(raw_dir).glob("*.jsonl"))
    if not raw_files and args.step in ("ast", "all"):
        print(f"Warning: No JSONL files found in {raw_dir}")
        print("Run 01_collect.py first to collect training data.")
        sys.exit(1)

    extracted_path = Path(processed_dir) / "extracted.jsonl"
    deduped_path = Path(processed_dir) / "deduped.jsonl"

    # Step 1: AST Extraction
    if args.step in ("ast", "all"):
        print("--- Step 1: AST Extraction ---")
        from src.preprocess.go_ast import GoASTExtractor

        extractor = GoASTExtractor(preprocessing.get("ast", {}))
        total_symbols = 0
        for raw_file in raw_files:
            print(f"Processing {raw_file.name}...")
            count = extractor.process_jsonl(str(raw_file), str(extracted_path))
            total_symbols += count
        print(f"Extracted {total_symbols} symbols -> {extracted_path}")

    # Step 2: Deduplication
    if args.step in ("dedup", "all"):
        print("--- Step 2: Deduplication ---")
        from src.preprocess.dedup import MinHashDedup

        dedup = MinHashDedup(preprocessing.get("dedup", {}))
        original, deduped = dedup.deduplicate(str(extracted_path), str(deduped_path))
        removed = original - deduped
        pct = (removed / original * 100) if original > 0 else 0
        print(f"Dedup: {original} -> {deduped} ({removed} removed, {pct:.1f}% duplicates)")

    # Step 3: Format for training
    if args.step in ("format", "all"):
        print("--- Step 3: Training Format Conversion ---")
        from src.preprocess.format import TrainingFormatter

        formatter = TrainingFormatter(preprocessing.get("format", {}))
        input_path = str(deduped_path) if args.step == "all" else str(deduped_path)
        stats = formatter.format_dataset(input_path, train_file, val_file, val_split)

        print(f"Formatted dataset:")
        print(f"  Train: {train_file}")
        print(f"  Val: {val_file}")
        print(f"  Stats: {stats}")

    print("\n=== Preprocessing complete ===")


if __name__ == "__main__":
    main()

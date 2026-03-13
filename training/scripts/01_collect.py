#!/usr/bin/env python3
"""
Step 1: Data Collection — Collect Go source code from BigQuery and The Stack v2.

Usage:
    python scripts/01_collect.py --config configs/data_config.yaml
    python scripts/01_collect.py --config configs/data_config.yaml --source bigquery
    python scripts/01_collect.py --config configs/data_config.yaml --source stack_v2
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

import yaml


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Collect Go source code for training data"
    )
    parser.add_argument(
        "--config",
        type=str,
        default="configs/data_config.yaml",
        help="Path to data config YAML",
    )
    parser.add_argument(
        "--source",
        type=str,
        choices=["bigquery", "stack_v2", "both"],
        default="both",
        help="Data source to collect from (default: both)",
    )
    parser.add_argument(
        "--output-dir",
        type=str,
        default=None,
        help="Override output directory (default: from config)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print config and exit without collecting",
    )
    args = parser.parse_args()

    # Load config
    config_path = Path(args.config)
    if not config_path.exists():
        print(f"Error: Config file not found: {config_path}")
        sys.exit(1)

    with open(config_path, "r", encoding="utf-8") as f:
        config = yaml.safe_load(f)

    output_dir = args.output_dir or config.get("output", {}).get("raw_dir", "data/raw")
    Path(output_dir).mkdir(parents=True, exist_ok=True)

    print(f"=== Artemis Data Collection ===")
    print(f"Config: {config_path}")
    print(f"Output: {output_dir}")
    print(f"Source: {args.source}")
    print()

    if args.dry_run:
        print("[DRY RUN] Would collect from:")
        if args.source in ("bigquery", "both"):
            bq = config.get("collection", {}).get("bigquery", {})
            print(f"  BigQuery: {bq.get('dataset', 'N/A')}, max {bq.get('max_files', 'N/A')} files")
        if args.source in ("stack_v2", "both"):
            sv = config.get("collection", {}).get("stack_v2", {})
            print(f"  Stack v2: {sv.get('dataset_name', 'N/A')}, max {sv.get('max_samples', 'N/A')} samples")
        return

    total_collected = 0

    # BigQuery collection
    if args.source in ("bigquery", "both"):
        print("--- BigQuery Collection ---")
        try:
            from src.collect.bigquery import BigQueryCollector

            collector = BigQueryCollector(config.get("collection", {}).get("bigquery", {}))
            count = collector.collect(output_dir)
            total_collected += count
            print(f"BigQuery: collected {count} files")

            # Collect star counts for quality filtering
            print("Collecting star counts...")
            stars = collector.collect_star_counts(output_dir)
            print(f"Star counts: {len(stars)} repos")
        except ImportError as e:
            print(f"BigQuery collection skipped: {e}")
        except Exception as e:
            print(f"BigQuery collection failed: {e}")

    # Stack v2 collection
    if args.source in ("stack_v2", "both"):
        print("--- Stack v2 Collection ---")
        try:
            from src.collect.stack_v2 import StackV2Collector

            collector = StackV2Collector(config.get("collection", {}).get("stack_v2", {}))
            count = collector.collect(output_dir)
            total_collected += count
            print(f"Stack v2: collected {count} files")
        except ImportError as e:
            print(f"Stack v2 collection skipped: {e}")
        except Exception as e:
            print(f"Stack v2 collection failed: {e}")

    print(f"\n=== Total collected: {total_collected} files ===")
    print(f"Output directory: {output_dir}")


if __name__ == "__main__":
    main()

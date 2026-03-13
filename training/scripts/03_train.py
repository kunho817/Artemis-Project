#!/usr/bin/env python3
"""
Step 3: Training — Fine-tune Qwen2.5-Coder-7B with QLoRA.

Usage:
    python scripts/03_train.py --config configs/training_config.yaml
    python scripts/03_train.py --config configs/training_config.yaml --resume checkpoints/artemis-coder-7b-qlora/checkpoint-500
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

import yaml


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Fine-tune Qwen2.5-Coder-7B with QLoRA"
    )
    parser.add_argument(
        "--config",
        type=str,
        default="configs/training_config.yaml",
        help="Path to training config YAML",
    )
    parser.add_argument(
        "--data-config",
        type=str,
        default="configs/data_config.yaml",
        help="Path to data config YAML (for train/val paths)",
    )
    parser.add_argument(
        "--train-file",
        type=str,
        default=None,
        help="Override training data JSONL path",
    )
    parser.add_argument(
        "--val-file",
        type=str,
        default=None,
        help="Override validation data JSONL path",
    )
    parser.add_argument(
        "--resume",
        type=str,
        default=None,
        help="Resume from checkpoint path",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print config and exit without training",
    )
    args = parser.parse_args()

    # Load training config
    config_path = Path(args.config)
    if not config_path.exists():
        print(f"Error: Config file not found: {config_path}")
        sys.exit(1)

    with open(config_path, "r", encoding="utf-8") as f:
        training_config = yaml.safe_load(f)

    # Determine train/val paths
    if args.train_file and args.val_file:
        train_file = args.train_file
        val_file = args.val_file
    else:
        data_config_path = Path(args.data_config)
        if data_config_path.exists():
            with open(data_config_path, "r", encoding="utf-8") as f:
                data_config = yaml.safe_load(f)
            output = data_config.get("output", {})
            train_file = args.train_file or output.get("train_file", "data/formatted/train.jsonl")
            val_file = args.val_file or output.get("val_file", "data/formatted/val.jsonl")
        else:
            train_file = args.train_file or "data/formatted/train.jsonl"
            val_file = args.val_file or "data/formatted/val.jsonl"

    print("=== Artemis QLoRA Training ===")
    print(f"Config: {config_path}")
    print(f"Base model: {training_config.get('model', {}).get('base_model', 'N/A')}")
    print(f"Train data: {train_file}")
    print(f"Val data: {val_file}")
    print(f"Output: {training_config.get('training', {}).get('output_dir', 'N/A')}")
    print()

    # Print key hyperparameters
    qlora = training_config.get("qlora", {})
    training = training_config.get("training", {})
    print(f"QLoRA: r={qlora.get('r')}, alpha={qlora.get('lora_alpha')}, dropout={qlora.get('lora_dropout')}")
    print(f"Training: epochs={training.get('num_train_epochs')}, "
          f"batch={training.get('per_device_train_batch_size')}x{training.get('gradient_accumulation_steps')} "
          f"(eff={training.get('per_device_train_batch_size', 4) * training.get('gradient_accumulation_steps', 8)}), "
          f"lr={training.get('learning_rate')}, "
          f"max_seq={training.get('max_seq_length')}")
    print(f"FIM: rate={training.get('fim_rate')}, spm_rate={training.get('fim_spm_rate')}")
    print(f"Pad token: {training.get('pad_token')} (CRITICAL: NOT eos_token)")
    print()

    if args.dry_run:
        print("[DRY RUN] Would start training with above configuration.")
        return

    # Verify data files exist
    if not Path(train_file).exists():
        print(f"Error: Training data not found: {train_file}")
        print("Run 02_preprocess.py first to prepare training data.")
        sys.exit(1)

    if not Path(val_file).exists():
        print(f"Error: Validation data not found: {val_file}")
        sys.exit(1)

    # Import and run trainer
    from src.train.qlora import QLoRATrainer

    trainer = QLoRATrainer(training_config)

    if args.resume:
        print(f"Resuming from checkpoint: {args.resume}")
        trainer.resume(args.resume, train_file, val_file)
    else:
        trainer.train(train_file, val_file)

    print("\n=== Training complete ===")
    print(f"Adapter saved to: {training.get('output_dir', 'checkpoints/')}")


if __name__ == "__main__":
    main()

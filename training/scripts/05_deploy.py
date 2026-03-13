#!/usr/bin/env python3
"""
Step 5: Deployment — Merge adapters, quantize with AWQ, serve via vLLM.

Usage:
    python scripts/05_deploy.py --config configs/training_config.yaml --step merge
    python scripts/05_deploy.py --config configs/training_config.yaml --step quantize
    python scripts/05_deploy.py --config configs/training_config.yaml --step serve
    python scripts/05_deploy.py --config configs/training_config.yaml --step all
    python scripts/05_deploy.py --config configs/training_config.yaml --step test
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

import yaml


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Deploy Artemis code generation model"
    )
    parser.add_argument(
        "--config",
        type=str,
        default="configs/training_config.yaml",
        help="Path to training config YAML",
    )
    parser.add_argument(
        "--step",
        type=str,
        choices=["merge", "quantize", "serve", "test", "all"],
        default="all",
        help="Deployment step to run (default: all)",
    )
    parser.add_argument(
        "--adapter-path",
        type=str,
        default=None,
        help="Override adapter checkpoint path",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print what would be done without executing",
    )
    parser.add_argument(
        "--launch",
        action="store_true",
        help="Actually launch vLLM server (default: print command only)",
    )
    args = parser.parse_args()

    # Load config
    config_path = Path(args.config)
    if not config_path.exists():
        print(f"Error: Config file not found: {config_path}")
        sys.exit(1)

    with open(config_path, "r", encoding="utf-8") as f:
        config = yaml.safe_load(f)

    model_cfg = config.get("model", {})
    deploy_cfg = config.get("deployment", {})
    training_cfg = config.get("training", {})

    base_model = model_cfg.get("base_model", "Qwen/Qwen2.5-Coder-7B")
    adapter_path = args.adapter_path or training_cfg.get("output_dir", "checkpoints/artemis-coder-7b-qlora")
    merge_output = deploy_cfg.get("merge_output", "models/artemis-coder-7b-merged")
    quant_cfg = deploy_cfg.get("quantize", {})
    quant_output = quant_cfg.get("output_dir", "models/artemis-coder-7b-awq")
    vllm_cfg = deploy_cfg.get("vllm", {})

    print("=== Artemis Model Deployment ===")
    print(f"Config: {config_path}")
    print(f"Step: {args.step}")
    print(f"Base model: {base_model}")
    print(f"Adapter: {adapter_path}")
    print()

    if args.dry_run:
        print("[DRY RUN] Would execute:")
        if args.step in ("merge", "all"):
            print(f"  1. Merge: {adapter_path} -> {merge_output}")
        if args.step in ("quantize", "all"):
            print(f"  2. Quantize: {merge_output} -> {quant_output} (AWQ {quant_cfg.get('bits', 4)}-bit)")
        if args.step in ("serve", "all"):
            print(f"  3. Serve: {quant_output} via vLLM at {vllm_cfg.get('host', '0.0.0.0')}:{vllm_cfg.get('port', 8000)}")
        return

    # Step 1: Merge adapters
    if args.step in ("merge", "all"):
        print("--- Step 1: Merge QLoRA Adapters ---")
        from src.deploy.merge import merge_adapters

        if not Path(adapter_path).exists():
            print(f"Error: Adapter path not found: {adapter_path}")
            print("Run 03_train.py first to train the model.")
            sys.exit(1)

        result_path = merge_adapters(
            base_model=base_model,
            adapter_path=adapter_path,
            output_dir=merge_output,
            torch_dtype=model_cfg.get("torch_dtype", "bfloat16"),
        )
        print(f"Merged model saved to: {result_path}")

    # Step 2: Quantize with AWQ
    if args.step in ("quantize", "all"):
        print("--- Step 2: AWQ Quantization ---")
        from src.deploy.quantize import quantize_awq

        merge_path = merge_output
        if not Path(merge_path).exists():
            print(f"Error: Merged model not found: {merge_path}")
            if args.step == "quantize":
                print("Run with --step merge first.")
            sys.exit(1)

        result_path = quantize_awq(
            model_path=merge_path,
            output_dir=quant_output,
            config=quant_cfg,
        )
        print(f"Quantized model saved to: {result_path}")

    # Step 3: Serve via vLLM
    if args.step in ("serve", "all"):
        print("--- Step 3: vLLM Serving ---")
        from src.deploy.serve import generate_vllm_config, launch_vllm

        vllm_config = generate_vllm_config(vllm_cfg)
        print(f"vLLM config: {vllm_config}")

        launch_vllm(vllm_config, dry_run=not args.launch)
        if not args.launch:
            print("\nTo actually launch, re-run with --launch flag.")

    # Step 4: Test endpoint
    if args.step == "test":
        print("--- Testing vLLM Endpoint ---")
        from src.deploy.serve import test_endpoint

        host = vllm_cfg.get("host", "localhost")
        port = vllm_cfg.get("port", 8000)

        # Use localhost for testing even if host is 0.0.0.0
        test_host = "localhost" if host == "0.0.0.0" else host

        result = test_endpoint(test_host, port)
        print(f"\nGenerated code:\n{result}")

    print("\n=== Deployment complete ===")


if __name__ == "__main__":
    main()

"""Merge QLoRA adapters back into a base model.

This module is intended for Artemis training/deployment pipelines.
"""

from __future__ import annotations

import argparse
import logging
from pathlib import Path

import torch
from peft import PeftModel
from transformers import AutoModelForCausalLM, AutoTokenizer


LOGGER = logging.getLogger(__name__)


def _resolve_torch_dtype(dtype_name: str) -> torch.dtype:
    mapping = {
        "float16": torch.float16,
        "fp16": torch.float16,
        "bfloat16": torch.bfloat16,
        "bf16": torch.bfloat16,
        "float32": torch.float32,
        "fp32": torch.float32,
    }
    key = (dtype_name or "").strip().lower()
    if key not in mapping:
        raise ValueError(
            f"Unsupported torch dtype '{dtype_name}'. "
            f"Supported: {', '.join(sorted(mapping.keys()))}"
        )
    return mapping[key]


def _model_size_gb(model: torch.nn.Module) -> float:
    total_bytes = 0
    for tensor in list(model.parameters()) + list(model.buffers()):
        total_bytes += tensor.numel() * tensor.element_size()
    return total_bytes / (1024**3)


def merge_adapters(
    base_model: str,
    adapter_path: str,
    output_dir: str,
    torch_dtype: str = "bfloat16",
) -> str:
    """Load base model + QLoRA adapter, merge, and save.

    Args:
        base_model: Hugging Face model ID or local model path.
        adapter_path: Path to PEFT/QLoRA adapter checkpoint.
        output_dir: Directory to write merged model and tokenizer.
        torch_dtype: Torch dtype string (e.g., bfloat16, float16, float32).

    Returns:
        The output directory path.
    """
    if not base_model:
        raise ValueError("base_model must be a non-empty string")
    if not adapter_path:
        raise ValueError("adapter_path must be a non-empty string")
    if not output_dir:
        raise ValueError("output_dir must be a non-empty string")

    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    adapter_fs_path = Path(adapter_path)
    if not adapter_fs_path.exists():
        raise FileNotFoundError(f"Adapter path does not exist: {adapter_path}")

    resolved_dtype = _resolve_torch_dtype(torch_dtype)
    LOGGER.info("Loading tokenizer from: %s", base_model)
    tokenizer = AutoTokenizer.from_pretrained(base_model, trust_remote_code=True)

    LOGGER.info("Loading base model from: %s (dtype=%s)", base_model, resolved_dtype)
    base = AutoModelForCausalLM.from_pretrained(
        base_model,
        torch_dtype=resolved_dtype,
        device_map="auto",
        trust_remote_code=True,
    )

    LOGGER.info("Loading adapter from: %s", adapter_path)
    peft_model = PeftModel.from_pretrained(base, adapter_path)

    size_before = _model_size_gb(peft_model)
    LOGGER.info("Model size before merge (params+buffers): %.3f GB", size_before)

    LOGGER.info("Merging adapters into base model...")
    merged_model = peft_model.merge_and_unload()

    size_after = _model_size_gb(merged_model)
    LOGGER.info("Model size after merge (params+buffers): %.3f GB", size_after)

    LOGGER.info("Saving merged model to: %s", output_path)
    merged_model.save_pretrained(output_path, safe_serialization=True)
    tokenizer.save_pretrained(output_path)

    LOGGER.info("Merge completed successfully.")
    return str(output_path)


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Merge QLoRA adapters into base model")
    parser.add_argument("--base-model", required=True, help="Base model path or HF model id")
    parser.add_argument("--adapter-path", required=True, help="Path to QLoRA adapter")
    parser.add_argument("--output-dir", required=True, help="Output directory for merged model")
    parser.add_argument(
        "--torch-dtype",
        default="bfloat16",
        help="Torch dtype for loading model (bfloat16, float16, float32)",
    )
    return parser


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s | %(levelname)s | %(message)s")
    args = _build_parser().parse_args()
    try:
        output = merge_adapters(
            base_model=args.base_model,
            adapter_path=args.adapter_path,
            output_dir=args.output_dir,
            torch_dtype=args.torch_dtype,
        )
        print(output)
    except Exception as exc:  # pragma: no cover
        LOGGER.exception("Merge failed: %s", exc)
        raise SystemExit(1) from exc

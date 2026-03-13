"""Quantize merged model with AutoAWQ 4-bit settings."""

from __future__ import annotations

import argparse
import logging
from pathlib import Path
from typing import Any

from awq import AutoAWQForCausalLM
from transformers import AutoTokenizer


LOGGER = logging.getLogger(__name__)


def _dir_size_bytes(path: Path) -> int:
    total = 0
    if not path.exists():
        return 0
    for fp in path.rglob("*"):
        if fp.is_file():
            total += fp.stat().st_size
    return total


def _format_size(num_bytes: int) -> str:
    units = ["B", "KB", "MB", "GB", "TB"]
    size = float(max(num_bytes, 0))
    idx = 0
    while size >= 1024 and idx < len(units) - 1:
        size /= 1024
        idx += 1
    return f"{size:.2f} {units[idx]}"


def quantize_awq(model_path: str, output_dir: str, config: dict | None = None) -> str:
    """Quantize a merged model using AutoAWQ.

    Default quantization config:
    - bits=4
    - group_size=128
    - zero_point=True
    """
    if not model_path:
        raise ValueError("model_path must be a non-empty string")
    if not output_dir:
        raise ValueError("output_dir must be a non-empty string")

    input_path = Path(model_path)
    if not input_path.exists():
        raise FileNotFoundError(f"Model path does not exist: {model_path}")

    out_path = Path(output_dir)
    out_path.mkdir(parents=True, exist_ok=True)

    quant_cfg: dict[str, Any] = {"bits": 4, "group_size": 128, "zero_point": True}
    if config:
        quant_cfg.update(config)

    if int(quant_cfg.get("bits", 4)) <= 0:
        raise ValueError(f"bits must be > 0, got: {quant_cfg.get('bits')}")
    if int(quant_cfg.get("group_size", 128)) <= 0:
        raise ValueError(f"group_size must be > 0, got: {quant_cfg.get('group_size')}")

    LOGGER.info("Loading tokenizer from: %s", input_path)
    tokenizer = AutoTokenizer.from_pretrained(str(input_path), trust_remote_code=True)

    LOGGER.info("Loading full-precision model from: %s", input_path)
    model = AutoAWQForCausalLM.from_pretrained(str(input_path), trust_remote_code=True)

    LOGGER.info("Starting AWQ quantization with config: %s", quant_cfg)
    model.quantize(tokenizer, quant_config=quant_cfg)

    LOGGER.info("Saving quantized model to: %s", out_path)
    model.save_quantized(str(out_path))
    tokenizer.save_pretrained(str(out_path))

    original_bytes = _dir_size_bytes(input_path)
    quantized_bytes = _dir_size_bytes(out_path)
    if quantized_bytes == 0:
        raise RuntimeError("Quantized output appears empty (0 bytes).")

    ratio = original_bytes / quantized_bytes if quantized_bytes else float("inf")
    print(f"Original size:  {_format_size(original_bytes)}")
    print(f"Quantized size: {_format_size(quantized_bytes)}")
    print(f"Compression ratio (orig/quant): {ratio:.2f}x")

    LOGGER.info("AWQ quantization completed successfully.")
    return str(out_path)


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Quantize model with AutoAWQ")
    parser.add_argument("--model-path", required=True, help="Path to merged full-precision model")
    parser.add_argument("--output-dir", required=True, help="Output directory for quantized model")
    parser.add_argument("--bits", type=int, default=4, help="Quantization bits (default: 4)")
    parser.add_argument("--group-size", type=int, default=128, help="AWQ group_size (default: 128)")
    parser.add_argument(
        "--zero-point",
        action=argparse.BooleanOptionalAction,
        default=True,
        help="Enable/disable zero_point (default: true)",
    )
    return parser


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s | %(levelname)s | %(message)s")
    args = _build_parser().parse_args()
    try:
        output = quantize_awq(
            model_path=args.model_path,
            output_dir=args.output_dir,
            config={
                "bits": args.bits,
                "group_size": args.group_size,
                "zero_point": args.zero_point,
            },
        )
        print(output)
    except Exception as exc:  # pragma: no cover
        LOGGER.exception("Quantization failed: %s", exc)
        raise SystemExit(1) from exc

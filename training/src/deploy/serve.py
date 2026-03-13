"""vLLM serving configuration, launcher, and endpoint test utilities."""

from __future__ import annotations

import argparse
import json
import logging
import subprocess
import time
from pathlib import Path
import requests
import yaml


LOGGER = logging.getLogger(__name__)


def generate_vllm_config(config: dict) -> dict:
    """Create normalized vLLM config dict from deployment.vllm section."""
    if not isinstance(config, dict):
        raise TypeError("config must be a dictionary")

    required = [
        "model",
        "host",
        "port",
        "gpu_memory_utilization",
        "max_model_len",
        "enforce_eager",
        "dtype",
        "tensor_parallel_size",
        "api_key",
    ]

    missing = [k for k in required if k not in config]
    if missing:
        raise KeyError(f"Missing required vLLM config keys: {', '.join(missing)}")

    normalized = {
        "model": str(config["model"]),
        "host": str(config["host"]),
        "port": int(config["port"]),
        "gpu_memory_utilization": float(config["gpu_memory_utilization"]),
        "max_model_len": int(config["max_model_len"]),
        "enforce_eager": bool(config["enforce_eager"]),
        "dtype": str(config["dtype"]),
        "tensor_parallel_size": int(config["tensor_parallel_size"]),
        "api_key": str(config["api_key"]),
    }

    if not normalized["model"]:
        raise ValueError("'model' must be a non-empty string")
    if not normalized["host"]:
        raise ValueError("'host' must be a non-empty string")
    if normalized["port"] <= 0:
        raise ValueError("'port' must be > 0")
    if not 0 < normalized["gpu_memory_utilization"] <= 1:
        raise ValueError("'gpu_memory_utilization' must be in range (0, 1]")
    if normalized["max_model_len"] <= 0:
        raise ValueError("'max_model_len' must be > 0")
    if normalized["tensor_parallel_size"] <= 0:
        raise ValueError("'tensor_parallel_size' must be > 0")

    return normalized


def _build_vllm_command(config: dict) -> list[str]:
    cmd = [
        "python",
        "-m",
        "vllm.entrypoints.openai.api_server",
        "--model",
        config["model"],
        "--host",
        config["host"],
        "--port",
        str(config["port"]),
        "--gpu-memory-utilization",
        str(config["gpu_memory_utilization"]),
        "--max-model-len",
        str(config["max_model_len"]),
        "--dtype",
        config["dtype"],
        "--tensor-parallel-size",
        str(config["tensor_parallel_size"]),
        "--api-key",
        config["api_key"],
    ]
    if config["enforce_eager"]:
        cmd.append("--enforce-eager")
    return cmd


def launch_vllm(config: dict, dry_run: bool = True):
    """Build and optionally launch vLLM OpenAI-compatible API server command."""
    validated = generate_vllm_config(config)
    cmd = _build_vllm_command(validated)
    pretty_cmd = " ".join(cmd)

    if dry_run:
        print(pretty_cmd)
        return pretty_cmd

    LOGGER.info("Launching vLLM server...")
    LOGGER.info("Command: %s", pretty_cmd)
    try:
        proc = subprocess.Popen(cmd)
    except OSError as exc:
        raise RuntimeError(f"Failed to launch vLLM process: {exc}") from exc
    return proc


def test_endpoint(host: str, port: int, prompt: str = "func main() {") -> str:
    """Send test completion request to vLLM OpenAI-compatible endpoint.

    Returns generated text and prints latency / tokens-per-second stats.
    """
    if not host:
        raise ValueError("host must be a non-empty string")
    if port <= 0:
        raise ValueError("port must be > 0")
    if not isinstance(prompt, str):
        raise TypeError("prompt must be a string")

    url = f"http://{host}:{port}/v1/completions"
    payload = {
        "model": "dummy",
        "prompt": prompt,
        "max_tokens": 128,
        "temperature": 0.2,
    }

    start = time.perf_counter()
    try:
        resp = requests.post(url, json=payload, timeout=90)
        resp.raise_for_status()
    except requests.RequestException as exc:
        raise RuntimeError(f"Request to vLLM endpoint failed: {exc}") from exc

    elapsed = time.perf_counter() - start
    data = resp.json()
    choices = data.get("choices", [])
    if not choices:
        raise RuntimeError(f"Unexpected response format, missing choices: {json.dumps(data)}")

    text = choices[0].get("text", "")
    usage = data.get("usage", {})
    completion_tokens = usage.get("completion_tokens")
    if isinstance(completion_tokens, int) and completion_tokens > 0 and elapsed > 0:
        tps = completion_tokens / elapsed
    else:
        tps = 0.0

    print(f"Latency: {elapsed:.3f}s")
    if completion_tokens is not None:
        print(f"Completion tokens: {completion_tokens}")
    print(f"Tokens/sec: {tps:.2f}")
    return text


def _load_config_from_yaml(config_path: str) -> dict:
    cfg_path = Path(config_path)
    if not cfg_path.exists():
        raise FileNotFoundError(f"Config file not found: {config_path}")

    try:
        with cfg_path.open("r", encoding="utf-8") as f:
            raw = yaml.safe_load(f)
    except yaml.YAMLError as exc:
        raise ValueError(f"Failed to parse YAML config: {exc}") from exc

    if not isinstance(raw, dict):
        raise ValueError("Top-level config must be a mapping")

    deployment = raw.get("deployment")
    if not isinstance(deployment, dict):
        raise KeyError("Missing 'deployment' section in config")

    vllm_cfg = deployment.get("vllm")
    if not isinstance(vllm_cfg, dict):
        raise KeyError("Missing 'deployment.vllm' section in config")

    return vllm_cfg


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="vLLM deployment launcher and tester")
    parser.add_argument(
        "--config-path",
        required=True,
        help="Path to training config yaml containing deployment.vllm",
    )
    parser.add_argument(
        "--launch",
        action="store_true",
        help="Launch vLLM server (default dry-run prints command)",
    )
    parser.add_argument(
        "--test",
        action="store_true",
        help="Run test request against /v1/completions",
    )
    parser.add_argument(
        "--prompt",
        default="func main() {",
        help="Prompt for endpoint test",
    )
    return parser


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s | %(levelname)s | %(message)s")
    args = _build_parser().parse_args()

    try:
        raw_cfg = _load_config_from_yaml(args.config_path)
        cfg = generate_vllm_config(raw_cfg)

        if args.launch:
            launch_vllm(cfg, dry_run=False)
        else:
            launch_vllm(cfg, dry_run=True)

        if args.test:
            output = test_endpoint(cfg["host"], cfg["port"], prompt=args.prompt)
            print(output)
    except Exception as exc:  # pragma: no cover
        LOGGER.exception("serve.py failed: %s", exc)
        raise SystemExit(1) from exc

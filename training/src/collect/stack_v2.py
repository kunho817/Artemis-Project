"""HuggingFace The Stack v2 collector for Go training data."""

from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any

import yaml
from datasets import load_dataset
from tqdm import tqdm


class StackV2Collector:
    """Collect Go files from The Stack v2 dataset and save them as JSONL."""

    def __init__(self, config: dict[str, Any]) -> None:
        """Initialize collector with the ``stack_v2`` section from data config.

        Args:
            config: Configuration dictionary for The Stack v2 collection.
        """
        self.config = config
        self.dataset_name = str(config.get("dataset_name", "bigcode/the-stack-v2-dedup"))
        self.split = str(config.get("split", "train"))
        self.language_filter = str(config.get("language_filter", "Go"))
        self.max_samples = int(config.get("max_samples", 500_000))
        self.streaming = bool(config.get("streaming", True))

    @staticmethod
    def _extract_first_string(sample: dict[str, Any], keys: list[str]) -> str:
        """Return the first non-empty string value from candidate keys."""
        for key in keys:
            value = sample.get(key)
            if isinstance(value, str) and value:
                return value
        return ""

    @staticmethod
    def _extract_repo(sample: dict[str, Any]) -> str:
        """Extract repository identifier from common Stack schema fields."""
        direct_candidates = [
            "repo",
            "repo_name",
            "repository_name",
            "repository",
            "max_stars_repo_name",
        ]
        repo = StackV2Collector._extract_first_string(sample, direct_candidates)
        if repo:
            return repo

        max_stars_repo = sample.get("max_stars_repo")
        if isinstance(max_stars_repo, dict):
            for key in ("repo_name", "name"):
                value = max_stars_repo.get(key)
                if isinstance(value, str) and value:
                    return value

        metadata = sample.get("metadata")
        if isinstance(metadata, dict):
            for key in ("repo_name", "repository", "repo"):
                value = metadata.get(key)
                if isinstance(value, str) and value:
                    return value

        return ""

    @staticmethod
    def _extract_language(sample: dict[str, Any]) -> str:
        """Extract language from common Stack schema fields."""
        lang = StackV2Collector._extract_first_string(
            sample,
            ["language", "lang", "programming_language"],
        )
        if lang:
            return lang

        metadata = sample.get("metadata")
        if isinstance(metadata, dict):
            for key in ("language", "lang"):
                value = metadata.get(key)
                if isinstance(value, str) and value:
                    return value

        return ""

    def collect(self, output_dir: str) -> int:
        """Collect Go files from The Stack v2 and write them into JSONL.

        Args:
            output_dir: Output directory where the JSONL file will be stored.

        Returns:
            Number of collected files written to disk.
        """
        output_path = Path(output_dir)
        output_path.mkdir(parents=True, exist_ok=True)
        jsonl_path = output_path / "go_stack_v2.jsonl"

        hf_token = os.getenv("HF_TOKEN")
        dataset_iterable = load_dataset(
            self.dataset_name,
            split=self.split,
            streaming=self.streaming,
            token=hf_token,
        )

        count = 0
        with jsonl_path.open("w", encoding="utf-8") as f_out:
            progress = tqdm(total=self.max_samples, desc="Collecting Go files from The Stack v2", unit="file")

            for sample in dataset_iterable:
                if count >= self.max_samples:
                    break

                if not isinstance(sample, dict):
                    continue

                language = self._extract_language(sample)
                if language.lower() != self.language_filter.lower():
                    continue

                content = self._extract_first_string(sample, ["content", "code", "text"])
                if not content:
                    continue

                path = self._extract_first_string(
                    sample,
                    ["path", "file_path", "filename", "blob_path"],
                )
                if not path:
                    path = "unknown.go"

                repo = self._extract_repo(sample)
                if not repo:
                    repo = "unknown"

                record = {
                    "path": path,
                    "repo": repo,
                    "content": content,
                    "size": len(content.encode("utf-8")),
                    "language": "Go",
                }

                f_out.write(json.dumps(record, ensure_ascii=False) + "\n")
                count += 1
                progress.update(1)

            progress.close()

        return count


if __name__ == "__main__":
    config_path = Path("configs/data_config.yaml")
    if not config_path.exists():
        raise FileNotFoundError(f"Config file not found: {config_path}")

    with config_path.open("r", encoding="utf-8") as fp:
        root_config: dict[str, Any] = yaml.safe_load(fp) or {}

    stack_config = dict(root_config.get("stack_v2", {}))
    collector = StackV2Collector(stack_config)
    collected = collector.collect(output_dir=str(Path("data/raw")))
    print(f"Collected {collected} Go files from The Stack v2.")

"""MinHash-based deduplication for JSONL code datasets."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from datasketch import MinHash, MinHashLSH
from tqdm import tqdm


class MinHashDedup:
    """Approximate near-duplicate remover using MinHash + LSH."""

    def __init__(self, config: dict[str, Any]) -> None:
        """Initialize deduplicator from dedup config section.

        Args:
            config: Dedup configuration dictionary.
        """
        self.config = config
        self.jaccard_threshold = float(config.get("jaccard_threshold", 0.7))
        self.shingle_size = int(config.get("shingle_size", 5))
        self.batch_size = int(config.get("batch_size", 10000))
        self.num_perm = int(config.get("num_perm", 128))
        self.content_key = str(config.get("content_key", "content"))

        self.lsh = MinHashLSH(threshold=self.jaccard_threshold, num_perm=self.num_perm)

    def build_index(self, input_path: str) -> int:
        """Build LSH index from all valid items in JSONL.

        Args:
            input_path: Input JSONL path.

        Returns:
            Number of indexed records.
        """
        self.lsh = MinHashLSH(threshold=self.jaccard_threshold, num_perm=self.num_perm)

        count = 0
        in_path = Path(input_path)
        with in_path.open("r", encoding="utf-8") as infile:
            for idx, raw_line in enumerate(tqdm(infile, desc="Building MinHash index", unit="item")):
                line = raw_line.strip()
                if not line:
                    continue

                try:
                    item = json.loads(line)
                except json.JSONDecodeError:
                    continue

                content = item.get(self.content_key)
                if not isinstance(content, str) or not content.strip():
                    continue

                mh = self._compute_minhash(content)
                self.lsh.insert(f"item_{idx}", mh)
                count += 1

        return count

    def deduplicate(self, input_path: str, output_path: str) -> tuple[int, int]:
        """Deduplicate input JSONL using batch processing and MinHash-LSH.

        Args:
            input_path: Input JSONL path.
            output_path: Output deduplicated JSONL path.

        Returns:
            Tuple of (original_count, deduplicated_count).
        """
        self.lsh = MinHashLSH(threshold=self.jaccard_threshold, num_perm=self.num_perm)

        in_path = Path(input_path)
        out_path = Path(output_path)
        out_path.parent.mkdir(parents=True, exist_ok=True)

        original_count = 0
        deduped_count = 0
        next_key = 0

        batch: list[dict[str, Any]] = []

        with in_path.open("r", encoding="utf-8") as infile, out_path.open(
            "w", encoding="utf-8"
        ) as outfile:
            for raw_line in tqdm(infile, desc="Deduplicating", unit="item"):
                line = raw_line.strip()
                if not line:
                    continue

                try:
                    item = json.loads(line)
                except json.JSONDecodeError:
                    continue

                content = item.get(self.content_key)
                if not isinstance(content, str) or not content.strip():
                    continue

                original_count += 1
                batch.append(item)

                if len(batch) >= self.batch_size:
                    kept, next_key = self._process_batch(batch=batch, start_key=next_key)
                    for keep_item, keep_mh in kept:
                        key = f"keep_{next_key}"
                        self.lsh.insert(key, keep_mh)
                        next_key += 1
                        outfile.write(json.dumps(keep_item, ensure_ascii=False) + "\n")
                        deduped_count += 1
                    batch.clear()

            if batch:
                kept, next_key = self._process_batch(batch=batch, start_key=next_key)
                for keep_item, keep_mh in kept:
                    key = f"keep_{next_key}"
                    self.lsh.insert(key, keep_mh)
                    next_key += 1
                    outfile.write(json.dumps(keep_item, ensure_ascii=False) + "\n")
                    deduped_count += 1

        return original_count, deduped_count

    def _process_batch(
        self, batch: list[dict[str, Any]], start_key: int
    ) -> tuple[list[tuple[dict[str, Any], MinHash]], int]:
        """Process a single batch against global index and intra-batch cache."""
        kept: list[tuple[dict[str, Any], MinHash]] = []
        local_minhashes: list[MinHash] = []
        local_key = start_key

        for item in batch:
            content = item[self.content_key]
            mh = self._compute_minhash(content)

            # Check against already accepted global items.
            global_hits = self.lsh.query(mh)
            is_duplicate = bool(global_hits)

            if not is_duplicate:
                # Check inside current batch with exact estimated similarity.
                for existing in local_minhashes:
                    if mh.jaccard(existing) >= self.jaccard_threshold:
                        is_duplicate = True
                        break

            if is_duplicate:
                continue

            kept.append((item, mh))
            local_minhashes.append(mh)
            local_key += 1

        return kept, local_key

    def _compute_minhash(self, content: str) -> MinHash:
        """Create MinHash from n-line shingles."""
        mh = MinHash(num_perm=self.num_perm)
        for shingle in self._line_shingles(content):
            mh.update(shingle.encode("utf-8"))
        return mh

    def _line_shingles(self, content: str) -> list[str]:
        """Split content into n-line sliding-window shingles."""
        lines = content.splitlines()

        if not lines:
            return [""]

        size = max(1, self.shingle_size)
        if len(lines) < size:
            return ["\n".join(lines)]

        shingles: list[str] = []
        for i in range(0, len(lines) - size + 1):
            shingles.append("\n".join(lines[i : i + size]))
        return shingles

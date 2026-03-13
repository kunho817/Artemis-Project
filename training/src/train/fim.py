"""FIM data collator for Qwen code-generation fine-tuning."""

from __future__ import annotations

import random
from typing import Any

import torch
from transformers import DataCollatorForLanguageModeling


FIM_PREFIX_TOKEN = "<|fim_prefix|>"
FIM_MIDDLE_TOKEN = "<|fim_middle|>"
FIM_SUFFIX_TOKEN = "<|fim_suffix|>"
FIM_PAD_TOKEN = "<|fim_pad|>"

FIM_PREFIX_ID = 151659
FIM_MIDDLE_ID = 151660
FIM_SUFFIX_ID = 151661
FIM_PAD_ID = 151662


def _token_id(tokenizer: Any, token: str, fallback: int) -> int:
    tok_id = tokenizer.convert_tokens_to_ids(token)
    return fallback if tok_id is None or tok_id < 0 else int(tok_id)


def apply_fim_transform(token_ids: list[int], tokenizer: Any, spm: bool = False) -> list[int]:
    """Apply FIM permutation to a tokenized sequence.

    Output ordering:
      - PSM (default): [fim_prefix] prefix [fim_suffix] suffix [fim_middle] middle
      - SPM:           [fim_suffix] suffix [fim_prefix] prefix [fim_middle] middle
    """
    if len(token_ids) < 3:
        return token_ids

    prefix_id = _token_id(tokenizer, FIM_PREFIX_TOKEN, FIM_PREFIX_ID)
    middle_id = _token_id(tokenizer, FIM_MIDDLE_TOKEN, FIM_MIDDLE_ID)
    suffix_id = _token_id(tokenizer, FIM_SUFFIX_TOKEN, FIM_SUFFIX_ID)

    left = random.randint(1, len(token_ids) - 2)
    right = random.randint(left + 1, len(token_ids) - 1)

    prefix = token_ids[:left]
    middle = token_ids[left:right]
    suffix = token_ids[right:]

    if spm:
        return [suffix_id, *suffix, prefix_id, *prefix, middle_id, *middle]
    return [prefix_id, *prefix, suffix_id, *suffix, middle_id, *middle]


class FIMDataCollator(DataCollatorForLanguageModeling):
    """Language-modeling collator with per-sample FIM augmentation."""

    def __init__(self, tokenizer: Any, fim_rate: float = 0.4, fim_spm_rate: float = 0.5):
        super().__init__(tokenizer=tokenizer, mlm=False)
        self.fim_rate = float(fim_rate)
        self.fim_spm_rate = float(fim_spm_rate)

        self.fim_prefix_id = _token_id(tokenizer, FIM_PREFIX_TOKEN, FIM_PREFIX_ID)
        self.fim_middle_id = _token_id(tokenizer, FIM_MIDDLE_TOKEN, FIM_MIDDLE_ID)
        self.fim_suffix_id = _token_id(tokenizer, FIM_SUFFIX_TOKEN, FIM_SUFFIX_ID)
        self.fim_pad_id = _token_id(tokenizer, FIM_PAD_TOKEN, FIM_PAD_ID)

        # CRITICAL: pad with <|fim_pad|>, not eos.
        self.tokenizer.pad_token = FIM_PAD_TOKEN
        self.tokenizer.pad_token_id = self.fim_pad_id

    def _extract_input_ids(self, feature: dict[str, Any]) -> list[int]:
        if "input_ids" in feature:
            values = feature["input_ids"]
            if isinstance(values, torch.Tensor):
                return values.tolist()
            return list(values)
        if "text" in feature:
            encoded = self.tokenizer(feature["text"], add_special_tokens=True, truncation=False)
            return list(encoded["input_ids"])
        raise ValueError("Each feature must contain either 'input_ids' or 'text'.")

    def __call__(self, features: list[dict[str, Any]]) -> dict[str, torch.Tensor]:
        batch_ids: list[list[int]] = []

        for feature in features:
            ids = self._extract_input_ids(feature)
            if random.random() < self.fim_rate:
                ids = apply_fim_transform(
                    token_ids=ids,
                    tokenizer=self.tokenizer,
                    spm=(random.random() < self.fim_spm_rate),
                )
            batch_ids.append(ids)

        max_len = max(len(x) for x in batch_ids) if batch_ids else 0

        input_ids = []
        attention_mask = []
        labels = []

        for ids in batch_ids:
            pad_len = max_len - len(ids)
            padded = ids + [self.fim_pad_id] * pad_len
            mask = [1] * len(ids) + [0] * pad_len
            label = ids + [-100] * pad_len

            input_ids.append(padded)
            attention_mask.append(mask)
            labels.append(label)

        return {
            "input_ids": torch.tensor(input_ids, dtype=torch.long),
            "attention_mask": torch.tensor(attention_mask, dtype=torch.long),
            "labels": torch.tensor(labels, dtype=torch.long),
        }


if __name__ == "__main__":
    from transformers import AutoTokenizer

    tokenizer = AutoTokenizer.from_pretrained("Qwen/Qwen2.5-Coder-7B", trust_remote_code=True)
    tokenizer.pad_token = FIM_PAD_TOKEN
    tokenizer.pad_token_id = FIM_PAD_ID

    collator = FIMDataCollator(tokenizer=tokenizer, fim_rate=1.0, fim_spm_rate=0.5)
    sample = [{"text": "def add(a, b):\n    return a + b\n"}, {"text": "print('hello')\n"}]
    out = collator(sample)

    print("input_ids shape:", tuple(out["input_ids"].shape))
    print("labels shape:", tuple(out["labels"].shape))
    print("first decoded:", tokenizer.decode(out["input_ids"][0].tolist(), skip_special_tokens=False))

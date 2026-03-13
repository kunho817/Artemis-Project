"""Training text formatting for multiple code-generation scenarios."""

from __future__ import annotations

import json
import random
import re
from pathlib import Path
from typing import Any

from tqdm import tqdm


class TrainingFormatter:
    """Convert extracted code items into SFT-ready ChatML/FIM records."""

    def __init__(self, config: dict[str, Any]) -> None:
        """Initialize formatter from format config section.

        Args:
            config: Formatting configuration dictionary.
        """
        self.config = config
        self.seed = int(config.get("seed", 42))
        self.rng = random.Random(self.seed)

        default_ratios: dict[str, float] = {
            "instruction": 0.30,
            "function_generation": 0.25,
            "fim_completion": 0.20,
            "bug_fix": 0.10,
            "code_review": 0.05,
            "file_generation": 0.10,
        }
        cfg_ratios = config.get("scenario_ratios", {})
        self.scenario_ratios = {
            name: float(cfg_ratios.get(name, default_value))
            for name, default_value in default_ratios.items()
        }

        self.system_prompts = {
            "instruction": str(
                config.get(
                    "instruction_system_prompt",
                    "You are an expert Go engineer. Follow the instruction and write correct, idiomatic Go code.",
                )
            ),
            "function_generation": str(
                config.get(
                    "function_generation_system_prompt",
                    "You generate function implementations from signatures and comments.",
                )
            ),
            "bug_fix": str(
                config.get(
                    "bug_fix_system_prompt",
                    "You are a Go debugging assistant. Fix the code while preserving intent.",
                )
            ),
            "code_review": str(
                config.get(
                    "code_review_system_prompt",
                    "You are a senior reviewer. Give concise, actionable review feedback.",
                )
            ),
            "file_generation": str(
                config.get(
                    "file_generation_system_prompt",
                    "You generate complete, production-quality Go files from requirements.",
                )
            ),
        }

        self.fim_prefix_token = str(config.get("fim_prefix_token", "<|fim_prefix|>"))
        self.fim_suffix_token = str(config.get("fim_suffix_token", "<|fim_suffix|>"))
        self.fim_middle_token = str(config.get("fim_middle_token", "<|fim_middle|>"))

    def format_instruction(self, item: dict[str, Any]) -> str:
        """Scenario 1: Natural language instruction -> code (ChatML)."""
        name = str(item.get("name") or "function")
        signature = str(item.get("signature") or "")
        doc = str(item.get("docstring") or "")
        imports = item.get("imports") or []
        body = str(item.get("body") or item.get("content") or "")

        user = (
            f"Write Go code for `{name}` based on this request:\n"
            f"- Signature: {signature or 'not provided'}\n"
            f"- Purpose: {doc or 'No explicit documentation provided'}\n"
            f"- Required imports context: {', '.join(imports) if imports else 'none'}\n"
            "Return only the final code."
        )
        assistant = body.strip() or signature.strip()

        return self._chatml(
            system=self.system_prompts["instruction"], user=user, assistant=assistant
        )

    def format_function_generation(self, item: dict[str, Any]) -> str:
        """Scenario 2: Signature + docstring -> function body (ChatML)."""
        signature = str(item.get("signature") or "")
        doc = str(item.get("docstring") or "")
        body = str(item.get("body") or "")

        user = (
            "Implement the function body for this Go declaration.\n"
            f"Signature:\n{signature or '// missing signature'}\n\n"
            f"Docstring:\n{doc or '// no docstring'}\n\n"
            "Return only the function body including braces."
        )
        assistant = body.strip() or "{}"

        return self._chatml(
            system=self.system_prompts["function_generation"],
            user=user,
            assistant=assistant,
        )

    def format_fim_completion(self, item: dict[str, Any]) -> str:
        """Scenario 3: Fill-in-the-middle with raw Qwen FIM tokens."""
        body = str(item.get("body") or item.get("content") or "")
        if not body:
            body = "{}"

        split_idx = self._random_split_index(body)
        prefix = body[:split_idx]
        middle = body[split_idx:]
        suffix = ""

        return (
            f"{self.fim_prefix_token}{prefix}"
            f"{self.fim_suffix_token}{suffix}"
            f"{self.fim_middle_token}{middle}"
        )

    def format_bug_fix(self, item: dict[str, Any]) -> str:
        """Scenario 4: Synthetic bug injection -> ask model to fix (ChatML)."""
        original = str(item.get("body") or item.get("content") or "")
        if not original:
            original = "{}"

        buggy = self._inject_bug(original)
        user = (
            "The following Go code contains synthetic bugs. Fix it and return corrected code only.\n\n"
            f"```go\n{buggy}\n```"
        )
        assistant = original.strip()

        return self._chatml(
            system=self.system_prompts["bug_fix"], user=user, assistant=assistant
        )

    def format_code_review(self, item: dict[str, Any]) -> str:
        """Scenario 5: Code snippet -> review comments (ChatML)."""
        code = str(item.get("body") or item.get("content") or "")
        if not code:
            code = "{}"

        review = self._make_review_comments(item=item, code=code)
        user = f"Review this Go code and provide concise comments.\n\n```go\n{code}\n```"

        return self._chatml(
            system=self.system_prompts["code_review"], user=user, assistant=review
        )

    def format_file_generation(self, item: dict[str, Any]) -> str:
        """Scenario 6: Description -> full file generation (ChatML)."""
        path = str(item.get("file_path") or item.get("path") or "unknown.go")
        imports = item.get("imports") or []
        signature = str(item.get("signature") or "")
        doc = str(item.get("docstring") or "")
        full_content = str(item.get("content") or "")

        if not full_content:
            body = str(item.get("body") or "")
            if signature:
                full_content = f"{signature}\n{body}".strip()
            else:
                full_content = body.strip() or "package main\n"

        user = (
            f"Generate a complete Go file for `{path}` with these requirements:\n"
            f"- Main declaration/context: {signature or 'not specified'}\n"
            f"- Intent: {doc or 'not specified'}\n"
            f"- Import context: {', '.join(imports) if imports else 'none'}\n"
            "Return the complete file content only."
        )

        return self._chatml(
            system=self.system_prompts["file_generation"],
            user=user,
            assistant=full_content,
        )

    def format_dataset(
        self, input_path: str, train_path: str, val_path: str, val_split: float
    ) -> dict[str, Any]:
        """Format deduplicated records into train/val JSONL for SFTTrainer.

        Output schema per line: {"text": "..."}
        """
        items = self._read_jsonl(input_path)

        self.rng.shuffle(items)
        total = len(items)
        val_count = int(total * float(val_split))
        val_indices = set(self.rng.sample(range(total), k=val_count)) if val_count > 0 else set()

        scenario_counts = {name: 0 for name in self.scenario_ratios}
        split_counts = {"train": 0, "val": 0}

        train_file = Path(train_path)
        val_file = Path(val_path)
        train_file.parent.mkdir(parents=True, exist_ok=True)
        val_file.parent.mkdir(parents=True, exist_ok=True)

        with train_file.open("w", encoding="utf-8") as train_out, val_file.open(
            "w", encoding="utf-8"
        ) as val_out:
            for idx, item in enumerate(tqdm(items, desc="Formatting dataset", unit="item")):
                scenario = self._choose_scenario()
                text = self._format_by_scenario(scenario, item)
                if not text.strip():
                    continue

                scenario_counts[scenario] += 1
                out_record = {"text": text}

                if idx in val_indices:
                    val_out.write(json.dumps(out_record, ensure_ascii=False) + "\n")
                    split_counts["val"] += 1
                else:
                    train_out.write(json.dumps(out_record, ensure_ascii=False) + "\n")
                    split_counts["train"] += 1

        return {
            "total_input": total,
            "train_count": split_counts["train"],
            "val_count": split_counts["val"],
            "scenarios": scenario_counts,
            "val_split": float(val_split),
        }

    def _chatml(self, system: str, user: str, assistant: str) -> str:
        return (
            f"<|im_start|>system\n{system}<|im_end|>\n"
            f"<|im_start|>user\n{user}<|im_end|>\n"
            f"<|im_start|>assistant\n{assistant}<|im_end|>"
        )

    def _choose_scenario(self) -> str:
        names = list(self.scenario_ratios.keys())
        weights = [max(0.0, self.scenario_ratios[n]) for n in names]
        total = sum(weights)
        if total <= 0:
            return names[0]
        normalized = [w / total for w in weights]
        return self.rng.choices(names, weights=normalized, k=1)[0]

    def _format_by_scenario(self, scenario: str, item: dict[str, Any]) -> str:
        if scenario == "instruction":
            return self.format_instruction(item)
        if scenario == "function_generation":
            return self.format_function_generation(item)
        if scenario == "fim_completion":
            return self.format_fim_completion(item)
        if scenario == "bug_fix":
            return self.format_bug_fix(item)
        if scenario == "code_review":
            return self.format_code_review(item)
        if scenario == "file_generation":
            return self.format_file_generation(item)
        return self.format_instruction(item)

    def _read_jsonl(self, input_path: str) -> list[dict[str, Any]]:
        records: list[dict[str, Any]] = []
        with Path(input_path).open("r", encoding="utf-8") as infile:
            for raw_line in infile:
                line = raw_line.strip()
                if not line:
                    continue
                try:
                    item = json.loads(line)
                except json.JSONDecodeError:
                    continue
                if isinstance(item, dict):
                    records.append(item)
        return records

    def _random_split_index(self, text: str) -> int:
        if len(text) <= 2:
            return len(text)

        min_ratio = float(self.config.get("fim_min_split_ratio", 0.2))
        max_ratio = float(self.config.get("fim_max_split_ratio", 0.8))
        min_idx = max(1, int(len(text) * min_ratio))
        max_idx = min(len(text) - 1, int(len(text) * max_ratio))
        if max_idx <= min_idx:
            return len(text) // 2
        return self.rng.randint(min_idx, max_idx)

    def _inject_bug(self, code: str) -> str:
        strategies = self.config.get(
            "bug_strategies",
            ["swap_operator", "rename_var", "remove_line", "off_by_one"],
        )
        selected = str(self.rng.choice(strategies)) if strategies else "swap_operator"

        if selected == "swap_operator":
            return self._bug_swap_operator(code)
        if selected == "rename_var":
            return self._bug_rename_var(code)
        if selected == "remove_line":
            return self._bug_remove_line(code)
        if selected == "off_by_one":
            return self._bug_off_by_one(code)
        return self._bug_swap_operator(code)

    def _bug_swap_operator(self, code: str) -> str:
        replacements = [
            (">=", "<="),
            ("<=", ">="),
            ("==", "!="),
            ("!=", "=="),
            ("+", "-"),
            ("-", "+"),
        ]
        for old, new in replacements:
            if old in code:
                return code.replace(old, new, 1)
        return code

    def _bug_rename_var(self, code: str) -> str:
        candidates = re.findall(r"\b[a-z][a-zA-Z0-9_]{2,}\b", code)
        banned = {
            "for",
            "if",
            "else",
            "return",
            "func",
            "type",
            "var",
            "const",
            "package",
            "import",
            "range",
            "switch",
            "case",
            "default",
            "go",
            "defer",
            "nil",
            "true",
            "false",
        }
        filtered = [c for c in candidates if c not in banned]
        if not filtered:
            return code

        target = self.rng.choice(filtered)
        broken = f"{target}_tmp"
        return re.sub(rf"\b{re.escape(target)}\b", broken, code, count=1)

    def _bug_remove_line(self, code: str) -> str:
        lines = code.splitlines()
        if len(lines) <= 2:
            return code

        removable = [
            idx
            for idx, ln in enumerate(lines)
            if ln.strip() and not ln.strip().startswith("//") and ln.strip() not in {"{", "}"}
        ]
        if not removable:
            return code

        remove_idx = self.rng.choice(removable)
        new_lines = lines[:remove_idx] + lines[remove_idx + 1 :]
        return "\n".join(new_lines)

    def _bug_off_by_one(self, code: str) -> str:
        patterns = [("< len(", "<= len("), ("<= len(", "< len("), ("i++", "i += 2")]
        for old, new in patterns:
            if old in code:
                return code.replace(old, new, 1)
        return code

    def _make_review_comments(self, item: dict[str, Any], code: str) -> str:
        comments: list[str] = []

        doc = str(item.get("docstring") or "")
        if not doc:
            comments.append("- Missing docstring/comment: add a short description of purpose.")

        if "panic(" in code:
            comments.append("- Avoid panic in library code; return errors when possible.")

        if "TODO" in code or "FIXME" in code:
            comments.append("- Resolve TODO/FIXME markers before production use.")

        if "fmt.Println" in code and "testing" not in str(item.get("file_path") or ""):
            comments.append("- Replace fmt.Println debugging with structured logging or remove it.")

        if len(code.splitlines()) > int(self.config.get("review_long_function_lines", 60)):
            comments.append("- Function is long; consider splitting into smaller helpers.")

        if not comments:
            comments.append("- Code looks clear overall; consider adding focused unit tests.")

        return "\n".join(comments)

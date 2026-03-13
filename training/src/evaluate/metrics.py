"""
Evaluation metrics for Artemis code generation model.
Measures code quality, correctness, and FIM completion accuracy.
"""

from __future__ import annotations

import re
import subprocess
import tempfile
from pathlib import Path
from typing import Optional


def exact_match(prediction: str, reference: str) -> float:
    """Exact match after stripping whitespace."""
    return 1.0 if prediction.strip() == reference.strip() else 0.0


def pass_at_k(n: int, c: int, k: int) -> float:
    """
    Compute pass@k metric.

    Args:
        n: total number of samples generated
        c: number of correct samples
        k: k in pass@k

    Returns:
        Estimated pass@k probability
    """
    if n - c < k:
        return 1.0
    return 1.0 - _comb(n - c, k) / _comb(n, k)


def _comb(n: int, k: int) -> float:
    """Compute combination C(n, k)."""
    if k > n:
        return 0.0
    if k == 0 or k == n:
        return 1.0
    k = min(k, n - k)
    result = 1.0
    for i in range(k):
        result = result * (n - i) / (i + 1)
    return result


def syntax_check_go(code: str) -> tuple[bool, str]:
    """
    Check if Go code is syntactically valid using `go vet` or `gofmt`.

    Returns:
        (is_valid, error_message)
    """
    with tempfile.NamedTemporaryFile(mode="w", suffix=".go", delete=False) as f:
        # Wrap in package if not present
        if "package " not in code:
            code = "package main\n\n" + code
        f.write(code)
        f.flush()
        tmp_path = f.name

    try:
        result = subprocess.run(
            ["gofmt", "-e", tmp_path],
            capture_output=True,
            text=True,
            timeout=10,
        )
        if result.returncode == 0:
            return True, ""
        return False, result.stderr.strip()
    except FileNotFoundError:
        # gofmt not available — skip syntax check
        return True, "gofmt not found, skipped"
    except subprocess.TimeoutExpired:
        return False, "syntax check timed out"
    finally:
        Path(tmp_path).unlink(missing_ok=True)


def compile_check_go(code: str) -> tuple[bool, str]:
    """
    Check if Go code compiles using `go build`.

    Returns:
        (compiles, error_message)
    """
    with tempfile.TemporaryDirectory() as tmpdir:
        tmpdir_path = Path(tmpdir)

        # Write go.mod
        (tmpdir_path / "go.mod").write_text("module test\n\ngo 1.21\n")

        # Wrap in package main if needed
        if "package " not in code:
            code = "package main\n\n" + code
        (tmpdir_path / "main.go").write_text(code)

        try:
            result = subprocess.run(
                ["go", "build", "./..."],
                capture_output=True,
                text=True,
                timeout=30,
                cwd=tmpdir,
            )
            if result.returncode == 0:
                return True, ""
            return False, result.stderr.strip()
        except FileNotFoundError:
            return True, "go not found, skipped"
        except subprocess.TimeoutExpired:
            return False, "compilation timed out"


def code_bleu_approx(prediction: str, reference: str) -> float:
    """
    Approximate CodeBLEU using token-level n-gram overlap.
    Simplified version — full CodeBLEU requires AST matching.

    Returns:
        Score between 0.0 and 1.0
    """
    pred_tokens = _tokenize_code(prediction)
    ref_tokens = _tokenize_code(reference)

    if not ref_tokens:
        return 0.0

    scores = []
    for n in range(1, 5):  # 1-gram to 4-gram
        pred_ngrams = _ngrams(pred_tokens, n)
        ref_ngrams = _ngrams(ref_tokens, n)
        if not ref_ngrams:
            continue
        overlap = sum(1 for ng in pred_ngrams if ng in ref_ngrams)
        precision = overlap / len(pred_ngrams) if pred_ngrams else 0.0
        scores.append(precision)

    if not scores:
        return 0.0

    # Geometric mean of n-gram precisions
    import math
    log_avg = sum(math.log(max(s, 1e-10)) for s in scores) / len(scores)
    return min(math.exp(log_avg), 1.0)


def _tokenize_code(code: str) -> list[str]:
    """Tokenize code into meaningful tokens (identifiers, operators, keywords)."""
    return re.findall(r"[a-zA-Z_]\w*|[0-9]+|[^\s]", code)


def _ngrams(tokens: list[str], n: int) -> list[tuple[str, ...]]:
    """Extract n-grams from token list."""
    return [tuple(tokens[i : i + n]) for i in range(len(tokens) - n + 1)]


def fim_accuracy(prediction: str, reference_middle: str) -> float:
    """
    Measure FIM completion accuracy.
    Compares predicted middle section against reference.

    Returns:
        Token-level F1 score
    """
    pred_tokens = set(_tokenize_code(prediction))
    ref_tokens = set(_tokenize_code(reference_middle))

    if not ref_tokens:
        return 1.0 if not pred_tokens else 0.0

    if not pred_tokens:
        return 0.0

    intersection = pred_tokens & ref_tokens
    precision = len(intersection) / len(pred_tokens)
    recall = len(intersection) / len(ref_tokens)

    if precision + recall == 0:
        return 0.0
    return 2 * precision * recall / (precision + recall)


class EvaluationSuite:
    """Runs a full evaluation suite on model outputs."""

    def __init__(self, check_syntax: bool = True, check_compile: bool = False):
        self.check_syntax = check_syntax
        self.check_compile = check_compile

    def evaluate_batch(
        self,
        predictions: list[str],
        references: list[str],
        modes: Optional[list[str]] = None,
    ) -> dict:
        """
        Evaluate a batch of predictions against references.

        Args:
            predictions: Model outputs
            references: Ground truth
            modes: Optional list of scenario types per item ("instruction", "fim", etc.)

        Returns:
            Dict with aggregate metrics
        """
        n = len(predictions)
        assert n == len(references), "predictions and references must have same length"

        results = {
            "n": n,
            "exact_match": 0.0,
            "code_bleu": 0.0,
            "syntax_valid": 0.0,
            "compile_valid": 0.0,
            "fim_accuracy": 0.0,
            "fim_count": 0,
        }

        for i, (pred, ref) in enumerate(zip(predictions, references)):
            results["exact_match"] += exact_match(pred, ref)
            results["code_bleu"] += code_bleu_approx(pred, ref)

            if self.check_syntax:
                valid, _ = syntax_check_go(pred)
                results["syntax_valid"] += 1.0 if valid else 0.0

            if self.check_compile:
                valid, _ = compile_check_go(pred)
                results["compile_valid"] += 1.0 if valid else 0.0

            if modes and i < len(modes) and modes[i] == "fim":
                results["fim_accuracy"] += fim_accuracy(pred, ref)
                results["fim_count"] += 1

        # Average
        for key in ["exact_match", "code_bleu", "syntax_valid", "compile_valid"]:
            results[key] /= max(n, 1)

        if results["fim_count"] > 0:
            results["fim_accuracy"] /= results["fim_count"]

        return results

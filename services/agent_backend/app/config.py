"""Configuration for Z.AI GLM Coding Plan model routing."""

from __future__ import annotations

from dataclasses import dataclass
import os


ZAI_CODING_BASE_URL = "https://api.z.ai/api/coding/paas/v4"


@dataclass(frozen=True)
class GLMModelProfile:
    name: str
    tier: str
    context_tokens: int
    default_max_output_tokens: int
    recommended_for: tuple[str, ...]


GLM_MODEL_PROFILES: dict[str, GLMModelProfile] = {
    "glm-5.1": GLMModelProfile(
        name="glm-5.1",
        tier="frontier",
        context_tokens=200_000,
        default_max_output_tokens=8_192,
        recommended_for=("orchestrator", "architect", "long_horizon_planner"),
    ),
    "glm-5": GLMModelProfile(
        name="glm-5",
        tier="high",
        context_tokens=128_000,
        default_max_output_tokens=8_192,
        recommended_for=("planner", "work_package_writer", "qa"),
    ),
    "glm-4.7": GLMModelProfile(
        name="glm-4.7",
        tier="balanced",
        context_tokens=128_000,
        default_max_output_tokens=6_144,
        recommended_for=("context_collector", "researcher", "summarizer"),
    ),
    "glm-4.6": GLMModelProfile(
        name="glm-4.6",
        tier="efficient",
        context_tokens=128_000,
        default_max_output_tokens=4_096,
        recommended_for=("validator", "classifier", "simple_qa"),
    ),
    "glm-4.5": GLMModelProfile(
        name="glm-4.5",
        tier="efficient",
        context_tokens=128_000,
        default_max_output_tokens=4_096,
        recommended_for=("fallback", "classification"),
    ),
    "glm-4.5-air": GLMModelProfile(
        name="glm-4.5-air",
        tier="fast",
        context_tokens=128_000,
        default_max_output_tokens=4_096,
        recommended_for=("fast_context", "light_validation"),
    ),
    "glm-4.5-flash": GLMModelProfile(
        name="glm-4.5-flash",
        tier="fastest",
        context_tokens=128_000,
        default_max_output_tokens=2_048,
        recommended_for=("cheap_classification", "routing"),
    ),
}


DEFAULT_ROLE_MODELS: dict[str, str] = {
    "orchestrator": "glm-5.1",
    "architect": "glm-5.1",
    "planner": "glm-5",
    "work_package_writer": "glm-5",
    "context_collector": "glm-4.7",
    "validator": "glm-4.6",
    "qa": "glm-4.7",
}


@dataclass(frozen=True)
class ModelSelection:
    role: str
    model: str
    base_url: str
    api_key: str | None
    profile: GLMModelProfile


def normalize_role(role: str) -> str:
    return role.strip().lower().replace("-", "_")


def model_for_role(role: str, env: dict[str, str] | None = None) -> ModelSelection:
    source = env if env is not None else os.environ
    normalized_role = normalize_role(role)
    role_env_name = f"ARTEMIS_GLM_MODEL_{normalized_role.upper()}"
    model = (
        source.get(role_env_name)
        or DEFAULT_ROLE_MODELS.get(normalized_role)
        or source.get("ARTEMIS_GLM_DEFAULT_MODEL")
        or source.get("ZAI_MODEL")
        or "glm-5.1"
    )
    if model not in GLM_MODEL_PROFILES:
        allowed = ", ".join(sorted(GLM_MODEL_PROFILES))
        raise ValueError(f"Unsupported GLM model '{model}'. Allowed: {allowed}")
    return ModelSelection(
        role=normalized_role,
        model=model,
        base_url=source.get("ZAI_BASE_URL", ZAI_CODING_BASE_URL).rstrip("/"),
        api_key=source.get("ZAI_API_KEY") or source.get("ZHIPU_API_KEY") or source.get("GLM_API_KEY"),
        profile=GLM_MODEL_PROFILES[model],
    )

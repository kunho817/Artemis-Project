"""LangChain adapter for Z.AI GLM Coding Plan."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from .config import ModelSelection, model_for_role


@dataclass
class GLMResponse:
    content: str
    model: str
    role: str


class GLMChatClient:
    """Lazy LangChain client for OpenAI-compatible Z.AI Coding Plan calls."""

    def __init__(self, role: str) -> None:
        self.selection: ModelSelection = model_for_role(role)

    @property
    def configured(self) -> bool:
        return bool(self.selection.api_key)

    def _build_langchain_model(self) -> Any:
        if not self.selection.api_key:
            raise RuntimeError("ZAI_API_KEY is not configured")
        try:
            from langchain_openai import ChatOpenAI
        except ImportError as exc:
            raise RuntimeError(
                "langchain-openai is required for live GLM calls. "
                "Install services/agent_backend dependencies first."
            ) from exc

        return ChatOpenAI(
            api_key=self.selection.api_key,
            base_url=self.selection.base_url,
            model=self.selection.model,
            temperature=0,
            max_tokens=self.selection.profile.default_max_output_tokens,
        )

    def invoke(self, messages: list[dict[str, str]]) -> GLMResponse:
        model = self._build_langchain_model()
        response = model.invoke(messages)
        content = getattr(response, "content", str(response))
        return GLMResponse(
            content=content,
            model=self.selection.model,
            role=self.selection.role,
        )

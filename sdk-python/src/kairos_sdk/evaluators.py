from __future__ import annotations

from typing import Protocol

from .models import EvaluationResult, PluginContext


class Evaluator(Protocol):
    def evaluate(self, prompt: str, output: str, context: PluginContext) -> EvaluationResult:
        ...

"""Runtime configuration for the optional AI sidecar."""

from __future__ import annotations

from dataclasses import dataclass
import os
from pathlib import Path


@dataclass(frozen=True)
class ServiceConfig:
    model_dir: Path
    mode: str


def load_config() -> ServiceConfig:
    model_dir = Path(os.environ.get("CARTOLENSIA_AI_MODEL_DIR", ".cartolensia/models")).resolve()
    mode = os.environ.get("CARTOLENSIA_AI_MODE", "dummy").strip().lower() or "dummy"
    return ServiceConfig(model_dir=model_dir, mode=mode)


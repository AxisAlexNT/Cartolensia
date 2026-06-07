"""Runtime configuration for the optional AI sidecar."""

from __future__ import annotations

from dataclasses import dataclass
import os
from pathlib import Path


@dataclass(frozen=True)
class ServiceConfig:
    model_dir: Path
    mode: str
    device: str
    classifier: str
    safety_model: str
    caption_model: str
    openclip_model: str
    openclip_pretrained: str
    safety_threshold: float


def load_config() -> ServiceConfig:
    model_dir = Path(os.environ.get("CARTOLENSIA_AI_MODEL_DIR", ".cartolensia/models")).resolve()
    mode = os.environ.get("CARTOLENSIA_AI_MODE", "auto").strip().lower() or "auto"
    threshold_raw = os.environ.get("CARTOLENSIA_AI_SAFETY_THRESHOLD", "0.75")
    try:
        threshold = float(threshold_raw)
    except ValueError:
        threshold = 0.75
    return ServiceConfig(
        model_dir=model_dir,
        mode=mode,
        device=os.environ.get("CARTOLENSIA_AI_DEVICE", "auto").strip().lower() or "auto",
        classifier=os.environ.get("CARTOLENSIA_AI_CLASSIFIER", "efficientnet_b0").strip().lower() or "efficientnet_b0",
        safety_model=os.environ.get("CARTOLENSIA_AI_SAFETY_MODEL", "Falconsai/nsfw_image_detection"),
        caption_model=os.environ.get("CARTOLENSIA_AI_CAPTION_MODEL", "Salesforce/blip-image-captioning-base"),
        openclip_model=os.environ.get("CARTOLENSIA_AI_OPENCLIP_MODEL", "ViT-B-32"),
        openclip_pretrained=os.environ.get(
            "CARTOLENSIA_AI_OPENCLIP_PRETRAINED",
            "hf-hub:laion/CLIP-ViT-B-32-laion2B-s34B-b79K",
        ),
        safety_threshold=threshold,
    )

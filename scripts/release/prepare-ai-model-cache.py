#!/usr/bin/env python3
"""Prepare the reviewed Cartolensia local AI model cache.

This script is intended for an Internet-connected staging machine before
building a private/offline full bundle. It stores downloads only in the
configured model directory and mirrors the paths expected by the AI sidecar.
"""

from __future__ import annotations

import argparse
import os
from pathlib import Path
import sys
import urllib.request


YUNET_URL = (
    "https://github.com/opencv/opencv_zoo/raw/main/models/"
    "face_detection_yunet/face_detection_yunet_2023mar.onnx"
)


def note(message: str) -> None:
    print(f"[cartolensia-model-cache] {message}", flush=True)


def configure_cache(model_dir: Path) -> None:
    model_dir.mkdir(parents=True, exist_ok=True)
    os.environ["TORCH_HOME"] = str(model_dir / "torch")
    os.environ["HF_HOME"] = str(model_dir / "huggingface")
    os.environ["HUGGINGFACE_HUB_CACHE"] = str(model_dir / "huggingface")
    os.environ["TRANSFORMERS_CACHE"] = str(model_dir / "huggingface" / "transformers")


def fetch_torchvision(model_dir: Path) -> None:
    note("downloading torchvision EfficientNet-B0 and MobileNetV3 weights")
    import torchvision.models as models  # type: ignore

    models.efficientnet_b0(weights=models.EfficientNet_B0_Weights.DEFAULT)
    models.mobilenet_v3_large(weights=models.MobileNet_V3_Large_Weights.DEFAULT)
    expected = model_dir / "torch" / "hub" / "checkpoints"
    note(f"torchvision checkpoint cache: {expected}")


def fetch_yunet(model_dir: Path) -> None:
    target = model_dir / "opencv" / "face_detection_yunet_2023mar.onnx"
    target.parent.mkdir(parents=True, exist_ok=True)
    if target.exists() and target.stat().st_size > 0:
        note(f"OpenCV YuNet already present: {target}")
        return
    note(f"downloading OpenCV YuNet from {YUNET_URL}")
    urllib.request.urlretrieve(YUNET_URL, target)


def snapshot_hf(repo_id: str, cache_dir: Path, allow_patterns: list[str] | None = None) -> None:
    from huggingface_hub import snapshot_download  # type: ignore

    note(f"downloading Hugging Face snapshot: {repo_id}")
    snapshot_download(
        repo_id=repo_id,
        cache_dir=str(cache_dir),
        allow_patterns=allow_patterns,
        local_files_only=False,
    )


def fetch_huggingface(model_dir: Path) -> None:
    hf_cache = model_dir / "huggingface"
    snapshot_hf("Falconsai/nsfw_image_detection", hf_cache)
    snapshot_hf("Salesforce/blip-image-captioning-base", hf_cache)
    snapshot_hf("laion/CLIP-ViT-B-32-laion2B-s34B-b79K", model_dir / "openclip")


def fetch_faster_whisper(model_dir: Path, model_name: str) -> None:
    note(f"downloading faster-whisper model: {model_name}")
    from faster_whisper import WhisperModel  # type: ignore

    WhisperModel(
        model_name,
        device="cpu",
        compute_type="int8",
        download_root=str(model_dir / "faster-whisper"),
        local_files_only=False,
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Prepare Cartolensia reviewed AI model cache")
    parser.add_argument("--model-dir", default=".cartolensia/models")
    parser.add_argument("--whisper-model", default="small", choices=["small", "medium"])
    parser.add_argument("--skip-whisper", action="store_true")
    parser.add_argument("--skip-huggingface", action="store_true")
    parser.add_argument("--skip-torchvision", action="store_true")
    parser.add_argument("--skip-yunet", action="store_true")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    model_dir = Path(args.model_dir).resolve()
    configure_cache(model_dir)
    try:
        if not args.skip_torchvision:
            fetch_torchvision(model_dir)
        if not args.skip_yunet:
            fetch_yunet(model_dir)
        if not args.skip_huggingface:
            fetch_huggingface(model_dir)
        if not args.skip_whisper:
            fetch_faster_whisper(model_dir, args.whisper_model)
    except Exception as exc:  # noqa: BLE001 - command-line tool reports actionable failure.
        print(f"[cartolensia-model-cache] failed: {exc}", file=sys.stderr)
        return 1
    note(f"model cache ready: {model_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

"""Lazy local inference backend for the optional AI sidecar."""

from __future__ import annotations

from dataclasses import dataclass
import os
from pathlib import Path
from typing import Any

from cartolensia_ai.config import ServiceConfig
from cartolensia_ai.image_io import load_pillow_image
from cartolensia_ai.schemas import ImageRequest, InferenceResponse, Prediction, TextRequest


CAPABILITIES = [
    "classify_image",
    "detect_faces",
    "safety_nsfw",
    "describe_image",
    "embed_image",
    "embed_text",
]


@dataclass
class LoadedModel:
    name: str
    model: Any
    preprocess: Any = None
    metadata: dict[str, Any] | None = None


class RealBackend:
    """Loads approved local models on demand and keeps them in process memory."""

    def __init__(self, config: ServiceConfig):
        self.config = config
        self._configure_cache_environment()
        self._device: str | None = None
        self._classification: LoadedModel | None = None
        self._safety: LoadedModel | None = None
        self._caption: LoadedModel | None = None
        self._openclip: LoadedModel | None = None
        self._yunet: Any | None = None

    def _configure_cache_environment(self) -> None:
        self.config.model_dir.mkdir(parents=True, exist_ok=True)
        os.environ.setdefault("TORCH_HOME", str(self.config.model_dir / "torch"))
        os.environ.setdefault("HF_HOME", str(self.config.model_dir / "huggingface"))
        os.environ.setdefault("HUGGINGFACE_HUB_CACHE", str(self.config.model_dir / "huggingface" / "hub"))
        os.environ.setdefault("TRANSFORMERS_CACHE", str(self.config.model_dir / "huggingface" / "transformers"))

    @property
    def device(self) -> str:
        if self._device is None:
            self._device = self._select_device()
        return self._device

    @property
    def model_state(self) -> str:
        if self.config.mode == "dummy":
            return "dummy"
        return "lazy_loaded"

    def metadata(self) -> dict[str, Any]:
        return {
            "backend": "real",
            "device": self._device or self.config.device,
            "model_dir": str(self.config.model_dir),
            "models": self.models_status(),
            "capabilities": CAPABILITIES,
        }

    def models_status(self) -> dict[str, Any]:
        yunet_path = self.config.model_dir / "opencv" / "face_detection_yunet_2023mar.onnx"
        return {
            "classifier": {
                "name": self.config.classifier,
                "loaded": self._classification is not None,
                "fallback": "mobilenet_v3_large",
            },
            "face_detector": {
                "name": "opencv_yunet_2023mar",
                "path": str(yunet_path),
                "available": yunet_path.exists(),
                "loaded": self._yunet is not None,
            },
            "safety": {
                "name": self.config.safety_model,
                "loaded": self._safety is not None,
                "threshold": self.config.safety_threshold,
            },
            "openclip": {
                "name": self.config.openclip_pretrained,
                "loaded": self._openclip is not None,
            },
            "caption": {
                "name": self.config.caption_model,
                "loaded": self._caption is not None,
            },
        }

    def classify_image(self, request: ImageRequest) -> InferenceResponse:
        try:
            image = load_pillow_image(request, self.config)
            loaded = self._load_classification()
            torch = _import_torch()
            tensor = loaded.preprocess(image).unsqueeze(0).to(self.device)
            with torch.no_grad():
                logits = loaded.model(tensor)
                probs = torch.nn.functional.softmax(logits[0], dim=0)
                values, indices = torch.topk(probs, int(request.options.get("top_k", 5) if request.options else 5))
            labels = loaded.metadata.get("categories", []) if loaded.metadata else []
            predictions = []
            for confidence, index in zip(values.detach().cpu().tolist(), indices.detach().cpu().tolist()):
                label = labels[index] if index < len(labels) else str(index)
                predictions.append(
                    Prediction(
                        label=label,
                        confidence=float(confidence),
                        metadata={"model": loaded.name, "index": int(index)},
                    )
                )
            return InferenceResponse(
                status="ok",
                endpoint="classify-image",
                predictions=predictions,
                metadata={"asset_id": request.asset_id, "device": self.device, "model": loaded.name},
            )
        except MissingModelError as exc:
            return _model_missing("classify-image", exc)
        except Exception as exc:
            return _error("classify-image", exc)

    def detect_faces(self, request: ImageRequest) -> InferenceResponse:
        try:
            image = load_pillow_image(request, self.config)
            detector = self._load_yunet(image.size)
            import cv2  # type: ignore
            import numpy as np

            rgb = np.array(image)
            bgr = cv2.cvtColor(rgb, cv2.COLOR_RGB2BGR)
            detector.setInputSize((image.width, image.height))
            _, faces = detector.detect(bgr)
            found: list[dict[str, Any]] = []
            predictions: list[Prediction] = []
            if faces is not None:
                for idx, row in enumerate(faces):
                    x, y, width, height = [float(value) for value in row[:4]]
                    confidence = float(row[-1])
                    face = {
                        "x": x,
                        "y": y,
                        "width": width,
                        "height": height,
                        "confidence": confidence,
                    }
                    found.append(face)
                    predictions.append(
                        Prediction(
                            label="face",
                            confidence=confidence,
                            metadata={"index": idx, **face, "model": "opencv_yunet_2023mar"},
                        )
                    )
            return InferenceResponse(
                status="ok",
                endpoint="detect-faces",
                predictions=predictions,
                metadata={"asset_id": request.asset_id, "faces": found, "model": "opencv_yunet_2023mar"},
            )
        except MissingModelError as exc:
            return _model_missing("detect-faces", exc)
        except Exception as exc:
            return _error("detect-faces", exc)

    def safety_nsfw(self, request: ImageRequest) -> InferenceResponse:
        try:
            image = load_pillow_image(request, self.config)
            loaded = self._load_safety()
            torch = _import_torch()
            processor = loaded.preprocess
            inputs = processor(images=image, return_tensors="pt")
            inputs = {key: value.to(self.device) for key, value in inputs.items()}
            with torch.no_grad():
                outputs = loaded.model(**inputs)
                probs = torch.nn.functional.softmax(outputs.logits[0], dim=0)
            id_to_label = getattr(loaded.model.config, "id2label", {}) or {}
            predictions = []
            unsafe_score = 0.0
            for idx, confidence in enumerate(probs.detach().cpu().tolist()):
                label = str(id_to_label.get(idx, idx))
                score = float(confidence)
                predictions.append(
                    Prediction(label=label, confidence=score, metadata={"model": self.config.safety_model})
                )
                if _unsafe_label(label):
                    unsafe_score = max(unsafe_score, score)
            predictions.sort(key=lambda item: item.confidence or 0, reverse=True)
            return InferenceResponse(
                status="ok",
                endpoint="safety-nsfw",
                predictions=predictions,
                metadata={
                    "asset_id": request.asset_id,
                    "model": self.config.safety_model,
                    "unsafe_score": unsafe_score,
                    "threshold": self.config.safety_threshold,
                    "needs_review": unsafe_score >= self.config.safety_threshold,
                    "device": self.device,
                },
            )
        except MissingModelError as exc:
            return _model_missing("safety-nsfw", exc)
        except Exception as exc:
            return _error("safety-nsfw", exc)

    def describe_image(self, request: ImageRequest) -> InferenceResponse:
        try:
            image = load_pillow_image(request, self.config)
            loaded = self._load_caption()
            inputs = loaded.preprocess(images=image, return_tensors="pt")
            inputs = {key: value.to(self.device) for key, value in inputs.items()}
            output = loaded.model.generate(**inputs, max_new_tokens=32)
            caption = loaded.preprocess.decode(output[0], skip_special_tokens=True).strip()
            return InferenceResponse(
                status="ok",
                endpoint="describe-image",
                predictions=[
                    Prediction(label=caption, confidence=None, metadata={"model": self.config.caption_model})
                ],
                metadata={"asset_id": request.asset_id, "caption": caption, "device": self.device},
            )
        except MissingModelError as exc:
            return _model_missing("describe-image", exc)
        except Exception as exc:
            return _error("describe-image", exc)

    def embed_image(self, request: ImageRequest) -> InferenceResponse:
        try:
            image = load_pillow_image(request, self.config)
            loaded = self._load_openclip()
            torch = _import_torch()
            tensor = loaded.preprocess["image"](image).unsqueeze(0).to(self.device)
            with torch.no_grad():
                vector = loaded.model.encode_image(tensor)
                vector = vector / vector.norm(dim=-1, keepdim=True)
            embedding = [float(value) for value in vector[0].detach().cpu().tolist()]
            return InferenceResponse(
                status="ok",
                endpoint="embed-image",
                predictions=[],
                metadata={
                    "asset_id": request.asset_id,
                    "model": self.config.openclip_pretrained,
                    "dimension": len(embedding),
                    "embedding": embedding,
                    "device": self.device,
                },
            )
        except MissingModelError as exc:
            return _model_missing("embed-image", exc)
        except Exception as exc:
            return _error("embed-image", exc)

    def embed_text(self, request: TextRequest) -> InferenceResponse:
        try:
            if not request.text.strip():
                raise ValueError("text query is empty")
            loaded = self._load_openclip()
            torch = _import_torch()
            tokens = loaded.preprocess["tokenizer"]([request.text]).to(self.device)
            with torch.no_grad():
                vector = loaded.model.encode_text(tokens)
                vector = vector / vector.norm(dim=-1, keepdim=True)
            embedding = [float(value) for value in vector[0].detach().cpu().tolist()]
            return InferenceResponse(
                status="ok",
                endpoint="embed-text",
                predictions=[],
                metadata={
                    "text": request.text,
                    "model": self.config.openclip_pretrained,
                    "dimension": len(embedding),
                    "embedding": embedding,
                    "device": self.device,
                },
            )
        except MissingModelError as exc:
            return _model_missing("embed-text", exc)
        except Exception as exc:
            return _error("embed-text", exc)

    def _select_device(self) -> str:
        requested = self.config.device
        if requested in {"cpu", "cuda"}:
            return requested if requested != "cuda" or _cuda_available() else "cpu"
        try:
            torch = _import_torch()
            if torch.cuda.is_available():
                best_index = 0
                best_memory = 0
                for idx in range(torch.cuda.device_count()):
                    props = torch.cuda.get_device_properties(idx)
                    if props.total_memory > best_memory:
                        best_memory = props.total_memory
                        best_index = idx
                return f"cuda:{best_index}"
        except Exception:
            return "cpu"
        return "cpu"

    def _load_classification(self) -> LoadedModel:
        if self._classification is not None:
            return self._classification
        try:
            import torchvision.models as models  # type: ignore
        except Exception as exc:
            raise MissingModelError("torchvision is not installed") from exc
        try:
            if self.config.classifier == "mobilenet_v3_large":
                weights = models.MobileNet_V3_Large_Weights.DEFAULT
                model = models.mobilenet_v3_large(weights=weights)
                name = "mobilenet_v3_large"
            else:
                weights = models.EfficientNet_B0_Weights.DEFAULT
                model = models.efficientnet_b0(weights=weights)
                name = "efficientnet_b0"
        except Exception as exc:
            raise MissingModelError(f"torchvision classification weights unavailable: {exc}") from exc
        model.eval().to(self.device)
        self._classification = LoadedModel(
            name=name,
            model=model,
            preprocess=weights.transforms(),
            metadata={"categories": weights.meta.get("categories", [])},
        )
        return self._classification

    def _load_yunet(self, image_size: tuple[int, int]) -> Any:
        if self._yunet is not None:
            return self._yunet
        path = self.config.model_dir / "opencv" / "face_detection_yunet_2023mar.onnx"
        if not path.exists():
            raise MissingModelError(f"YuNet model missing at {path}")
        try:
            import cv2  # type: ignore
        except Exception as exc:
            raise MissingModelError("opencv-python-headless is not installed") from exc
        self._yunet = cv2.FaceDetectorYN_create(str(path), "", image_size, 0.6, 0.3, 5000)
        return self._yunet

    def _load_safety(self) -> LoadedModel:
        if self._safety is not None:
            return self._safety
        try:
            from transformers import AutoImageProcessor, AutoModelForImageClassification  # type: ignore
        except Exception as exc:
            raise MissingModelError("transformers is not installed") from exc
        try:
            model_path = _hf_snapshot_path(self.config.model_dir, self.config.safety_model)
            processor = AutoImageProcessor.from_pretrained(str(model_path), local_files_only=True)
            model = AutoModelForImageClassification.from_pretrained(str(model_path), local_files_only=True)
        except Exception as exc:
            raise MissingModelError(f"safety model unavailable in local cache: {exc}") from exc
        model.eval().to(self.device)
        self._safety = LoadedModel(name=self.config.safety_model, model=model, preprocess=processor)
        return self._safety

    def _load_caption(self) -> LoadedModel:
        if self._caption is not None:
            return self._caption
        try:
            from transformers import BlipForConditionalGeneration, BlipProcessor  # type: ignore
        except Exception as exc:
            raise MissingModelError("transformers BLIP classes are not installed") from exc
        try:
            model_path = _hf_snapshot_path(self.config.model_dir, self.config.caption_model)
            processor = BlipProcessor.from_pretrained(str(model_path), local_files_only=True)
            model = BlipForConditionalGeneration.from_pretrained(str(model_path), local_files_only=True)
        except Exception as exc:
            raise MissingModelError(f"caption model unavailable in local cache: {exc}") from exc
        model.eval().to(self.device)
        self._caption = LoadedModel(name=self.config.caption_model, model=model, preprocess=processor)
        return self._caption

    def _load_openclip(self) -> LoadedModel:
        if self._openclip is not None:
            return self._openclip
        try:
            import open_clip  # type: ignore
        except Exception as exc:
            raise MissingModelError("open-clip-torch is not installed") from exc
        try:
            snapshot = _openclip_snapshot_path(self.config.model_dir, "laion/CLIP-ViT-B-32-laion2B-s34B-b79K")
            weights = snapshot / "open_clip_model.safetensors"
            model, _, preprocess = open_clip.create_model_and_transforms(
                self.config.openclip_model,
                pretrained=str(weights),
                cache_dir=str(self.config.model_dir / "openclip"),
            )
        except Exception as exc:
            raise MissingModelError(f"OpenCLIP model unavailable in local cache: {exc}") from exc
        tokenizer = open_clip.get_tokenizer(self.config.openclip_model)
        model.eval().to(self.device)
        self._openclip = LoadedModel(
            name=self.config.openclip_pretrained,
            model=model,
            preprocess={"image": preprocess, "tokenizer": tokenizer},
        )
        return self._openclip


class MissingModelError(RuntimeError):
    pass


def _hf_snapshot_path(model_dir: Path, model_id: str) -> Path:
    repo = "models--" + model_id.replace("/", "--")
    base = model_dir / "huggingface" / repo / "snapshots"
    if not base.exists():
        raise MissingModelError(f"model snapshot directory missing: {base}")
    snapshots = sorted([path for path in base.iterdir() if path.is_dir()], key=lambda path: path.stat().st_mtime, reverse=True)
    for snapshot in snapshots:
        if (snapshot / "config.json").exists():
            return snapshot
    raise MissingModelError(f"no usable model snapshot found under {base}")


def _openclip_snapshot_path(model_dir: Path, model_id: str) -> Path:
    repo = "models--" + model_id.replace("/", "--")
    base = model_dir / "openclip" / repo / "snapshots"
    if not base.exists():
        raise MissingModelError(f"OpenCLIP snapshot directory missing: {base}")
    snapshots = sorted([path for path in base.iterdir() if path.is_dir()], key=lambda path: path.stat().st_mtime, reverse=True)
    for snapshot in snapshots:
        if (snapshot / "open_clip_model.safetensors").exists() and (snapshot / "open_clip_config.json").exists():
            return snapshot
    raise MissingModelError(f"no usable OpenCLIP snapshot found under {base}")


def _import_torch() -> Any:
    try:
        import torch  # type: ignore
    except Exception as exc:
        raise MissingModelError("torch is not installed") from exc
    return torch


def _cuda_available() -> bool:
    try:
        return bool(_import_torch().cuda.is_available())
    except Exception:
        return False


def _unsafe_label(label: str) -> bool:
    normalized = label.strip().lower()
    return any(part in normalized for part in ("unsafe", "nsfw", "porn", "sexy", "hentai"))


def _model_missing(endpoint: str, error: Exception) -> InferenceResponse:
    return InferenceResponse(status="model_missing", endpoint=endpoint, reason=str(error), metadata={})


def _error(endpoint: str, error: Exception) -> InferenceResponse:
    return InferenceResponse(status="error", endpoint=endpoint, reason=str(error), metadata={})

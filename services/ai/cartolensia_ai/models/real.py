"""Lazy local inference backend for the optional AI sidecar."""

from __future__ import annotations

import csv
from contextlib import contextmanager
from dataclasses import dataclass
import importlib.util
import math
import os
from pathlib import Path
import shutil
import struct
import subprocess
import sys
import tempfile
import threading
from typing import Any

from cartolensia_ai.config import ServiceConfig
from cartolensia_ai.image_io import load_pillow_image, materialize_media_input, read_image_bytes
from cartolensia_ai.schemas import ImageRequest, InferenceResponse, MediaRequest, Prediction, TextRequest


CAPABILITIES = [
    "classify_image",
    "detect_faces",
    "safety_nsfw",
    "describe_image",
    "embed_image",
    "embed_text",
    "ocr_image",
    "transcribe_audio",
    "analyze_audio",
    "music_midi",
    "music_stems",
]

DEFAULT_OCR_LANGUAGES = ["eng", "rus", "hye", "chi_sim", "chi_tra"]
OCR_LANGUAGE_ALIASES = {
    "english": "eng",
    "en": "eng",
    "eng": "eng",
    "russian": "rus",
    "ru": "rus",
    "rus": "rus",
    "armenian": "hye",
    "hy": "hye",
    "hye": "hye",
    "chinese": "chi_sim",
    "zh": "chi_sim",
    "zh-cn": "chi_sim",
    "zh-hans": "chi_sim",
    "simplified_chinese": "chi_sim",
    "chi_sim": "chi_sim",
    "zh-tw": "chi_tra",
    "zh-hant": "chi_tra",
    "traditional_chinese": "chi_tra",
    "chi_tra": "chi_tra",
}

ASR_LANGUAGE_ALIASES = {
    "auto": "",
    "": "",
    "english": "en",
    "en": "en",
    "eng": "en",
    "russian": "ru",
    "ru": "ru",
    "rus": "ru",
    "armenian": "hy",
    "hy": "hy",
    "hye": "hy",
    "chinese": "zh",
    "zh": "zh",
    "zh-cn": "zh",
    "zh-tw": "zh",
    "chi_sim": "zh",
    "chi_tra": "zh",
}


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
        self._asr: LoadedModel | None = None
        # Most local inference libraries used here keep native process-global
        # state. In particular, OpenCV DNN, PyTorch CUDA, Transformers, and
        # CTranslate2 can crash the interpreter when models are loaded or used
        # concurrently without coordination. Keep the queue dense in the Go
        # worker, but gate native inference inside this process.
        self._model_lock = threading.RLock()
        self._torch_slots = threading.BoundedSemaphore(_env_int("CARTOLENSIA_AI_TORCH_PARALLELISM", 1, 1, 4))
        self._opencv_lock = threading.Lock()
        self._asr_slots = threading.BoundedSemaphore(_env_int("CARTOLENSIA_AI_ASR_PARALLELISM", 1, 1, 2))
        self._music_slots = threading.BoundedSemaphore(_env_int("CARTOLENSIA_AI_MUSIC_PARALLELISM", 1, 1, 2))

    def _configure_cache_environment(self) -> None:
        self.config.model_dir.mkdir(parents=True, exist_ok=True)
        self.config.cache_dir.mkdir(parents=True, exist_ok=True)
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
            "cache_dir": str(self.config.cache_dir),
            "models": self.models_status(),
            "capabilities": CAPABILITIES,
            "concurrency": {
                "torch_parallelism": _env_int("CARTOLENSIA_AI_TORCH_PARALLELISM", 1, 1, 4),
                "asr_parallelism": _env_int("CARTOLENSIA_AI_ASR_PARALLELISM", 1, 1, 2),
                "music_parallelism": _env_int("CARTOLENSIA_AI_MUSIC_PARALLELISM", 1, 1, 2),
                "opencv_parallelism": 1,
            },
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
            "ocr": _tesseract_status(),
            "asr": _asr_status(self.config.model_dir, self._asr is not None),
            "audio_analysis": _audio_analysis_status(),
            "music_midi": _music_midi_status(self.config.cache_dir),
            "music_stems": _music_stems_status(self.config.cache_dir),
        }

    def classify_image(self, request: ImageRequest) -> InferenceResponse:
        try:
            image = load_pillow_image(request, self.config)
            loaded = self._load_classification()
            torch = _import_torch()
            tensor = loaded.preprocess(image).unsqueeze(0).to(self.device)
            with self._torch_inference(), torch.no_grad():
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
            with self._opencv_lock:
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
            with self._torch_inference(), torch.no_grad():
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
            with self._torch_inference():
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
            with self._torch_inference(), torch.no_grad():
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
            with self._torch_inference(), torch.no_grad():
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

    def ocr_image(self, request: ImageRequest) -> InferenceResponse:
        try:
            tesseract = shutil.which("tesseract")
            if not tesseract:
                raise MissingModelError("tesseract executable is not installed")
            available_languages = _available_tesseract_languages(tesseract)
            requested_languages = _requested_ocr_languages(request.options)
            missing_languages = [lang for lang in requested_languages if lang not in available_languages]
            if missing_languages:
                packages = [f"tesseract-ocr-{lang.replace('_', '-')}" for lang in missing_languages]
                raise MissingModelError(
                    "missing Tesseract language data: "
                    + ", ".join(missing_languages)
                    + "; install "
                    + ", ".join(packages)
                )

            image_bytes = read_image_bytes(request, self.config)
            timeout = _int_option(request.options, "timeout_seconds", 45)
            psm = str(request.options.get("psm", "3") if request.options else "3")
            min_confidence = _float_option(request.options, "min_confidence", 0.35)
            with tempfile.NamedTemporaryFile(prefix="cartolensia-ocr-", suffix=".img", delete=True) as handle:
                handle.write(image_bytes)
                handle.flush()
                command = [
                    tesseract,
                    handle.name,
                    "stdout",
                    "-l",
                    "+".join(requested_languages),
                    "--psm",
                    psm,
                    "tsv",
                ]
                proc = subprocess.run(
                    command,
                    check=False,
                    capture_output=True,
                    text=True,
                    timeout=max(5, min(timeout, 180)),
                )
            if proc.returncode != 0:
                reason = (proc.stderr or proc.stdout or "tesseract returned a non-zero status").strip()
                return InferenceResponse(
                    status="error",
                    endpoint="ocr-image",
                    reason=reason[-2000:],
                    metadata={
                        "asset_id": request.asset_id,
                        "engine": "tesseract",
                        "available_languages": sorted(available_languages),
                        "requested_languages": requested_languages,
                    },
                )
            blocks = _parse_tesseract_tsv(proc.stdout, requested_languages, min_confidence)
            full_text = "\n".join(block["text"] for block in blocks if block.get("text")).strip()
            return InferenceResponse(
                status="ok",
                endpoint="ocr-image",
                predictions=[],
                metadata={
                    "asset_id": request.asset_id,
                    "engine": "tesseract",
                    "model": "tesseract",
                    "languages": requested_languages,
                    "available_languages": sorted(available_languages),
                    "min_confidence": min_confidence,
                    "blocks": blocks,
                    "text": full_text,
                    "block_count": len(blocks),
                },
            )
        except MissingModelError as exc:
            return InferenceResponse(
                status="model_missing",
                endpoint="ocr-image",
                reason=str(exc),
                metadata={"engine": "tesseract", **_tesseract_status()},
            )
        except subprocess.TimeoutExpired as exc:
            return InferenceResponse(
                status="error",
                endpoint="ocr-image",
                reason=f"tesseract timed out after {exc.timeout} seconds",
                metadata={"engine": "tesseract"},
            )
        except Exception as exc:
            return _error("ocr-image", exc)

    def transcribe_audio(self, request: MediaRequest) -> InferenceResponse:
        temp_path: Path | None = None
        should_delete = False
        try:
            model_name = str((request.options or {}).get("model") or "small").strip() or "small"
            if model_name not in {"tiny", "base", "small", "medium"} and not Path(model_name).exists():
                raise MissingModelError("unsupported ASR model; allowed values: tiny, base, small, medium, or a safe local model path")
            language = _requested_asr_language(request.options)
            timeout = _int_option(request.options, "timeout_seconds", 0)
            suffix = str((request.options or {}).get("suffix") or ".media")
            temp_path, should_delete = materialize_media_input(request, self.config, suffix=suffix)
            loaded = self._load_asr(model_name)
            whisper_model = loaded.model
            beam_size = _int_option(request.options, "beam_size", 5)
            vad_filter = _bool_option(request.options, "vad_filter", True)
            kwargs: dict[str, Any] = {
                "beam_size": max(1, min(beam_size, 10)),
                "vad_filter": vad_filter,
            }
            if language:
                kwargs["language"] = language
            with self._asr_slots:
                segments_iter, info = whisper_model.transcribe(str(temp_path), **kwargs)
                segments: list[dict[str, Any]] = []
                pieces: list[str] = []
                for index, segment in enumerate(segments_iter):
                    text = str(getattr(segment, "text", "") or "").strip()
                    if not text:
                        continue
                    start = float(getattr(segment, "start", 0.0) or 0.0)
                    end = float(getattr(segment, "end", start) or start)
                    no_speech = getattr(segment, "no_speech_prob", None)
                    avg_logprob = getattr(segment, "avg_logprob", None)
                    confidence = _segment_confidence(no_speech, avg_logprob)
                    item = {
                        "index": index,
                        "start_ms": int(max(0, round(start * 1000))),
                        "end_ms": int(max(0, round(end * 1000))),
                        "start_seconds": start,
                        "end_seconds": end,
                        "text": text,
                        "confidence": confidence,
                        "avg_logprob": avg_logprob,
                        "no_speech_prob": no_speech,
                    }
                    segments.append(item)
                    pieces.append(text)
                full_text = "\n".join(pieces).strip()
                detected_language = str(getattr(info, "language", "") or language or "")
                language_probability = getattr(info, "language_probability", None)
                duration = getattr(info, "duration", None)
            return InferenceResponse(
                status="ok",
                endpoint="transcribe-audio",
                predictions=[],
                metadata={
                    "asset_id": request.asset_id,
                    "engine": "faster-whisper",
                    "provider": "faster-whisper",
                    "model": loaded.name,
                    "device": loaded.metadata.get("device") if loaded.metadata else "cpu",
                    "compute_type": loaded.metadata.get("compute_type") if loaded.metadata else "",
                    "language": detected_language,
                    "language_probability": language_probability,
                    "duration_seconds": duration,
                    "segments": segments,
                    "segment_count": len(segments),
                    "text": full_text,
                    "timeout_seconds": timeout,
                    "vad_filter": vad_filter,
                },
            )
        except MissingModelError as exc:
            return InferenceResponse(
                status="model_missing",
                endpoint="transcribe-audio",
                reason=str(exc),
                metadata={"engine": "faster-whisper", **_asr_status(self.config.model_dir, self._asr is not None)},
            )
        except Exception as exc:
            return _error("transcribe-audio", exc)
        finally:
            if should_delete and temp_path is not None:
                try:
                    temp_path.unlink(missing_ok=True)
                except Exception:
                    pass

    def analyze_audio(self, request: MediaRequest) -> InferenceResponse:
        temp_path: Path | None = None
        should_delete = False
        try:
            librosa = _import_optional("librosa")
            np = _import_optional("numpy")
            suffix = str((request.options or {}).get("suffix") or ".media")
            temp_path, should_delete = materialize_media_input(request, self.config, suffix=suffix)
            max_seconds = _float_option_with_env(
                request.options,
                "max_analysis_seconds",
                "CARTOLENSIA_AI_AUDIO_ANALYSIS_SECONDS",
                180.0,
            )
            target_sr = int(_float_option_with_env(
                request.options,
                "analysis_sample_rate_hz",
                "CARTOLENSIA_AI_AUDIO_ANALYSIS_SAMPLE_RATE",
                22050.0,
            ))
            target_sr = max(8000, min(target_sr, 48000))
            max_seconds = max(10.0, min(max_seconds, 30.0 * 60.0))
            source_duration = None
            try:
                source_duration = float(librosa.get_duration(path=str(temp_path)))
            except Exception:
                source_duration = None
            y, sr = librosa.load(str(temp_path), sr=target_sr, mono=True, duration=max_seconds)
            if y is None or len(y) == 0:
                raise ValueError("audio decode produced no samples")
            duration = float(librosa.get_duration(y=y, sr=sr))
            reported_duration = source_duration if source_duration and source_duration > 0 else duration
            tempo_values = librosa.beat.tempo(y=y, sr=sr, aggregate=None)
            tempo = _median_float(tempo_values, np)
            rms = librosa.feature.rms(y=y)
            rms_mean = _mean_float(rms, np)
            loudness = 20.0 * math.log10(max(rms_mean, 1e-9))
            zcr = _mean_float(librosa.feature.zero_crossing_rate(y), np)
            flatness = _mean_float(librosa.feature.spectral_flatness(y=y), np)
            centroid = _mean_float(librosa.feature.spectral_centroid(y=y, sr=sr), np)
            bandwidth = _mean_float(librosa.feature.spectral_bandwidth(y=y, sr=sr), np)
            chroma = librosa.feature.chroma_stft(y=y, sr=sr)
            key, mode, key_confidence = _estimate_key(chroma, np)
            speech_music_ratio = max(0.0, min(1.0, (flatness * 2.5) + (zcr * 3.0)))
            genre_labels = _audio_feature_labels(tempo, speech_music_ratio, loudness)
            return InferenceResponse(
                status="ok",
                endpoint="analyze-audio",
                predictions=[
                    Prediction(label=label, confidence=None, metadata={"source": "local_audio_features"})
                    for label in genre_labels
                ],
                metadata={
                    "asset_id": request.asset_id,
                    "engine": "librosa",
                    "model": "librosa_audio_features",
                    "duration_seconds": reported_duration,
                    "analysis_duration_seconds": duration,
                    "analysis_limited": bool(source_duration and source_duration > duration + 1.0),
                    "sample_rate_hz": int(sr),
                    "analysis_sample_rate_hz": int(sr),
                    "tempo_bpm": tempo,
                    "key": key,
                    "mode": mode,
                    "key_confidence": key_confidence,
                    "loudness": loudness,
                    "speech_music_ratio": speech_music_ratio,
                    "genre_labels": genre_labels,
                    "genre_status": "heuristic_labels_model_missing",
                    "spectral": {
                        "rms_mean": rms_mean,
                        "zero_crossing_rate": zcr,
                        "spectral_flatness": flatness,
                        "spectral_centroid_hz": centroid,
                        "spectral_bandwidth_hz": bandwidth,
                    },
                },
            )
        except MissingModelError as exc:
            return InferenceResponse(
                status="model_missing",
                endpoint="analyze-audio",
                reason=str(exc),
                metadata={"engine": "librosa", **_audio_analysis_status()},
            )
        except Exception as exc:
            return _error("analyze-audio", exc)
        finally:
            if should_delete and temp_path is not None:
                try:
                    temp_path.unlink(missing_ok=True)
                except Exception:
                    pass

    def music_to_midi(self, request: MediaRequest) -> InferenceResponse:
        temp_path: Path | None = None
        should_delete = False
        try:
            package_available = importlib.util.find_spec("basic_pitch") is not None
            cli_executable = _basic_pitch_executable(self.config.cache_dir)
            if not package_available and cli_executable is None:
                raise MissingModelError(
                    "basic-pitch is not installed in the Cartolensia AI environment or as an isolated component CLI"
                )
            predict_and_save = None
            if package_available:
                from basic_pitch.inference import predict_and_save as loaded_predict_and_save  # type: ignore

                predict_and_save = loaded_predict_and_save

            suffix = str((request.options or {}).get("suffix") or ".media")
            temp_path, should_delete = materialize_media_input(request, self.config, suffix=suffix)
            output_dir = _music_output_dir(self.config.cache_dir, "music-midi", request.asset_id)
            output_dir.mkdir(parents=True, exist_ok=True)
            with self._music_slots:
                if predict_and_save is not None:
                    try:
                        predict_and_save(
                            [str(temp_path)],
                            str(output_dir),
                            save_midi=True,
                            sonify_midi=False,
                            save_model_outputs=False,
                            save_notes=False,
                        )
                    except TypeError:
                        predict_and_save([str(temp_path)], str(output_dir), True, False, False, False)
                elif cli_executable is not None:
                    _run_basic_pitch_cli(cli_executable, temp_path, output_dir, _int_option(request.options, "timeout_seconds", 7200))
            midi_files = sorted(output_dir.rglob("*.mid")) + sorted(output_dir.rglob("*.midi"))
            if not midi_files:
                raise RuntimeError("basic-pitch completed but did not produce a MIDI file")
            midi_path = midi_files[0]
            summary = _midi_summary(midi_path)
            instruments = summary.get("instruments") or ["pitch_contours"]
            note_count = int(summary.get("note_count") or 0)
            return InferenceResponse(
                status="ok",
                endpoint="music-to-midi",
                predictions=[
                    Prediction(label="music:midi_available", confidence=None, metadata={"provider": "basic_pitch", "note_count": note_count}),
                    *[
                        Prediction(label=f"music:instrument:{instrument}", confidence=None, metadata={"provider": "basic_pitch"})
                        for instrument in instruments
                    ],
                ],
                metadata={
                    "asset_id": request.asset_id,
                    "provider": "basic_pitch",
                    "engine": "basic_pitch",
                    "model": "basic_pitch",
                    "runtime": "python_api" if package_available else "isolated_cli",
                    "executable": str(cli_executable) if cli_executable else None,
                    "status": "succeeded",
                    "cache_path": str(midi_path),
                    "output_dir": str(output_dir),
                    "note_count": note_count,
                    "instrument_count": int(summary.get("instrument_count") or len(instruments)),
                    "instruments": instruments,
                    "duration_seconds": summary.get("duration_seconds"),
                    "summary": summary.get("summary") or f"Basic Pitch produced {note_count} MIDI note events.",
                    "safe_note": "MIDI output is stored under the Cartolensia cache directory only; originals are never modified.",
                },
            )
        except MissingModelError as exc:
            return InferenceResponse(
                status="model_missing",
                endpoint="music-to-midi",
                reason=str(exc),
                metadata={"provider": "basic_pitch", **_music_midi_status(self.config.cache_dir)},
            )
        except Exception as exc:
            return _error("music-to-midi", exc)
        finally:
            if should_delete and temp_path is not None:
                try:
                    temp_path.unlink(missing_ok=True)
                except Exception:
                    pass

    def separate_music(self, request: MediaRequest) -> InferenceResponse:
        temp_path: Path | None = None
        should_delete = False
        try:
            module_available = importlib.util.find_spec("demucs") is not None
            executable = _demucs_executable(self.config.cache_dir)
            if not module_available and not executable:
                raise MissingModelError(
                    "demucs is not installed in the Cartolensia AI environment or as an isolated component CLI"
                )
            model = str((request.options or {}).get("model") or os.environ.get("CARTOLENSIA_MUSIC_STEMS_MODEL") or "htdemucs").strip()
            stem_set = str((request.options or {}).get("stem_set") or "vocals_drums_bass_other").strip() or "vocals_drums_bass_other"
            output_format = str(
                (request.options or {}).get("format")
                or (request.options or {}).get("output_format")
                or os.environ.get("CARTOLENSIA_MUSIC_STEMS_FORMAT")
                or "flac"
            ).strip().lower()
            if output_format not in {"flac", "mp3", "wav"}:
                output_format = "flac"
            try:
                default_mp3_bitrate = int(os.environ.get("CARTOLENSIA_MUSIC_STEMS_MP3_BITRATE", "192") or "192")
            except ValueError:
                default_mp3_bitrate = 192
            mp3_bitrate = _int_option(request.options, "mp3_bitrate", default_mp3_bitrate)
            mp3_bitrate = max(96, min(mp3_bitrate, 320))
            device = str((request.options or {}).get("device") or os.environ.get("CARTOLENSIA_MUSIC_STEMS_DEVICE") or "").strip()
            timeout = _int_option(request.options, "timeout_seconds", 7200)
            timeout = max(60, min(timeout, 24 * 60 * 60))
            suffix = str((request.options or {}).get("suffix") or ".media")
            temp_path, should_delete = materialize_media_input(request, self.config, suffix=suffix)
            output_dir = _music_output_dir(self.config.cache_dir, "music-stems", request.asset_id) / _safe_token(model) / _safe_token(output_format)
            output_dir.mkdir(parents=True, exist_ok=True)
            if module_available:
                command = [sys.executable, "-m", "demucs.separate", "-n", model, "-o", str(output_dir)]
                isolated_cli = False
            else:
                command = [str(executable), "-n", model, "-o", str(output_dir)]
                isolated_cli = True
            if device:
                command.extend(["-d", device])
            if output_format == "flac":
                command.append("--flac")
            elif output_format == "mp3":
                command.extend(["--mp3", "--mp3-bitrate", str(mp3_bitrate)])
            command.append(str(temp_path))
            env = _isolated_python_cli_env() if isolated_cli else dict(os.environ)
            env.setdefault("TF_CPP_MIN_LOG_LEVEL", "2")
            with self._music_slots:
                proc = subprocess.run(command, check=False, capture_output=True, text=True, timeout=timeout, env=env)
            if proc.returncode != 0:
                reason = (proc.stderr or proc.stdout or f"demucs exited with {proc.returncode}").strip()
                return InferenceResponse(
                    status="error",
                    endpoint="separate-music",
                    reason=reason[-2000:],
                    metadata={"provider": "demucs", "model": model, "output_dir": str(output_dir)},
                )
            stems = _collect_stems(output_dir)
            if not stems:
                raise RuntimeError("demucs completed but did not produce stem audio files")
            return InferenceResponse(
                status="ok",
                endpoint="separate-music",
                predictions=[
                    Prediction(label="music:stems_available", confidence=None, metadata={"provider": "demucs", "stem_count": len(stems)}),
                    *[
                        Prediction(label=f"music:stem:{stem['name']}", confidence=None, metadata={"provider": "demucs", "model": model})
                        for stem in stems
                    ],
                ],
                metadata={
                    "asset_id": request.asset_id,
                    "provider": "demucs",
                    "engine": "demucs",
                    "model": model,
                    "runtime": "python_module" if module_available else "isolated_cli",
                    "executable": str(executable) if executable else None,
                    "status": "succeeded",
                    "stem_set": stem_set,
                    "output_format": output_format,
                    "compressed": output_format != "wav",
                    "mp3_bitrate": mp3_bitrate if output_format == "mp3" else None,
                    "output_dir": str(output_dir),
                    "stems": stems,
                    "stem_count": len(stems),
                    "supported_stems": ["vocals", "drums", "bass", "other"],
                    "instrument_stem_note": "The installed Demucs provider separates vocals/drums/bass/other. Piano, strings, brass, reeds, and mallet instruments are exposed through MIDI transcription when available; dedicated multi-instrument stem models can be added later as reviewed components.",
                    "safe_note": "Stem audio is stored under the Cartolensia cache directory only; originals are never modified.",
                },
            )
        except MissingModelError as exc:
            return InferenceResponse(
                status="model_missing",
                endpoint="separate-music",
                reason=str(exc),
                metadata={"provider": "demucs", **_music_stems_status(self.config.cache_dir)},
            )
        except subprocess.TimeoutExpired as exc:
            return InferenceResponse(
                status="error",
                endpoint="separate-music",
                reason=f"demucs timed out after {exc.timeout} seconds",
                metadata={"provider": "demucs"},
            )
        except Exception as exc:
            return _error("separate-music", exc)
        finally:
            if should_delete and temp_path is not None:
                try:
                    temp_path.unlink(missing_ok=True)
                except Exception:
                    pass

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

    @contextmanager
    def _torch_inference(self) -> Any:
        self._torch_slots.acquire()
        try:
            yield
        finally:
            self._torch_slots.release()

    def _load_classification(self) -> LoadedModel:
        if self._classification is not None:
            return self._classification
        with self._model_lock:
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
        with self._model_lock:
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
        with self._model_lock:
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
        with self._model_lock:
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
        with self._model_lock:
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

    def _load_asr(self, model_name: str) -> LoadedModel:
        if self._asr is not None and self._asr.name == model_name:
            return self._asr
        with self._model_lock:
            if self._asr is not None and self._asr.name == model_name:
                return self._asr
            try:
                from faster_whisper import WhisperModel  # type: ignore
            except Exception as exc:
                raise MissingModelError("faster-whisper is not installed in .cartolensia/ai-venv") from exc
            model_root = self.config.model_dir / "faster-whisper"
            model_root.mkdir(parents=True, exist_ok=True)
            device, device_index = _asr_device(self.device)
            compute_type = "float16" if device == "cuda" else "int8"
            local_only = bool(os.environ.get("CARTOLENSIA_ASR_LOCAL_ONLY") == "1")
            try:
                model = WhisperModel(
                    model_name,
                    device=device,
                    device_index=device_index,
                    compute_type=compute_type,
                    download_root=str(model_root),
                    local_files_only=local_only,
                )
            except Exception as exc:
                raise MissingModelError(f"faster-whisper model {model_name!r} is unavailable: {exc}") from exc
            self._asr = LoadedModel(
                name=model_name,
                model=model,
                metadata={"device": device, "device_index": device_index, "compute_type": compute_type, "download_root": str(model_root)},
            )
            return self._asr


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


def _tesseract_status() -> dict[str, Any]:
    path = shutil.which("tesseract")
    if not path:
        return {
            "name": "tesseract",
            "available": False,
            "loaded": False,
            "available_languages": [],
            "required_languages": DEFAULT_OCR_LANGUAGES,
            "missing_languages": DEFAULT_OCR_LANGUAGES,
        }
    languages = _available_tesseract_languages(path)
    missing = [lang for lang in DEFAULT_OCR_LANGUAGES if lang not in languages]
    return {
        "name": "tesseract",
        "available": len(missing) == 0,
        "loaded": True,
        "path": path,
        "available_languages": sorted(languages),
        "required_languages": DEFAULT_OCR_LANGUAGES,
        "missing_languages": missing,
    }


def _asr_status(model_dir: Path, loaded: bool) -> dict[str, Any]:
    package_available = importlib.util.find_spec("faster_whisper") is not None
    ctranslate_available = importlib.util.find_spec("ctranslate2") is not None
    model_root = model_dir / "faster-whisper"
    model_entries = []
    if model_root.exists():
        model_entries = sorted(path.name for path in model_root.iterdir() if path.is_dir())
    return {
        "name": "faster-whisper",
        "available": package_available and ctranslate_available,
        "loaded": loaded,
        "package_available": package_available,
        "ctranslate2_available": ctranslate_available,
        "download_root": str(model_root),
        "models_present": model_entries,
        "supported_languages": ["auto", "en", "ru", "hy", "zh"],
        "default_model": "small",
    }


def _audio_analysis_status() -> dict[str, Any]:
    return {
        "name": "librosa",
        "available": importlib.util.find_spec("librosa") is not None and importlib.util.find_spec("soundfile") is not None,
        "librosa_available": importlib.util.find_spec("librosa") is not None,
        "soundfile_available": importlib.util.find_spec("soundfile") is not None,
        "features": ["tempo_bpm", "key", "mode", "loudness", "speech_music_ratio", "spectral_summary"],
        "genre": "heuristic_only_model_missing",
    }


def _music_midi_status(cache_dir: Path) -> dict[str, Any]:
    package_available = importlib.util.find_spec("basic_pitch") is not None
    executable = _basic_pitch_executable(cache_dir)
    return {
        "name": "basic_pitch",
        "available": package_available or executable is not None,
        "package_available": package_available,
        "cli_available": executable is not None,
        "executable": str(executable) if executable else None,
        "provider": "basic_pitch",
        "cache_dir": str(cache_dir / "music-midi"),
        "outputs": ["midi"],
        "default_for_batch": True,
    }


def _component_roots(cache_dir: Path) -> list[Path]:
    roots: list[Path] = []
    env_root = os.environ.get("CARTOLENSIA_COMPONENT_DIR", "").strip()
    if env_root:
        roots.append(Path(env_root))
    roots.append(cache_dir.parent / "components")
    roots.append(Path(".cartolensia") / "components")
    unique: list[Path] = []
    for root in roots:
        root = root.expanduser()
        if root not in unique:
            unique.append(root)
    return unique


def _basic_pitch_executable(cache_dir: Path) -> Path | None:
    candidates: list[Path] = []
    env_bin = os.environ.get("CARTOLENSIA_BASIC_PITCH_BIN", "").strip()
    if env_bin:
        candidates.append(Path(env_bin))
    for root in _component_roots(cache_dir):
        component_root = root / "music-basic-pitch"
        candidates.extend(
            [
                component_root / "venv" / "bin" / "basic-pitch",
                component_root / "bin" / "basic-pitch",
                component_root / "basic-pitch",
            ]
        )
    which = shutil.which("basic-pitch")
    if which:
        candidates.append(Path(which))
    for candidate in candidates:
        try:
            if candidate.exists() and candidate.is_file():
                return candidate
        except OSError:
            continue
    return None


def _demucs_executable(cache_dir: Path) -> Path | None:
    candidates: list[Path] = []
    env_bin = os.environ.get("CARTOLENSIA_DEMUCS_BIN", "").strip()
    if env_bin:
        candidates.append(Path(env_bin))
    for root in _component_roots(cache_dir):
        component_root = root / "music-demucs"
        candidates.extend(
            [
                component_root / "venv" / "bin" / "demucs",
                component_root / "bin" / "demucs",
                component_root / "demucs",
            ]
        )
    which = shutil.which("demucs")
    if which:
        candidates.append(Path(which))
    for candidate in candidates:
        try:
            if candidate.exists() and candidate.is_file():
                return candidate
        except OSError:
            continue
    return None


def _run_basic_pitch_cli(executable: Path, input_path: Path, output_dir: Path, timeout_seconds: int) -> None:
    timeout_seconds = max(60, min(timeout_seconds, 24 * 60 * 60))
    env = _isolated_python_cli_env()
    command = [str(executable), str(output_dir), str(input_path), "--save-midi"]
    proc = subprocess.run(command, check=False, capture_output=True, text=True, timeout=timeout_seconds, env=env)
    if proc.returncode == 0:
        return
    stderr = (proc.stderr or proc.stdout or "").strip()
    # Older CLI releases differ in option names. Retry the documented minimal
    # invocation before surfacing the failure.
    if "no such option" in stderr.lower() or "unrecognized" in stderr.lower():
        fallback = [str(executable), str(output_dir), str(input_path)]
        proc = subprocess.run(fallback, check=False, capture_output=True, text=True, timeout=timeout_seconds, env=env)
        if proc.returncode == 0:
            return
        stderr = (proc.stderr or proc.stdout or "").strip()
    if len(stderr) > 4000:
        stderr = stderr[-4000:]
    raise RuntimeError(f"basic-pitch CLI failed with exit code {proc.returncode}: {stderr}")


def _isolated_python_cli_env() -> dict[str, str]:
    env = dict(os.environ)
    env.pop("PYTHONHOME", None)
    env.pop("PYTHONPATH", None)
    env["PYTHONNOUSERSITE"] = "1"
    env.setdefault("TF_CPP_MIN_LOG_LEVEL", "2")
    return env


def _music_stems_status(cache_dir: Path) -> dict[str, Any]:
    package_available = importlib.util.find_spec("demucs") is not None
    executable = _demucs_executable(cache_dir)
    return {
        "name": "demucs",
        "available": package_available or executable is not None,
        "package_available": package_available,
        "executable": str(executable) if executable else "",
        "provider": "demucs",
        "cache_dir": str(cache_dir / "music-stems"),
        "outputs": ["vocals", "drums", "bass", "other"],
        "default_output_format": os.environ.get("CARTOLENSIA_MUSIC_STEMS_FORMAT", "flac"),
        "compressed_outputs": True,
        "instrument_stem_note": "Demucs does not provide reliable piano/strings/brass/reed/glockenspiel stems in this profile; use MIDI transcription for instrument-level notes or install a future reviewed multi-instrument provider.",
        "default_for_batch": False,
        "on_demand": True,
    }


def _music_output_dir(cache_dir: Path, kind: str, asset_id: str) -> Path:
    return cache_dir / kind / _safe_token(asset_id or "anonymous")


def _safe_token(value: str) -> str:
    cleaned = "".join(ch if ch.isalnum() or ch in {"-", "_", "."} else "_" for ch in value.strip())
    cleaned = cleaned.strip("._")
    return cleaned[:160] or "item"


def _midi_summary(path: Path) -> dict[str, Any]:
    try:
        pretty_midi = _import_optional("pretty_midi")
        midi = pretty_midi.PrettyMIDI(str(path))
        instruments = [instrument.name or f"program_{instrument.program}" for instrument in midi.instruments]
        note_count = sum(len(instrument.notes) for instrument in midi.instruments)
        duration = float(midi.get_end_time())
        names = sorted(set(instruments)) or ["pitch_contours"]
        return {
            "note_count": note_count,
            "instrument_count": len(names),
            "instruments": names,
            "duration_seconds": duration,
            "summary": f"{note_count} MIDI notes across {len(names)} instrument/program track(s).",
        }
    except Exception:
        pass
    try:
        mido = _import_optional("mido")
        midi = mido.MidiFile(str(path))
        note_count = 0
        channels: set[str] = set()
        programs: set[str] = set()
        for track in midi.tracks:
            for message in track:
                channel = getattr(message, "channel", None)
                if channel is not None:
                    channels.add(f"channel_{int(channel) + 1}")
                if getattr(message, "type", "") == "program_change":
                    programs.add(f"program_{getattr(message, 'program', 0)}")
                if getattr(message, "type", "") == "note_on" and int(getattr(message, "velocity", 0) or 0) > 0:
                    note_count += 1
        instruments = sorted(programs or channels or {"pitch_contours"})
        return {
            "note_count": note_count,
            "instrument_count": len(instruments),
            "instruments": instruments,
            "duration_seconds": None,
            "summary": f"{note_count} MIDI note-on events across {len(instruments)} channel/program group(s).",
        }
    except Exception:
        return _midi_summary_builtin(path)


def _midi_summary_builtin(path: Path) -> dict[str, Any]:
    """Read basic Standard MIDI File statistics without optional packages."""

    try:
        data = path.read_bytes()
        if len(data) < 14 or data[:4] != b"MThd":
            raise ValueError("not a Standard MIDI File")
        header_len = int.from_bytes(data[4:8], "big")
        if len(data) < 8 + header_len or header_len < 6:
            raise ValueError("truncated MIDI header")
        _, track_count, division = struct.unpack(">HHH", data[8:14])
        offset = 8 + header_len
        note_count = 0
        max_ticks = 0
        tempo_us_per_quarter = 500_000
        programs: set[str] = set()
        channels: set[str] = set()
        for _ in range(track_count):
            if offset + 8 > len(data) or data[offset : offset + 4] != b"MTrk":
                break
            length = int.from_bytes(data[offset + 4 : offset + 8], "big")
            pos = offset + 8
            end = min(pos + length, len(data))
            offset = end
            tick = 0
            running_status: int | None = None
            while pos < end:
                delta, pos = _midi_read_vlq(data, pos, end)
                tick += delta
                if pos >= end:
                    break
                first = data[pos]
                if first < 0x80:
                    if running_status is None:
                        break
                    status = running_status
                    data_start = pos
                else:
                    status = first
                    pos += 1
                    data_start = pos
                    if status < 0xF0:
                        running_status = status
                    else:
                        running_status = None

                if status == 0xFF:
                    if pos >= end:
                        break
                    meta_type = data[pos]
                    pos += 1
                    size, pos = _midi_read_vlq(data, pos, end)
                    payload = data[pos : min(pos + size, end)]
                    if meta_type == 0x51 and len(payload) == 3:
                        tempo_us_per_quarter = int.from_bytes(payload, "big")
                    pos = min(pos + size, end)
                    continue
                if status in {0xF0, 0xF7}:
                    size, pos = _midi_read_vlq(data, pos, end)
                    pos = min(pos + size, end)
                    continue
                if status >= 0xF0:
                    continue

                event = status & 0xF0
                channel = status & 0x0F
                size = 1 if event in {0xC0, 0xD0} else 2
                if data_start + size > end:
                    break
                values = data[data_start : data_start + size]
                pos = data_start + size
                channels.add(f"channel_{channel + 1}")
                if event == 0xC0:
                    programs.add(f"program_{int(values[0])}")
                elif event == 0x90 and size == 2 and int(values[1]) > 0:
                    note_count += 1
            max_ticks = max(max_ticks, tick)
        instruments = sorted(programs or channels or {"pitch_contours"})
        duration = None
        if division and division < 0x8000:
            duration = float(max_ticks * tempo_us_per_quarter / division / 1_000_000)
        return {
            "note_count": note_count,
            "instrument_count": len(instruments),
            "instruments": instruments,
            "duration_seconds": duration,
            "summary": f"{note_count} MIDI note-on events across {len(instruments)} channel/program group(s).",
        }
    except Exception:
        return {
            "note_count": 0,
            "instrument_count": 1,
            "instruments": ["pitch_contours"],
            "duration_seconds": None,
            "summary": "MIDI file produced; built-in parser could not inspect this file.",
        }


def _midi_read_vlq(data: bytes, pos: int, end: int) -> tuple[int, int]:
    value = 0
    for _ in range(4):
        if pos >= end:
            return value, pos
        byte = data[pos]
        pos += 1
        value = (value << 7) | (byte & 0x7F)
        if byte < 0x80:
            return value, pos
    return value, pos


def _collect_stems(root: Path) -> list[dict[str, Any]]:
    stems: list[dict[str, Any]] = []
    for path in sorted(root.rglob("*")):
        if not path.is_file():
            continue
        if path.suffix.lower() not in {".wav", ".mp3", ".flac", ".ogg", ".m4a"}:
            continue
        try:
            size = path.stat().st_size
        except OSError:
            size = 0
        stems.append(
            {
                "name": _safe_token(path.stem).lower(),
                "cache_path": str(path),
                "size_bytes": size,
                "mime": _stem_mime(path),
            }
        )
    return stems


def _stem_mime(path: Path) -> str:
    suffix = path.suffix.lower()
    if suffix == ".wav":
        return "audio/wav"
    if suffix == ".flac":
        return "audio/flac"
    if suffix == ".mp3":
        return "audio/mpeg"
    if suffix == ".ogg":
        return "audio/ogg"
    if suffix == ".m4a":
        return "audio/mp4"
    return "application/octet-stream"


def _import_optional(module_name: str) -> Any:
    if importlib.util.find_spec(module_name) is None:
        raise MissingModelError(f"Python package {module_name!r} is not installed in the Cartolensia AI environment")
    return __import__(module_name)


def _env_int(name: str, default: int, minimum: int, maximum: int) -> int:
    try:
        value = int(os.environ.get(name, ""))
    except Exception:
        value = default
    return max(minimum, min(maximum, value))


def _mean_float(values: Any, np: Any) -> float:
    try:
        if values is None:
            return 0.0
        return float(np.nan_to_num(np.asarray(values, dtype=float), nan=0.0).mean())
    except Exception:
        return 0.0


def _median_float(values: Any, np: Any) -> float | None:
    try:
        array = np.asarray(values, dtype=float)
        array = array[np.isfinite(array)]
        if array.size == 0:
            return None
        value = float(np.median(array))
        if value <= 0:
            return None
        return value
    except Exception:
        return None


def _estimate_key(chroma: Any, np: Any) -> tuple[str, str, float]:
    names = ["C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"]
    try:
        profile = np.asarray(chroma, dtype=float).mean(axis=1)
        if profile.size < 12:
            return "", "", 0.0
        index = int(np.argmax(profile[:12]))
        total = float(profile[:12].sum())
        confidence = float(profile[index] / total) if total > 0 else 0.0
        major_thirds = profile[(index + 4) % 12] + profile[(index + 7) % 12]
        minor_thirds = profile[(index + 3) % 12] + profile[(index + 7) % 12]
        mode = "major" if major_thirds >= minor_thirds else "minor"
        return names[index], mode, confidence
    except Exception:
        return "", "", 0.0


def _audio_feature_labels(tempo: float | None, speech_music_ratio: float, loudness: float) -> list[str]:
    labels: list[str] = []
    if speech_music_ratio >= 0.55:
        labels.append("speech-like")
    else:
        labels.append("music-like")
    if tempo is None:
        labels.append("tempo-uncertain")
    elif tempo < 80:
        labels.append("slow")
    elif tempo > 140:
        labels.append("fast")
    else:
        labels.append("mid-tempo")
    if loudness < -35:
        labels.append("quiet")
    elif loudness > -12:
        labels.append("loud")
    return labels


def _available_tesseract_languages(tesseract_path: str) -> set[str]:
    try:
        proc = subprocess.run(
            [tesseract_path, "--list-langs"],
            check=False,
            capture_output=True,
            text=True,
            timeout=10,
        )
    except Exception:
        return set()
    languages: set[str] = set()
    for line in (proc.stdout + "\n" + proc.stderr).splitlines():
        value = line.strip()
        if not value or value.lower().startswith("list of available"):
            continue
        if all(ch.isalnum() or ch == "_" for ch in value):
            languages.add(value)
    return languages


def _requested_ocr_languages(options: dict[str, Any] | None) -> list[str]:
    raw = None if not options else options.get("languages") or options.get("language")
    values: list[str]
    if raw is None or raw == "":
        values = DEFAULT_OCR_LANGUAGES
    elif isinstance(raw, str):
        values = [part.strip() for part in raw.replace("+", ",").split(",")]
    elif isinstance(raw, list):
        values = [str(part).strip() for part in raw]
    else:
        values = [str(raw).strip()]
    normalized: list[str] = []
    for value in values:
        if not value:
            continue
        key = value.lower().replace(" ", "_")
        normalized.append(OCR_LANGUAGE_ALIASES.get(key, key))
    if not normalized:
        normalized = DEFAULT_OCR_LANGUAGES
    return list(dict.fromkeys(normalized))


def _requested_asr_language(options: dict[str, Any] | None) -> str:
    raw = "" if not options else str(options.get("language") or options.get("languages") or "").strip()
    if "," in raw or "+" in raw:
        raw = raw.replace("+", ",").split(",", 1)[0].strip()
    key = raw.lower().replace(" ", "_")
    return ASR_LANGUAGE_ALIASES.get(key, key)


def _int_option(options: dict[str, Any] | None, key: str, default: int) -> int:
    if not options or key not in options:
        return default
    try:
        return int(options[key])
    except Exception:
        return default


def _float_option(options: dict[str, Any] | None, key: str, default: float) -> float:
    if not options or key not in options:
        return default
    try:
        return float(options[key])
    except Exception:
        return default


def _float_option_with_env(options: dict[str, Any] | None, key: str, env_name: str, default: float) -> float:
    if options and key in options and options[key] not in {None, ""}:
        try:
            return float(options[key])
        except Exception:
            return default
    try:
        return float(os.environ.get(env_name, ""))
    except Exception:
        return default


def _bool_option(options: dict[str, Any] | None, key: str, default: bool) -> bool:
    if not options or key not in options:
        return default
    value = options[key]
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        return value.strip().lower() in {"1", "true", "yes", "on"}
    return bool(value)


def _asr_device(selected_device: str) -> tuple[str, int]:
    if selected_device.startswith("cuda"):
        parts = selected_device.split(":", 1)
        index = 0
        if len(parts) == 2:
            try:
                index = int(parts[1])
            except ValueError:
                index = 0
        return "cuda", max(0, index)
    return "cpu", 0


def _segment_confidence(no_speech: Any, avg_logprob: Any) -> float | None:
    if no_speech is not None:
        try:
            return max(0.0, min(1.0, 1.0 - float(no_speech)))
        except Exception:
            pass
    if avg_logprob is not None:
        try:
            return max(0.0, min(1.0, math.exp(float(avg_logprob))))
        except Exception:
            return None
    return None


def _parse_tesseract_tsv(raw: str, languages: list[str], min_confidence: float) -> list[dict[str, Any]]:
    words: list[dict[str, Any]] = []
    reader = csv.DictReader(raw.splitlines(), delimiter="\t")
    for row in reader:
        text = (row.get("text") or "").strip()
        if not text:
            continue
        confidence = _parse_tesseract_confidence(row.get("conf"))
        if confidence is None:
            continue
        if confidence < min_confidence:
            continue
        try:
            left = float(row.get("left") or 0)
            top = float(row.get("top") or 0)
            width = float(row.get("width") or 0)
            height = float(row.get("height") or 0)
        except ValueError:
            continue
        if width <= 0 or height <= 0:
            continue
        words.append(
            {
                "text": text,
                "confidence": confidence,
                "x": left,
                "y": top,
                "width": width,
                "height": height,
                "block_num": row.get("block_num") or "0",
                "par_num": row.get("par_num") or "0",
                "line_num": row.get("line_num") or "0",
            }
        )
    grouped: dict[tuple[str, str, str], list[dict[str, Any]]] = {}
    for word in words:
        key = (str(word["block_num"]), str(word["par_num"]), str(word["line_num"]))
        grouped.setdefault(key, []).append(word)
    blocks: list[dict[str, Any]] = []
    for key, items in grouped.items():
        items.sort(key=lambda item: (float(item["y"]), float(item["x"])))
        min_x = min(float(item["x"]) for item in items)
        min_y = min(float(item["y"]) for item in items)
        max_x = max(float(item["x"]) + float(item["width"]) for item in items)
        max_y = max(float(item["y"]) + float(item["height"]) for item in items)
        confidence = sum(float(item["confidence"]) for item in items) / len(items)
        if confidence < min_confidence:
            continue
        blocks.append(
            {
                "text": " ".join(str(item["text"]) for item in items),
                "confidence": confidence,
                "x": min_x,
                "y": min_y,
                "width": max_x - min_x,
                "height": max_y - min_y,
                "language": "+".join(languages),
                "engine": "tesseract",
                "block": int(key[0]) if key[0].isdigit() else key[0],
                "paragraph": int(key[1]) if key[1].isdigit() else key[1],
                "line": int(key[2]) if key[2].isdigit() else key[2],
            }
        )
    blocks.sort(key=lambda item: (float(item["y"]), float(item["x"])))
    return blocks


def _parse_tesseract_confidence(raw: str | None) -> float | None:
    try:
        value = float(raw or "")
    except ValueError:
        return None
    if value < 0:
        return None
    return max(0.0, min(value / 100.0, 1.0))


def _model_missing(endpoint: str, error: Exception) -> InferenceResponse:
    return InferenceResponse(status="model_missing", endpoint=endpoint, reason=str(error), metadata={})


def _error(endpoint: str, error: Exception) -> InferenceResponse:
    return InferenceResponse(status="error", endpoint=endpoint, reason=str(error), metadata={})

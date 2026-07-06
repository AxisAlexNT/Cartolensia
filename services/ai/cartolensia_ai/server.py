"""FastAPI entrypoint for the optional Cartolensia AI sidecar."""

from __future__ import annotations

import argparse

from fastapi import FastAPI
import uvicorn

from cartolensia_ai import __version__
from cartolensia_ai.config import ServiceConfig, load_config
from cartolensia_ai.image_io import describe_optional_pillow
from cartolensia_ai.models.dummy import DummyBackend
from cartolensia_ai.models.real import CAPABILITIES, RealBackend
from cartolensia_ai.schemas import CapabilityResponse, ImageRequest, InferenceResponse, MediaRequest, TextRequest


def create_app(config: ServiceConfig | None = None) -> FastAPI:
    cfg = config or load_config()
    backend = DummyBackend() if cfg.mode == "dummy" else RealBackend(cfg)
    app = FastAPI(title="Cartolensia AI Sidecar", version=__version__)

    def capabilities() -> CapabilityResponse:
        model_state = getattr(backend, "model_state", "unknown")
        metadata = backend.metadata() if hasattr(backend, "metadata") else {}
        status = "not_configured" if cfg.mode == "dummy" else "ok"
        return CapabilityResponse(
            service="cartolensia-ai",
            status=status,
            mode=cfg.mode,
            capabilities=CAPABILITIES,
            model_state=model_state,
            model_dir=str(cfg.model_dir),
            safe_note="models are loaded locally from the configured cache; no remote APIs are used",
            device=metadata.get("device"),
            models=metadata.get("models", {}),
        )

    @app.get("/health")
    def health() -> dict[str, object]:
        return {
            "status": "ok",
            "service": "cartolensia-ai",
            "version": __version__,
            "capabilities": capabilities().model_dump(),
            "pillow": describe_optional_pillow(),
        }

    @app.get("/capabilities", response_model=CapabilityResponse)
    def get_capabilities() -> CapabilityResponse:
        return capabilities()

    @app.post("/classify-image", response_model=InferenceResponse, status_code=202)
    def classify_image(request: ImageRequest) -> InferenceResponse:
        if isinstance(backend, DummyBackend):
            return backend.infer("classify-image", request)
        return backend.classify_image(request)

    @app.post("/detect-faces", response_model=InferenceResponse, status_code=202)
    def detect_faces(request: ImageRequest) -> InferenceResponse:
        if isinstance(backend, DummyBackend):
            return backend.infer("detect-faces", request)
        return backend.detect_faces(request)

    @app.post("/safety-nsfw", response_model=InferenceResponse, status_code=202)
    def safety_nsfw(request: ImageRequest) -> InferenceResponse:
        if isinstance(backend, DummyBackend):
            return backend.infer("safety-nsfw", request)
        return backend.safety_nsfw(request)

    @app.post("/describe-image", response_model=InferenceResponse, status_code=202)
    def describe_image(request: ImageRequest) -> InferenceResponse:
        if isinstance(backend, DummyBackend):
            return backend.infer("describe-image", request)
        return backend.describe_image(request)

    @app.post("/embed-image", response_model=InferenceResponse, status_code=202)
    def embed_image(request: ImageRequest) -> InferenceResponse:
        if isinstance(backend, DummyBackend):
            return backend.infer("embed-image", request)
        return backend.embed_image(request)

    @app.post("/embed-text", response_model=InferenceResponse, status_code=202)
    def embed_text(request: TextRequest) -> InferenceResponse:
        if isinstance(backend, DummyBackend):
            return InferenceResponse(
                status="not_configured",
                endpoint="embed-text",
                reason="dummy worker has no model configured",
                metadata={"text": request.text},
            )
        return backend.embed_text(request)

    @app.post("/ocr-image", response_model=InferenceResponse, status_code=202)
    def ocr_image(request: ImageRequest) -> InferenceResponse:
        if isinstance(backend, DummyBackend):
            return backend.infer("ocr-image", request)
        return backend.ocr_image(request)

    @app.post("/transcribe-audio", response_model=InferenceResponse, status_code=202)
    def transcribe_audio(request: MediaRequest) -> InferenceResponse:
        if isinstance(backend, DummyBackend):
            return backend.infer("transcribe-audio", request)
        return backend.transcribe_audio(request)

    @app.post("/analyze-audio", response_model=InferenceResponse, status_code=202)
    def analyze_audio(request: MediaRequest) -> InferenceResponse:
        if isinstance(backend, DummyBackend):
            return backend.infer("analyze-audio", request)
        return backend.analyze_audio(request)

    @app.post("/music-to-midi", response_model=InferenceResponse, status_code=202)
    def music_to_midi(request: MediaRequest) -> InferenceResponse:
        if isinstance(backend, DummyBackend):
            return backend.infer("music-to-midi", request)
        return backend.music_to_midi(request)

    @app.post("/separate-music", response_model=InferenceResponse, status_code=202)
    def separate_music(request: MediaRequest) -> InferenceResponse:
        if isinstance(backend, DummyBackend):
            return backend.infer("separate-music", request)
        return backend.separate_music(request)

    return app


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run the Cartolensia AI sidecar")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=19090)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    uvicorn.run(create_app(), host=args.host, port=args.port, log_level="info")


if __name__ == "__main__":
    main()

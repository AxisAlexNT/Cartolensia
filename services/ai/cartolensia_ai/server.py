"""FastAPI entrypoint for the optional Cartolensia AI sidecar."""

from __future__ import annotations

import argparse

from fastapi import FastAPI
import uvicorn

from cartolensia_ai import __version__
from cartolensia_ai.config import ServiceConfig, load_config
from cartolensia_ai.image_io import describe_optional_pillow
from cartolensia_ai.models.dummy import CAPABILITIES, DummyBackend
from cartolensia_ai.schemas import CapabilityResponse, ImageRequest, InferenceResponse


def create_app(config: ServiceConfig | None = None) -> FastAPI:
    cfg = config or load_config()
    backend = DummyBackend()
    app = FastAPI(title="Cartolensia AI Sidecar", version=__version__)

    def capabilities() -> CapabilityResponse:
        return CapabilityResponse(
            service="cartolensia-ai",
            status="not_configured",
            mode=cfg.mode,
            capabilities=CAPABILITIES,
            model_state=backend.model_state,
            model_dir=str(cfg.model_dir),
            safe_note="dummy mode does not run inference or download models",
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
        return backend.infer("classify-image", request)

    @app.post("/detect-faces", response_model=InferenceResponse, status_code=202)
    def detect_faces(request: ImageRequest) -> InferenceResponse:
        return backend.infer("detect-faces", request)

    @app.post("/describe-image", response_model=InferenceResponse, status_code=202)
    def describe_image(request: ImageRequest) -> InferenceResponse:
        return backend.infer("describe-image", request)

    @app.post("/embed-image", response_model=InferenceResponse, status_code=202)
    def embed_image(request: ImageRequest) -> InferenceResponse:
        return backend.infer("embed-image", request)

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


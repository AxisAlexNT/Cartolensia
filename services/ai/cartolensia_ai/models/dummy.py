"""Dummy model backend used before model downloads are approved."""

from __future__ import annotations

from typing import Any

from cartolensia_ai.schemas import ImageRequest, InferenceResponse


CAPABILITIES = [
    "classify_image",
    "detect_faces",
    "describe_image",
    "embed_image",
    "ocr_image",
    "transcribe_audio",
    "analyze_audio",
    "music_midi",
    "music_stems",
]


class DummyBackend:
    status = "not_configured"
    model_state = "model_missing"

    def infer(self, endpoint: str, request: ImageRequest) -> InferenceResponse:
        return InferenceResponse(
            status="not_configured",
            endpoint=endpoint,
            predictions=[],
            reason="dummy worker has no model configured",
            metadata={"asset_id": request.asset_id, "options": request.options},
        )

    def metadata(self) -> dict[str, Any]:
        return {
            "backend": "dummy",
            "status": self.status,
            "model_state": self.model_state,
            "capabilities": CAPABILITIES,
        }

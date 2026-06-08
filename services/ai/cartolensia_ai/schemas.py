"""Request and response schemas for the AI sidecar contract."""

from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, Field


class ImageRequest(BaseModel):
    asset_id: str | None = None
    media_url: str | None = None
    storage_url: str | None = None
    options: dict[str, Any] = Field(default_factory=dict)


class MediaRequest(BaseModel):
    asset_id: str | None = None
    media_url: str | None = None
    storage_url: str | None = None
    options: dict[str, Any] = Field(default_factory=dict)


class TextRequest(BaseModel):
    text: str
    options: dict[str, Any] = Field(default_factory=dict)


class Prediction(BaseModel):
    label: str
    confidence: float | None = None
    metadata: dict[str, Any] = Field(default_factory=dict)


class InferenceResponse(BaseModel):
    status: Literal["ok", "not_configured", "model_missing", "unsupported", "error"]
    endpoint: str
    predictions: list[Prediction] = Field(default_factory=list)
    reason: str | None = None
    metadata: dict[str, Any] = Field(default_factory=dict)


class CapabilityResponse(BaseModel):
    service: str
    status: Literal["ok", "not_configured", "partial", "error"]
    mode: str
    capabilities: list[str]
    model_state: str
    model_dir: str
    safe_note: str
    device: str | None = None
    models: dict[str, Any] = Field(default_factory=dict)

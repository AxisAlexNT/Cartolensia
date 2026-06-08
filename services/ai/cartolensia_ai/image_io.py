"""Image loading helpers for future AI models.

The dummy sidecar does not load media. This module centralizes the future
boundary so model code can enforce path and URL policy in one place.
"""

from __future__ import annotations

from io import BytesIO
import os
from pathlib import Path
import tempfile
from typing import Any
from urllib.parse import urlparse
from urllib.request import Request, urlopen

from cartolensia_ai.config import ServiceConfig
from cartolensia_ai.schemas import ImageRequest, MediaRequest


MAX_IMAGE_BYTES = 128 * 1024 * 1024
MAX_MEDIA_BYTES = int(os.environ.get("CARTOLENSIA_AI_MAX_MEDIA_BYTES", str(2 * 1024 * 1024 * 1024)))


def describe_optional_pillow() -> dict[str, object]:
    try:
        import PIL  # type: ignore
    except Exception:
        return {"available": False, "version": None}
    return {"available": True, "version": getattr(PIL, "__version__", "unknown")}


def path_is_under(path: Path, root: Path) -> bool:
    try:
        path.resolve().relative_to(root.resolve())
    except ValueError:
        return False
    return True


def load_pillow_image(request: ImageRequest, config: ServiceConfig) -> Any:
    """Load a request image while enforcing the sidecar's local-only boundary."""

    try:
        from PIL import Image  # type: ignore
    except Exception as exc:  # pragma: no cover - import availability is environment-specific.
        raise RuntimeError("Pillow is not installed in the AI sidecar environment") from exc

    payload = _read_image_bytes(request, config)
    with Image.open(BytesIO(payload)) as image:
        return image.convert("RGB").copy()


def read_image_bytes(request: ImageRequest, config: ServiceConfig) -> bytes:
    """Read bounded image bytes while enforcing the sidecar input boundary."""

    return _read_image_bytes(request, config)


def materialize_media_input(request: MediaRequest, config: ServiceConfig, suffix: str = ".media") -> tuple[Path, bool]:
    """Return a local media path for model code.

    Localhost URLs are copied to /tmp because ASR libraries need a filesystem
    path. Safe already-local cache/temp paths are returned without copying.
    The boolean indicates whether the caller owns and should delete the path.
    """

    if request.media_url:
        return _copy_localhost_url_to_temp(request.media_url, suffix), True
    if request.storage_url:
        return _safe_path(request.storage_url, config), False
    candidate = request.options.get("path") if request.options else None
    if isinstance(candidate, str) and candidate:
        return _safe_path(candidate, config), False
    raise ValueError("media request requires media_url or a safe local path")


def _read_image_bytes(request: ImageRequest, config: ServiceConfig) -> bytes:
    if request.media_url:
        return _read_localhost_url(request.media_url)
    if request.storage_url:
        return _read_safe_path(request.storage_url, config)
    candidate = request.options.get("path") if request.options else None
    if isinstance(candidate, str) and candidate:
        return _read_safe_path(candidate, config)
    raise ValueError("image request requires media_url or a safe local path")


def _read_localhost_url(raw_url: str) -> bytes:
    parsed = urlparse(raw_url)
    if parsed.scheme not in {"http", "https"}:
        raise ValueError("only http(s) media_url values are supported")
    host = (parsed.hostname or "").lower()
    if host not in {"127.0.0.1", "localhost", "::1"}:
        raise ValueError("AI sidecar only accepts localhost media_url inputs")
    req = Request(raw_url, headers={"User-Agent": "Cartolensia-AI/0.1"})
    with urlopen(req, timeout=20) as response:  # nosec B310 - URL is localhost-gated above.
        return _bounded_read(response)


def _read_safe_path(raw_path: str, config: ServiceConfig) -> bytes:
    path = _safe_path(raw_path, config)
    with path.open("rb") as handle:
        return _bounded_read(handle)


def _safe_path(raw_path: str, config: ServiceConfig) -> Path:
    path = Path(raw_path).expanduser().resolve()
    cwd = Path.cwd().resolve()
    allowed_roots = [
        Path("/tmp").resolve(),
        config.model_dir.parent.resolve(),
        cwd / ".cartolensia",
    ]
    if not any(path_is_under(path, root) for root in allowed_roots):
        raise ValueError("local AI input path is outside allowed cache/temp roots")
    return path


def _bounded_read(handle: Any) -> bytes:
    data = handle.read(MAX_IMAGE_BYTES + 1)
    if len(data) > MAX_IMAGE_BYTES:
        raise ValueError("image input exceeds AI sidecar size limit")
    return data


def _copy_localhost_url_to_temp(raw_url: str, suffix: str) -> Path:
    parsed = urlparse(raw_url)
    if parsed.scheme not in {"http", "https"}:
        raise ValueError("only http(s) media_url values are supported")
    host = (parsed.hostname or "").lower()
    if host not in {"127.0.0.1", "localhost", "::1"}:
        raise ValueError("AI sidecar only accepts localhost media_url inputs")
    safe_suffix = suffix if suffix.startswith(".") and "/" not in suffix and "\\" not in suffix else ".media"
    req = Request(raw_url, headers={"User-Agent": "Cartolensia-AI/0.1"})
    total = 0
    with urlopen(req, timeout=30) as response:  # nosec B310 - URL is localhost-gated above.
        with tempfile.NamedTemporaryFile(prefix="cartolensia-ai-media-", suffix=safe_suffix, delete=False) as handle:
            temp_path = Path(handle.name)
            try:
                while True:
                    chunk = response.read(1024 * 1024)
                    if not chunk:
                        break
                    total += len(chunk)
                    if total > MAX_MEDIA_BYTES:
                        raise ValueError("media input exceeds AI sidecar size limit")
                    handle.write(chunk)
            except Exception:
                try:
                    temp_path.unlink(missing_ok=True)
                finally:
                    raise
            return temp_path

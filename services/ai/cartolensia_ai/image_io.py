"""Image loading helpers for future AI models.

The dummy sidecar does not load media. This module centralizes the future
boundary so model code can enforce path and URL policy in one place.
"""

from __future__ import annotations

from pathlib import Path


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


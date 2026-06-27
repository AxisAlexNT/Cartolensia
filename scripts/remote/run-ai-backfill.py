#!/usr/bin/env python3
"""Low-concurrency Cartolensia AI backfill driver.

This script is intended for production hosts with large read-only original
stores. It selects assets that are missing durable metadata rows and feeds
small batches to the authenticated Cartolensia API. Originals are read only
through Cartolensia media URLs; all outputs are stored in PostgreSQL metadata.
"""

from __future__ import annotations

import http.cookiejar
import http.client
import json
import os
from pathlib import Path
import ssl
import subprocess
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone
from typing import Iterable


def env_int(name: str, default: int) -> int:
    raw = os.environ.get(name, "").strip()
    if not raw:
        return default
    try:
        return int(raw)
    except ValueError:
        return default


BASE_URL = os.environ.get("CARTOLENSIA_BACKFILL_BASE_URL", "https://127.0.0.1:18443").rstrip("/")
ADMIN_EMAIL = os.environ.get("CARTOLENSIA_BACKFILL_ADMIN_EMAIL", "admin@example.local")
PASSWORD_FILE = os.environ.get("CARTOLENSIA_BACKFILL_PASSWORD_FILE", "/etc/cartolensia/admin-password")
PSQL = os.environ.get("CARTOLENSIA_BACKFILL_PSQL", "/opt/cartolensia/current/external/postgres/bin/psql")
DSN = os.environ.get("CARTOLENSIA_DATABASE_URL", "")

PHOTO_BATCH = max(1, env_int("CARTOLENSIA_AI_BACKFILL_PHOTO_BATCH", 8))
OCR_BATCH = max(1, env_int("CARTOLENSIA_AI_BACKFILL_OCR_BATCH", 4))
AUDIO_BATCH = max(1, env_int("CARTOLENSIA_AI_BACKFILL_AUDIO_BATCH", 2))
TRANSCRIBE_BATCH = max(1, env_int("CARTOLENSIA_AI_BACKFILL_TRANSCRIBE_BATCH", 1))
VIDEO_TRANSCRIBE_BATCH = max(1, env_int("CARTOLENSIA_AI_BACKFILL_VIDEO_TRANSCRIBE_BATCH", 1))
MAX_AUDIO_SECONDS = env_int("CARTOLENSIA_AI_BACKFILL_MAX_AUDIO_SECONDS", 900)
MAX_VIDEO_SECONDS = env_int("CARTOLENSIA_AI_BACKFILL_MAX_VIDEO_SECONDS", 900)
SLEEP_SECONDS = max(0, env_int("CARTOLENSIA_AI_BACKFILL_SLEEP_SECONDS", 5))
IDLE_SLEEP_SECONDS = max(10, env_int("CARTOLENSIA_AI_BACKFILL_IDLE_SLEEP_SECONDS", 300))
MAX_ITERATIONS = env_int("CARTOLENSIA_AI_BACKFILL_MAX_ITERATIONS", 0)
STATE_DIR = Path(os.environ.get("CARTOLENSIA_AI_BACKFILL_STATE_DIR", "/var/lib/cartolensia/run/ai-backfill-state"))
API_RETRIES = max(1, env_int("CARTOLENSIA_AI_BACKFILL_API_RETRIES", 6))
API_RETRY_SLEEP_SECONDS = max(1, env_int("CARTOLENSIA_AI_BACKFILL_API_RETRY_SLEEP_SECONDS", 10))
MAX_PROBE_LIMIT = max(100, env_int("CARTOLENSIA_AI_BACKFILL_MAX_PROBE_LIMIT", 50000))


def log(message: str) -> None:
    stamp = datetime.now(timezone.utc).isoformat(timespec="seconds")
    print(f"{stamp} {message}", flush=True)


def run_psql(sql: str) -> list[str]:
    if not DSN:
        raise RuntimeError("CARTOLENSIA_DATABASE_URL is required")
    proc = subprocess.run(
        [PSQL, DSN, "-At", "-c", sql],
        check=True,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    return [line.strip() for line in proc.stdout.splitlines() if line.strip()]


class API:
    def __init__(self) -> None:
        ctx = ssl._create_unverified_context()
        self.cookies = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(self.cookies),
            urllib.request.HTTPSHandler(context=ctx),
        )
        self.csrf_header = "X-CSRF-Token"
        self.csrf_token = ""

    def request(self, method: str, path: str, payload: dict | None = None, timeout: int = 3600) -> dict:
        body = None
        headers: dict[str, str] = {}
        if payload is not None:
            body = json.dumps(payload).encode("utf-8")
            headers["Content-Type"] = "application/json"
        if method not in ("GET", "HEAD") and self.csrf_token:
            headers[self.csrf_header] = self.csrf_token
        req = urllib.request.Request(BASE_URL + path, data=body, headers=headers, method=method)
        last_error: Exception | None = None
        for attempt in range(1, API_RETRIES + 1):
            try:
                with self.opener.open(req, timeout=timeout) as resp:
                    raw = resp.read()
                    return json.loads(raw or b"{}")
            except urllib.error.HTTPError as exc:
                body_text = exc.read(2048).decode("utf-8", "replace")
                if 500 <= exc.code < 600 and attempt < API_RETRIES:
                    last_error = RuntimeError(f"{method} {path} failed with HTTP {exc.code}: {body_text}")
                else:
                    raise RuntimeError(f"{method} {path} failed with HTTP {exc.code}: {body_text}") from exc
            except urllib.error.URLError as exc:
                last_error = exc
            except (http.client.RemoteDisconnected, TimeoutError, ConnectionResetError, ConnectionAbortedError) as exc:
                last_error = exc
            if attempt < API_RETRIES:
                log(f"{method} {path}: API unavailable ({last_error}); retry {attempt}/{API_RETRIES}")
                time.sleep(API_RETRY_SLEEP_SECONDS)
        raise RuntimeError(f"{method} {path} failed after {API_RETRIES} attempts: {last_error}")

    def login(self) -> None:
        with open(PASSWORD_FILE, "r", encoding="utf-8") as handle:
            password = handle.read().rstrip("\r\n")
        self.request("POST", "/api/v1/auth/login", {"email": ADMIN_EMAIL, "password": password}, timeout=60)
        csrf = self.request("GET", "/api/v1/auth/csrf", timeout=60)
        self.csrf_header = csrf.get("header") or self.csrf_header
        self.csrf_token = csrf.get("token") or ""


def ids_for(sql: str, limit: int) -> list[str]:
    return run_psql(sql.replace("$LIMIT", str(limit)))


def seen_path(task_name: str) -> Path:
    safe = "".join(ch if ch.isalnum() or ch in ("-", "_") else "_" for ch in task_name)
    return STATE_DIR / f"{safe}.seen"


def load_seen(task_name: str) -> set[str]:
    path = seen_path(task_name)
    if not path.exists():
        return set()
    with path.open("r", encoding="utf-8") as handle:
        return {line.strip() for line in handle if line.strip()}


def mark_seen(task_name: str, ids: Iterable[str]) -> None:
    ids = list(ids)
    if not ids:
        return
    STATE_DIR.mkdir(parents=True, exist_ok=True)
    with seen_path(task_name).open("a", encoding="utf-8") as handle:
        for asset_id in ids:
            handle.write(asset_id + "\n")


def ids_for_task(task: dict) -> list[str]:
    batch = int(task["batch"])
    seen = load_seen(str(task["name"]))
    probe_limit = max(batch * 50, 100)
    last_seen_count = -1
    while probe_limit <= MAX_PROBE_LIMIT:
        candidates = ids_for(str(task["sql"]), probe_limit)
        unseen = [asset_id for asset_id in candidates if asset_id not in seen]
        if unseen or len(candidates) < probe_limit:
            return unseen[:batch]
        if len(candidates) == last_seen_count:
            return []
        last_seen_count = len(candidates)
        probe_limit = min(probe_limit * 4, MAX_PROBE_LIMIT + 1)
    candidates = ids_for(str(task["sql"]), MAX_PROBE_LIMIT)
    unseen = [asset_id for asset_id in candidates if asset_id not in seen]
    if not unseen and candidates:
        log(
            f"{task['name']}: first {len(candidates)} missing candidates are already marked seen; "
            "increase CARTOLENSIA_AI_BACKFILL_MAX_PROBE_LIMIT or clear the task seen file to revisit them"
        )
    return unseen[:batch]


def quoted_ids(ids: Iterable[str]) -> str:
    return ", ".join(json.dumps(asset_id) for asset_id in ids)


TASKS = [
    {
        "name": "classify",
        "endpoint": "/api/v1/ai/jobs/classify",
        "batch": PHOTO_BATCH,
        "sql": """
            select a.id::text
            from assets a
            where a.media_kind='photo'
              and not exists (
                select 1 from ai_predictions p
                where p.asset_id=a.id and p.task='classify_image'
              )
            order by coalesce(a.taken_at, a.first_seen_at, a.updated_at), a.id
            limit $LIMIT
        """,
    },
    {
        "name": "safety",
        "endpoint": "/api/v1/ai/jobs/safety",
        "batch": PHOTO_BATCH,
        "sql": """
            select a.id::text
            from assets a
            where a.media_kind='photo'
              and not exists (
                select 1 from ai_predictions p
                where p.asset_id=a.id and p.task='safety_nsfw'
              )
            order by coalesce(a.taken_at, a.first_seen_at, a.updated_at), a.id
            limit $LIMIT
        """,
    },
    {
        "name": "caption",
        "endpoint": "/api/v1/ai/jobs/describe",
        "batch": PHOTO_BATCH,
        "sql": """
            select a.id::text
            from assets a
            where a.media_kind='photo'
              and not exists (
                select 1 from ai_predictions p
                where p.asset_id=a.id and p.task='describe_image'
              )
            order by coalesce(a.taken_at, a.first_seen_at, a.updated_at), a.id
            limit $LIMIT
        """,
    },
    {
        "name": "embed",
        "endpoint": "/api/v1/ai/jobs/embed",
        "batch": PHOTO_BATCH,
        "sql": """
            select a.id::text
            from assets a
            where a.media_kind='photo'
              and not exists (select 1 from asset_embeddings e where e.asset_id=a.id)
            order by coalesce(a.taken_at, a.first_seen_at, a.updated_at), a.id
            limit $LIMIT
        """,
    },
    {
        "name": "ocr",
        "endpoint": "/api/v1/ai/jobs/ocr",
        "batch": OCR_BATCH,
        "sql": """
            select a.id::text
            from assets a
            where a.media_kind='photo'
              and not exists (
                select 1 from ai_predictions p
                where p.asset_id=a.id and p.task='ocr_image'
              )
            order by coalesce(a.taken_at, a.first_seen_at, a.updated_at), a.id
            limit $LIMIT
        """,
    },
    {
        "name": "faces",
        "endpoint": "/api/v1/ai/jobs/faces",
        "batch": PHOTO_BATCH,
        "sql": """
            select a.id::text
            from assets a
            where a.media_kind='photo'
              and not exists (
                select 1 from face_detections f
                where f.asset_id=a.id
              )
            order by coalesce(a.taken_at, a.first_seen_at, a.updated_at), a.id
            limit $LIMIT
        """,
    },
    {
        "name": "audio_features",
        "endpoint": "/api/v1/ai/jobs/audio-analyze",
        "batch": AUDIO_BATCH,
        "sql": """
            select a.id::text
            from assets a
            where a.media_kind='audio'
              and not exists (select 1 from audio_features f where f.asset_id=a.id)
            order by coalesce(a.taken_at, a.first_seen_at, a.updated_at), a.id
            limit $LIMIT
        """,
    },
    {
        "name": "audio_transcript",
        "endpoint": "/api/v1/ai/jobs/transcribe",
        "batch": TRANSCRIBE_BATCH,
        "sql": f"""
            select a.id::text
            from assets a
            where a.media_kind='audio'
              and not exists (select 1 from asset_transcripts t where t.asset_id=a.id)
              and case
                    when coalesce(a.metadata_json->>'duration_seconds','') ~ '^[0-9]+([.][0-9]+)?$'
                    then (a.metadata_json->>'duration_seconds')::float
                    else 0
                  end <= {MAX_AUDIO_SECONDS}
            order by coalesce(a.taken_at, a.first_seen_at, a.updated_at), a.id
            limit $LIMIT
        """,
    },
    {
        "name": "video_transcript",
        "endpoint": "/api/v1/ai/jobs/transcribe",
        "batch": VIDEO_TRANSCRIBE_BATCH,
        "sql": f"""
            select a.id::text
            from assets a
            where a.media_kind='video'
              and not exists (select 1 from asset_transcripts t where t.asset_id=a.id)
              and case
                    when coalesce(a.metadata_json->>'duration_seconds','') ~ '^[0-9]+([.][0-9]+)?$'
                    then (a.metadata_json->>'duration_seconds')::float
                    else 0
                  end <= {MAX_VIDEO_SECONDS}
            order by coalesce(a.taken_at, a.first_seen_at, a.updated_at), a.id
            limit $LIMIT
        """,
    },
]


def main() -> int:
    api = API()
    api.login()
    log(f"AI backfill started against {BASE_URL}; batches are small and missing-work only")
    iteration = 0
    while True:
        iteration += 1
        did_work = False
        for task in TASKS:
            ids = ids_for_task(task)
            if not ids:
                log(f"{task['name']}: no missing assets in this pass")
                continue
            did_work = True
            log(f"{task['name']}: running {len(ids)} assets [{quoted_ids(ids[:3])}{'...' if len(ids) > 3 else ''}]")
            result = api.request(
                "POST",
                str(task["endpoint"]),
                {"scope": "selected", "asset_ids": ids, "limit": len(ids)},
                timeout=7200,
            )
            log(
                f"{task['name']}: {result.get('status')} "
                f"processed={result.get('processed')} skipped={result.get('skipped')} "
                f"stored={result.get('stored')} errors={len(result.get('errors') or [])}"
            )
            if not (result.get("errors") or []):
                mark_seen(str(task["name"]), ids)
            if SLEEP_SECONDS:
                time.sleep(SLEEP_SECONDS)
        if MAX_ITERATIONS and iteration >= MAX_ITERATIONS:
            log("AI backfill reached configured iteration limit")
            return 0
        if not did_work:
            log(f"AI backfill idle; sleeping {IDLE_SLEEP_SECONDS}s")
            time.sleep(IDLE_SLEEP_SECONDS)


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        log("AI backfill interrupted")
        raise SystemExit(130)

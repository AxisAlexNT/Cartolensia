#!/usr/bin/env python3
"""Backfill PDF/text document extraction and local LLM summaries.

The worker is designed for production hosts with read-only originals. It reads
document assets from configured storage roots, extracts text into PostgreSQL,
and optionally summarizes that text with a local Ollama-compatible endpoint.
It never writes beside originals.
"""

from __future__ import annotations

import json
import os
from pathlib import Path
import html
import re
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
import uuid
from datetime import datetime, timezone
from typing import Any


def env_int(name: str, default: int) -> int:
    raw = os.environ.get(name, "").strip()
    if not raw:
        return default
    try:
        return int(raw)
    except ValueError:
        return default


def env_bool(name: str, default: bool) -> bool:
    raw = os.environ.get(name, "").strip().lower()
    if not raw:
        return default
    return raw in {"1", "true", "yes", "on"}


PSQL = os.environ.get("CARTOLENSIA_DOCUMENT_BACKFILL_PSQL", "/opt/cartolensia/current/external/postgres/bin/psql")
DSN = os.environ.get(
    "CARTOLENSIA_DATABASE_URL",
    "postgresql://cartolensia@127.0.0.1:15432/cartolensia?sslmode=disable",
)
WORKER_ID = os.environ.get("CARTOLENSIA_DOCUMENT_BACKFILL_WORKER_ID", "remote-document-backfill")
EXTENSIONS = [
    ext.strip().lower().lstrip(".")
    for ext in os.environ.get("CARTOLENSIA_DOCUMENT_BACKFILL_EXTENSIONS", "pdf").split(",")
    if ext.strip()
]
LIMIT = env_int("CARTOLENSIA_DOCUMENT_BACKFILL_LIMIT", -1)
MAX_TEXT_CHARS = env_int("CARTOLENSIA_DOCUMENT_BACKFILL_MAX_TEXT_CHARS", 300_000)
MAX_MARKDOWN_CHARS = env_int("CARTOLENSIA_DOCUMENT_BACKFILL_MAX_MARKDOWN_CHARS", 300_000)
MAX_PAGES = env_int("CARTOLENSIA_DOCUMENT_BACKFILL_MAX_PAGES", 0)
OCR_MAX_PAGES = env_int("CARTOLENSIA_DOCUMENT_BACKFILL_OCR_MAX_PAGES", 50)
OCR_DPI = env_int("CARTOLENSIA_DOCUMENT_BACKFILL_OCR_DPI", 150)
TEXT_READ_BYTES = env_int("CARTOLENSIA_DOCUMENT_BACKFILL_TEXT_READ_BYTES", 1_000_000)
PREFER_MARKER = env_bool("CARTOLENSIA_DOCUMENT_BACKFILL_PREFER_MARKER", True)
MARKER_BIN = os.environ.get("CARTOLENSIA_DOCUMENT_BACKFILL_MARKER_BIN", "marker_single")
MARKER_PYTHON = os.environ.get("CARTOLENSIA_DOCUMENT_BACKFILL_MARKER_PYTHON", "").strip()
MARKER_FORCE_OCR = env_bool("CARTOLENSIA_DOCUMENT_BACKFILL_MARKER_FORCE_OCR", True)
MARKER_TIMEOUT_SECONDS = env_int("CARTOLENSIA_DOCUMENT_BACKFILL_MARKER_TIMEOUT_SECONDS", 1200)
REPROCESS_NON_MARKER = env_bool("CARTOLENSIA_DOCUMENT_BACKFILL_REPROCESS_NON_MARKER", False)
REPROCESS_MISSING_SUMMARY = env_bool("CARTOLENSIA_DOCUMENT_BACKFILL_REPROCESS_MISSING_SUMMARY", False)
SHARD_COUNT = max(1, env_int("CARTOLENSIA_DOCUMENT_BACKFILL_SHARD_COUNT", 1))
SHARD_INDEX = env_int("CARTOLENSIA_DOCUMENT_BACKFILL_SHARD_INDEX", 0)
SUMMARIZE = env_bool("CARTOLENSIA_DOCUMENT_BACKFILL_SUMMARIZE", True)
SUMMARY_MAX_CHARS = env_int("CARTOLENSIA_DOCUMENT_BACKFILL_SUMMARY_MAX_CHARS", 14_000)
OLLAMA_ENDPOINT = os.environ.get("CARTOLENSIA_DOCUMENT_BACKFILL_LLM_ENDPOINT", "http://127.0.0.1:11434").rstrip("/")
OLLAMA_MODEL = os.environ.get("CARTOLENSIA_DOCUMENT_BACKFILL_LLM_MODEL", "qwen3:8b")
SUMMARY_TIMEOUT_SECONDS = env_int("CARTOLENSIA_DOCUMENT_BACKFILL_SUMMARY_TIMEOUT_SECONDS", 240)
STATE_DIR = Path(os.environ.get("CARTOLENSIA_DOCUMENT_BACKFILL_STATE_DIR", "/var/lib/cartolensia/run/document-backfill"))
OCR_TMP_ROOT = Path(os.environ.get("CARTOLENSIA_DOCUMENT_BACKFILL_TMP_DIR", "/tmp/cartolensia-document-ocr"))
PROGRESS_EVERY = max(1, env_int("CARTOLENSIA_DOCUMENT_BACKFILL_PROGRESS_EVERY", 5))


def log(message: str) -> None:
    stamp = datetime.now(timezone.utc).isoformat(timespec="seconds")
    print(f"{stamp} {message}", flush=True)


def sql_literal(value: Any) -> str:
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return str(value)
    text = str(value)
    return "'" + text.replace("'", "''") + "'"


def run_psql(sql: str) -> list[str]:
    proc = subprocess.run(
        [PSQL, DSN, "-At", "-F", "\t", "-v", "ON_ERROR_STOP=1", "-c", sql],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=True,
    )
    return [line.rstrip("\n") for line in proc.stdout.splitlines() if line.strip()]


def run_psql_file(sql: str) -> None:
    with tempfile.NamedTemporaryFile("w", encoding="utf-8", suffix=".sql", delete=False) as handle:
        handle.write(sql)
        temp_path = handle.name
    try:
        subprocess.run([PSQL, DSN, "-q", "-v", "ON_ERROR_STOP=1", "-f", temp_path], check=True)
    finally:
        try:
            os.unlink(temp_path)
        except FileNotFoundError:
            pass


def json_literal(value: Any) -> str:
    return sql_literal(json.dumps(value, ensure_ascii=False, separators=(",", ":"))) + "::jsonb"


def now_iso() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


class Job:
    def __init__(self, total: int) -> None:
        self.id = str(uuid.uuid4())
        self.total = total
        payload = {
            "extensions": EXTENSIONS,
            "limit": LIMIT,
            "max_pages": MAX_PAGES,
            "ocr_max_pages": OCR_MAX_PAGES,
            "summarize": SUMMARIZE,
            "llm_model": OLLAMA_MODEL if SUMMARIZE else "",
            "reprocess_non_marker": REPROCESS_NON_MARKER,
            "reprocess_missing_summary": REPROCESS_MISSING_SUMMARY,
            "shard_count": SHARD_COUNT,
            "shard_index": SHARD_INDEX,
            "safe_note": "documents are read from configured storage roots; outputs are stored in PostgreSQL only",
        }
        run_psql(
            f"""
            insert into jobs(id, kind, status, payload_json, counters_json, progress_current, progress_total,
                attempts, max_attempts, worker_id, started_at)
            values(
                {sql_literal(self.id)}::uuid, 'document_extract', 'running', {json_literal(payload)},
                '{{}}'::jsonb, 0, {total}, 1, 1, {sql_literal(WORKER_ID)}, now()
            )
            on conflict(id) do nothing
            """
        )

    def log(self, level: str, message: str) -> None:
        run_psql(
            f"""
            insert into job_logs(job_id, level, message, created_at)
            values({sql_literal(self.id)}::uuid, {sql_literal(level)}, {sql_literal(message)}, now())
            """
        )

    def update(self, current: int, counters: dict[str, int | str], status: str = "running", error: str = "") -> None:
        finished = "now()" if status in {"succeeded", "failed", "cancelled"} else "null"
        run_psql(
            f"""
            update jobs
            set status={sql_literal(status)},
                counters_json={json_literal(counters)},
                progress_current={current},
                progress_total={self.total},
                finished_at={finished},
                error={sql_literal(error) if error else 'null'}
            where id={sql_literal(self.id)}::uuid
            """
        )

    def cancel_requested(self) -> bool:
        rows = run_psql(f"select 1 from jobs where id={sql_literal(self.id)}::uuid and cancel_requested_at is not null")
        return bool(rows)


def select_candidates() -> list[dict[str, str]]:
    if not EXTENSIONS:
        raise RuntimeError("CARTOLENSIA_DOCUMENT_BACKFILL_EXTENSIONS resolved to an empty list")
    if SHARD_INDEX < 0 or SHARD_INDEX >= SHARD_COUNT:
        raise RuntimeError(
            f"CARTOLENSIA_DOCUMENT_BACKFILL_SHARD_INDEX must be in [0,{SHARD_COUNT - 1}], got {SHARD_INDEX}"
        )
    ext_sql = ", ".join(sql_literal(ext) for ext in EXTENSIONS)
    limit_sql = "" if LIMIT < 0 else f"limit {LIMIT}"
    shard_sql = ""
    if SHARD_COUNT > 1:
        # Use the high 60 bits of md5(asset_id) to avoid signed int64 overflow
        # while keeping deterministic, portable shard assignment in PostgreSQL.
        shard_sql = (
            "and mod(('x' || substr(md5(a.id::text), 1, 15))::bit(60)::bigint, "
            f"{SHARD_COUNT}) = {SHARD_INDEX}"
        )
    work_predicates = ["not exists (select 1 from document_text d where d.asset_id=a.id)"]
    if REPROCESS_NON_MARKER:
        work_predicates.append(
            "exists (select 1 from document_text d where d.asset_id=a.id "
            "and coalesce(d.metadata_json->>'extractor','') <> 'marker')"
        )
    if REPROCESS_MISSING_SUMMARY:
        work_predicates.append(
            "exists (select 1 from document_text d where d.asset_id=a.id "
            "and not (d.metadata_json ? 'llm_summary'))"
        )
    work_sql = " or ".join(work_predicates)
    rows = run_psql(
        f"""
        with latest_location as (
            select distinct on (l.asset_id)
                l.asset_id::text,
                s.name as storage_name,
                s.root as storage_root,
                s.mode as storage_mode,
                l.relative_path,
                l.file_name,
                lower(trim(leading '.' from l.extension)) as extension,
                l.mime_type,
                l.size_bytes::text,
                l.url
            from asset_locations l
            join storage_backends s on s.id = l.storage_id
            where l.media_kind='document'
              and lower(trim(leading '.' from l.extension)) in ({ext_sql})
            order by l.asset_id, l.last_seen_at desc
        )
        select a.id::text, a.display_name, ll.storage_name, ll.storage_root, ll.storage_mode,
               ll.relative_path, ll.file_name, ll.extension, ll.mime_type, ll.size_bytes, ll.url
        from assets a
        join latest_location ll on ll.asset_id=a.id::text
        where a.media_kind='document'
          and ({work_sql})
          {shard_sql}
        order by coalesce(a.taken_at, a.first_seen_at, a.updated_at), a.id
        {limit_sql}
        """
    )
    out: list[dict[str, str]] = []
    for line in rows:
        parts = line.split("\t")
        if len(parts) != 11:
            log(f"skipping malformed psql row with {len(parts)} columns")
            continue
        out.append(
            {
                "asset_id": parts[0],
                "display_name": parts[1],
                "storage_name": parts[2],
                "storage_root": parts[3],
                "storage_mode": parts[4],
                "relative_path": parts[5],
                "file_name": parts[6],
                "extension": parts[7],
                "mime_type": parts[8],
                "size_bytes": parts[9],
                "url": parts[10],
            }
        )
    return out


def resolve_original_path(row: dict[str, str]) -> Path:
    root = Path(row["storage_root"]).resolve(strict=True)
    candidate = (root / row["relative_path"]).resolve(strict=False)
    try:
        candidate.relative_to(root)
    except ValueError as exc:
        raise RuntimeError(f"resolved path escapes storage root: {candidate}") from exc
    if not candidate.exists():
        raise FileNotFoundError(str(candidate))
    if not candidate.is_file():
        raise RuntimeError(f"not a regular file: {candidate}")
    return candidate


def run_command(args: list[str], timeout: int = 300) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        args,
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=timeout,
    )


def command_exists(name: str) -> bool:
    return shutil.which(name) is not None


def marker_command_base() -> list[str]:
    if MARKER_PYTHON:
        return [
            MARKER_PYTHON,
            "-c",
            "from marker.scripts.convert_single import convert_single_cli; raise SystemExit(convert_single_cli())",
        ]
    return [MARKER_BIN]


def marker_available() -> bool:
    if MARKER_PYTHON:
        return Path(MARKER_PYTHON).exists()
    return command_exists(MARKER_BIN)


def pdf_page_count(path: Path) -> int:
    if command_exists("pdfinfo"):
        proc = run_command(["pdfinfo", str(path)], timeout=60)
        for line in proc.stdout.splitlines():
            if line.lower().startswith("pages:"):
                try:
                    return int(line.split(":", 1)[1].strip())
                except ValueError:
                    pass
    if command_exists("mutool"):
        proc = run_command(["mutool", "info", str(path)], timeout=60)
        for line in (proc.stdout + "\n" + proc.stderr).splitlines():
            if line.lower().startswith("pages:"):
                try:
                    return int(line.split(":", 1)[1].strip())
                except ValueError:
                    pass
    return 0


def truncate_text(text: str, max_chars: int) -> str:
    if max_chars > 0 and len(text) > max_chars:
        return text[:max_chars] + "\n\n[Cartolensia truncated stored document text at configured limit.]"
    return text


def extract_pdf_text(path: Path) -> tuple[str, str, dict[str, Any]]:
    metadata: dict[str, Any] = {
        "extractor": "marker",
        "marker_attempted": False,
        "marker_bin": MARKER_BIN,
        "marker_python": MARKER_PYTHON,
        "marker_force_ocr": MARKER_FORCE_OCR,
        "ocr_attempted": False,
        "ocr_engine": "",
        "max_pages": MAX_PAGES,
        "ocr_max_pages": OCR_MAX_PAGES,
    }
    if PREFER_MARKER:
        marker_text, marker_markdown = extract_pdf_marker(path, metadata)
        if marker_text or marker_markdown:
            return (
                truncate_text(marker_text or marker_markdown, MAX_TEXT_CHARS),
                truncate_text(marker_markdown or marker_text, MAX_MARKDOWN_CHARS),
                metadata,
            )

    metadata["extractor"] = "pdftotext"
    text = ""
    if command_exists("pdftotext"):
        args = ["pdftotext", "-layout", "-enc", "UTF-8"]
        if MAX_PAGES > 0:
            args.extend(["-f", "1", "-l", str(MAX_PAGES)])
        args.extend([str(path), "-"])
        proc = run_command(args, timeout=900)
        metadata["pdftotext_returncode"] = proc.returncode
        if proc.returncode == 0:
            text = proc.stdout.strip()
        else:
            metadata["pdftotext_error"] = proc.stderr[-2000:]
    else:
        metadata["pdftotext_missing"] = True

    if text:
        markdown = "# " + path.name + "\n\n" + text
        return truncate_text(text, MAX_TEXT_CHARS), truncate_text(markdown, MAX_MARKDOWN_CHARS), metadata

    ocr_text = extract_pdf_ocr(path, metadata)
    markdown = "# " + path.name + "\n\n" + ocr_text if ocr_text else ""
    return truncate_text(ocr_text, MAX_TEXT_CHARS), truncate_text(markdown, MAX_MARKDOWN_CHARS), metadata


def extract_pdf_marker(path: Path, metadata: dict[str, Any]) -> tuple[str, str]:
    metadata["marker_attempted"] = True
    if not marker_available():
        metadata["marker_error"] = f"Marker runner is not installed: {MARKER_PYTHON or MARKER_BIN}"
        return "", ""

    with tempfile.TemporaryDirectory(prefix="marker-", dir=str(OCR_TMP_ROOT if OCR_TMP_ROOT.exists() else "/tmp")) as temp_dir:
        args = marker_command_base() + [
            str(path),
            "--output_dir",
            temp_dir,
            "--output_format",
            "markdown",
        ]
        if MARKER_FORCE_OCR:
            args.extend(["--converter_cls", "marker.converters.ocr.OCRConverter"])
        if MAX_PAGES > 0:
            # Marker currently does not expose a stable page-range flag across all
            # supported versions. Keep the full-file call and rely on the outer
            # timeout rather than passing a version-fragile option.
            metadata["marker_page_limit_note"] = "MAX_PAGES is enforced by fallback extractors only"
        try:
            proc = run_command(args, timeout=MARKER_TIMEOUT_SECONDS)
        except subprocess.TimeoutExpired:
            metadata["marker_error"] = f"marker timed out after {MARKER_TIMEOUT_SECONDS}s"
            return "", ""
        metadata["marker_returncode"] = proc.returncode
        if proc.returncode != 0:
            metadata["marker_error"] = (proc.stderr or proc.stdout)[-4000:]
            return "", ""
        markdown_files = sorted(Path(temp_dir).rglob("*.md"), key=lambda p: p.stat().st_mtime, reverse=True)
        if markdown_files:
            markdown = markdown_files[0].read_text(encoding="utf-8", errors="replace").strip()
            metadata["marker_output"] = markdown_files[0].name
        else:
            json_files = sorted(
                (p for p in Path(temp_dir).rglob("*.json") if not p.name.endswith("_meta.json")),
                key=lambda p: p.stat().st_size,
                reverse=True,
            )
            if not json_files:
                metadata["marker_error"] = "marker completed but produced no markdown or document JSON file"
                metadata["marker_stdout"] = proc.stdout[-2000:]
                metadata["marker_stderr"] = proc.stderr[-2000:]
                return "", ""
            try:
                marker_json = json.loads(json_files[0].read_text(encoding="utf-8", errors="replace"))
                markdown = marker_json_to_markdown(marker_json, title=path.name)
                metadata["marker_output"] = json_files[0].name
                metadata["marker_output_kind"] = "json_text_projection"
            except (OSError, json.JSONDecodeError) as exc:
                metadata["marker_error"] = f"marker JSON parse failed: {exc}"
                metadata["marker_stdout"] = proc.stdout[-2000:]
                metadata["marker_stderr"] = proc.stderr[-2000:]
                return "", ""
        if not markdown.strip():
            metadata["marker_error"] = "marker completed but produced empty markdown/text"
            metadata["marker_stdout"] = proc.stdout[-2000:]
            metadata["marker_stderr"] = proc.stderr[-2000:]
            return "", ""
        metadata["extractor"] = "marker"
        metadata["marker_stdout"] = proc.stdout[-2000:]
        metadata["marker_stderr"] = proc.stderr[-2000:]
        return markdown, markdown


def marker_json_to_markdown(value: Any, title: str) -> str:
    blocks: list[str] = []

    def walk(node: Any) -> None:
        if isinstance(node, dict):
            html_text = node.get("html")
            plain_text = node.get("text")
            if isinstance(html_text, str) and html_text.strip():
                blocks.append(clean_marker_text(html_text))
            elif isinstance(plain_text, str) and plain_text.strip():
                blocks.append(clean_marker_text(plain_text))
            for child in node.get("children") or []:
                walk(child)
        elif isinstance(node, list):
            for item in node:
                walk(item)

    walk(value)
    normalized: list[str] = []
    previous = ""
    for block in blocks:
        block = block.strip()
        if not block or block == previous:
            continue
        normalized.append(block)
        previous = block
    body = "\n\n".join(normalized).strip()
    if not body:
        return ""
    return f"# {title}\n\n{body}"


def clean_marker_text(text: str) -> str:
    text = re.sub(r"<br\s*/?>", "\n", text, flags=re.IGNORECASE)
    text = re.sub(r"</p\s*>", "\n", text, flags=re.IGNORECASE)
    text = re.sub(r"<[^>]+>", "", text)
    return html.unescape(text).replace("\r\n", "\n").replace("\r", "\n").strip()


def extract_pdf_ocr(path: Path, metadata: dict[str, Any]) -> str:
    metadata["ocr_attempted"] = True
    if not command_exists("mutool"):
        metadata["ocr_error"] = "mutool is not installed"
        return ""
    if not command_exists("tesseract"):
        metadata["ocr_error"] = "tesseract is not installed"
        return ""
    metadata["extractor"] = "mutool+tesseract"
    metadata["ocr_engine"] = "tesseract"
    page_count = pdf_page_count(path)
    pages_to_process = page_count or OCR_MAX_PAGES
    if OCR_MAX_PAGES > 0:
        pages_to_process = min(pages_to_process, OCR_MAX_PAGES)
    if pages_to_process <= 0:
        pages_to_process = 1

    OCR_TMP_ROOT.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="pdf-ocr-", dir=str(OCR_TMP_ROOT)) as temp_dir:
        pattern = str(Path(temp_dir) / "page-%03d.png")
        draw = run_command(
            ["mutool", "draw", "-q", "-r", str(OCR_DPI), "-o", pattern, str(path), f"1-{pages_to_process}"],
            timeout=max(300, pages_to_process * 45),
        )
        metadata["mutool_returncode"] = draw.returncode
        if draw.returncode != 0:
            metadata["mutool_error"] = draw.stderr[-2000:]
            return ""
        chunks: list[str] = []
        for image_path in sorted(Path(temp_dir).glob("page-*.png")):
            proc = run_command(
                [
                    "tesseract",
                    str(image_path),
                    "stdout",
                    "-l",
                    "eng+rus+hye+chi_sim+chi_tra",
                    "--psm",
                    "6",
                ],
                timeout=180,
            )
            if proc.returncode == 0 and proc.stdout.strip():
                chunks.append(proc.stdout.strip())
        metadata["ocr_pages_processed"] = len(chunks)
        return "\n\n".join(chunks)


def extract_text_document(path: Path, extension: str) -> tuple[str, str, dict[str, Any]]:
    data = path.read_bytes()[:TEXT_READ_BYTES]
    text = data.decode("utf-8", "replace").strip()
    metadata = {"extractor": "plain_text", "bytes_read": len(data), "extension": extension}
    if extension in {"md", "markdown"}:
        return truncate_text(text, MAX_TEXT_CHARS), truncate_text(text, MAX_MARKDOWN_CHARS), metadata
    return truncate_text(text, MAX_TEXT_CHARS), truncate_text("```\n" + text + "\n```", MAX_MARKDOWN_CHARS), metadata


def summarize(text: str, title: str) -> tuple[str, dict[str, Any]]:
    if not SUMMARIZE or not text.strip():
        return "", {"llm_summary_status": "skipped"}
    prompt = (
        "You summarize local archive documents for Cartolensia. "
        "Use only the provided text. Do not invent facts. "
        "Return a concise same-language summary with key people, places, dates, and topics when present.\n\n"
        f"Document title: {title}\n\n"
        f"Document text:\n{text[:SUMMARY_MAX_CHARS]}"
    )
    payload = {
        "model": OLLAMA_MODEL,
        "prompt": prompt,
        "stream": False,
        "options": {"temperature": 0.1, "num_ctx": 8192},
    }
    request = urllib.request.Request(
        OLLAMA_ENDPOINT + "/api/generate",
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=SUMMARY_TIMEOUT_SECONDS) as response:
            raw = json.loads(response.read().decode("utf-8", "replace"))
        return str(raw.get("response") or "").strip(), {
            "llm_summary_status": "ok",
            "llm_model": OLLAMA_MODEL,
            "llm_endpoint": OLLAMA_ENDPOINT,
            "llm_summary_at": now_iso(),
        }
    except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as exc:
        return "", {
            "llm_summary_status": "failed",
            "llm_model": OLLAMA_MODEL,
            "llm_error": str(exc),
        }


def store_document(row: dict[str, str], text: str, markdown: str, engine: str, page_count: int, metadata: dict[str, Any]) -> None:
    metadata = dict(metadata)
    metadata.update(
        {
            "source_url": row["url"],
            "storage_name": row["storage_name"],
            "relative_path": row["relative_path"],
            "document_extracted_at": now_iso(),
            "originals_write_policy": "read_only_originals_untouched",
        }
    )
    sql = f"""
    begin;
    insert into document_text(asset_id, page_count, title, author, text, markdown, engine, metadata_json)
    values(
        {sql_literal(row['asset_id'])}::uuid,
        {page_count},
        {sql_literal(row['display_name'])},
        '',
        {sql_literal(text)},
        {sql_literal(markdown)},
        {sql_literal(engine)},
        {json_literal(metadata)}
    )
    on conflict(asset_id) do update set
        page_count=excluded.page_count,
        title=excluded.title,
        text=excluded.text,
        markdown=excluded.markdown,
        engine=excluded.engine,
        created_at=now(),
        metadata_json=excluded.metadata_json;
    update assets
    set metadata_json = metadata_json || {json_literal({'document_text_extracted': True, 'document_text_engine': engine, 'document_text_extracted_at': now_iso()})},
        updated_at=now()
    where id={sql_literal(row['asset_id'])}::uuid;
    commit;
    """
    run_psql_file(sql)


def process_one(row: dict[str, str]) -> tuple[bool, str]:
    path = resolve_original_path(row)
    extension = row["extension"].lower()
    page_count = 0
    if extension == "pdf":
        page_count = pdf_page_count(path)
        text, markdown, metadata = extract_pdf_text(path)
        engine = metadata.get("extractor") or "pdf"
    elif extension in {"txt", "md", "markdown"}:
        text, markdown, metadata = extract_text_document(path, extension)
        engine = str(metadata.get("extractor") or "plain_text")
    else:
        return False, f"unsupported document extension {extension}"

    summary, summary_meta = summarize(text or markdown, row["display_name"])
    metadata.update(summary_meta)
    if summary:
        metadata["llm_summary"] = summary
        markdown = (markdown + "\n\n## Local LLM summary\n\n" + summary).strip()
    if not text.strip() and not markdown.strip():
        metadata["empty_document_text"] = True
    store_document(row, text, markdown, str(engine), page_count, metadata)
    return True, str(engine)


def main() -> int:
    if not Path(PSQL).exists() and shutil.which(PSQL) is None:
        raise RuntimeError(f"psql not found: {PSQL}")
    STATE_DIR.mkdir(parents=True, exist_ok=True)
    candidates = select_candidates()
    job = Job(total=len(candidates))
    counters: dict[str, int | str] = {
        "selected": len(candidates),
        "processed": 0,
        "stored": 0,
        "skipped": 0,
        "errors": 0,
        "summarize": int(SUMMARIZE),
        "reprocess_non_marker": int(REPROCESS_NON_MARKER),
        "reprocess_missing_summary": int(REPROCESS_MISSING_SUMMARY),
        "shard_count": SHARD_COUNT,
        "shard_index": SHARD_INDEX,
    }
    job.log("info", f"document backfill selected {len(candidates)} missing documents for extensions {','.join(EXTENSIONS)}")
    log(f"document backfill selected {len(candidates)} documents")
    started = time.time()
    for index, row in enumerate(candidates, start=1):
        if job.cancel_requested():
            counters["cancelled_at"] = index
            job.update(index - 1, counters, status="cancelled")
            job.log("warn", "document backfill cancelled")
            return 0
        try:
            ok, reason = process_one(row)
            counters["processed"] = int(counters["processed"]) + 1
            if ok:
                counters["stored"] = int(counters["stored"]) + 1
                log(f"{index}/{len(candidates)} stored {row['display_name']} via {reason}")
            else:
                counters["skipped"] = int(counters["skipped"]) + 1
                job.log("warn", f"{row['display_name']}: {reason}")
        except Exception as exc:  # noqa: BLE001 - production backfill should continue per-file.
            counters["processed"] = int(counters["processed"]) + 1
            counters["errors"] = int(counters["errors"]) + 1
            message = f"{row.get('display_name', row.get('asset_id', 'unknown'))}: {exc}"
            log("ERROR " + message)
            job.log("error", message[:4000])
        if index % PROGRESS_EVERY == 0 or index == len(candidates):
            elapsed = max(1.0, time.time() - started)
            counters["docs_per_hour"] = int((int(counters["processed"]) / elapsed) * 3600)
            job.update(index, counters)
    status = "succeeded" if int(counters["errors"]) == 0 else "succeeded"
    job.update(len(candidates), counters, status=status)
    job.log("info", f"document backfill finished: {json.dumps(counters, sort_keys=True)}")
    log("document backfill finished")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        log("document backfill interrupted")
        raise SystemExit(130)

# Next Interactive Preflight

Date: 2026-06-07

This preflight audited the current live real-peek app and local hardware/tooling for the next implementation pass. It did not start new real-data scans, did not reset PostgreSQL, did not pull Docker images, did not download models, and did not modify `/mnt/Models/rclone`.

## Live App State

Live service:

- URL: `http://127.0.0.1:18080`
- Config: `.cartolensia/runtime/realpeek.yaml`
- Storage: `rclone_peek`
- Root: `/mnt/Models/rclone`
- Mode: `strict_read_only`
- Real archive actions remain bounded to indexed prefixes only.

Queried endpoints:

- `GET /api/v1/stats`
- `GET /api/v1/assets?limit=5`
- `GET /api/v1/gps/tracks`
- `GET /api/v1/map/status`
- `GET /api/v1/transcoding/capabilities`
- `GET /api/v1/transcoding/presets`
- `GET /api/v1/ai/status`
- `GET /api/v1/ai/workers`
- `GET /api/v1/settings`
- `GET /api/v1/settings/schema`
- `GET /api/v1/storages`
- `GET /api/v1/search?q=jpg`
- `GET /api/v1/jobs?limit=20`

Current counts:

- Assets: `54`
- Locations: `54`
- Photos: `48`
- Videos: `2`
- Track files: `4`
- Parsed GPS/KML summaries: `4`
- Hashed: `54`
- Unhashed: `0`
- Geotagged photos: `48`
- Duplicate groups: `0`
- Total bytes: `619580406`

Jobs:

- The latest 20 jobs are terminal states; no running job was observed.
- Recent scoped `Cartolensia-photos` discovery jobs hit the explicit `max_files=50` bound and report `complete=false`; missing marking remained disabled.
- Historical 0-target hash/preview jobs remain in job history. This is not active, but the next implementation should make 0-target jobs rejected or reported as no-op with explicit reason.

Storage:

- `/api/v1/storages` returns one storage:
  - name `rclone_peek`
  - kind `fs`
  - root `/mnt/Models/rclone`
  - mode `strict_read_only`
- There is no runtime add/edit storage API yet; the WebUI only displays and drafts pending YAML.

Map and tracks:

- `/api/v1/map/status` reports PostGIS available, `48` geotagged assets, `4` tracks, `screen_distance` clustering, and the Cartolensia OSM tile proxy.
- `/api/v1/map/tracks` returns large simplified GeoJSON, but GPS Track Manager still shows a table plus basic selected-track panel rather than a dedicated route/detail page with charts.
- Track summaries include KML records with synthetic timestamps because the source KML geometry was salvage-parsed from truncated XML. GPX records have real timestamps.

Transcoding:

- ffmpeg and ffprobe are available to the app.
- App capability detection reports `h264_nvenc`, `hevc_nvenc`, `av1_nvenc`, `h264_vaapi`, `hevc_vaapi`, `av1_vaapi`, QSV encoders, and CPU encoders.
- Presets:
  - `original`, built-in, available.
  - `h264_720p_lan`, built-in CPU/libx264, available.
  - `h264_low_bitrate`, built-in CPU/libx264, available.
  - `av1_low_bitrate`, built-in, disabled with browser-safe AV1 reason.
  - `nv-750k`, custom, NVIDIA/h264/h264_nvenc, available according to static validation.
- Existing validation checks encoder names and coarse hardware hints, but it does not run an actual ffmpeg dry-run against the selected hardware/encoder. This likely explains the custom NVIDIA preset failure.

AI:

- `/api/v1/ai/status` reports `enabled=false`, `inference_running=false`, vector store `not_configured`, model cache `.cartolensia/models`, and model downloads explicit only.
- `/api/v1/ai/workers` reports CPU/NVIDIA/ROCm/Intel worker profiles but all are `not_configured`.
- The dummy worker exists at `services/ai/worker.py`, but it is not packaged as an importable `python -m cartolensia_ai.server` module yet.

Settings schema:

- `/api/v1/settings/schema` covers runtime settings for indexing, preview, map, and transcoding plus restart-required pending settings for metadata, GPS, preview, map, and transcoding.
- Settings still need better visual separation, margin/layout cleanup, and stricter per-tab UI coverage.
- Preview policy is ambiguous: runtime defaults currently include `indexing.previews_after_index=true`, which conflicts with the desired on-the-fly-by-default/no-write-amplification policy.

Search:

- `/api/v1/search?q=jpg` returns `48` results with match explanations.
- Current search is an MVP; it should become the central Explorer search with structured parsing, ranking, filters, and database-backed queries.

## Local Hardware And Tooling

Host:

- Kernel: `Linux avx512 6.8.0-100-lowlatency ... x86_64`
- CPU: AMD Ryzen 9 7900X, `24` threads, AVX2/AVX512 available.
- PCI GPUs:
  - NVIDIA GA102 GeForce RTX 3090 Ti.
  - AMD/ATI Raphael integrated GPU.

NVIDIA:

- `nvidia-smi` works.
- Driver: `570.124.06`.
- CUDA runtime reported by driver: `12.8`.
- GPU: NVIDIA GeForce RTX 3090 Ti, `23028 MiB`.
- GPU load at probe time: idle.

AMD/iGPU/VAAPI:

- `lspci` shows AMD/ATI Raphael integrated GPU.
- `/dev/dri` was not present in the shell environment.
- `vainfo` is not installed.
- App capability endpoint currently reports `dev_dri=true`, `vaapi=true`, `qsv=true`; this conflicts with the shell probe. The next implementation should make backend hardware detection report both encoder availability and actual device-node availability separately.

ffmpeg/ffprobe:

- ffmpeg: `6.1.1-3ubuntu5`.
- ffprobe: `6.1.1-3ubuntu5`.
- Hardware acceleration methods advertised: `vdpau`, `cuda`, `vaapi`, `qsv`, `drm`, `opencl`, `vulkan`.
- Encoders advertised:
  - NVIDIA: `h264_nvenc`, `hevc_nvenc`, `av1_nvenc`.
  - VAAPI: `h264_vaapi`, `hevc_vaapi`, `av1_vaapi`, `mjpeg_vaapi`, `mpeg2_vaapi`, `vp8_vaapi`, `vp9_vaapi`.
  - QSV: `h264_qsv`, `hevc_qsv`, `av1_qsv`, `mjpeg_qsv`, `mpeg2_qsv`, `vp9_qsv`.
  - CPU: `libx264`, `libx265`, `libsvtav1`, `libaom-av1`, `librav1e`.
- Feasible next validation:
  - Native NVIDIA dry-run should be feasible.
  - VAAPI/QSV should be disabled or marked unverified until `/dev/dri` exists or a configured device path is supplied.

Docker:

- Docker CLI: `29.2.1`.
- Docker Compose: `v5.0.2`.
- Initial non-escalated Docker API reads were denied by the sandbox.
- Escalated read-only `docker info` and `docker images` were approved and run.
- Docker runtimes reported only `runc` / `io.containerd.runc.v2`; no `nvidia` runtime was shown.
- Local images include `debian:bookworm-slim`, `postgis/postgis:16-3.4`, `rust`, and unrelated images. No CUDA/PyTorch/ROCm/Intel AI image was local.
- No Docker image was pulled and no container was started.

Python:

- Python: `3.12.3`.
- Native imports present: `PIL`.
- Native imports missing: `torch`, `torchvision`, `fastapi`, `uvicorn`, `cv2`, `onnxruntime`.
- Native AI implementation will require installing dependencies in a virtual environment or container image after approval.

Frontend packages:

- `ol` `10.9.0`, license `BSD-2-Clause`.
- `bootstrap` `5.3.8`, license `MIT`.
- `bootstrap-icons` `1.13.1`, license `MIT`.
- `hls.js`, license `Apache-2.0`.
- No additional chart package is required for the next pass if track profiles are implemented as plain SVG/canvas components.

## Diagnosis Summary

- Track Manager already has backend summaries and point APIs, but the UI lacks a proper detail route with map/profile charts.
- Track media during-track query likely returns track assets because the current UI/API call does not filter out track media kinds or because the selected KML synthetic timestamps overlap only track-like assets. The next pass should add `media_kind`/`exclude_track_assets` parameters and default to photo/video only.
- Custom NVIDIA preset failure is likely not a model/hardware absence problem. The machine has RTX 3090 Ti and ffmpeg advertises NVENC. The missing piece is profile-specific ffmpeg command validation, stderr display, and correct NVENC options for bitrate/scale/HLS.
- Asset detail uses the cached preview first for photos, which is visibly low resolution. The next pass should use original-quality viewing for detail, plus view-size/on-demand preview variants to avoid always loading full originals.
- Runtime storage configuration does not exist yet. Any implementation must keep `/mnt/Models/rclone` locked to strict read-only unless an explicit confirmation phrase and tested protection model exist.
- Preview generation defaults currently lean toward persistent generation. The desired policy is on-the-fly default with optional persistent generation.
- Docker GPU support is not ready: host NVIDIA is available, but Docker runtime lacks `nvidia` in `docker info`. Native GPU path should be implemented first.

## Approvals Required Before Long Implementation

No approval is needed to implement Go/Vue code, docs, tests, or local dummy AI contracts.

Approval needed before installing dependencies:

- Python virtual environment creation and dependency installation, e.g. `python3 -m venv .cartolensia/ai-venv` and `.cartolensia/ai-venv/bin/pip install ...`.
- Docker image pulls or builds that require missing base images, especially CUDA/PyTorch/ROCm/Intel images.
- Model downloads into `.cartolensia/models`.
- Optional frontend chart dependency if plain SVG is rejected later. Current recommendation: no chart dependency.

Approval needed before hardware tests:

- Short native ffmpeg NVENC dry-run on the already indexed 7-second video asset, outputting only to `/tmp` or `.cartolensia/realpeek-cache/transcode-test`.
- Docker `--gpus all` probe if a local CUDA image is later present or after approving a small CUDA runtime pull.

Approval explicitly not requested in this preflight:

- No large model downloads.
- No CUDA/PyTorch/ROCm image pulls.
- No long transcode jobs.
- No new real-data scans.
- No DB reset.

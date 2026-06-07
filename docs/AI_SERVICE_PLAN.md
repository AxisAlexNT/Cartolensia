# AI Service Plan

This plan prepares the next implementation pass for real AI sidecars while preserving Cartolensia's safety rules. It does not approve model downloads by itself.

## Goals

- Keep all AI model files, caches, temporary images, and outputs outside `/mnt/Models/rclone`.
- Support native and Docker deployment.
- Make every service entrypoint explicit as `server`.
- Implement useful dummy/no-model behavior first, then enable real models only after dependency/model approval.
- Store AI outputs in Cartolensia metadata tables through backend jobs, not in original media folders.

## Service Layout

Target package:

```text
services/ai/
  pyproject.toml
  README.md
  cartolensia_ai/
    __init__.py
    server.py
    config.py
    schemas.py
    image_io.py
    models/
      __init__.py
      dummy.py
      classification.py
      faces.py
      captions.py
      embeddings.py
  requirements-cpu.txt
  requirements-nvidia.txt
  requirements-rocm.txt
  requirements-intel.txt
```

Required native entrypoint:

```bash
python -m cartolensia_ai.server --host 127.0.0.1 --port 19090
```

Docker entrypoint:

```bash
python -m cartolensia_ai.server --host 0.0.0.0 --port 8090
```

Environment:

- `CARTOLENSIA_AI_PROFILE=cpu|nvidia|rocm|intel`
- `CARTOLENSIA_MODEL_CACHE_DIR=.cartolensia/models` for native
- `CARTOLENSIA_MODEL_CACHE_DIR=/models` for Docker
- `CARTOLENSIA_AI_DUMMY=1` default until models are configured
- `CARTOLENSIA_AI_DEVICE=cpu|cuda|rocm|xpu|auto`

## HTTP Contract

Existing dummy endpoints become stable:

- `GET /health`
- `GET /capabilities`
- `POST /classify-image`
- `POST /detect-faces`
- `POST /describe-image`
- `POST /embed-image`

Request policy:

- The backend should pass temporary cache-scoped file paths or signed/read-only media URLs, never write into originals.
- Each request includes `asset_id`, `media_url` or `cache_input_path`, requested model namespace/version, and output policy.
- Each endpoint returns `status`, `model_state`, `predictions`, `warnings`, and `timings`.

Dummy behavior:

- Always returns `202 not_configured` with empty predictions.
- Does not download models.
- Does not touch real archive storage.

## Backend Integration

Packages likely to change:

- `internal/server`
- `internal/catalog`
- `internal/database`
- `internal/jobs`
- `internal/workers`
- `internal/config`

APIs to add/harden:

- `GET /api/v1/ai/workers`
- `GET /api/v1/ai/workers/{id}`
- `POST /api/v1/ai/jobs/classify`
- `POST /api/v1/ai/jobs/faces`
- `POST /api/v1/ai/jobs/describe`
- `POST /api/v1/ai/jobs/embed`
- `GET /api/v1/ai/jobs/{id}`
- `GET /api/v1/ai/predictions?asset_id=...`
- `GET /api/v1/search` integration for `tag:`, `category:`, `caption:`, and embedding status.

Jobs:

- `ai_classify`
- `ai_detect_faces`
- `ai_describe`
- `ai_embed`

Safety:

- Jobs accept selected assets, albums, or explicit bounded scopes only.
- No AI job runs against `storage=all` real archive scope.
- Backend should copy/decode input into a cache/work path when the sidecar cannot read original storage.
- Temporary inputs are cleaned after job completion.

## Schema

Existing migration foundation:

- `asset_tags`
- `ai_predictions`
- `face_detections`
- `face_clusters`
- `user_preferences`

Next migration additions if needed:

- `ai_models(id, namespace, version, modality, provider, license, cache_path, status, metadata_json)`
- `ai_worker_status(worker_id, profile, endpoint, status, last_seen_at, capabilities_json, error)`
- `asset_embeddings(asset_id, model_id, vector_json, dimensions, created_at)` as JSON fallback until a real `VectorStore` is configured.

Do not require pgvector for startup.

## Docker Profiles

Current Compose profiles exist but all use the dummy CPU Dockerfile. Target:

- `ai-cpu`: CPU PyTorch/ONNX runtime.
- `ai-nvidia`: CUDA/PyTorch image with GPU device reservations.
- `ai-rocm`: ROCm/PyTorch image for AMD GPU when supported.
- `ai-intel`: Intel/XPU or OpenVINO profile.

Do not pull or build heavy images without approval.

Current Docker probe:

- Docker CLI and Compose are installed.
- Docker runtimes show only `runc`; no `nvidia` runtime is configured.
- No CUDA/PyTorch/ROCm image is local.

Next implementation should:

- Keep Docker profiles syntactically correct.
- Add GPU runtime/device reservations behind profiles.
- Add docs explaining that NVIDIA Docker Toolkit is required before `ai-nvidia` works.
- Provide native NVIDIA path first, because host `nvidia-smi` works.

## Proposed Dependencies

No dependency installation is approved by this plan.

Base native/server dependencies:

- `fastapi`
  - License: commonly MIT, must verify from installed package metadata before use.
  - Needed for robust HTTP API, OpenAPI docs, async request handling.
  - Optional alternative: keep stdlib server for dummy mode, but real multipart/streaming is easier with FastAPI.
- `uvicorn`
  - License: commonly BSD-3-Clause, must verify locally before use.
  - Needed to run FastAPI.
- `pillow`
  - Already importable as `PIL`.
  - Needed for image decode/resize.
- `numpy`
  - License commonly BSD-style, must verify.
  - Needed by image/model pipelines.

PyTorch dependencies:

- `torch`
  - License commonly BSD-style, must verify from package metadata.
  - Native import is currently missing.
  - Required for torchvision/CLIP-style local inference.
- `torchvision`
  - License commonly BSD-style, must verify.
  - Native import is currently missing.
  - Candidate for image classification and transforms.

Alternative/optional dependencies:

- `onnxruntime`
  - License commonly MIT, must verify.
  - Useful for CPU face detection/classification with ONNX models.
- `opencv-python-headless`
  - License commonly Apache-2.0 for package/project components but can have codec caveats; must verify carefully.
  - Useful for face detection/classical image operations.
  - Optional; avoid unless needed.

## Proposed Model Paths

Do not download models during preflight.

Recommended staged approach:

1. Dummy/no-model mode.
   - No download.
   - Tests exercise end-to-end backend/job/sidecar flow.
2. Classification MVP.
   - Candidate: torchvision ResNet/MobileNet/EfficientNet weights.
   - Estimated size: tens of MB depending on model.
   - License/provenance: must verify model weight terms before download; code package license is not enough.
   - Works offline after cache.
3. Face detection MVP.
   - Candidate A: OpenCV Haar/LBP cascade if license/provenance can be verified and package supplies it.
   - Candidate B: ONNX face detector with explicit compatible license.
   - Candidate C: defer real faces and keep dummy if model licensing is unclear.
   - Size: likely under 50 MB for small detectors, but must verify.
4. Captioning/description.
   - Candidate: small BLIP/ViT-GPT2-style model is likely hundreds of MB or more.
   - Recommendation: defer real captioning until classification/face pipeline is stable.
5. Embeddings/search.
   - Candidate: CLIP-compatible model.
   - Size: likely hundreds of MB.
   - License/provenance and vector store plan must be verified before download.

Cache location:

- Native: `.cartolensia/models`
- Docker: bind `.cartolensia/models:/models`
- Never `/mnt/Models/rclone`

## WebUI

Pages/components likely to change:

- `webui/src/App.vue`
- `webui/src/api.ts`
- Optional split into `webui/src/pages/BaseAIPage.vue` later.

UI additions:

- Worker connection cards: native endpoint and Docker profile status.
- Model cache status and disk usage.
- Approval-required model download buttons disabled by default.
- Run classification/face/description on selected assets only.
- Predictions/tags table on asset detail and AI Classification page.
- Search filters for AI categories/tags/captions.

## Tests

Required:

- Python dummy worker unit smoke.
- Backend worker registry with not-configured state.
- Backend AI job no-worker path.
- Dummy worker HTTP contract.
- Prediction storage roundtrip.
- Search by tag/category after synthetic inserted predictions.
- Frontend build.

Optional after dependency approval:

- CPU model smoke on synthetic image under `/tmp`.
- Native NVIDIA torch CUDA probe.
- Docker GPU probe with approved image.

## Approval Checklist

Before the long run can install/run real AI:

- Approve Python dependency installation into `.cartolensia/ai-venv` or Docker image build.
- Approve any Docker base image pull.
- Approve any model download, with model name, license, size, and cache location.
- Approve native GPU smoke if it runs real inference or a CUDA tensor test.
- Approve Docker `--gpus all` probe if a CUDA image must be pulled.

## 2026-06-07 Implementation Status

Implemented:

- Packaged dummy sidecar under `services/ai/cartolensia_ai`.
- Native server entrypoint:
  - `python -m cartolensia_ai.server --host 127.0.0.1 --port 19090`
- Docker entrypoint:
  - `python -m cartolensia_ai.server --host 0.0.0.0 --port 8090`
- Endpoints:
  - `GET /health`
  - `GET /capabilities`
  - `POST /classify-image`
  - `POST /detect-faces`
  - `POST /describe-image`
  - `POST /embed-image`
- Behavior:
  - returns `not_configured`/`model_missing`;
  - does not download models;
  - does not run inference;
  - does not write to original media roots.
- Backend `/api/v1/ai/workers` now probes `http://127.0.0.1:19090/health` and reports the local dummy worker when it is running.
- `.cartolensia/ai-venv` was created and populated with the approved lightweight dependencies only:
  - `fastapi`;
  - `uvicorn`;
  - `numpy`.

Still gated:

- Torch/torchvision installation.
- CUDA/ROCm/Intel AI containers.
- Model downloads.
- Real classification, face detection, captions, and embeddings.

Safety:

- Model/cache location remains `.cartolensia/models`.
- No model files were downloaded.
- Nothing was written under `/mnt/Models/rclone`.

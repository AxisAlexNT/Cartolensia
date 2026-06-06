# Plugin Model

Cartolensia is plugin-oriented, but the MVP should keep plugin execution simple and safe. The first plugin implementation is manifest discovery, dependency validation, and built-in stub registration.

## MVP Plugin Runtime

- Built-in Go plugin descriptors compiled into the backend.
- Optional filesystem manifests at `plugins/<id>/plugin.yaml`.
- Dependency topological sort before registration.
- Backend feature flags and WebUI extension metadata exposed through the core API.
- No dynamic Go `.so` loading.
- No sidecar process execution in the MVP.

## Manifest Shape

Implemented manifest fields:

```yaml
id: example-sidecar
name: Example Sidecar
version: 0.1.0
description: User-managed HTTP sidecar example.
depends_on:
  - ai-base
runtime: sidecar_http
status: loaded
capabilities:
  - embeddings.generate
permissions:
  - read
  - media:read
sidecar_http:
  base_url: http://127.0.0.1:19090
  health_path: /health
```

Validation rules:

- `id` uses lowercase letters, numbers, dots, and hyphens.
- `version` is required and should be semver-compatible.
- dependencies must exist.
- dependency graph must be acyclic.
- duplicate plugin IDs are invalid.
- empty runtime defaults to `builtin`.
- supported runtimes are currently `builtin` and `sidecar_http`.
- `sidecar_http` requires `sidecar_http.base_url`; `health_path` defaults to `/health`.

## Planned Built-In Stubs

- `albums`: database-backed virtual album grouping skeleton.
- `mapview`: map-first media browsing and clustering skeleton.
- `gpstracks`: track ingestion, linking, and live video-track sync skeleton.
- `transcoding`: safe transcoding manager skeleton; never writes into originals.
- `ai-base`: AI runtime and future VectorStore skeleton.
- `ai-classification`: transport/place classification workflow skeleton, depends on `ai-base`.

The current backend exposes these through:

- `GET /api/v1/plugins`
- `GET /api/v1/plugins/{id}`
- `GET /api/v1/plugins/{id}/health`
- `POST /api/v1/plugins/rescan`

`POST /api/v1/plugins/rescan` reloads built-in and filesystem manifests from `plugins/<id>/plugin.yaml` and is treated as a write-like endpoint by auth. `GET /api/v1/plugins/{id}/health` reports built-ins as loaded. Sidecar health is currently a stub and reports that active sidecar probing is not implemented; the core does not contact arbitrary sidecar URLs yet.

## Future Runtime Types

Sidecar HTTP plugins are the next planned runtime once core contracts stabilize. Sidecar plugins should communicate with core through authenticated local APIs, scoped API tokens, and explicit capability manifests, not direct uncontrolled database access.

Sidecar safety rules:

- Cartolensia does not auto-start arbitrary plugin binaries.
- Sidecars are user-managed services.
- Core-to-sidecar calls will use configured secrets or scoped API tokens.
- Sidecar permissions must be declared in the manifest and enforced by core before write-like actions.
- WebUI plugin assets are future static bundles; the current UI renders status/config information from manifest data only.

Sidecar gRPC can be considered later for higher-throughput plugin calls.

Go `.so` plugins are experimental developer-mode only and should not be the default distribution mechanism.

## WebUI Extensions

Plugin WebUI assets may later live under `plugins/<id>/webui/dist/` and be served as static bundles after manifest validation. For now, plugin navigation entries, detail pages, health state, dependencies, capabilities, and unavailable states are generated from backend plugin descriptors.

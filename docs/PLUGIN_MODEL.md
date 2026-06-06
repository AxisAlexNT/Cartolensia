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

Initial manifest fields:

```yaml
id: core.discovery
name: Discovery
version: 0.1.0
description: Indexes configured read-only storage roots.
depends_on:
  - core.explorer
backend:
  kind: builtin
webui:
  mount: discovery
```

Validation rules:

- `id` uses lowercase letters, numbers, dots, and hyphens.
- `version` is required and should be semver-compatible.
- dependencies must exist;
- dependency graph must be acyclic;
- duplicate plugin IDs are invalid.

## Planned Built-In Stubs

- `albums`: database-backed virtual album grouping skeleton.
- `mapview`: map-first media browsing and clustering skeleton.
- `gpstracks`: track ingestion, linking, and live video-track sync skeleton.
- `transcoding`: safe transcoding manager skeleton; never writes into originals.
- `ai-base`: AI runtime and future VectorStore skeleton.
- `ai-classification`: transport/place classification workflow skeleton, depends on `ai-base`.

The current backend exposes these through `GET /api/v1/plugins`. `POST /api/v1/plugins/rescan` reloads built-in and filesystem manifests from `plugins/<id>/plugin.yaml`.

## Future Runtime Types

Sidecar HTTP plugins are the next planned runtime once core contracts stabilize. Sidecar plugins should communicate with core through authenticated local APIs, not direct uncontrolled database access.

Sidecar gRPC can be considered later for higher-throughput plugin calls.

Go `.so` plugins are experimental developer-mode only and should not be the default distribution mechanism.

## WebUI Extensions

Plugin WebUI assets may later live under `plugins/<id>/webui/dist/`. For MVP, plugin navigation entries and unavailable states can be generated from backend plugin descriptors.

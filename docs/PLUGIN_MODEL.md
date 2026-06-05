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

- `core.explorer`: file/folder browsing surface.
- `core.discovery`: discovery and hash job orchestration.
- `media.map`: map browsing placeholder.
- `tracks.manager`: GPS track manager placeholder.
- `video.transcode`: transcoding manager placeholder.
- `ai.base`: AI hardware and model manager placeholder.
- `ai.classification`: classification workflow placeholder.

## Future Runtime Types

Sidecar HTTP plugins are the next planned runtime once core contracts stabilize. Sidecar plugins should communicate with core through authenticated local APIs, not direct uncontrolled database access.

Sidecar gRPC can be considered later for higher-throughput plugin calls.

Go `.so` plugins are experimental developer-mode only and should not be the default distribution mechanism.

## WebUI Extensions

Plugin WebUI assets may later live under `plugins/<id>/webui/dist/`. For MVP, plugin navigation entries and unavailable states can be generated from backend plugin descriptors.

# Cartolensia AI Sidecar

This optional service implements the HTTP contract used by Cartolensia AI
plugins. The current implementation is a dummy/no-model worker: it proves the
runtime, health checks, and request/response shapes without downloading model
weights or running inference.

Run natively after installing the package into a local virtual environment:

```bash
python -m cartolensia_ai.server --host 127.0.0.1 --port 19090
```

Docker profiles use the same entrypoint with `--host 0.0.0.0 --port 8090`.
Model and cache directories must stay outside original media roots.


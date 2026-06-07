# Cartolensia AI Sidecar

This optional service implements the HTTP contract used by Cartolensia AI
plugins. It supports two modes:

- `dummy`: verifies health checks and request/response shapes without loading
  models.
- `auto`/local inference: lazy-loads approved local models from the configured
  cache and runs classification, face detection, safety classification,
  captioning, and image/text embeddings.

Run natively after installing the package into a local virtual environment:

```bash
python -m cartolensia_ai.server --host 127.0.0.1 --port 19090
```

Docker profiles use the same entrypoint with `--host 0.0.0.0 --port 8090`.
Model and cache directories must stay outside original media roots.

The sidecar accepts only localhost media URLs or safe local temporary/cache
paths. It never writes to original media roots and never uses remote inference
APIs. Model files should be cached under `.cartolensia/models` for native runs
or a mounted `/models` directory for Docker runs.

# Offline Components

Cartolensia uses a Component Manager so optional tools can be supplied without Internet access.

## What Can Be Provided Offline

- ffmpeg / ffprobe
- Tesseract OCR
- OCR language packs
- Python AI runtime
- approved AI model caches
- Marker / PDF tooling
- offline map data bundles
- PostgreSQL runtime, if you choose the bundled database path

## Component Manager Actions

On Settings -> Components, each card provides:

- download instructions;
- archive import;
- directory/path import;
- installation checks;
- enable/disable toggles;
- logs and provenance notes.

## Safe Import Rules

Accepted input types:

- directory
- `.tar`
- `.tar.gz`
- `.zip`
- `.7z` when the extractor is available locally

Safe extraction rules:

- reject absolute paths;
- reject `..`;
- reject symlink escapes;
- extract only under the configured component root.

## Where To Place Things

Suggested component locations:

- `/var/lib/cartolensia/components`
- `/var/lib/cartolensia/models`
- `/var/lib/cartolensia/cache`

These paths are writable and live outside `/originals`.

## Where To Get The Files

Download the desired tools on an Internet-connected machine, verify the license/provenance, then transfer the files to the air-gapped host.

Documented examples:

- ffmpeg / ffprobe build archives;
- Tesseract binaries and tessdata packages;
- PyTorch wheels and the repo-local Python runtime;
- approved model caches for EfficientNet-B0, MobileNetV3, YuNet, Falconsai NSFW, OpenCLIP ViT-B/32, BLIP base, and faster-whisper small;
- PMTiles/MBTiles or a self-hosted map cache bundle.

## Not Bundled By Default

The production release does not automatically bundle:

- GPU drivers;
- public map tiles;
- public geocoders;
- unreviewed model weights;
- nonfree FFmpeg builds.

If a component is missing, Cartolensia should show it as missing rather than silently downloading it.

## Private Full Bundle

For machines that may never have Internet access, build the private tar.zst bundle on a connected staging host:

```bash
cp config/local-full-tarzst-build.env.example config/local-full-tarzst-build.env
bash scripts/release/build-local-full-tarzst.sh config/local-full-tarzst-build.env
```

The extracted bundle includes `bin/start-cartolensia`, `bin/start-postgres`, `bin/start-ai-executor`, `bin/start-transcode-node`, `bin/backup-db`, and `bin/diagnose`.

For a remote AI node, start the selected executor flavor on that node:

```bash
./bin/start-ai-executor cpu-avx2 0.0.0.0 19090
```

Then point the main node at it with `config/remote-executors.local.env`:

```bash
CARTOLENSIA_AI_WORKER_ENDPOINT=http://ai-node:19090
```

GPU executor flavors still require host GPU drivers and container/device passthrough. The archive can bundle Python packages and model files, but it cannot bundle kernel drivers, NVIDIA modules, ROCm host support, Intel `/dev/dri` access, or Proxmox LXC device permissions.

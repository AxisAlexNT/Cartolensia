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

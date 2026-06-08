# Air-Gapped Install

This guide assumes the target machine has no Internet access.

## What You Need

Bring a release archive built on an Internet-connected machine:

- Cartolensia offline `.7z` archive;
- optional reviewed component archives for ffmpeg, Tesseract, Python AI runtime, models, or map tiles;
- PostgreSQL runtime if you want the bundled database path;
- a copy of `.env.production` or a machine-specific config file.

Do not place any of these under `/originals`.

## Extract

Extract the release bundle to a writable installation directory, for example `/opt/cartolensia`.

The extracted layout contains:

- `bin/cartolensia`
- `webui/dist/`
- `config/production.yaml`
- `config/production-container.yaml`
- `config/offline-airgap.yaml`
- `start-cartolensia.sh`
- `stop-cartolensia.sh`
- `licenses/`
- `components-manifest.json`

## Mount Originals

Mount the archive read-only at `/originals`.

Supported source types:

- local filesystem
- NFS
- SMB/CIFS
- mounted object-storage view

The app never writes to `/originals`.

## Component Workflow

Use Settings -> Components after first launch.

For each missing optional tool or model, you can:

- read download instructions;
- provide a local archive;
- provide a directory/path;
- import an offline bundle;
- verify installation;
- enable or disable the component.

Supported archive types for import:

- `.tar`
- `.tar.gz`
- `.zip`
- `.7z` when an extractor is available
- directories

Component imports are extracted only under the configured component directory, such as:

- `/var/lib/cartolensia/components`
- `/var/lib/cartolensia/models`

Archives with absolute paths, `..`, symlink escapes, or other unsafe entries are rejected.

## Offline Maps

Cartolensia does not bulk-prefetch public map data.

For offline map viewing, provide one of:

- PMTiles;
- MBTiles;
- a self-hosted tile service;
- a user-provided cache bundle.

## No Internet Assumptions

The offline release does not require:

- CDN fonts or icons;
- public map tiles;
- public geocoding APIs;
- model downloads;
- OCR language pack downloads;
- ffmpeg or PostgreSQL package downloads;
- Python package downloads.

If something is missing, the UI should show the component as missing and explain how to provide it locally.

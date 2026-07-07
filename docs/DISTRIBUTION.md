# Offline Distribution

Cartolensia can build a Linux x86_64 offline archive intended for machines with no Internet access. The distribution is assembled from normal package-manager inputs and includes a license/notice bundle so redistribution decisions are auditable.

## Goals

- Include the Cartolensia Go backend binary.
- Include the built Vue WebUI.
- Include launcher scripts and default offline configs.
- Include production-ready config templates for `/originals` deployments.
- Include a production compose file and `.env.production.example`.
- Include optional local runtime tools:
  - ffmpeg and ffprobe;
  - Tesseract OCR and installed language data;
  - PostgreSQL runtime binaries for a local metadata database;
  - Python runtime and AI sidecar packages.
- Include an AGPL source snapshot.
- Produce a `.7z` archive and `.sha256` checksum for the normal offline release path.
- Produce a private/local `.tar.zst` full bundle when explicitly built on an Internet-connected operator machine.

## Non-Goals And Limits

- GPU drivers are not bundled. GPU AI can only work on hosts that already have compatible kernel/GPU drivers.
- Public map tiles, public geocoding data, and paid/proprietary codecs are not bundled automatically.
- Model weights are bundled only when explicitly requested and when a license-reviewed model cache is available.
- Elasticsearch/OpenSearch are not bundled.
- The archive does not assume Internet access for fonts, icons, map tiles, OCR language packs, ffmpeg, Tesseract, PostgreSQL tools, or Python dependencies.

## Production Target

Production deployments are expected to mount the original archive at `/originals` and keep it read-only.

Default writable locations live outside the archive, for example:

- `/var/lib/cartolensia/cache`
- `/var/lib/cartolensia/models`
- `/var/lib/cartolensia/components`
- `/var/lib/cartolensia/exports`

The production templates shipped with the release bundle are:

- `config/production.yaml`
- `config/production-container.yaml`
- `config/offline-airgap.yaml`
- `.env.production.example`
- `docker-compose.production.yml`

## Local Build

Install normal build prerequisites on the build machine:

- Go 1.22+;
- Node.js and npm;
- Python 3.11+ if bundling AI;
- `7z`/`p7zip`;
- optional `ffmpeg`, `tesseract`, OCR language packs, and PostgreSQL packages.

Build a compact runtime/OCR-capable archive:

```bash
CARTOLENSIA_DIST_AI_FLAVOR=runtime \
CARTOLENSIA_DIST_INCLUDE_TOOLS=1 \
CARTOLENSIA_DIST_INCLUDE_POSTGRES=1 \
bash scripts/dist/build-offline-linux.sh
```

Build a CPU AI archive:

```bash
CARTOLENSIA_DIST_AI_FLAVOR=cpu \
CARTOLENSIA_BUNDLED_PYTHON_ROOT="$pythonLocation" \
bash scripts/dist/build-offline-linux.sh
```

Bundle a reviewed local model cache:

```bash
CARTOLENSIA_DIST_INCLUDE_MODELS=1 \
CARTOLENSIA_MODELS_DIR=.cartolensia/models \
bash scripts/dist/build-offline-linux.sh
```

Build a full local bundle from pre-downloaded official tools, runtimes, and model caches:

```bash
cp config/local-full-build.env.example config/local-full-build.env
$EDITOR config/local-full-build.env
bash scripts/release/build-local-full.sh
```

The local full-bundle config is intended for an Internet-connected staging machine. It points at safe, official source roots and local extraction directories so you can download or unpack reviewed tools first, then create the final 7z archive without relying on GitHub release publication.

The output appears under `dist/`:

- `cartolensia-<version>-linux-x86_64-offline.7z`
- `cartolensia-<version>-linux-x86_64-offline.7z.sha256`
- `cartolensia-<version>-linux-x86_64-offline-RELEASE_NOTES.md`

## Private Local Full `tar.zst` Bundle

For a self-contained private bundle that is not uploaded to GitHub Releases, use the local full tar.zst builder. This path is intended for an Internet-connected staging machine and can download/copy reviewed external payloads into one archive:

- Cartolensia backend binary and built WebUI;
- production configs and launcher/maintenance scripts;
- BtbN FFmpeg GPL shared Linux x86_64 build from:
  `https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-linux64-gpl-shared.tar.xz`;
- Tesseract executable and detected tessdata from the build host;
- PostgreSQL server/client binaries from the configured `pg_config`;
- Python AI executor environments for `cpu-avx2`, `cpu-avx512`, `nvidia-cu128`, `intel-arc`, and `rocm-radeon`;
- reviewed AI model cache, including torchvision classifiers, YuNet, Falconsai NSFW, OpenCLIP, BLIP, and faster-whisper small when model preparation is enabled;
- reviewed music components when enabled:
  - Basic Pitch in an isolated Python 3.11 component with TensorFlow-compatible NumPy/Protobuf pins for compact MIDI transcription;
  - Demucs for on-demand vocals/drums/bass/other separation, writing compressed FLAC stems by default instead of uncompressed WAV;
- optional operator-provided offline map bundle.

Demucs does not provide reliable piano, vibraphone/glockenspiel, reed, brass,
or string audio stems in the default Cartolensia profile. Those instrument
classes are represented through MIDI transcription when available. A future
multi-instrument stem provider can be added as a reviewed component without
changing originals.

Build:

```bash
cp config/local-full-tarzst-build.env.example config/local-full-tarzst-build.env
$EDITOR config/local-full-tarzst-build.env
bash scripts/release/build-local-full-tarzst.sh config/local-full-tarzst-build.env
```

Quick smoke build without external downloads:

```bash
bash scripts/release/smoke-local-full-tarzst.sh
```

The full bundle output is:

```text
dist/release/cartolensia-...-full.tar.zst
dist/release/cartolensia-...-full.tar.zst.sha256
```

For a private 7-Zip bundle, run:

```bash
bash scripts/release/build-local-full-7z.sh config/local-full-tarzst-build.env
```

The 7-Zip wrapper emits:

```text
dist/release/cartolensia-local-full-linux-x86_64-<version>.7z
dist/release/cartolensia-local-full-linux-x86_64-<version>.7z.sha256
```

Extract on the target host:

```bash
tar --zstd -xf cartolensia-...-full.tar.zst
cd cartolensia-.../
export CARTOLENSIA_ADMIN_PASSWORD='replace-with-a-long-secret'
./bin/first-run
./bin/status
```

Mount originals read-only at `/originals` before indexing:

```bash
mkdir -p /originals
# mount local/NFS/SMB/object-storage view read-only according to the host policy
```

The archive is self-contained after extraction, but the target host still must provide kernel/device access for GPUs. Linux GPU drivers, `/dev/dri`, NVIDIA driver modules, ROCm kernel support, and LXC device passthrough cannot be bundled into a tarball. If GPU access is unavailable, use the CPU executor flavor.

### Boot-Managed Remote Host

For a VM, bare-metal host, or unprivileged LXC where the bundle should run on
boot, create the dedicated runtime account and systemd units once from a
workstation that can SSH to the host:

```bash
ssh <admin-user>@<remote-host> "sudo env CARTOLENSIA_PUBKEY='$(cat ~/.ssh/cartolensia_remote_ed25519.pub)' bash -s" \
  < scripts/remote/bootstrap-cartolensia-user.sh
```

The bootstrap creates the `cartolensia` user, grants `docker`, `video`, and
`render` group membership, writes `/etc/cartolensia/cartolensia.env`, enables
`cartolensia-postgres`, `cartolensia-ai`, and `cartolensia` systemd services,
and keeps originals mounted separately at `/originals`. After copying the
bundle to `/opt/cartolensia/releases/<version>` and pointing
`/opt/cartolensia/current` at it, start the services:

```bash
sudo systemctl start cartolensia-postgres cartolensia-ai cartolensia
sudo systemctl status cartolensia
```

For SMB/CIFS originals, keep credentials outside the repo and mount the share
read-only. A typical `/etc/fstab` entry looks like:

```fstab
//fileserver.example/media /originals cifs credentials=/etc/cartolensia/smb-originals.credentials,ro,nosuid,nodev,noexec,uid=cartolensia,gid=cartolensia,file_mode=0440,dir_mode=0550,iocharset=utf8,vers=3.1.1,nofail,_netdev,x-systemd.automount 0 0
```

`/etc/cartolensia/smb-originals.credentials` must be mode `0600` or stricter and
must never be committed. Confirm the mount with `findmnt -T /originals` and a
negative write test as the `cartolensia` user before indexing. Runtime writes
belong under `/var/lib/cartolensia`; the bootstrap sets
`CARTOLENSIA_COMPONENT_DIR=/var/lib/cartolensia/components`,
`CARTOLENSIA_MODEL_DIR=/var/lib/cartolensia/models`, and
`CARTOLENSIA_AI_MODEL_DIR=/var/lib/cartolensia/models`. Optional Python
packages installed after deployment should go under
`/var/lib/cartolensia/ai-extra-site`, which the generated service environment
adds to `PYTHONPATH`. This keeps mutable AI/runtime additions outside the
immutable release directory while still allowing the Component Manager and AI
sidecar to find them. Cache, exports, logs, and PostgreSQL data also stay under
`/var/lib/cartolensia`.

For NVIDIA AI/transcoding the host must provide the NVIDIA driver and, for
containerized GPU use, NVIDIA Container Toolkit. For Ryzen/Radeon VAAPI
transcoding the `cartolensia` service user must be able to read
`/dev/dri/renderD*`; the bootstrap adds `video` and `render` groups and sets
`LIBVA_DRIVER_NAME=radeonsi`/`VDPAU_DRIVER=radeonsi` defaults.

### Git-Based Remote Upgrades

Boot-managed hosts can be upgraded from Git without re-indexing the archive.
The metadata database and component/model directories live under
`/var/lib/cartolensia` and are preserved across release swaps.

On the remote host:

```bash
sudo CARTOLENSIA_REPO_URL=https://github.com/<owner>/<repo>.git \
  CARTOLENSIA_BRANCH=main \
  bash /opt/cartolensia/current/scripts/remote/upgrade-cartolensia-from-git.sh
```

The upgrade script:

- verifies that `/originals` is a mounted filesystem and has read-only mount
  options;
- does not create write-probe files under `/originals`;
- writes a PostgreSQL custom-format backup to
  `/var/lib/cartolensia/exports/backups`;
- fetches the configured Git branch into `/opt/cartolensia/source`;
- copies the current release forward so bundled ffmpeg, PostgreSQL,
  Tesseract, Python runtime, and other reviewed payloads remain available;
- overlays the freshly built backend, WebUI, configs, scripts, docs, and
  migrations;
- moves `/opt/cartolensia/current` atomically to the new release and leaves
  `/opt/cartolensia/previous` pointing at the old release;
- restarts only `cartolensia-ai` and `cartolensia` by default.

If the target is air-gapped, build a new local full bundle on a connected
machine instead and extract it into `/opt/cartolensia/releases/<version>`.
Then move `/opt/cartolensia/current` to that release and restart services. The
same `/var/lib/cartolensia` database/cache/component layout is reused, so a
full re-index is not required.

### Remote Executors

AI is a real sidecar service and can run on a separate machine:

```bash
# On the AI node:
./bin/start-ai-executor nvidia-cu128 0.0.0.0 19090

# On the main Cartolensia node:
cat >config/remote-executors.local.env <<'EOF'
CARTOLENSIA_AI_WORKER_ENDPOINT=http://ai-node.example:19090
EOF
./bin/start-cartolensia
```

Available AI executor flavor directories:

- `cpu-avx2`: CPU fallback for common x86_64 hosts;
- `cpu-avx512`: CPU fallback for AVX-512 hosts; current PyTorch CPU wheels dispatch at runtime and are not a separate AVX-512-only build;
- `nvidia-cu128`: CUDA 12.8 PyTorch wheels for hosts with compatible NVIDIA drivers, such as RTX 3090 Ti-class systems;
- `intel-arc`: configurable Intel/XPU executor slot; defaults to CPU wheels unless an reviewed Intel wheel index is configured;
- `rocm-radeon`: configurable ROCm executor slot; defaults to CPU wheels unless a reviewed ROCm wheel index is configured. Radeon 740M support depends on host ROCm/kernel support and may not be available in upstream wheels.

Live transcoding is currently an in-process ffmpeg session service. The full bundle includes `bin/start-transcode-node` for running a transcode-capable Cartolensia node on a separate host/port with the same read-only originals and PostgreSQL access, but a separate distributed transcode executor protocol is not yet implemented. Route/proxy transcode API requests to that node when using this split layout.

## GitHub Actions Release Build

The workflow `.github/workflows/offline-release.yml` can be started manually from GitHub Actions. It:

1. checks out source;
2. installs Go, Node.js, Python, p7zip, ffmpeg, Tesseract language packs, and PostgreSQL from the runner package manager;
3. runs Go/WebUI verification;
4. builds the offline `.7z`;
5. uploads the archive as a workflow artifact;
6. creates or updates a GitHub release and attaches the `.7z` plus checksum.

Workflow inputs:

- `tag_name`: release tag to create/update.
- `release_name`: release title.
- `prerelease`: whether to mark the release as prerelease.
- `ai_flavor`: `none`, `runtime`, `cpu`, or `cuda128`.
- `include_postgres`: include PostgreSQL binaries.
- `include_tools`: include ffmpeg/ffprobe/Tesseract.
- `include_python_runtime`: include the Python sidecar runtime without model weights.
- `include_models`: copy `.cartolensia/models` if a reviewed cache exists in the runner workspace.
- `include_offline_maps`: copy a reviewed offline map bundle if an operator-provided cache is present.

Use `runtime` for an OCR-capable sidecar without heavy PyTorch model packages. Use `cpu` for a package that can run approved local AI models on CPU after weights are bundled. Use `cuda128` only after reviewing PyTorch CUDA wheel and NVIDIA component redistribution terms.

## Archive Layout

```text
cartolensia-.../
  bin/cartolensia
  webui/dist/
  config/production.yaml
  config/production-container.yaml
  config/offline-airgap.yaml
  .env.production.example
  docker-compose.production.yml
  config/offline-memory.yaml
  config/offline-postgres.yaml
  start-cartolensia.sh
  stop-cartolensia.sh
  external/bin/
  external/lib/
  external/postgres/
  external/share/tessdata/
  python/
  ai/python-site/
  services/ai/
  .cartolensia/models/
  media/
  runtime/
  logs/
  licenses/
  source/cartolensia-source.tar.gz
```

`start-cartolensia.sh` uses bundled PostgreSQL when present, otherwise it falls back to memory mode. It starts the AI sidecar when a bundled Python environment exists. The media directory defaults to `strict_read_only`.

## License And Redistribution Review

Every archive contains:

- `licenses/PROJECT-LICENSE.txt`;
- `licenses/THIRD_PARTY_NOTICES.md`;
- `licenses/go-modules.txt`;
- `licenses/npm-production-tree.json`;
- `components-manifest.json` and `licenses/components-manifest.json`;
- `licenses/python-packages.txt` when AI is bundled;
- Debian/Ubuntu copyright files for copied system packages when available;
- `source/cartolensia-source.tar.gz` when source bundling is enabled.

The component manifest records each staged component key, name, category, source type, package-relative path, version string, license note, provenance URL, and redistribution note. It is intended for release review and for the in-app Component Manager import flow.

The packager captures the bundled FFmpeg configure line to `licenses/ffmpeg-configure.txt` when FFmpeg is included. A configure line containing `--enable-nonfree` fails packaging by default because that build is not suitable for ordinary redistribution. `--enable-gpl` is recorded in `licenses/build-manifest.env` so release operators can mark the archive as a GPL-tools bundle. Only set `CARTOLENSIA_DIST_ALLOW_NONFREE_FFMPEG=1` for private/internal packages after a separate legal review.

The local full `tar.zst` builder records the BtbN FFmpeg configure line in `licenses/ffmpeg-version.txt` and fails on `--enable-nonfree`. The approved BtbN URL above is a GPL shared build, so the resulting private bundle must be treated as a GPL-tools bundle and should include the generated manifests/notices when transferred internally.

Before publishing a public release, review:

- ffmpeg build license and codec flags;
- Tesseract and language data licenses;
- PostgreSQL package notices;
- Python package metadata;
- model weight licenses and training-data provenance;
- CUDA/NVIDIA redistribution terms when using CUDA wheels.

The package builder records dependency facts but does not replace legal review.

## Multimodal Component Packaging

Audio/document/ASR features are component-managed in offline packages.

- FFmpeg/FFprobe are required for rich audio/video metadata and are listed in the component manifest when bundled.
- Tesseract and tessdata language packs are required for OCR and are listed individually.
- faster-whisper, ctranslate2, librosa/scipy, Basic Pitch, Demucs, Marker/PDF tooling, and any advanced video/genre/music models are optional payloads. Bundle them only after license/provenance review.
- ASR and document models must be stored under package-local component/model/runtime directories, never under original media roots.
- Offline packages should expose missing ASR/Marker/genre/music components as actionable Component Manager states if they are not bundled.
- Current ASR/audio/music component keys are `asr-faster-whisper`, `asr-ctranslate2`, `asr-model-small`, `asr-model-medium`, `audio-librosa`, `audio-soundfile`, `music-basic-pitch`, `music-demucs`, and optional future `music-mt3`.
- `faster-whisper-small` can be bundled only when model weight provenance has been reviewed and the package manifest records the source cache path under `.cartolensia/models/faster-whisper`.
- PyMuPDF is AGPL-3.0-or-later and is tracked as `document-pymupdf`; include it only when the release package and notices satisfy its terms.
- Audio feature labels are currently heuristic when no reviewed genre model is bundled; do not advertise offline packages as having a production genre classifier unless a reviewed model is included.
- Basic Pitch MIDI outputs are compact enough for selected and broad backfill jobs. Demucs stem outputs are large and should be kept on-demand or limited to selected assets/albums, with cache cleanup policy documented for the target installation.
- The local-full package builder installs music packages after the core AI sidecar environment. Demucs is best-effort and does not block the core bundle by default. Basic Pitch is best-effort because the upstream package currently targets Python 3.7-3.11; on Python 3.12+ the bundle records `music-basic-pitch` as missing unless the operator provides a reviewed compatible component archive.

The current Linux package flow can include the application, WebUI, OCR/media tools, PostgreSQL runtime, Python runtime, and reviewed model cache. It does not bundle host GPU drivers, public map data, online geocoders, or unreviewed model weights.
Production containers should use the shipped compose file plus a user-provided `.env.production` or shell environment. The app should remain in `strict_read_only` mode for the archive storage unless a separate tested write path exists.

## 2026-06-09 Offline Dependency Placement Notes

The live real-peek environment currently has the core multimedia stack available through Component Manager checks:

- FFmpeg/FFprobe system tools;
- Tesseract plus English, Russian, Armenian, Simplified Chinese, and Traditional Chinese tessdata;
- Python AI runtime with PyTorch/torchvision, OpenCV YuNet, Falconsai NSFW, OpenCLIP, BLIP, faster-whisper small, CTranslate2, librosa, SoundFile, PyMuPDF, and optionally reviewed Basic Pitch/Demucs music packages.

For an offline target machine, place reviewed component payloads under the extracted package's `.cartolensia/components` and `.cartolensia/models` directories, or provide paths through Settings -> Components after launch. Do not place tools, model weights, OCR data, ASR models, or Python environments under the original media root.

The following optional components are intentionally not bundled/downloaded until a reviewed source URL or operator-provided archive/path exists:

- `vmaf`: current FFmpeg lacks `libvmaf`; bundle only a reviewed libvmaf/FFmpeg component and record its flags.
- `asr-model-medium`: optional quality upgrade over installed faster-whisper small.
- `mobilenetv3-large`: optional classifier fallback weights; EfficientNet-B0 is the active classifier.
- `music-basic-pitch`: optional music-to-MIDI provider; on Python 3.12+ prefer a reviewed Python 3.11 component archive until upstream package compatibility changes.
- `music-demucs`: on-demand stem separation package and model weights; include only with provenance review because output cache can grow quickly.
- `music-mt3`: optional future multi-instrument transcription provider; not required for the default Basic Pitch MIDI path.

For a self-sufficient offline package, include:

- Cartolensia binary and `webui/dist`;
- PostgreSQL runtime or a documented external PostgreSQL service;
- `config/offline-postgres.yaml` or a machine-specific config derived from it;
- reviewed media tools under `external/` or `.cartolensia/components`;
- `.cartolensia/ai-venv` or a rebuilt Python runtime/wheelhouse;
- reviewed `.cartolensia/models` cache;
- `components-manifest.json`, license notices, and source archive.

Run the package with media mounted read-only and configure storage mode as `strict_read_only` unless a separate journaled write path has been implemented and tested.

## Essential Backup Export

`scripts/remote/create-essential-export.sh` creates a single `.7z` archive intended for operator backup/restore, not application redistribution. It includes a PostgreSQL custom-format dump, redacted production config, storage manifest, and restore notes. It intentionally excludes originals, preview/cache thumbnails, component caches, model caches, local secret files, and packaged binaries.

Treat this archive as sensitive because the database dump may contain private metadata, password hashes, sessions, public/private flags, OCR text, transcripts, captions, and storage paths. Transfer it only over authenticated channels such as SSH. For full disaster recovery, separately back up local secrets and any externally mounted original media according to the deployment's access-control policy.

## Private Local Full 7z Bundle

For a private, non-public, operator-reviewed bundle, use:

```bash
bash scripts/release/build-local-full-7z.sh
```

This is a convenience wrapper around the local full bundle builder with
`CARTOLENSIA_LOCAL_FULL_ARCHIVE_FORMAT=7z`. The same license checks and manifest
generation are used. Keep these defaults unless you have reviewed every payload:

- no writes to originals;
- no nonfree FFmpeg unless explicitly allowed after review;
- AI models only from reviewed local caches;
- Python runtime and LLM executor settings recorded in package config;
- local LLM endpoints configured through `config/llm-executor.env` and
  `bin/start-llm-executor`.

The full bundle is intended for private transfer to air-gapped or low-connectivity
hosts. It is not a GitHub release artifact by default and should not be published
without a separate redistribution review of FFmpeg, CUDA/PyTorch, Tesseract,
model weights, PostgreSQL runtime files, Python wheels, and all generated
license notices.

# Transcoding Hardware Plan

This plan targets the observed custom NVIDIA preset failure while keeping all outputs cache-scoped and outside original media storage.

## Current State

Live video asset selected for planning:

- Asset ID: `ce8b4866-33bd-474e-84ab-a0fd9388a313`
- File: `PXL_20260516_163309946.mp4`
- Codec: HEVC
- Resolution: `3840x2160`
- Duration: `7.0979` seconds
- Original storage: `rclone_peek`, `strict_read_only`

Current stream options:

- `original`: direct immutable original stream.
- `h264_720p_lan`: CPU/libx264 HLS, available.
- `h264_low_bitrate`: CPU/libx264 HLS, available.
- `av1_low_bitrate`: disabled.
- `nv-750k`: custom NVIDIA/H.264 preset using `h264_nvenc`, available by static validation but reported by the user as failing.

Host hardware:

- NVIDIA GeForce RTX 3090 Ti detected by `nvidia-smi`.
- Driver `570.124.06`, CUDA `12.8`.
- ffmpeg advertises `h264_nvenc`, `hevc_nvenc`, and `av1_nvenc`.
- ffmpeg advertises VAAPI/QSV encoders, but `/dev/dri` is absent in the shell environment and `vainfo` is missing.
- Docker runtimes do not show `nvidia`; Docker GPU path is not ready.

## Diagnosis

Current preset validation is insufficient:

- It checks whether the encoder name appears in `ffmpeg -encoders`.
- It checks broad hardware hints.
- It does not run a real ffmpeg validation command.
- It does not distinguish:
  - encoder compiled into ffmpeg;
  - device available;
  - driver accepts session;
  - selected parameters are legal for encoder;
  - output muxer/HLS options are browser-safe.

Likely bug for `nv-750k`:

- The preset stores `parameter_value: "750"`.
- The backend `safeBitrateParameter` may interpret this as `750` bits/s instead of `750k`.
- NVENC command uses generic CPU-oriented options such as `-preset veryfast`, which is not a valid NVENC preset on many ffmpeg builds.
- Hardware options do not include explicit upload/scale strategy, e.g. CPU scale before NVENC vs CUDA scale.

## Target UX

Video quality UI:

- Basic dropdown:
  - Original/direct
  - H.264 720p LAN
  - H.264 low bitrate
  - saved custom presets
  - Advanced/Custom
- Advanced controls in a modal/popover with focus priority over gallery keyboard navigation.
- While the modal is open:
  - arrow keys and WASD navigate fields or text inputs;
  - gallery previous/next and image panning are suspended.
- Buttons:
  - `Apply` starts a temporary session using unsaved settings.
  - `Test current hardware configuration` runs a short dry-run validation.
  - `Save preset` persists after validation or explicit override.
  - `Remove preset` only for custom presets.

## Backend APIs

Add or harden:

- `POST /api/v1/transcoding/presets/validate`
  - validates preset fields and optionally runs a short ffmpeg dry-run.
- `POST /api/v1/transcoding/hardware-test`
  - runs bounded tests for selected hardware/encoder.
- `POST /api/v1/media/{asset_id}/transcode-session`
  - accepts either `profile` or inline `preset` for Apply-before-save.
- `GET /api/v1/media/transcode-sessions/{id}/status`
  - include full stderr tail, command summary, profile, encoder, hardware, output bytes, segment count.
- `DELETE /api/v1/media/transcode-sessions/{id}/stop`
  - already present; keep path safety.

Do not add destructive or original-writing endpoints.

## Command Builder Rules

CPU/libx264:

- `-c:v libx264`
- `-preset veryfast`
- quality mode: `-crf N`
- bitrate mode: `-b:v 1500k`
- `-pix_fmt yuv420p`
- optional `scale='min(1280,iw)':-2`

NVIDIA H.264:

- `-c:v h264_nvenc`
- use NVENC presets such as `p4`, `p5`, or `p6`, not `veryfast`.
- quality mode:
  - prefer `-rc vbr -cq N -b:v 0` or a tested ffmpeg-compatible equivalent.
- bitrate mode:
  - normalize bare numeric UI input: `750` means `750k` unless suffixed.
  - set `-b:v`, `-maxrate`, `-bufsize`.
- set `-pix_fmt yuv420p`.
- start with CPU scaling via `-vf scale='min(width,iw)':-2` for reliability.
- later optional CUDA scaling:
  - `-hwaccel cuda`
  - `-hwaccel_output_format cuda`
  - `scale_cuda` only after validation.

NVIDIA HEVC:

- `-c:v hevc_nvenc`
- similar RC/bitrate normalization.
- browser compatibility must be marked lower than H.264.

NVIDIA AV1:

- RTX 3090 Ti does not support AV1 NVENC encoding. Even if ffmpeg advertises `av1_nvenc`, validation should fail on this GPU and mark unavailable.
- Keep AV1 disabled unless hardware validation succeeds.

VAAPI/QSV:

- Disable or mark unverified unless `/dev/dri` is present and accessible.
- Add explicit device path settings later.

## Validation Flow

Static validation:

- required fields;
- known hardware value;
- encoder exists in ffmpeg encoder list;
- mode and parameter range valid;
- cache path safe.

Dry-run validation:

- user-triggered only;
- writes output to `/tmp/cartolensia-transcode-test-*` or `.cartolensia/realpeek-cache/transcode-test`;
- reads only the selected original through a safe temp path or current storage adapter;
- limits duration to `2-3` seconds:
  - `-t 2`
  - `-f null -` for encoder-only test; or
  - tiny HLS with one segment if muxer behavior is being tested.
- captures stderr and exit code.
- deletes dry-run output after recording results.

Runtime validation:

- session status includes command summary, stderr tail, segment count, playlist readiness, output bytes.
- failed sessions remain visible until cleanup so the UI can show exact ffmpeg errors.

## Frontend Tasks

Files:

- `webui/src/App.vue`
- `webui/src/api.ts`
- optional later component split:
  - `webui/src/components/TranscodeSettingsModal.vue`
  - `webui/src/components/VideoPlayer.vue`

Required behavior:

- Advanced settings become modal/popover with focus trap.
- Gallery keyboard handlers ignore arrow/WASD while transcode modal is open.
- Apply button starts a session with unsaved inline settings.
- Save button persists only custom preset metadata.
- Hardware test button shows pass/fail, stderr tail, and suggested fallback.
- Preset dropdown shows validation state and last test result.
- If hardware fails, fallback to Original/direct and keep message visible.

## Tests

Unit tests:

- bitrate normalization: `750` -> `750k`, `1500k` unchanged.
- NVENC command uses NVENC-compatible preset values.
- AV1 NVENC disabled on unsupported GPU validation result.
- inline preset session command builder.
- validation endpoint rejects invalid hardware/encoder/mode.
- path safety for test and session output roots.

Integration/smoke after approval:

- short native ffmpeg dry-run for `h264_nvenc` against the current 7-second video.
- low-bitrate CPU HLS still works.
- invalid NVENC option shows stderr in API/UI.

## Approvals Needed

Needed before implementation run if hardware validation runs real ffmpeg commands:

- Approval for a short native ffmpeg NVENC dry-run on the indexed 7-second video, output only under `.cartolensia/realpeek-cache/transcode-test` or `/tmp`.

Needed before Docker GPU validation:

- Approval to install/configure NVIDIA Container Toolkit if absent. This is outside the repo and should not be done by Codex without explicit system-level approval.
- Approval to pull a CUDA runtime probe image if no suitable local image exists.

Not needed:

- No approval needed to implement command builders, validation endpoint, UI modal, tests using sample stderr/encoder output, or docs.

## Safety

- Never write HLS, test outputs, logs, or presets into `/mnt/Models/rclone`.
- Never transcode into original storage.
- Direct/original remains the default.
- Every generated file path must be checked to stay under the configured cache/test root.

## 2026-06-07 Implementation Status

Implemented:

- `POST /api/v1/transcoding/presets/validate`
- `POST /api/v1/transcoding/hardware-test`
- Inline unsaved preset support for `POST /api/v1/media/{asset_id}/transcode-session`
- Session status now includes:
  - command summary;
  - profile/preset;
  - hardware;
  - encoder;
  - stderr tail;
  - output bytes;
  - segment count;
  - readiness state.
- UI advanced controls now have:
  - Apply;
  - Test current hardware configuration;
  - Save preset;
  - Remove custom preset;
  - modal keyboard isolation from gallery navigation.

Command-builder changes:

- Bare numeric bitrate values are normalized as kilobits per second:
  - `750` -> `750k`
  - `1500k` stays `1500k`
- NVIDIA H.264 uses:
  - `-c:v h264_nvenc`
  - `-preset p5`
  - `-b:v <bitrate>`
  - `-maxrate <bitrate>`
  - `-bufsize 2x<bitrate>`
- CPU H.264 still uses `libx264`.
- HLS scale filters use simple explicit forms such as `scale=w=1280:h=-2` to avoid filter parser failures.

Dry-run result:

- Input: current indexed 7-second real-peek video `PXL_20260516_163309946.mp4`.
- Output: null muxer only; no media file written.
- Sandboxed ffmpeg result:
  - failed with `CUDA_ERROR_NO_DEVICE`, confirming sandbox GPU isolation.
- Native/outside-sandbox result:
  - `h264_nvenc` dry-run succeeded;
  - encoded roughly `1.9s` of video;
  - no output file was written.
- Live Cartolensia hardware-test endpoint result:
  - `dry_run_ok: true`;
  - command summary redacted the absolute original path;
  - warning shown for bare numeric bitrate interpretation.

Remaining:

- Browser-level HLS playback should still be manually checked after each UI build.
- AV1 remains disabled unless a future dry-run proves encoder support on the target GPU.
- VAAPI/QSV remain environment-dependent and require `/dev/dri` access.

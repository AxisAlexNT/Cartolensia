# Next Long Run Plan

Date: 2026-06-07

This is the implementation target for the next large run. It must continue from the current live real-peek state without resetting PostgreSQL unless explicitly requested. It must not scan new real-data prefixes, mark missing files, or write anything under `/mnt/Models/rclone`.

Safety baseline:

- Real archive storage `rclone_peek` remains `strict_read_only`.
- Generated previews, tiles, transcodes, exports, model files, and temp files stay under repo-local ignored paths such as `.cartolensia/realpeek-cache`, `.cartolensia/exports`, and `.cartolensia/models`.
- No model downloads, Docker image pulls, or long ffmpeg hardware tests happen without explicit approval.
- No commit and no push.

Current audited state:

- `54` indexed assets, `54` hashed, `48` geotagged photos, `2` videos, `4` parsed GPX/KML track summaries.
- Latest jobs are terminal; no running job was observed.
- Host has AMD Ryzen 9 7900X and NVIDIA RTX 3090 Ti. `nvidia-smi` works.
- Docker does not currently report an NVIDIA runtime.
- ffmpeg advertises NVENC, VAAPI, QSV, and CPU encoders, but `/dev/dri` is absent in the shell environment.
- AI workers are configured as contracts only and are not running real models.

## 1. GPS/KML Track Detail Page And Charts

Files/packages likely to change:

- `internal/server/extended.go`
- `internal/catalog/extended_store.go`
- `internal/database/extended.go`
- `webui/src/App.vue`
- `webui/src/api.ts`
- `webui/src/style.css`

APIs to add or harden:

- `GET /api/v1/gps/tracks/{track_asset_id}` with full summary, source asset, source format, bbox, distance, duration, elevation range, and timestamp policy.
- `GET /api/v1/gps/tracks/{track_asset_id}/points?max_points=...&simplify=true`.
- `GET /api/v1/gps/tracks/{track_asset_id}/profile?metric=altitude|speed`.
- `GET /api/v1/gps/tracks/{track_asset_id}/assets?media_kind=photo,video&exclude_track_assets=true`.
- `GET /api/v1/gps/tracks/{track_asset_id}/nearby-assets?distance_m=...&media_kind=photo,video`.

Frontend work:

- Add a real track detail view instead of only a selected table panel.
- Show OpenLayers track overlay.
- Add altitude profile chart using plain SVG/canvas first; no chart dependency is needed unless SVG proves inadequate.
- Add speed profile chart when timestamps exist.
- Show stats: distance, duration, elevation min/max, point count, source file, source format, bbox.
- Add buttons:
  - show media during this track by time;
  - show media near this track by geotag distance;
  - show track on map;
  - open original source asset.

Tests:

- Track detail API returns no-time KML safely.
- Profile API returns altitude and speed series for timestamped GPX.
- Time-based media query excludes track assets by default.
- Nearby media query uses geotags and bounded haversine/polyline distance when PostGIS is unavailable.
- Frontend build.

Acceptance:

- Clicking a track in GPS/KML Track Manager opens a useful detail page with map and charts.
- "Show media during track" returns photos/videos by default, not GPX/KML files.
- Tracks without timestamps still render geometry and statistics.

## 2. Map Track Popups And Media Queries

Files/packages likely to change:

- `internal/server/extended.go`
- `internal/catalog/extended_store.go`
- `internal/database/extended.go`
- `webui/src/App.vue`
- `webui/src/api.ts`

APIs:

- Continue using `GET /api/v1/gps/tracks/{id}/point-info?lat=...&lon=...`.
- Harden `GET /api/v1/gps/tracks/{id}/assets`.
- Harden `GET /api/v1/gps/tracks/{id}/nearby-assets`.
- Optionally add `GET /api/v1/map/track-click-info` if the map needs combined point/media data.

Frontend work:

- Ensure layer priority remains base tiles, tracks, asset points/clusters, popups.
- Track clicks open an OpenLayers popup, not immediate navigation.
- Track popup shows nearest point, click coordinate, distance, relative time, absolute timestamp, speed, elevation, and summary stats.
- Popup actions call the track media APIs and show results in a drawer or panel.

Tests:

- Asset/cluster click takes precedence over track click.
- Track click returns point info with and without timestamps.
- Nearby media works for geotagged assets.

Acceptance:

- Users can inspect track context on the map before navigating away.
- Nearby and during-track media queries are clear and bounded.

## 3. Track Preview And Thumbnail Improvements

Files/packages likely to change:

- `internal/server/track_preview.go`
- `internal/server/extended.go`
- `internal/catalog/extended_store.go`
- `internal/database/extended.go`
- `webui/src/App.vue`
- `webui/src/style.css`

APIs:

- `GET /api/v1/media/{asset_id}/track-preview`
- `GET /api/v1/media/{asset_id}/track-thumbnail`
- Reuse `preview_cache_entries` for thumbnail metadata when practical.

Implementation:

- Keep the dark fallback thumbnail renderer as the fast default path.
- Add optional OSM-background thumbnail rendering only when configured and safe.
- Use cached OSM tiles through Cartolensia tile proxy/cache only; no bulk prefetch.
- Store thumbnails under preview cache root.
- Add size and TTL limits through runtime settings.

Frontend work:

- Track tiles show track thumbnails.
- Track assets in gallery overlay show an interactive OpenLayers preview instead of "preview not possible".
- Asset detail for track assets shows track preview and source metadata.

Tests:

- Thumbnail path stays inside cache root.
- Dark fallback renderer works without network.
- OSM-background disabled setting is respected.
- Track preview API returns valid GeoJSON/summary.

Acceptance:

- GPX/KML/KMZ/GPZ assets are first-class media in gallery/detail views.
- Thumbnail generation never writes near originals.

## 4. Gallery Modal Focus And Input Handling

Files/packages likely to change:

- `webui/src/App.vue`
- `webui/src/style.css`

Implementation:

- Add explicit modal focus state for gallery, video settings, album picker, and track popups.
- While advanced video settings are open, arrow keys and WASD must navigate controls or edit fields, not switch assets or pan images.
- Preserve gallery behavior when no modal has focus:
  - ArrowLeft/ArrowRight switch assets.
  - WASD pans zoomed images.
  - wheel zooms at cursor.
  - mouse/touch/pen drag pans.
  - pinch zooms.
  - Escape closes active overlay/modal in correct order.

Tests:

- Frontend build.
- Manual checklist in report for keyboard focus behavior.

Acceptance:

- Gallery navigation is predictable and does not fight form controls.

## 5. Advanced Transcoding Modal, Presets, Apply, And Hardware Tests

Files/packages likely to change:

- `internal/server/transcode_sessions.go`
- `internal/server/extended.go`
- `internal/catalog/extended_store.go`
- `internal/database/extended.go`
- `migrations/009_transcoding_validation_preferences.sql` if needed
- `webui/src/App.vue`
- `webui/src/api.ts`

APIs:

- `POST /api/v1/transcoding/presets/validate`
- `POST /api/v1/transcoding/hardware-test`
- `POST /api/v1/media/{asset_id}/transcode-session` with either saved profile or inline unsaved preset.
- `GET /api/v1/media/transcode-sessions/{id}/status` with stderr tail, command summary, output bytes, segment count, and readiness.

Backend work:

- Normalize bitrate input: bare numeric `750` should mean `750k` for video bitrate UI.
- Use encoder-specific command rules:
  - CPU/libx264: `-preset veryfast`, `-crf` or `-b:v`.
  - NVIDIA/H.264: `h264_nvenc`, NVENC presets such as `p4`/`p5`, tested rate-control flags.
  - NVIDIA/HEVC: `hevc_nvenc`, lower browser compatibility warning.
  - AV1: disable on RTX 3090 Ti unless actual hardware dry-run proves support.
  - VAAPI/QSV: mark unavailable/unverified unless `/dev/dri` is present and accessible.
- Capture ffmpeg stderr and expose it in status.
- Add bounded dry-run validation only behind user approval or explicit UI action.
- All outputs stay under transcode cache or `/tmp` test root.

Frontend work:

- Basic quality dropdown:
  - Original/direct;
  - built-ins;
  - saved custom presets;
  - Advanced/Custom.
- Advanced modal:
  - hardware selector;
  - codec selector;
  - encoder selector;
  - mode selector;
  - parameter input;
  - Apply button;
  - Test current hardware configuration button;
  - Save preset button;
  - Remove custom preset button.
- Hardware/preset validation status is visible before Save.
- Failed transcode falls back to Original and shows actionable stderr summary.

Tests:

- Bitrate normalization.
- NVENC command builder uses NVENC-compatible preset values.
- Invalid encoder/hardware/mode rejected.
- Inline preset sessions are path-safe.
- Built-ins cannot be removed.
- Frontend build.

Acceptance:

- Custom NVIDIA presets are validated before use and cannot silently fail with unsupported browser source errors.
- "Apply" tests an unsaved configuration; "Save preset" persists metadata only.

## 6. High-Resolution Asset View And Preview Policy

Files/packages likely to change:

- `internal/server/preview.go` or existing media/preview handlers
- `internal/server/extended.go`
- `webui/src/App.vue`
- `webui/src/api.ts`
- `webui/src/style.css`

APIs:

- Harden existing preview endpoint with variants:
  - thumbnail;
  - fit-to-view;
  - original direct.
- Add or extend `GET /api/v1/media/{asset_id}/view` if a dedicated view-size endpoint is cleaner.

Policy:

- Default should avoid write amplification:
  - on-the-fly previews are default;
  - persistent preview generation is optional;
  - Discovery checkbox and Settings default must be synchronized.
- Persistent preview jobs must be explicitly selected and bounded.
- Originals are never modified.

Frontend work:

- Asset detail should use original-quality image or a view-size on-demand preview, not only low-resolution cached thumbnail.
- Gallery overlay uses original-quality source for opened image, with preview only as placeholder.
- Settings and Discovery show the same default preview policy.

Tests:

- View-size preview path safety.
- Persistent preview disabled by default.
- Discovery stage options mirror runtime settings.
- Frontend build.

Acceptance:

- Detail page no longer looks low-resolution for photos.
- Preview cache behavior is explicit and conservative.

## 7. Runtime Storage Configuration

Files/packages likely to change:

- `internal/config`
- `internal/storage`
- `internal/server`
- `internal/catalog`
- `internal/database`
- `webui/src/App.vue`
- `webui/src/api.ts`
- `docs/SECURITY.md`
- `docs/OPERATIONS.md`

APIs:

- `GET /api/v1/storages`
- `POST /api/v1/storages`
- `PATCH /api/v1/storages/{name}`
- `GET /api/v1/storages/{name}/validate`
- Optional pending-YAML path for restart-required storage root changes.

Protection modes:

- `strict_read_only`
- `read_only`
- `journaled_deferred`
- `read_write` as future/disabled unless fully implemented

Safety:

- `/mnt/Models/rclone` and paths under it default to `strict_read_only`.
- Changing real archive protection requires a confirmation phrase and remains blocked until destructive modes are implemented/tested.
- Runtime additions can be used for synthetic/test roots first.

Tests:

- Adding a synthetic strict read-only storage.
- Rejecting unsafe real archive protection downgrade.
- Pending restart for root/mode changes.
- No writes during validation.

Acceptance:

- Storage page can add/configure safe storages at runtime without weakening real archive safety.

## 8. Map Layout, Debug Toggle, And Media-During-Track Fixes

Files/packages likely to change:

- `webui/src/App.vue`
- `webui/src/style.css`
- `internal/server/extended.go`

Frontend work:

- Make map occupy more of the viewport.
- Hide raw JSON debug behind a collapsed toggle by default.
- Add visible filter reset state.
- Ensure track/media action results are shown as media cards/table, not mixed track assets.

Backend work:

- Add media-kind defaults and `exclude_track_assets=true`.
- Return reason fields when no media match:
  - no timestamps;
  - no geotags;
  - filters exclude all;
  - selected track has no temporal overlap.

Tests:

- Map debug collapsed by default via build/manual checklist.
- Track media query excludes tracks by default.
- No-time KML returns a clear reason for time-based media.

Acceptance:

- Map is usable for inspection without debug clutter.
- "Show media during track" returns meaningful media or a clear explanation.

## 9. Settings Tab Cleanup And Plugin Settings

Files/packages likely to change:

- `internal/server/settings.go`
- `internal/server/extended.go`
- `internal/catalog/extended_store.go`
- `internal/database/extended.go`
- `webui/src/App.vue`
- `webui/src/api.ts`
- `webui/src/style.css`

APIs:

- `GET /api/v1/settings`
- `GET /api/v1/settings/schema`
- `PATCH /api/v1/settings/runtime`
- `GET /api/v1/settings/pending`
- `PATCH /api/v1/settings/pending`
- `DELETE /api/v1/settings/pending`
- `GET /api/v1/settings/pending/download`
- `GET /api/v1/plugins/{id}/settings/schema`
- `GET /api/v1/plugins/{id}/settings`
- `PATCH /api/v1/plugins/{id}/settings`

Frontend work:

- Tabs:
  - General;
  - Server/HTTP/HTTPS;
  - Storage;
  - Indexing/Discovery;
  - Metadata/EXIF;
  - Preview Cache;
  - Map/Tiles;
  - GPS/KML Tracks;
  - Transcoding;
  - AI/Vector;
  - Auth/Security;
  - Backups/DB Export;
  - Plugins;
  - Raw YAML / Effective Config.
- Each tab shows only relevant settings.
- Add margins, spacing, and state badges:
  - Runtime;
  - Restart required;
  - Plugin setting.
- Plugin tab gets second-row plugin tabs and UI/YAML toggle.

Tests:

- Each settings tab renders a distinct schema group.
- GPS/KML Tracks tab is not empty.
- Runtime setting roundtrip.
- Pending YAML setting roundtrip.
- Plugin setting roundtrip.
- Secret fields are masked.

Acceptance:

- Settings page is usable as a product UI, not just raw config.

## 10. Bootstrap/Material-Like Polish

Current dependencies:

- `bootstrap` `5.3.8`, MIT.
- `bootstrap-icons` `1.13.1`, MIT.
- Bundled locally through Vite; no CDN required.

Files likely to change:

- `webui/src/main.ts`
- `webui/src/App.vue`
- `webui/src/style.css`

Work:

- Continue migrating forms/buttons/cards/nav toward Bootstrap styling.
- Use Bootstrap Icons for nav/actions where helpful.
- Preserve dark theme and dense workbench layout.
- Do not add external fonts or CDN assets.

Tests:

- `npm --prefix webui run build`
- Inspect built output for obvious remote CDN references if new packages are added.

Acceptance:

- UI is visually more consistent without changing safety-critical workflows.

## 11. AI Sidecar Services And Model-Ready Foundation

Detailed plan: [AI_SERVICE_PLAN.md](AI_SERVICE_PLAN.md)

Files/packages likely to change:

- `services/ai/`
- `docker/ai/`
- `docker-compose.yml`
- `internal/server`
- `internal/catalog`
- `internal/database`
- `internal/jobs`
- `internal/workers`
- `webui/src/App.vue`
- `webui/src/api.ts`

Implementation order:

1. Package the dummy worker as `python -m cartolensia_ai.server`.
2. Add native and Docker entrypoints named `server`.
3. Add AI worker registry with health/capability polling.
4. Add backend jobs:
   - `ai_classify`;
   - `ai_detect_faces`;
   - `ai_describe`;
   - `ai_embed`.
5. Store dummy/no-model predictions in `ai_predictions` and `asset_tags` only when explicitly run on selected/bounded scopes.
6. Add model-cache settings and docs.
7. Add Docker Compose profiles for CPU/NVIDIA/ROCm/Intel, but do not pull/build heavy images by default.

Approvals required before real inference:

- Python dependency install into a venv or AI image build.
- Model downloads into `.cartolensia/models`.
- Docker GPU image pulls/builds.

Acceptance:

- AI worker service can run locally in dummy mode with the `server` entrypoint.
- Backend can call dummy worker and persist structured no-model results.
- UI clearly shows not-configured state and next actions.

## 12. Universal Explorer Search Improvements

Files/packages likely to change:

- `internal/server/search.go` or `internal/server/extended.go`
- `internal/catalog`
- `internal/database`
- `webui/src/App.vue`
- `webui/src/api.ts`

API:

- Continue `GET /api/v1/search?q=...`.
- Add structured filters:
  - `media_kind`;
  - `extension`;
  - `hash_prefix`;
  - `date_from`;
  - `date_to`;
  - `camera`;
  - `album_id`;
  - `track_id`;
  - `tag`;
  - pagination and sort.

Query language:

- Plain filename/path terms.
- `ext:jpg`, `kind:photo`, `hash:abc123`.
- Dates: `YYYY`, `YYYY-MM`, `YYYY-MM-DD`, `YYYY-MM..YYYY-MM`.
- `camera:Pixel`, `album:name`, `track:name`, `tag:name`.
- Quoted phrases.

Response:

- Asset results.
- Match explanations.
- Parsed tokens.
- Unsupported-token warnings.

Tests:

- Filename/path search.
- Extension/media kind.
- Date and date range.
- Hash prefix.
- EXIF camera make/model.
- Album and track filters.
- Tag/category filters.

Acceptance:

- Explorer has one central search box that explains why each result matched.

## 13. Dependency And Model Approval List

Already installed and license-checked locally:

- `ol` `10.9.0`, BSD-2-Clause.
- `bootstrap` `5.3.8`, MIT.
- `bootstrap-icons` `1.13.1`, MIT.
- `hls.js`, Apache-2.0.

Recommended no-new-dependency path:

- Track altitude/speed charts implemented as plain SVG/canvas.
- Keep current frontend dependencies.

Approvals required:

- Short native ffmpeg NVENC validation on current 7-second real-peek video, output only to `/tmp` or `.cartolensia/realpeek-cache/transcode-test`.
- Python venv/dependency install for AI service if moving beyond stdlib dummy worker.
- Docker image pulls/builds for CUDA/PyTorch/ROCm/Intel images.
- Model downloads into `.cartolensia/models`.

Model proposals are documented in [AI_SERVICE_PLAN.md](AI_SERVICE_PLAN.md). Do not download models in the implementation pass unless the user explicitly approves them.

## 14. Verification Plan

Required before ending the next implementation run:

```bash
gofmt -w $(find internal cmd -name '*.go' -print)
git diff --check
GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...
go test ./...
npm --prefix webui run build
bash scripts/smoke-test.sh
docker compose -f docker-compose.yml -f docker-compose.dev.yml config
bash scripts/test-db.sh
```

Additional targeted checks:

- Track detail page opens for all 4 current tracks.
- Altitude/speed charts render for GPX and degrade cleanly for no-time KML.
- Track media during-track returns photo/video results or a clear no-match reason.
- Advanced transcode modal focus suppresses gallery arrow/WASD shortcuts.
- Custom NVIDIA preset validation shows ffmpeg command/stderr status.
- Asset detail photo view is high-resolution or view-size generated, not thumbnail-only.
- Storage page refuses unsafe `/mnt/Models/rclone` mode downgrade.
- Preview generation default is on-demand unless explicitly enabled.
- Settings tabs show distinct settings.
- AI dummy sidecar can start natively and report `/health`.
- Explorer universal search supports filename/date/hash.

End-of-run reports:

- Update `RUN_REPORT.md`.
- Update `.cartolensia/runtime/REAL_PEEK_STATUS.md` if live state changes.
- Update `.cartolensia/runtime/REAL_PEEK_FIX_STATUS.md` if live app is restarted or manually validated.
- Confirm no writes to `/mnt/Models/rclone`, no commit, and no push.

# Security

Cartolensia is still pre-release software. The current security model is intended for local/self-hosted development and cautious private deployments, not exposed public hosting.

## Auth Modes

`dev_no_auth`

- Explicit development mode.
- Fixture workflows remain easy.
- Backend status includes a warning when auth is disabled.
- Do not expose this mode to untrusted networks.

`local`

- Local admin user is bootstrapped from config plus `CARTOLENSIA_ADMIN_PASSWORD` or an ignored password file.
- No production admin password is hardcoded.
- Bootstrap is idempotent; password rotation from bootstrap config is explicit.
- Passwords are hashed before storage.
- Login creates persisted sessions.
- Logout and password change invalidate appropriate session state.

OAuth/OIDC remains disabled-by-default stub behavior.

## Sessions And CSRF

Session cookies are:

- `HttpOnly`
- `SameSite=Lax`
- `Secure` when configured for HTTPS deployments

Cookie-authenticated write requests must include a CSRF header. Get it from:

```http
GET /api/v1/auth/csrf
```

Bearer API token requests do not need CSRF because they are not browser-cookie authenticated.

## API Token Scopes

Implemented scopes:

- `read`
- `write`
- `jobs:write`
- `plugins:write`
- `media:read`
- `admin`

Admin sessions can perform all actions. API tokens must carry a sufficient scope; role alone is not enough for a token without scopes.

Protected write-like endpoints include discovery/hash/metadata/preview starts, plugin rescan, job cancel/retry, password change, and token creation/revocation.

## Storage Safety

Original media is immutable in the implemented adapter.

- Storage mode defaults to `strict_read_only`.
- Filesystem writes, deletes, moves, and mkdir operations return explicit read-only errors.
- Path traversal and absolute paths are rejected.
- Recursive discovery skips symlinks.
- Opening a symlink that escapes the root is rejected.
- Original media is streamed only through the registry.

Preview files are generated only under Cartolensia cache/work directories. Preview cache cleanup verifies deletion targets stay inside the cache root.

Scoped dry-run discovery is guarded separately:

- storage must exist and be `strict_read_only`;
- prefixes are required and cannot be empty/root;
- default `max_files` is 50;
- `mark_missing` is rejected;
- current dry-run behavior is report-only and does not index assets.

## Media And External Tools

ffprobe and ffmpeg are detected best-effort. Missing tools do not fail discovery or core startup. Transcoding capability APIs are inventory only; no transcoding job writes originals.

## AI And Dependency Provenance

AI/vector APIs are status and contract stubs. The backend does not download models, run inference, or require PyTorch. Future AI plugins should declare model namespace/version, modality, provenance, and permissions in manifests.

Do not copy third-party source into the repository. Add dependencies only through normal package managers and document why they are needed.

Current added dependency notes:

- `ol` is bundled through npm for OpenLayers map rendering; local package metadata reports `BSD-2-Clause`.
- `github.com/rwcarlsen/goexif` is used for server-side JPEG EXIF parsing; the cached module license is BSD-style and compatible with the project policy.
- EXIF parsing errors are non-fatal and are recorded as metadata; timezone-less EXIF datetimes are stored as raw metadata instead of being blindly promoted to `taken_at`.

## Known Limitations

- Local auth is admin-centric and not yet a multi-user sharing system.
- CSRF tokens are stateless per session token and rotate when the session token changes.
- No brute-force throttling or account lockout is implemented yet.
- API tokens are bearer secrets; store them carefully.
- Sidecar plugin health probing is a stub and sidecar execution is not implemented.
- Real archive scan procedures must be supervised and bounded until rescan/missing-file semantics are fully hardened.

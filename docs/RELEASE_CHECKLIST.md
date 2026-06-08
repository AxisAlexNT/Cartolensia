# Release Checklist

Use this checklist before publishing a release archive.

## Build

- [ ] `go test ./...`
- [ ] `npm --prefix webui run build`
- [ ] `docker compose -f docker-compose.production.yml config`
- [ ] `bash scripts/release/build-linux.sh`

## License Review

- [ ] FFmpeg configure line reviewed
- [ ] no `--enable-nonfree` unless a private/internal package is being built
- [ ] `licenses/PROJECT-LICENSE.txt` present
- [ ] `licenses/THIRD_PARTY_NOTICES.md` present
- [ ] `licenses/go-modules.txt` present
- [ ] `components-manifest.json` present
- [ ] model provenance reviewed if `include_models` is enabled

## Production Readiness

- [ ] production config targets `/originals`
- [ ] storage mode is `strict_read_only`
- [ ] `dev_no_auth` is not the production default
- [ ] PostgreSQL URL is set
- [ ] cache/model/component/export dirs are outside `/originals`
- [ ] admin bootstrap password comes from env or file
- [ ] offline maps are not assumed unless explicitly supplied

## Smoke Tests

- [ ] `bash scripts/release/check-licenses.sh`
- [ ] `bash scripts/release/smoke-release.sh`
- [ ] `bash scripts/release/smoke-production-compose.sh`

## Release Notes

Record:

- build mode;
- bundled tools;
- bundled model caches;
- unsupported features;
- known limitations;
- provenance caveats.

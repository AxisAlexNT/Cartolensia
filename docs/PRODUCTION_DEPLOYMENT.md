# Production Deployment

This page describes the intended production target for Cartolensia.

## Deployment Shapes

Cartolensia can run in:

- a VM;
- bare metal;
- a container stack.

The original media archive is mounted at `/originals` and remains read-only.

## Recommended Directory Layout

Use writable paths outside the archive:

- `/var/lib/cartolensia/cache`
- `/var/lib/cartolensia/models`
- `/var/lib/cartolensia/components`
- `/var/lib/cartolensia/exports`

Do not point these paths back into `/originals`.

## Million-File Archives

For archives with up to around a million files:

- keep discovery prefix-scoped;
- use bounded folder workers and file workers;
- leave preview generation off unless needed;
- paginate every list endpoint;
- prefer search filters over unbounded listing;
- keep map queries bbox/zoom limited;
- scope AI/OCR/ASR jobs to the current selection or current prefix.

The discovery page defaults to `max_files = -1` for normal indexing, which means unlimited. Dry-run previews remain capped and are labeled as preview-only.

## Host Or VM Example

1. Mount the archive read-only:

```bash
mount -o ro /dev/sdX /originals
```

2. Provide a PostgreSQL database URL.
3. Start with:

```bash
go run ./cmd/cartolensia -config config/production.yaml
```

4. Open the app at the configured local bind address.

## Container Example

1. Copy `.env.production.example` to `.env.production`.
2. Adjust the database password and admin bootstrap values.
3. Mount `/originals:ro`.
4. Start:

```bash
docker compose -f docker-compose.production.yml --env-file .env.production up -d
```

The container example expects:

- a PostgreSQL service from the compose file;
- local cache/model/component volumes;
- no `dev_no_auth` mode.

## Authentication

Production auth is local bootstrap, not a hardcoded password:

- `auth.mode: local`
- `auth.admin_email` configured
- bootstrap password from `CARTOLENSIA_ADMIN_PASSWORD` or a password file

For reverse-proxy TLS, set `cookie_secure: true` in the runtime config or use a derived production config.

## Updates

Upgrade by replacing the application binary and WebUI assets, then restart the service. Do not re-run destructive archive operations against `/originals`.

If you move or add files under `/originals`, re-run discovery with the same storage name and prefix. The pipeline is intended to be safely re-runnable.

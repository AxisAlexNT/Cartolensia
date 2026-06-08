# Installation

Cartolensia can be run in two main ways:

1. from the repository for development and fixture testing;
2. from a production release bundle for an offline archive mounted at `/originals`.

## Development

Install Go 1.22+, Node.js 22+, and PostgreSQL 16 if you want the full local stack. Then run:

```bash
go test ./...
npm --prefix webui ci
npm --prefix webui run build
go run ./cmd/cartolensia -config config/dev-postgres.yaml
```

The default development config uses `dev_no_auth`. Do not use it for production.

## Production Host Or VM

Use `config/production.yaml` for a bare-metal host or VM and mount originals read-only at `/originals`.

Typical steps:

1. Provision PostgreSQL 16 or use the packaged PostgreSQL runtime from the release bundle.
2. Mount the archive read-only at `/originals`.
3. Create writable runtime directories outside originals, for example:
   - `/var/lib/cartolensia/cache`
   - `/var/lib/cartolensia/models`
   - `/var/lib/cartolensia/components`
   - `/var/lib/cartolensia/exports`
4. Set `CARTOLENSIA_DATABASE_URL` and the first admin password through `CARTOLENSIA_ADMIN_PASSWORD` or `CARTOLENSIA_ADMIN_PASSWORD_FILE`.
5. Start the app with:

```bash
go run ./cmd/cartolensia -config config/production.yaml
```

or with the offline release bundle:

```bash
./start-cartolensia.sh
```

## Production Container

Use `docker-compose.production.yml` with `config/production-container.yaml`.

Example:

```bash
cp .env.production.example .env.production
$EDITOR .env.production
docker compose -f docker-compose.production.yml --env-file .env.production up -d
```

The container example assumes:

- `/originals` is mounted read-only;
- PostgreSQL runs as the `postgres` service in the compose file;
- the app stores cache and optional component/model data under `/var/lib/cartolensia`.

## First Login

Production auth uses local bootstrap:

- `auth.mode: local`
- `auth.admin_email` set in config
- `CARTOLENSIA_ADMIN_PASSWORD` or `CARTOLENSIA_ADMIN_PASSWORD_FILE`

After the first login, rotate the password if required by your deployment policy.

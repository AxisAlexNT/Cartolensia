#!/usr/bin/env bash
set -euo pipefail

# Upgrade a boot-managed Cartolensia host from a Git repository while preserving
# PostgreSQL metadata, local component/model caches, and the read-only originals
# mount. Run on the Cartolensia host as root or through sudo.

CARTOLENSIA_USER="${CARTOLENSIA_USER:-cartolensia}"
CARTOLENSIA_APP_ROOT="${CARTOLENSIA_APP_ROOT:-/opt/cartolensia}"
CARTOLENSIA_CURRENT="${CARTOLENSIA_CURRENT:-${CARTOLENSIA_APP_ROOT}/current}"
CARTOLENSIA_RELEASES_DIR="${CARTOLENSIA_RELEASES_DIR:-${CARTOLENSIA_APP_ROOT}/releases}"
CARTOLENSIA_SOURCE_DIR="${CARTOLENSIA_SOURCE_DIR:-${CARTOLENSIA_APP_ROOT}/source}"
CARTOLENSIA_DATA_DIR="${CARTOLENSIA_DATA_DIR:-/var/lib/cartolensia}"
CARTOLENSIA_CONFIG_DIR="${CARTOLENSIA_CONFIG_DIR:-/etc/cartolensia}"
CARTOLENSIA_ORIGINALS="${CARTOLENSIA_ORIGINALS:-/originals}"
CARTOLENSIA_REPO_URL="${CARTOLENSIA_REPO_URL:-https://github.com/AxisAlexNT/Cartolensia.git}"
CARTOLENSIA_BRANCH="${CARTOLENSIA_BRANCH:-main}"
CARTOLENSIA_POSTGRES_PORT="${CARTOLENSIA_POSTGRES_PORT:-15432}"
CARTOLENSIA_DB_NAME="${CARTOLENSIA_DB_NAME:-cartolensia}"
CARTOLENSIA_DB_USER="${CARTOLENSIA_DB_USER:-cartolensia}"
CARTOLENSIA_DB_HOST="${CARTOLENSIA_DB_HOST:-${CARTOLENSIA_DATA_DIR}/run}"
SKIP_BACKUP="${CARTOLENSIA_SKIP_DB_BACKUP:-0}"

if [ "$(id -u)" -ne 0 ]; then
  echo "This script must run as root or through sudo." >&2
  exit 1
fi

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Required command is missing: $1" >&2
    exit 1
  }
}

need_cmd git
need_cmd rsync
need_cmd go
need_cmd npm
need_cmd findmnt

if [ ! -e "${CARTOLENSIA_ORIGINALS}" ]; then
  echo "Originals mount path does not exist: ${CARTOLENSIA_ORIGINALS}" >&2
  exit 1
fi

mount_opts="$(findmnt -T "${CARTOLENSIA_ORIGINALS}" -no OPTIONS 2>/dev/null || true)"
if [ -z "${mount_opts}" ]; then
  echo "Originals path is not a mounted filesystem: ${CARTOLENSIA_ORIGINALS}" >&2
  exit 1
fi
case ",${mount_opts}," in
  *,ro,*) ;;
  *)
    echo "Originals mount is not read-only according to findmnt: ${CARTOLENSIA_ORIGINALS}" >&2
    echo "Mount options: ${mount_opts}" >&2
    exit 1
    ;;
esac

install -d -m 0755 "${CARTOLENSIA_APP_ROOT}" "${CARTOLENSIA_RELEASES_DIR}"
install -d -m 0750 -o "${CARTOLENSIA_USER}" -g "${CARTOLENSIA_USER}" \
  "${CARTOLENSIA_DATA_DIR}/exports/backups" \
  "${CARTOLENSIA_DATA_DIR}/cache" \
  "${CARTOLENSIA_DATA_DIR}/cache/go-build" \
  "${CARTOLENSIA_DATA_DIR}/components" \
  "${CARTOLENSIA_DATA_DIR}/models" \
  "${CARTOLENSIA_DATA_DIR}/logs"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
current_target=""
if [ -L "${CARTOLENSIA_CURRENT}" ]; then
  current_target="$(readlink -f "${CARTOLENSIA_CURRENT}")"
elif [ -d "${CARTOLENSIA_CURRENT}" ]; then
  current_target="${CARTOLENSIA_CURRENT}"
fi

if [ -n "${current_target}" ] && [ ! -d "${current_target}" ]; then
  echo "Current release target is not a directory: ${current_target}" >&2
  exit 1
fi

if [ "${SKIP_BACKUP}" != "1" ]; then
  pg_dump_bin=""
  if [ -n "${current_target}" ] && [ -x "${current_target}/components/postgres/bin/pg_dump" ]; then
    pg_dump_bin="${current_target}/components/postgres/bin/pg_dump"
  elif command -v pg_dump >/dev/null 2>&1; then
    pg_dump_bin="$(command -v pg_dump)"
  fi
  if [ -z "${pg_dump_bin}" ]; then
    echo "pg_dump is not available; set CARTOLENSIA_SKIP_DB_BACKUP=1 only if you made a backup manually." >&2
    exit 1
  fi
  backup_file="${CARTOLENSIA_DATA_DIR}/exports/backups/cartolensia-${timestamp}.dump"
  echo "Writing PostgreSQL backup: ${backup_file}"
  sudo -u "${CARTOLENSIA_USER}" env \
    PGHOST="${CARTOLENSIA_DB_HOST}" \
    PGPORT="${CARTOLENSIA_POSTGRES_PORT}" \
    PGUSER="${CARTOLENSIA_DB_USER}" \
    "${pg_dump_bin}" -Fc -f "${backup_file}" "${CARTOLENSIA_DB_NAME}"
fi

if [ ! -d "${CARTOLENSIA_SOURCE_DIR}/.git" ]; then
  install -d -m 0755 "$(dirname "${CARTOLENSIA_SOURCE_DIR}")"
  git clone "${CARTOLENSIA_REPO_URL}" "${CARTOLENSIA_SOURCE_DIR}"
fi
chown -R "${CARTOLENSIA_USER}:${CARTOLENSIA_USER}" "${CARTOLENSIA_SOURCE_DIR}" "${CARTOLENSIA_DATA_DIR}/cache/go-build"

sudo -u "${CARTOLENSIA_USER}" git -C "${CARTOLENSIA_SOURCE_DIR}" fetch --prune origin
sudo -u "${CARTOLENSIA_USER}" git -C "${CARTOLENSIA_SOURCE_DIR}" checkout "${CARTOLENSIA_BRANCH}"
sudo -u "${CARTOLENSIA_USER}" git -C "${CARTOLENSIA_SOURCE_DIR}" pull --ff-only origin "${CARTOLENSIA_BRANCH}"
commit="$(git -C "${CARTOLENSIA_SOURCE_DIR}" rev-parse --short=12 HEAD)"
new_release="${CARTOLENSIA_RELEASES_DIR}/git-${timestamp}-${commit}"

echo "Preparing release: ${new_release}"
if [ -n "${current_target}" ]; then
  rsync -a --exclude '.git' "${current_target}/" "${new_release}/"
else
  install -d -m 0755 "${new_release}"
fi

install -d -m 0755 "${new_release}/bin" "${new_release}/webui/dist"
chown -R "${CARTOLENSIA_USER}:${CARTOLENSIA_USER}" "${new_release}"

echo "Building backend"
sudo -u "${CARTOLENSIA_USER}" env \
  GOCACHE="${CARTOLENSIA_DATA_DIR}/cache/go-build" \
  GOTOOLCHAIN=local \
  go -C "${CARTOLENSIA_SOURCE_DIR}" build -o "${new_release}/bin/cartolensia" ./cmd/cartolensia

echo "Building WebUI"
sudo -u "${CARTOLENSIA_USER}" npm --prefix "${CARTOLENSIA_SOURCE_DIR}/webui" ci
sudo -u "${CARTOLENSIA_USER}" npm --prefix "${CARTOLENSIA_SOURCE_DIR}/webui" run build
rsync -a --delete "${CARTOLENSIA_SOURCE_DIR}/webui/dist/" "${new_release}/webui/dist/"

echo "Overlaying scripts, configs, docs, and migrations"
rsync -a --delete "${CARTOLENSIA_SOURCE_DIR}/config/" "${new_release}/config/"
rsync -a --delete "${CARTOLENSIA_SOURCE_DIR}/scripts/" "${new_release}/scripts/"
rsync -a --delete "${CARTOLENSIA_SOURCE_DIR}/docs/" "${new_release}/docs/"
rsync -a --delete "${CARTOLENSIA_SOURCE_DIR}/migrations/" "${new_release}/migrations/"
rsync -a "${CARTOLENSIA_SOURCE_DIR}/README.md" "${CARTOLENSIA_SOURCE_DIR}/AGENTS.md" "${new_release}/"

cat >"${new_release}/VERSION" <<EOF
source=${CARTOLENSIA_REPO_URL}
branch=${CARTOLENSIA_BRANCH}
commit=${commit}
built_at=${timestamp}
EOF

chown -R "${CARTOLENSIA_USER}:${CARTOLENSIA_USER}" "${new_release}"

if [ -n "${current_target}" ]; then
  ln -sfn "${current_target}" "${CARTOLENSIA_APP_ROOT}/previous"
  chown -h "${CARTOLENSIA_USER}:${CARTOLENSIA_USER}" "${CARTOLENSIA_APP_ROOT}/previous"
fi

ln -sfn "${new_release}" "${CARTOLENSIA_CURRENT}"
chown -h "${CARTOLENSIA_USER}:${CARTOLENSIA_USER}" "${CARTOLENSIA_CURRENT}"

systemctl daemon-reload
systemctl restart cartolensia-ai.service
systemctl restart cartolensia.service

echo "Upgrade complete."
echo "Current release: ${new_release}"
echo "Previous release symlink: ${CARTOLENSIA_APP_ROOT}/previous"
echo "Database backup directory: ${CARTOLENSIA_DATA_DIR}/exports/backups"
echo "Originals were checked as read-only by mount options and were not written."

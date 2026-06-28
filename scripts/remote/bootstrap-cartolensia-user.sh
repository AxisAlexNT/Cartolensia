#!/usr/bin/env bash
set -euo pipefail

# Run on the remote host as root, usually from an operator workstation through:
#   ssh <admin-user>@<remote-host> "sudo env CARTOLENSIA_PUBKEY='$(cat ~/.ssh/cartolensia_remote_ed25519.pub)' bash -s" \
#     < scripts/remote/bootstrap-cartolensia-user.sh

CARTOLENSIA_USER="${CARTOLENSIA_USER:-cartolensia}"
CARTOLENSIA_UID="${CARTOLENSIA_UID:-}"
CARTOLENSIA_HOME="${CARTOLENSIA_HOME:-/home/${CARTOLENSIA_USER}}"
CARTOLENSIA_APP_ROOT="${CARTOLENSIA_APP_ROOT:-/opt/cartolensia}"
CARTOLENSIA_CURRENT="${CARTOLENSIA_CURRENT:-${CARTOLENSIA_APP_ROOT}/current}"
CARTOLENSIA_DATA_DIR="${CARTOLENSIA_DATA_DIR:-/var/lib/cartolensia}"
CARTOLENSIA_CONFIG_DIR="${CARTOLENSIA_CONFIG_DIR:-/etc/cartolensia}"
CARTOLENSIA_ORIGINALS="${CARTOLENSIA_ORIGINALS:-/originals}"
CARTOLENSIA_PUBKEY="${CARTOLENSIA_PUBKEY:-}"
AI_FLAVOR="${CARTOLENSIA_AI_FLAVOR:-nvidia-cu128}"
AI_BIND="${CARTOLENSIA_AI_BIND:-0.0.0.0}"
AI_PORT="${CARTOLENSIA_AI_PORT:-19090}"
HTTP_ADDR="${CARTOLENSIA_HTTP_ADDR:-:18080}"
HTTPS_ADDR="${CARTOLENSIA_HTTP_TLS_ADDR:-:18443}"
HTTPS_HOSTS="${CARTOLENSIA_HTTP_TLS_HOSTS:-127.0.0.1,localhost}"
POSTGRES_PORT="${CARTOLENSIA_POSTGRES_PORT:-15432}"
GRANT_PASSWORDLESS_SUDO="${CARTOLENSIA_GRANT_PASSWORDLESS_SUDO:-0}"

if [ "$(id -u)" -ne 0 ]; then
  echo "This script must run as root or through sudo." >&2
  exit 1
fi
if [ -z "${CARTOLENSIA_PUBKEY}" ]; then
  echo "CARTOLENSIA_PUBKEY is required." >&2
  exit 1
fi

ensure_group() {
  getent group "$1" >/dev/null || groupadd "$1"
}

ensure_group docker
ensure_group video
ensure_group render

if ! id "${CARTOLENSIA_USER}" >/dev/null 2>&1; then
  args=(-m -d "${CARTOLENSIA_HOME}" -s /bin/bash)
  if [ -n "${CARTOLENSIA_UID}" ]; then
    args+=(-u "${CARTOLENSIA_UID}")
  fi
  useradd "${args[@]}" "${CARTOLENSIA_USER}"
fi

usermod -aG docker,video,render "${CARTOLENSIA_USER}"
if getent group sudo >/dev/null; then
  usermod -aG sudo "${CARTOLENSIA_USER}" || true
elif getent group wheel >/dev/null; then
  usermod -aG wheel "${CARTOLENSIA_USER}" || true
fi

install -d -m 0700 -o "${CARTOLENSIA_USER}" -g "${CARTOLENSIA_USER}" "${CARTOLENSIA_HOME}/.ssh"
auth_keys="${CARTOLENSIA_HOME}/.ssh/authorized_keys"
touch "${auth_keys}"
chmod 0600 "${auth_keys}"
chown "${CARTOLENSIA_USER}:${CARTOLENSIA_USER}" "${auth_keys}"
if ! grep -qxF "${CARTOLENSIA_PUBKEY}" "${auth_keys}"; then
  printf '%s\n' "${CARTOLENSIA_PUBKEY}" >>"${auth_keys}"
fi

install -d -m 0755 "${CARTOLENSIA_APP_ROOT}"
install -d -m 0750 -o "${CARTOLENSIA_USER}" -g "${CARTOLENSIA_USER}" "${CARTOLENSIA_DATA_DIR}" "${CARTOLENSIA_DATA_DIR}/"{cache,components,models,exports,logs,run,postgres,ai-extra-site}
install -d -m 0755 "${CARTOLENSIA_ORIGINALS}"
install -d -m 0750 "${CARTOLENSIA_CONFIG_DIR}"
chown -R "${CARTOLENSIA_USER}:${CARTOLENSIA_USER}" "${CARTOLENSIA_APP_ROOT}" "${CARTOLENSIA_DATA_DIR}"

admin_password_file="${CARTOLENSIA_CONFIG_DIR}/admin-password"
if [ ! -s "${admin_password_file}" ]; then
  umask 077
  tr -dc 'A-Za-z0-9_@%+=:,.~-' </dev/urandom | head -c 32 >"${admin_password_file}"
  printf '\n' >>"${admin_password_file}"
fi

cat >"${CARTOLENSIA_CONFIG_DIR}/cartolensia.env" <<ENV
CARTOLENSIA_HOME=${CARTOLENSIA_CURRENT}
CARTOLENSIA_DATA_DIR=${CARTOLENSIA_DATA_DIR}
CARTOLENSIA_CONFIG=${CARTOLENSIA_CURRENT}/config/production-bundle.yaml
CARTOLENSIA_ADMIN_PASSWORD_FILE=${admin_password_file}
CARTOLENSIA_HTTP_ADDR=${HTTP_ADDR}
CARTOLENSIA_HTTP_TLS_ADDR=${HTTPS_ADDR}
CARTOLENSIA_HTTP_REDIRECT_HTTP_TO_HTTPS=true
CARTOLENSIA_HTTP_TLS_AUTO_SELF_SIGNED=true
CARTOLENSIA_HTTP_TLS_HOSTS=${HTTPS_HOSTS}
CARTOLENSIA_AUTH_COOKIE_SECURE=true
CARTOLENSIA_POSTGRES_PORT=${POSTGRES_PORT}
CARTOLENSIA_AI_WORKER_ENDPOINT=http://127.0.0.1:${AI_PORT}
CARTOLENSIA_COMPONENT_DIR=${CARTOLENSIA_DATA_DIR}/components
CARTOLENSIA_AI_MODEL_DIR=${CARTOLENSIA_DATA_DIR}/models
CARTOLENSIA_MODEL_DIR=${CARTOLENSIA_DATA_DIR}/models
CARTOLENSIA_AI_EXTRA_SITE=${CARTOLENSIA_DATA_DIR}/ai-extra-site
PYTHONPATH=${CARTOLENSIA_DATA_DIR}/ai-extra-site
CARTOLENSIA_AI_FLAVOR=${AI_FLAVOR}
CARTOLENSIA_LIBVA_DRIVER_NAME=radeonsi
CARTOLENSIA_VDPAU_DRIVER=radeonsi
CARTOLENSIA_TRANSCODE_PREFERRED_ACCELERATORS=nvidia,vaapi,cpu
ENV
chmod 0640 "${CARTOLENSIA_CONFIG_DIR}/cartolensia.env" "${admin_password_file}"
chown root:"${CARTOLENSIA_USER}" "${CARTOLENSIA_CONFIG_DIR}/cartolensia.env" "${admin_password_file}"

if [ "${GRANT_PASSWORDLESS_SUDO}" = "1" ]; then
  cat >"/etc/sudoers.d/90-cartolensia" <<EOF
${CARTOLENSIA_USER} ALL=(ALL) NOPASSWD:ALL
EOF
  chmod 0440 "/etc/sudoers.d/90-cartolensia"
fi

cat >/etc/systemd/system/cartolensia-postgres.service <<'UNIT'
[Unit]
Description=Cartolensia bundled PostgreSQL
After=network-online.target
Wants=network-online.target
ConditionPathExists=/opt/cartolensia/current/components/postgres/bin/postgres

[Service]
Type=simple
User=cartolensia
Group=cartolensia
SupplementaryGroups=video render docker
EnvironmentFile=/etc/cartolensia/cartolensia.env
WorkingDirectory=/opt/cartolensia/current
ExecStartPre=/bin/bash -lc 'source /opt/cartolensia/current/bin/cartolensia-env; exec /opt/cartolensia/current/bin/ensure-postgres-db'
ExecStart=/bin/bash -lc 'source /opt/cartolensia/current/bin/cartolensia-env; exec components/postgres/bin/postgres -D "$CARTOLENSIA_DATA_DIR/postgres" -p "${CARTOLENSIA_POSTGRES_PORT:-15432}" -k "$CARTOLENSIA_DATA_DIR/run" ${CARTOLENSIA_POSTGRES_TUNING_OPTS:--c dynamic_shared_memory_type=mmap -c wal_compression=on -c checkpoint_timeout=15min -c checkpoint_completion_target=0.9 -c max_wal_size=8GB -c effective_io_concurrency=200 -c random_page_cost=1.1 -c maintenance_work_mem=512MB -c autovacuum_vacuum_scale_factor=0.05 -c autovacuum_analyze_scale_factor=0.02}'
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
UNIT

cat >/etc/systemd/system/cartolensia-ai.service <<'UNIT'
[Unit]
Description=Cartolensia AI executor
After=network-online.target
Wants=network-online.target
ConditionPathExists=/opt/cartolensia/current/bin/start-ai-executor

[Service]
Type=simple
User=cartolensia
Group=cartolensia
SupplementaryGroups=video render docker
EnvironmentFile=/etc/cartolensia/cartolensia.env
WorkingDirectory=/opt/cartolensia/current
Environment=NVIDIA_VISIBLE_DEVICES=all
Environment=NVIDIA_DRIVER_CAPABILITIES=compute,video,utility
Environment=LIBVA_DRIVER_NAME=radeonsi
Environment=VDPAU_DRIVER=radeonsi
ExecStart=/bin/bash -lc 'source /opt/cartolensia/current/bin/cartolensia-env; exec ./bin/start-ai-executor "${CARTOLENSIA_AI_FLAVOR:-nvidia-cu128}" "${CARTOLENSIA_AI_BIND:-0.0.0.0}" "${CARTOLENSIA_AI_PORT:-19090}"'
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT

cat >/etc/systemd/system/cartolensia.service <<'UNIT'
[Unit]
Description=Cartolensia multimedia archive
After=network-online.target cartolensia-postgres.service cartolensia-ai.service
Wants=network-online.target cartolensia-postgres.service cartolensia-ai.service
ConditionPathExists=/opt/cartolensia/current/bin/cartolensia

[Service]
Type=simple
User=cartolensia
Group=cartolensia
SupplementaryGroups=video render docker
EnvironmentFile=/etc/cartolensia/cartolensia.env
WorkingDirectory=/opt/cartolensia/current
Environment=NVIDIA_VISIBLE_DEVICES=all
Environment=NVIDIA_DRIVER_CAPABILITIES=compute,video,utility
Environment=LIBVA_DRIVER_NAME=radeonsi
Environment=VDPAU_DRIVER=radeonsi
ExecStart=/bin/bash -lc 'source /opt/cartolensia/current/bin/cartolensia-env; exec ./bin/cartolensia -config "$CARTOLENSIA_CONFIG"'
Restart=on-failure
RestartSec=5
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable cartolensia-postgres.service cartolensia-ai.service cartolensia.service

cat <<EOF
Remote bootstrap complete.

Next steps on the remote host:
1. Copy/extract the Cartolensia bundle into ${CARTOLENSIA_APP_ROOT}/releases/<version>.
2. Point ${CARTOLENSIA_CURRENT} to that extracted directory:
   sudo ln -sfn ${CARTOLENSIA_APP_ROOT}/releases/<version> ${CARTOLENSIA_CURRENT}
   sudo chown -h ${CARTOLENSIA_USER}:${CARTOLENSIA_USER} ${CARTOLENSIA_CURRENT}
3. Ensure originals are mounted read-only at ${CARTOLENSIA_ORIGINALS}.
4. Start services:
   sudo systemctl start cartolensia-postgres cartolensia-ai cartolensia
5. Login with admin password from ${admin_password_file}.

SSH from the operator workstation should work with the alias/key configured there,
for example:
   ssh <cartolensia-host-alias>
EOF

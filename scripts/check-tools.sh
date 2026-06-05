#!/usr/bin/env bash
set -u

status=0

print_version() {
  "$@" 2>&1 | sed -n '1p'
}

check_required() {
  local label="$1"
  local command_name="$2"
  shift 2

  if command -v "${command_name}" >/dev/null 2>&1; then
    printf '%-16s found    ' "${label}"
    print_version "$@"
  else
    printf '%-16s missing\n' "${label}"
    status=1
  fi
}

check_optional() {
  local label="$1"
  local command_name="$2"
  shift 2

  if command -v "${command_name}" >/dev/null 2>&1; then
    printf '%-16s found    ' "${label}"
    print_version "$@"
    return 0
  fi

  printf '%-16s missing\n' "${label}"
  return 1
}

check_required "go" "go" go version
check_required "node" "node" node --version

npm_available=0
if check_optional "npm" "npm" npm --version; then
  npm_available=1
fi
if check_optional "pnpm" "pnpm" pnpm --version; then
  npm_available=1
fi
if [ "${npm_available}" -eq 0 ]; then
  status=1
fi

check_required "docker" "docker" docker --version

if command -v docker >/dev/null 2>&1; then
  printf '%-16s ' "docker compose"
  if docker compose version >/dev/null 2>&1; then
    printf 'found    '
    docker compose version
  else
    printf 'missing\n'
    status=1
  fi
else
  printf '%-16s missing\n' "docker compose"
  status=1
fi

check_required "psql" "psql" psql --version
check_required "ffmpeg" "ffmpeg" ffmpeg -version
check_required "ffprobe" "ffprobe" ffprobe -version

exit "${status}"

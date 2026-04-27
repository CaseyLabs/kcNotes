#!/bin/sh
set -eu

BASE_URL=${1:-${BASE_URL:-http://localhost:5555}}
SMOKE_TIMEOUT_SECONDS=${SMOKE_TIMEOUT_SECONDS:-60}

check() {
  url=$1
  expected=${2:-200}
  started_at=$(date +%s)
  code=000
  while :; do
    if ! code=$(curl -sS -o /dev/null -w "%{http_code}" "${url}" 2>/dev/null); then
      code=000
    fi
    if [ "${code}" = "${expected}" ]; then
      break
    fi
    now=$(date +%s)
    if [ $((now - started_at)) -ge "${SMOKE_TIMEOUT_SECONDS}" ]; then
      printf 'smoke check failed: %s expected %s got %s\n' "${url}" "${expected}" "${code}" >&2
      exit 1
    fi
    sleep 1
  done
  printf 'ok %s %s\n' "${code}" "${url}"
}

check "${BASE_URL}/healthz" 200
check "${BASE_URL}/" 200
check "${BASE_URL}/admin/login" 200

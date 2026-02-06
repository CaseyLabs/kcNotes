#!/bin/sh
set -eu

BASE_URL=${1:-${BASE_URL:-http://localhost:5555}}

check() {
  url=$1
  expected=${2:-200}
  code=$(curl -sS -o /dev/null -w "%{http_code}" "${url}")
  if [ "${code}" != "${expected}" ]; then
    printf 'smoke check failed: %s expected %s got %s\n' "${url}" "${expected}" "${code}" >&2
    exit 1
  fi
  printf 'ok %s %s\n' "${code}" "${url}"
}

check "${BASE_URL}/healthz" 200
check "${BASE_URL}/" 200
check "${BASE_URL}/admin/login" 200

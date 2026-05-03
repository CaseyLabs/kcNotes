#!/bin/sh
set -eu

src_dir=${1:-src}
cd "${src_dir}"

mkdir -p /workspace/.cache
lock_hash=$(sha256sum package-lock.json | awk '{print $1}')
marker=node_modules/.kcnotes-package-lock.sha256

if [ "${CI:-}" = 'true' ] || [ ! -d node_modules ] || [ ! -f "${marker}" ] || [ "$(cat "${marker}")" != "${lock_hash}" ]; then
	npm ci --no-fund --no-audit --silent
	mkdir -p node_modules
	printf '%s\n' "${lock_hash}" >"${marker}"
fi

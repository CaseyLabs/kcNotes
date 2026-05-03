#!/bin/sh
set -eu

src_dir=${1:-src}
cd "${src_dir}"

mkdir -p /workspace/.cache
install_hash=$(sha256sum package.json package-lock.json | sha256sum | awk '{print $1}')
marker=node_modules/.kcnotes-npm-install-inputs.sha256

if [ ! -d node_modules ] || [ ! -f "${marker}" ] || [ "$(cat "${marker}")" != "${install_hash}" ]; then
	npm ci --no-fund --no-audit --silent
	mkdir -p node_modules
	printf '%s\n' "${install_hash}" >"${marker}"
fi

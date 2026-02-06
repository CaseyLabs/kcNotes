#!/bin/sh
set -eu

PROJECT_CFG_FILE=${1:-${PROJECT_CFG_FILE:-config/project.cfg}}
. ./scripts/app-lib.sh

ensure_dev_image
docker run --rm -it --user "${docker_uid}:${docker_gid}" \
  -e HOME="${docker_home}" \
  -e XDG_CACHE_HOME="${docker_cache_home}" \
  -v "${docker_home_source}:${docker_home}" \
  -v "${docker_tmpdir}:/tmp" \
  -v "$(pwd):/workspace" \
  -w /workspace \
  "${app_image}" \
  sh

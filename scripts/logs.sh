#!/bin/sh
set -eu

PROJECT_CFG_FILE=${1:-${PROJECT_CFG_FILE:-config/project.cfg}}
. ./scripts/app-lib.sh
docker ps -aq --filter "label=com.kcnotes.project=${app_label}" | while IFS= read -r container_id; do
  [ -n "${container_id}" ] || continue
  docker logs "${container_id}"
done

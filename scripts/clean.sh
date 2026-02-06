#!/bin/sh
set -eu

PROJECT_CFG_FILE=${1:-${PROJECT_CFG_FILE:-config/project.cfg}}
. ./scripts/app-lib.sh

stop_app_containers
docker image rm "${app_image}" >/dev/null 2>&1 || true

for path in dist .cache .tmp src/data src/dist src/node_modules src/.cache src/.config src/web/static/css/app.css; do
  [ -e "$path" ] || continue
  chmod -R u+w "$path" 2>/dev/null || true
done

rm -rf dist .cache .tmp src/data src/dist src/node_modules src/.cache src/.config src/web/static/css/app.css

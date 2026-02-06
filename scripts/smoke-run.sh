#!/bin/sh
set -eu

PROJECT_CFG_FILE=${1:-${PROJECT_CFG_FILE:-config/project.cfg}}
. ./scripts/app-lib.sh

ensure_dev_image
if [ -n "${BASE_URL:-}" ]; then
  exec ./scripts/smoke.sh "${BASE_URL}"
fi
run_in_dev_container "cd src && mkdir -p /workspace/.cache && GOFLAGS='-tags=sqlite_fts5' go run ./cmd/cms -mode serve >/tmp/cms-smoke.log 2>&1 & sleep 2; /workspace/scripts/smoke.sh http://127.0.0.1:8080"

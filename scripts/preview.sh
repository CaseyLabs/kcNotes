#!/bin/sh
set -eu

PROJECT_CFG_FILE=${1:-${PROJECT_CFG_FILE:-config/project.cfg}}
. ./scripts/app-lib.sh

ensure_dev_image
stop_app_containers
run_web_container "preview" "cd src && mkdir -p /workspace/.cache && PUBLISH_OUT_DIR=\"\${PUBLISH_OUT_DIR:-../dist/site}\" PREVIEW_HTTP_ADDR=':8080' GOFLAGS='-tags=sqlite_fts5' go run ./cmd/cms -mode preview"

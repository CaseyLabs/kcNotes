#!/bin/sh
set -eu

PROJECT_CFG_FILE=${1:-${PROJECT_CFG_FILE:-config/project.cfg}}
. ./scripts/app-lib.sh

ensure_dev_image
stop_app_containers
printf '\n==> Run kcNotes\n'
run_web_container "run" "cd src && mkdir -p /workspace/.cache && GOFLAGS='-tags=sqlite_fts5' go run ./cmd/cms -mode serve"

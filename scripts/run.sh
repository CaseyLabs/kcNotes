#!/bin/sh
set -eu

PROJECT_CFG_FILE=${1:-${PROJECT_CFG_FILE:-config/project.cfg}}
. ./scripts/app-lib.sh

ensure_dev_image
stop_app_containers
printf '\n==> Run kcNotes\n'
run_web_container "run" "$(build_css_command) && GOFLAGS='-tags=sqlite_fts5' go run ./cmd/cms -mode migrate && GOFLAGS='-tags=sqlite_fts5' go run ./cmd/cms -mode serve"
wait_for_http_ok "http://127.0.0.1:${host_port}/healthz" 60
printf 'Ready: http://localhost:%s\n' "${host_port}"

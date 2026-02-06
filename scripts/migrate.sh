#!/bin/sh
set -eu

PROJECT_CFG_FILE=${1:-${PROJECT_CFG_FILE:-config/project.cfg}}
. ./scripts/app-lib.sh

ensure_dev_image
run_in_dev_container "cd src && mkdir -p /workspace/.cache && GOFLAGS='-tags=sqlite_fts5' go run ./cmd/cms -mode migrate"

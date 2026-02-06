#!/bin/sh
set -eu

PROJECT_CFG_FILE=${1:-${PROJECT_CFG_FILE:-config/project.cfg}}
. ./scripts/app-lib.sh

printf '\n==> Build kcNotes image\n'
build_dev_image

printf '\n==> Build kcNotes app\n'
ensure_dev_image
run_in_dev_container "cd src && mkdir -p /workspace/.cache && GOFLAGS='-tags=sqlite_fts5' go build ./..."

printf '\n==> Build summary\n'
printf '%s\n' "Image: ${app_image}"
printf '%s\n' "Project config: ${PROJECT_CFG_FILE}"
printf '%s\n' 'Result: build passed'

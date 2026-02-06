#!/bin/sh
set -eu

PROJECT_CFG_FILE=${1:-${PROJECT_CFG_FILE:-config/project.cfg}}
. ./scripts/app-lib.sh

ensure_dev_image
run_in_dev_container "cd src && mkdir -p /workspace/.cache && npm ci --no-fund --no-audit && BROWSERSLIST_IGNORE_OLD_DATA=1 npm run build:css"

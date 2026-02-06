#!/bin/sh
set -eu

PROJECT_CFG_FILE=${1:-${PROJECT_CFG_FILE:-config/project.cfg}}
. ./scripts/app-lib.sh
stop_app_containers

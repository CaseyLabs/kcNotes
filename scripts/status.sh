#!/bin/sh
set -eu

PROJECT_CFG_FILE=${1:-${PROJECT_CFG_FILE:-config/project.cfg}}
. ./scripts/app-lib.sh

printf '\n==> Image\n'
docker image ls "${app_image}" || true
printf '\n==> Containers\n'
docker ps -a --filter "label=com.kcnotes.project=${app_label}" || true

#!/bin/sh
# shellcheck shell=sh disable=SC1090
set -eu

PROJECT_CFG_FILE=${PROJECT_CFG_FILE:-config/project.cfg}
project_cfg_file=${PROJECT_CFG_FILE}
case "${project_cfg_file}" in
/* | ./* | ../*) ;;
*) project_cfg_file="./${project_cfg_file}" ;;
esac
[ -f "${project_cfg_file}" ] || {
  printf 'missing %s; set PROJECT_CFG_FILE to an existing config file\n' "${project_cfg_file}" >&2
  exit 1
}

. "${project_cfg_file}"

project_name=${PROJECT_NAME:-kcnotes}
project_image=${PROJECT_IMAGE:-${project_name}:local}
app_image=${APP_IMAGE:-${project_image}}
app_label=$(printf '%s' "${project_name}" | tr -c '[:alnum:]_.-' '-')
docker_uid=${DOCKER_UID:-$(id -u)}
docker_gid=${DOCKER_GID:-$(id -g)}
docker_home=${DOCKER_HOME:-/tmp/kcnotes-home}
docker_cache_home=${DOCKER_CACHE_HOME:-${docker_home}/.cache}
docker_home_source=${DOCKER_HOME_SOURCE:-$(pwd)/.cache/docker-home}
docker_tmpdir=${DOCKER_TMPDIR:-$(pwd)/.cache/docker-tmp}
host_port=${HOST_PORT:-5555}

mkdir -p "${docker_home_source}" "${docker_tmpdir}"

build_dev_image() {
  if [ -n "${DOCKER_BUILD_EXTRA_ARGS:-}" ]; then
    # shellcheck disable=SC2086
    docker buildx build --load \
      ${DOCKER_BUILD_EXTRA_ARGS} \
      --build-arg DEV_BASE_IMAGE="${DEV_GO_IMAGE_LOCK:-${DEV_GO_IMAGE}}" \
      -f Dockerfile \
      -t "${app_image}" .
  else
    docker build \
      --build-arg DEV_BASE_IMAGE="${DEV_GO_IMAGE_LOCK:-${DEV_GO_IMAGE}}" \
      -f Dockerfile \
      -t "${app_image}" .
  fi
}

ensure_dev_image() {
  if ! docker image inspect "${app_image}" >/dev/null 2>&1; then
    build_dev_image
  fi
}

run_in_dev_container() {
  command_string=$1
  docker run --rm --user "${docker_uid}:${docker_gid}" \
    --cap-drop=ALL \
    --security-opt=no-new-privileges:true \
    -e HOME="${docker_home}" \
    -e XDG_CACHE_HOME="${docker_cache_home}" \
    -e GOCACHE=/workspace/.cache/go-build \
    -e GOMODCACHE=/workspace/.cache/go-mod \
    -e npm_config_cache=/workspace/.cache/npm \
    -e APP_ENV \
    -e DB_MODE \
    -e DATABASE_URL \
    -e DATABASE_AUTH_TOKEN \
    -e DB_PATH \
    -e REPLICA_SYNC_INTERVAL \
    -e REPLICA_READ_YOUR_WRITES \
    -e UPLOAD_DIR \
    -e SESSION_COOKIE_NAME \
    -e CSRF_COOKIE_NAME \
    -e SESSION_TTL \
    -e COOKIE_SECURE \
    -e LOGIN_LOCKOUT_THRESHOLD \
    -e LOGIN_LOCKOUT_WINDOW \
    -e LOGIN_LOCKOUT_DURATION \
    -e MEDIA_USER_QUOTA_BYTES \
    -e MEDIA_TOTAL_QUOTA_BYTES \
    -e TRUSTED_PROXY_CIDRS \
    -e ENFORCE_TRUSTED_PROXY_CIDRS \
    -e SITE_BASE_URL \
    -e ADMIN_BASE_URL \
    -e ADMIN_COOKIE_PATH \
    -e PUBLISH_OUT_DIR \
    -e PUBLISH_INCLUDE_DRAFTS \
    -e PREVIEW_HTTP_ADDR \
    -e EMAIL \
    -e PASSWORD \
    -e ROLE \
    -v "${docker_home_source}:${docker_home}" \
    -v "${docker_tmpdir}:/tmp" \
    -v "$(pwd):/workspace" \
    -w /workspace \
    "${app_image}" \
    sh -ceu "${command_string}"
}

port_in_use() {
  candidate_port=$1
  if command -v ss >/dev/null 2>&1; then
    ss -H -ltn 2>/dev/null | awk '{print $4}' | grep -Eq ":${candidate_port}\$"
    return $?
  fi
  return 1
}

select_host_port() {
  if [ -z "${HOST_PORT+x}" ]; then
    candidate_port=${host_port}
    while port_in_use "${candidate_port}"; do
      candidate_port=$((candidate_port + 1))
    done
    if [ "${candidate_port}" != "${host_port}" ]; then
      printf 'port %s is busy; using %s (override with HOST_PORT)\n' "${host_port}" "${candidate_port}" >&2
      host_port=${candidate_port}
    fi
  fi
}

stop_app_containers() {
  container_ids=$(docker ps -aq --filter "label=com.kcnotes.project=${app_label}")
  if [ -n "${container_ids}" ]; then
    # shellcheck disable=SC2086
    docker rm -f ${container_ids} >/dev/null
  fi
}

run_web_container() {
  role_name=$1
  command_string=$2
  select_host_port
  container_name="${project_name}-${role_name}"
  docker rm -f "${container_name}" >/dev/null 2>&1 || true
  docker run -d \
    --name "${container_name}" \
    --label "com.kcnotes.project=${app_label}" \
    --label "com.kcnotes.role=${role_name}" \
    --user "${docker_uid}:${docker_gid}" \
    --cap-drop=ALL \
    --security-opt=no-new-privileges:true \
    -e HOME="${docker_home}" \
    -e XDG_CACHE_HOME="${docker_cache_home}" \
    -e GOCACHE=/workspace/.cache/go-build \
    -e GOMODCACHE=/workspace/.cache/go-mod \
    -e npm_config_cache=/workspace/.cache/npm \
    -e APP_ENV \
    -e DB_MODE \
    -e DATABASE_URL \
    -e DATABASE_AUTH_TOKEN \
    -e DB_PATH \
    -e REPLICA_SYNC_INTERVAL \
    -e REPLICA_READ_YOUR_WRITES \
    -e UPLOAD_DIR \
    -e SESSION_COOKIE_NAME \
    -e CSRF_COOKIE_NAME \
    -e SESSION_TTL \
    -e COOKIE_SECURE \
    -e LOGIN_LOCKOUT_THRESHOLD \
    -e LOGIN_LOCKOUT_WINDOW \
    -e LOGIN_LOCKOUT_DURATION \
    -e MEDIA_USER_QUOTA_BYTES \
    -e MEDIA_TOTAL_QUOTA_BYTES \
    -e TRUSTED_PROXY_CIDRS \
    -e ENFORCE_TRUSTED_PROXY_CIDRS \
    -e SITE_BASE_URL \
    -e ADMIN_BASE_URL \
    -e ADMIN_COOKIE_PATH \
    -e PUBLISH_OUT_DIR \
    -e PUBLISH_INCLUDE_DRAFTS \
    -e PREVIEW_HTTP_ADDR \
    -v "${docker_home_source}:${docker_home}" \
    -v "${docker_tmpdir}:/tmp" \
    -v "$(pwd):/workspace" \
    -w /workspace \
    -p "${host_port}:8080" \
    "${app_image}" \
    sh -ceu "${command_string}"
  printf 'URL: http://localhost:%s\n' "${host_port}"
}

wait_for_http_ok() {
  url=$1
  timeout_seconds=${2:-60}
  started_at=$(date +%s)
  while :; do
    if curl -fsS -o /dev/null "${url}" >/dev/null 2>&1; then
      return 0
    fi
    now=$(date +%s)
    if [ $((now - started_at)) -ge "${timeout_seconds}" ]; then
      printf 'timed out waiting for %s\n' "${url}" >&2
      return 1
    fi
    sleep 1
  done
}

build_css_command() {
  printf "%s" "cd src && mkdir -p /workspace/.cache && npm ci --no-fund --no-audit && BROWSERSLIST_IGNORE_OLD_DATA=1 npm run build:css"
}

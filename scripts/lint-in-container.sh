#!/bin/sh
set -eu

PROJECT_CFG_FILE=${1:-${PROJECT_CFG_FILE:-config/project.cfg}}
project_cfg_file=${PROJECT_CFG_FILE}
case "${project_cfg_file}" in
/* | ./* | ../*) ;;
*) project_cfg_file="./${project_cfg_file}" ;;
esac

[ -f "${project_cfg_file}" ] || {
  printf 'missing %s; set PROJECT_CFG_FILE to an existing config file\n' "${project_cfg_file}" >&2
  exit 1
}

# shellcheck disable=SC1090
. "${project_cfg_file}"

fail_if_file_has_content() {
  output_file=$1
  help_message=$2
  if [ -s "${output_file}" ]; then
    cat "${output_file}"
    echo "${help_message}"
    exit 1
  fi
}

mkdir -p /workspace/.cache/bin
export PATH=/workspace/.cache/bin:/usr/local/go/bin:"${PATH}"
export GOFLAGS='-tags=sqlite_fts5'

echo '[lint] Installing pinned lint/format tools'
GOBIN=/workspace/.cache/bin go install "mvdan.cc/sh/v3/cmd/shfmt@${DEV_LINT_SHFMT_VERSION}"
GOBIN=/workspace/.cache/bin go install "honnef.co/go/tools/cmd/staticcheck@${DEV_LINT_STATICCHECK_VERSION}"
GOBIN=/workspace/.cache/bin go install "github.com/checkmake/checkmake/cmd/checkmake@${DEV_LINT_CHECKMAKE_VERSION}"

cd /workspace/src
npm ci --no-fund --no-audit --silent

echo '[fmt-check] gofmt'
GOFMT_OUT=/tmp/gofmt.out
find . -type f -name '*.go' -not -path './node_modules/*' -print0 | xargs -0 -r gofmt -l >"${GOFMT_OUT}"
fail_if_file_has_content "${GOFMT_OUT}" 'Run: gofmt -w <files>'

echo '[fmt-check] prettier'
find . -type f \
  \( -name '*.js' -o -name '*.css' -o -name '*.html' -o -name '*.yml' -o -name '*.yaml' \) \
  -not -path './node_modules/*' \
  -not -path './web/static/css/app.css' \
  -not -path './web/static/js/vendor/*' \
  -print0 | xargs -0 -r npx --yes "prettier@${DEV_LINT_PRETTIER_VERSION}" --check
npx --yes "prettier@${DEV_LINT_PRETTIER_VERSION}" --check README.md 2>/dev/null || true

echo '[lint] go vet'
go vet ./...

echo '[lint] staticcheck'
staticcheck ./...

echo '[lint] shellcheck'
cd /workspace
find scripts -type f -name '*.sh' -print0 | xargs -0 -r shellcheck -x -e SC1091

echo '[lint] jshint'
cd /workspace/src
find . -type f -name '*.js' \
  -not -path './node_modules/*' \
  -not -path './web/static/js/vendor/*' \
  -print0 | xargs -0 -r npx --yes "jshint@${DEV_LINT_JSHINT_VERSION}"

echo '[lint] markdownlint'
cd /workspace
npx --yes "markdownlint-cli2@${DEV_LINT_MARKDOWNLINT_CLI2_VERSION}" README.md docs/*.md src/*.md

echo '[lint] checkmake'
checkmake Makefile

echo '[lint] Completed successfully.'

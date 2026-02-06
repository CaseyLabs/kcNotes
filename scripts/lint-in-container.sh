#!/bin/sh
set -eu

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
GOBIN=/workspace/.cache/bin go install mvdan.cc/sh/v3/cmd/shfmt@v3.12.0
GOBIN=/workspace/.cache/bin go install honnef.co/go/tools/cmd/staticcheck@2025.1.1
GOBIN=/workspace/.cache/bin go install github.com/checkmake/checkmake/cmd/checkmake@v0.3.2

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
  -not -path './web/static/js/vendor/*' \
  -print0 | xargs -0 -r npx --yes prettier@3.8.1 --check
npx --yes prettier@3.8.1 --check README.md 2>/dev/null || true

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
  -print0 | xargs -0 -r npx --yes jshint@2.13.6

echo '[lint] markdownlint'
cd /workspace
npx --yes markdownlint-cli2@0.20.0 README.md docs/*.md src/*.md

echo '[lint] checkmake'
checkmake Makefile

echo '[lint] Completed successfully.'

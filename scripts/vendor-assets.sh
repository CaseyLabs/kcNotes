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

command -v npm >/dev/null 2>&1 || {
	printf '%s\n' 'missing required command: npm' >&2
	exit 1
}

command -v node >/dev/null 2>&1 || {
	printf '%s\n' 'missing required command: node' >&2
	exit 1
}

command -v perl >/dev/null 2>&1 || {
	printf '%s\n' 'missing required command: perl' >&2
	exit 1
}

cd src
npm install --package-lock-only --ignore-scripts --no-audit --no-fund
npm ci --ignore-scripts --no-audit --no-fund --silent

htmx_version=$(node -p "require('./node_modules/htmx.org/package.json').version")
htmx_source="node_modules/htmx.org/dist/htmx.min.js"
htmx_target="web/static/js/vendor/htmx-${htmx_version}.min.js"

[ -f "${htmx_source}" ] || {
	printf 'missing %s after npm install\n' "${htmx_source}" >&2
	exit 1
}

rm -f web/static/js/vendor/htmx-*.min.js
{
	printf '%s\n' '/*'
	printf '  Vendored dependency note:\n'
	printf '  - This is HTMX v%s, copied from the npm htmx.org package for self-hosting.\n' "${htmx_version}"
	printf '  - Regenerate it with: make vendor-assets\n'
	printf '  - Do not hand-edit vendored third-party code; update the package selector instead.\n'
	printf '%s\n' '*/'
	cat "${htmx_source}"
} >"${htmx_target}"

cd ..
perl -0pi -e "s#js/vendor/htmx-[0-9]+\\.[0-9]+\\.[0-9]+\\.min\\.js#js/vendor/htmx-${htmx_version}.min.js#g" \
	src/web/templates/layouts/base.tmpl

printf 'vendored HTMX %s into %s\n' "${htmx_version}" "src/${htmx_target}"

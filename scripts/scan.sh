#!/bin/sh
# shellcheck shell=sh disable=SC1090
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
. "${project_cfg_file}"

docker_uid=${DOCKER_UID:-$(id -u)}
docker_gid=${DOCKER_GID:-$(id -g)}
scan_workflow_checks=${SCAN_WORKFLOW_CHECKS:-true}

list_workflow_entries() {
	for workflow in .github/workflows/*.yml; do
		awk -v workflow="${workflow}" '
			/^[[:space:]]*-[[:space:]]+uses:[[:space:]]+/ || /^[[:space:]]+uses:[[:space:]]+/ {
				ref = $0
				sub(/^[[:space:]]*-[[:space:]]+uses:[[:space:]]+/, "", ref)
				sub(/^[[:space:]]*uses:[[:space:]]+/, "", ref)
				comment = ""
				if (match(ref, /[[:space:]]+#.*$/)) {
					comment = substr(ref, RSTART + 1)
					sub(/^[[:space:]]+/, "", comment)
					sub(/^#[[:space:]]*/, "", comment)
					sub(/[[:space:]]+$/, "", comment)
					sub(/[[:space:]]+#.*$/, "", ref)
				}
				sub(/[[:space:]]+$/, "", ref)
				if (ref ~ /^(\.\/|\.\.\/)/) {
					next
				}
				printf "%s\t%s\t%s\n", workflow, ref, comment
			}
		' "${workflow}"
	done
}

check_workflow_action_pins() {
	list_workflow_entries | while IFS="$(printf '\t')" read -r workflow ref comment; do
		case "${ref}" in
		*/*@[0-9a-f][0-9a-f][0-9a-f][0-9a-f]*)
			sha=${ref##*@}
			printf '%s\n' "${sha}" | grep -Eq '^[0-9a-f]{40}$' || {
				printf '%s must pin actions by full SHA: %s\n' "${workflow}" "${ref}" >&2
				exit 1
			}
			;;
		*)
			printf '%s uses an invalid action ref: %s\n' "${workflow}" "${ref}" >&2
			exit 1
			;;
		esac
		case "${comment}" in
		v*) ;;
		*)
			printf '%s must keep a reviewed release tag comment for %s\n' "${workflow}" "${ref}" >&2
			exit 1
			;;
		esac
	done
}

check_workflow_trigger_policy() {
	awk '
		{
			line = $0
			sub(/[[:space:]]+#.*$/, "", line)
			if (line ~ /(^|[^A-Za-z0-9_-])pull_request_target([^A-Za-z0-9_-]|$)/) {
				printf "%s:%d: pull_request_target is not allowed in workflows\n", FILENAME, FNR
				found = 1
			}
		}
		END { exit found ? 1 : 0 }
	' .github/workflows/*.yml
}

printf '\n==> Run script syntax checks\n'
find scripts -type f -name '*.sh' -print | LC_ALL=C sort | while IFS= read -r path; do
	sh -n "${path}"
done

secret_scan_image=${DEV_SCAN_GITLEAKS_IMAGE_LOCK}
case "${secret_scan_image}" in
*@sha256:*) ;;
*)
	printf 'DEV_SCAN_GITLEAKS_IMAGE_LOCK must be pinned by digest\n' >&2
	exit 1
	;;
esac

printf '\n==> Run secret scan\n'
if [ -d .git ]; then
	docker run --rm --user "${docker_uid}:${docker_gid}" \
		--cap-drop=ALL \
		--security-opt=no-new-privileges:true \
		-v "$(pwd):/repo" \
		-w /repo \
		"${secret_scan_image}" \
		detect --source /repo --no-banner --redact --exit-code 1
else
	docker run --rm --user "${docker_uid}:${docker_gid}" \
		--cap-drop=ALL \
		--security-opt=no-new-privileges:true \
		-v "$(pwd):/repo" \
		"${secret_scan_image}" \
		detect --source /repo --no-git --no-banner --redact --exit-code 1
fi

if [ "${scan_workflow_checks}" = 'true' ]; then
	printf '\n==> Run workflow lint\n'
	docker run --rm --user "${docker_uid}:${docker_gid}" \
		--cap-drop=ALL \
		--security-opt=no-new-privileges:true \
		-v "$(pwd):/workspace" \
		-w /workspace \
		"${DEV_SCAN_ACTIONLINT_IMAGE_LOCK}" \
		-shellcheck= \
		-pyflakes= \
		.github/workflows/*.yml

	check_workflow_action_pins
	check_workflow_trigger_policy
else
	printf '\n==> Skip workflow lint\n'
	printf '%s\n' 'SCAN_WORKFLOW_CHECKS=false; secret scan still ran.'
fi

printf '\n==> Scan summary\n'
printf '%s\n' 'Result: scan passed'

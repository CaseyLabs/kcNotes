# Makefile
PROJECT_CFG_FILE ?= config/project.cfg
PROJECT_NAME ?= kcnotes-dev
PROJECT_IMAGE ?= $(PROJECT_NAME):local

# Dynamically generate Makefile commands
PHONY_TARGETS := $(shell awk '/^[[:alnum:]_-]+:([^=]|$$).*##(@internal)? / { sub(/:.*/, "", $$1); print $$1 }' $(lastword $(MAKEFILE_LIST)))
.PHONY: $(PHONY_TARGETS)
.PHONY: all clean test

help: ##@show available options
	@printf '\nAvailable targets:\n\n'
	@awk 'BEGIN { FS = ":.*## " } /^[[:alnum:]_-]+:([^=]|$$).*## / { printf "  %-24s %s\n", $$1, $$2 }' $(lastword $(MAKEFILE_LIST))

all: build

build: ## builds the kcNotes development image and app
	sh scripts/build.sh "$(PROJECT_CFG_FILE)"

test: ## runs kcNotes tests
	sh scripts/test.sh "$(PROJECT_CFG_FILE)"

css: ## builds the kcNotes Tailwind CSS bundle
	sh scripts/css.sh "$(PROJECT_CFG_FILE)"

lint: ## runs kcNotes linters and format checks
	sh scripts/lint.sh "$(PROJECT_CFG_FILE)"

migrate: ## runs kcNotes database migrations
	sh scripts/migrate.sh "$(PROJECT_CFG_FILE)"

create-user: ## creates a disabled kcNotes user placeholder; set EMAIL and ROLE
	sh scripts/create-user.sh "$(PROJECT_CFG_FILE)"

publish: ## publishes the static kcNotes site
	sh scripts/publish.sh "$(PROJECT_CFG_FILE)"

preview: ## previews the published static kcNotes site
	sh scripts/preview.sh "$(PROJECT_CFG_FILE)"

smoke: ## runs kcNotes HTTP smoke checks
	sh scripts/smoke-run.sh "$(PROJECT_CFG_FILE)"

run: ## runs the kcNotes app container
	sh scripts/run.sh "$(PROJECT_CFG_FILE)"

stop: ## stops the running kcNotes app container
	sh scripts/stop.sh "$(PROJECT_CFG_FILE)"

status: ## shows the built image and running containers
	sh scripts/status.sh "$(PROJECT_CFG_FILE)"

logs: ## prints logs from running containers
	sh scripts/logs.sh "$(PROJECT_CFG_FILE)"

clean: ## removes artifacts, caches, and the local container image
	sh scripts/clean.sh "$(PROJECT_CFG_FILE)"

shell: ## opens a shell in the kcNotes image
	sh scripts/shell.sh "$(PROJECT_CFG_FILE)"

update: ## refreshes pinned SHA hashes
	sh scripts/update.sh "$(PROJECT_CFG_FILE)"

vendor-assets: ## refreshes checked-in third-party static assets
	sh scripts/vendor-assets.sh "$(PROJECT_CFG_FILE)"

renovate: ## runs self-hosted Renovate for this repository
	sh scripts/renovate.sh "$(PROJECT_CFG_FILE)"

scan: ## runs kcNotes security scans and workflow checks
	sh scripts/scan.sh "$(PROJECT_CFG_FILE)"

dist: ## builds kcNotes release artifacts and integrity outputs
	sh scripts/dist.sh "$(PROJECT_CFG_FILE)"

# Project Build Commands

Use `make help` as the source of truth for available root commands. The root
targets are intentionally small; scripts under `scripts/` hold the implementation
details so local use and GitHub Actions share the same behavior.

## App Commands

- `make build`: builds the kcNotes development image and app.
- `make test`: runs Go tests.
- `make css`: builds the Tailwind CSS bundle.
- `make lint`: runs format and lint checks.
- `make migrate`: applies database migrations.
- `make create-user`: creates a user from `EMAIL`, `PASSWORD`, and `ROLE`.
- `make run`: starts the CMS container, building first when needed.
- `make stop`: stops managed kcNotes containers.
- `make status`: shows local image and container state.
- `make logs`: prints logs from managed kcNotes containers.
- `make publish`: writes static site output to `dist/site` unless overridden.
- `make preview`: serves published static site output.
- `make smoke`: runs HTTP smoke probes.
- `make clean`: removes generated artifacts, caches, local DB files, and the
  local image.
- `make shell`: opens a shell in the project image.

## Maintenance Commands

- `make scan`: runs secret scanning, workflow linting, and workflow policy
  checks.
- `make update`: resolves reviewed image selectors into digest locks and syncs
  generated references.
- `make vendor-assets`: refreshes checked-in third-party static assets.
- `make renovate`: runs self-hosted Renovate when enabled.
- `make dist`: builds kcNotes release artifacts and integrity outputs under `dist/`.

## Common Inputs

- `PROJECT_CFG_FILE`: selects the project config file. Defaults to
  `config/project.cfg`.
- `ENABLE_SBOM`, `ENABLE_GRYPE`, `GRYPE_FAIL_ON`: control release integrity
  outputs used by `make dist` and the release workflow.
- `DOCKER_BUILD_EXTRA_ARGS`: lets CI provide Buildx cache options without
  changing local defaults.
- `DOCKER_UID`, `DOCKER_GID`, `DOCKER_HOME`, `DOCKER_HOME_SOURCE`, and
  `DOCKER_TMPDIR`: control container user and cache paths for bind-mounted
  workflows.
- `HOST_PORT`: pins the host port for `make run` and `make preview`.
- `PUBLISH_OUT_DIR`: overrides static publish output.

Project-specific reviewed values belong in `config/project.cfg`. Local-only
values belong in the environment.

## Why The Workflow Is Container-First

The default workflow avoids requiring a host-installed language toolchain. Builds,
tests, scans, and release steps run through Docker images selected by
`config/project.cfg` and locked by `config/lockfile.cfg`.

This keeps local development, CI, and release behavior close together. It also
makes dependency changes reviewable: normal version selectors live in
`config/project.cfg`, while immutable lock values are refreshed by `make update`.
Checked-in third-party browser assets are refreshed by `make vendor-assets` from
npm-managed selectors.

## CI Alignment

GitHub Actions should stay thin wrappers around `make` targets. If a workflow
needs new behavior, add it to the relevant script or target first, then call that
same entrypoint from CI.

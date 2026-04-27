# Dependency Update Guide

This repository uses both Dependabot and self-hosted Renovate. They cover different
parts of the repository and use cooldowns so maintainers have time to review new
upstream releases before routine update pull requests appear.

## Dependabot

Dependabot is configured in `.github/dependabot.yml`.

It updates:

- GitHub Actions used by workflows
- Dockerfile dependency references when present
- Go module dependencies in `src/go.mod`
- npm dependencies in `src/package-lock.json`

Routine update PRs wait for the configured cooldown period. This timing defense
helps avoid immediately adopting a newly published action or image before yanks,
malicious releases, or incident reports have time to surface.

## Renovate

Self-hosted Renovate is configured in `.github/renovate.json` and runs through
`make renovate`.

It updates reviewed image and tool selectors in `config/project.cfg`, plus the
npm selector for the checked-in HTMX browser asset. After updating those
selectors, Renovate is allowed to run:

```sh
sh scripts/update.sh config/project.cfg
sh scripts/vendor-assets.sh config/project.cfg
```

Those commands refresh immutable locks in `config/lockfile.cfg`, sync related
generated references, refresh `src/package-lock.json`, and copy the pinned HTMX
asset into `src/web/static/js/vendor/`. Keeping selectors and generated outputs
together makes dependency changes easier to review.

## GitHub App Token

The Renovate workflow mints a GitHub App token instead of using a broad personal
access token. The app token can be scoped to the repository and permissions that
Renovate needs to open pull requests and report status.

To disable self-hosted Renovate, set this in `config/project.cfg`:

```sh
DEV_SCAN_ENABLE_RENOVATE=false
```

## Review Expectations

For dependency update PRs:

- confirm the version selector changed for the intended dependency
- confirm `config/lockfile.cfg` changed when a locked image was updated
- confirm vendored browser assets changed when their npm selector changed
- keep full-SHA action pins and reviewed tag comments intact
- run the relevant `make` target locally or rely on the matching required check
- pay extra attention to updates for build, release, scan, and credentialed
  workflow tools

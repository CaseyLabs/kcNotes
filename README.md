# kcNotes

kcNotes is content management system (CMS), written in Go + HTMX, with container-driven workflows.

## Demo

![kcNotes Demo](docs/demo.png)

## Features

- Server-rendered Go templates and HTMX admin flows.
- Self-hosted static assets and Tailwind CSS.
- SQLite/libSQL storage with local, remote Turso, and optional replica modes.
- Session auth, CSRF protection, MFA, RBAC, rate limiting, and account lockout.
- Posts/pages, media uploads, settings, audit log, search, and static publishing.
- Root-owned Docker, Make, GitHub Actions, security scan, dependency update, and release integrity workflows.

## Quick Start

Run the following commands in a Terminal:

```bash
make css
make migrate
EMAIL=admin@example.com PASSWORD='change-me-now' ROLE=admin make create-user
make run
make publish
make preview
make smoke
make stop
make clean
```

- `make run` and `make preview` expose the app on `http://localhost:5555` by default.
- If port `5555` is in use, the scripts try the next free port unless you set `HOST_PORT`
  - For example `HOST_PORT=8081 make run`.

## Build Commands

Use `make help` as the source of truth.

Core app commands:

- `make build`: builds the development image and app.
- `make test`: runs Go tests.
- `make css`: builds Tailwind CSS.
- `make lint`: runs format and lint checks.
- `make migrate`: applies database migrations.
- `make create-user`: creates a user from `EMAIL`, `PASSWORD`, and `ROLE`.
- `make run`: applies pending migrations, then starts the CMS.
- `make publish`: writes the static site to `dist/site` unless `PUBLISH_OUT_DIR` is set.
- `make preview`: serves the published static site.
- `make smoke`: runs HTTP smoke probes.
- `make stop`: stops managed kcNotes containers.
- `make logs`: prints managed container logs.
- `make status`: shows the app image and managed containers.
- `make clean`: removes generated artifacts, caches, local DB files, and managed containers.

Maintenance commands:

- `make scan`: runs secret scanning and workflow policy checks.
- `make dist`: builds kcNotes release artifacts and integrity outputs under `dist/`.
- `make update`: refreshes pinned image digests and workflow allowlist docs.
- `make renovate`: runs self-hosted Renovate.

## Public Routes

- Health: `http://localhost:5555/healthz`
- Home: `http://localhost:5555/`
- Post: `http://localhost:5555/p/{slug}/`
- Page: `http://localhost:5555/page/{slug}/`

## Admin Routes

- Login: `http://localhost:5555/admin/login`
- Dashboard: `http://localhost:5555/admin`
- Posts: `http://localhost:5555/admin/posts`
- Media: `http://localhost:5555/admin/media`
- Users: `http://localhost:5555/admin/users`
- Settings: `http://localhost:5555/admin/settings`
- Audit log: `http://localhost:5555/admin/audit`

## CLI Modes

The root Make targets run these modes in the project container:

- `go run ./cmd/cms -mode serve`
- `go run ./cmd/cms -mode migrate`
- `go run ./cmd/cms -mode create-user -email admin@example.com -password 'change-me-now' -role admin`
- `go run ./cmd/cms -mode publish`
- `go run ./cmd/cms -mode preview`

## Configuration

Common environment variables:

| Variable                      | Default                           | Description                                                                   |
| ----------------------------- | --------------------------------- | ----------------------------------------------------------------------------- |
| `DB_MODE`                     | `local`                           | Database mode. Valid values: `local`, `remote`, `replica`.                    |
| `DB_PATH`                     | `./data/cms.db`                   | Filesystem path for the database. Used by `local` and `replica`.              |
| `DATABASE_URL`                | —                                 | Required for `remote` and `replica` modes.                                    |
| `DATABASE_AUTH_TOKEN`         | —                                 | Required for Turso/libSQL authentication when not embedded in `DATABASE_URL`. |
| `REPLICA_SYNC_INTERVAL`       | `2s`                              | Interval for syncing replica with remote.                                     |
| `REPLICA_READ_YOUR_WRITES`    | `true`                            | Ensures reads reflect recent writes in replica mode.                          |
| `SESSION_COOKIE_NAME`         | `cms_session`                     | Name of the session cookie.                                                   |
| `CSRF_COOKIE_NAME`            | `cms_csrf`                        | Name of the CSRF cookie.                                                      |
| `ADMIN_COOKIE_PATH`           | `/admin`                          | Path scope for admin cookies.                                                 |
| `SESSION_TTL`                 | `24h`                             | Session time-to-live.                                                         |
| `COOKIE_SECURE`               | `false` (dev), `true` (otherwise) | Whether cookies are marked as secure.                                         |
| `UPLOAD_DIR`                  | `./data/uploads`                  | Directory for uploaded files (inside `src/` at runtime).                      |
| `MEDIA_USER_QUOTA_BYTES`      | `209715200`                       | Per-user media storage quota in bytes.                                        |
| `MEDIA_TOTAL_QUOTA_BYTES`     | `2147483648`                      | Total media storage quota in bytes.                                           |
| `LOGIN_LOCKOUT_THRESHOLD`     | `8`                               | Failed login attempts before lockout.                                         |
| `LOGIN_LOCKOUT_WINDOW`        | `15m`                             | Time window for counting failed login attempts.                               |
| `LOGIN_LOCKOUT_DURATION`      | `15m`                             | Duration of account lockout after threshold is reached.                       |
| `SITE_BASE_URL`               | —                                 | Base URL used for canonical links, RSS, and sitemap generation.               |
| `PUBLISH_OUT_DIR`             | `../dist/site`                    | Output directory for published site (from inside `src/`).                     |
| `PUBLISH_INCLUDE_DRAFTS`      | `false`                           | Whether to include draft content in published output.                         |
| `PREVIEW_HTTP_ADDR`           | `:8080`                           | Address used by preview server (in container).                                |
| `TRUSTED_PROXY_CIDRS`         | —                                 | List of trusted proxy CIDR ranges.                                            |
| `ENFORCE_TRUSTED_PROXY_CIDRS` | —                                 | Whether to enforce trusted proxy CIDR checks.                                 |

## Remote DB Examples

Remote Turso/libSQL mode:

```bash
export DB_MODE=remote
export DATABASE_URL='libsql://<db-name>-<org>.turso.io'
export DATABASE_AUTH_TOKEN='<token>'
make migrate
make run
```

Embedded replica mode:

```bash
export DB_MODE=replica
export DB_PATH=./data/replica.db
export DATABASE_URL='libsql://<db-name>-<org>.turso.io'
export DATABASE_AUTH_TOKEN='<token>'
export REPLICA_SYNC_INTERVAL=2s
export REPLICA_READ_YOUR_WRITES=true
make migrate
make run
```

## Smoke Checks

After deploy, run:

```bash
BASE_URL='https://admin.example.com' make smoke
```

## Current Actions

- `actions/attest-build-provenance@a2bbfa25375fe432b6a289bc6b6cd05ecd0c4c32`
- `actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd`
- `actions/create-github-app-token@1b10c78c7865c340bc4f6099eb2f838309f1e8c3`
- `actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a`
- `docker/setup-buildx-action@4d04d5d9486b7bd6fa91e7baf45bbb4f8b9deedd`

### Allowlist Guidance

GitHub Actions must stay pinned by full commit SHA with a reviewed version
comment in workflow files. Run `make update` after changing workflow action
references so this list stays current.

## License

This project is free for personal use. Commercial usage requires a license. See [`LICENSE.md`](LICENSE.md) for more details.

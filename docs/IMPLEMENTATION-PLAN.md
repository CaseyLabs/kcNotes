# Implementation Plan

Date: 2026-05-01

## Purpose

This document is intended to be the current project tracker:

- What the CMS is trying to become.
- What is already implemented in the current repository.
- What still needs to be implemented before the required scope is complete.
- Which items are optional backlog rather than required completion work.

## Project Goals

Build a secure, server-rendered Go CMS with a WordPress-admin-style experience:

- Go backend using server-rendered HTML and a long-running HTTP process.
- HTMX admin UI for partial updates without becoming a SPA.
- Tailwind-based styling with a minimal, responsive admin interface.
- SQLite-compatible data model:
  - local SQLite/libSQL file for development and CI;
  - remote Turso/libSQL for staging/production;
  - optional embedded replica mode with explicit read-your-writes behavior.
- Static publishing mode that generates a complete static site from CMS content.
- Containerized development and CI workflows through Docker and Make.
- Security-first defaults for auth, sessions, CSRF, content rendering, uploads,
  deployment, and public admin exposure.
- Teaching-oriented code comments in new or updated source files.

## Implemented Already

The current working tree substantially implements the original milestones and
the UI refresh.

### Application Structure

- Go application entrypoint at `cmd/cms/main.go`.
- Internal package layout for app wiring, HTTP handlers/middleware/routes/views,
  domain types, SQLite store, content rendering, and static publishing.
- Server-side Go templates under `web/templates`.
- Self-hosted static assets under `web/static`.

### Containerized Workflow

- `Makefile` targets for common workflows:
  - `make image`
  - `make deps`
  - `make css`
  - `make build`
  - `make test`
  - `make lint`
  - `make migrate`
  - `make publish`
  - `make preview`
  - `make run`
  - `make stop`
  - `make smoke`
  - `make clean`
- Docker-based helper scripts under `scripts/`.
- Containers run with host UID/GID and project-mounted workspace.
- Run/preview scripts auto-select a nearby free host port when `HOST_PORT` is
  not pinned.

### Routing and HTMX Contract

- Public routes:
  - `GET /healthz`
  - `GET /`
  - `GET /p/{slug}/`
  - `GET /page/{slug}/`
- Admin routes:
  - login/logout;
  - dashboard;
  - posts/pages list, create, edit, quick edit, publish, unpublish, delete;
  - media upload/browse/file preview;
  - users;
  - settings;
  - audit log;
  - passkey setup, sign-in, enrollment, and passkey management.
- HTMX-aware table/form partials.
- Standard partial targets exist for `#flash`, `#modal`, `#posts-table`, and
  row-level post swaps.
- Successful HTMX writes that navigate use `HX-Redirect`.
- Validation and middleware errors are rendered as fragments where applicable.

### Authentication and Authorization

- Passkey-only session-based login/logout using WebAuthn with required user
  verification.
- Secure cookie attributes including `HttpOnly`, `SameSite=Lax`, configurable
  `Secure`, and configurable admin cookie path.
- First-admin browser setup at `/admin/setup`, available only while the users
  table is empty.
- Discoverable passkey login at `/admin/login` with required user verification.
- User onboarding through single-use, expiring enrollment invitations instead
  of passwords.
- Authenticated passkey management at `/admin/passkeys`, with a guard that
  keeps at least one passkey per user.
- WebAuthn credential, challenge, and enrollment-invitation persistence.
- Password login, password reset, TOTP, recovery-code login, and `/admin/mfa`
  are intentionally not routed or documented as active workflows.
- RBAC roles:
  - `admin`;
  - `editor`;
  - `author`.
- Author ownership enforcement on post write paths.
- Disabled users are rejected during session loading.
- Admin-only user management.
- Self-lockout guards for admin user updates.

### Login Hardening

- Per-IP and per-account login rate limiting.
- Per-account lockout/backoff after repeated failed authentication attempts.
- Sensitive endpoint rate limiting.
- Login success/failure audit events.

### CSRF and Security Headers

- CSRF middleware for state-changing admin requests.
- Hidden `_csrf` fields on forms.
- HTMX global `X-CSRF-Token` injection from `web/static/js/app.js`.
- Security headers:
  - `Content-Security-Policy` with `script-src 'self'`;
  - `X-Frame-Options: DENY`;
  - `X-Content-Type-Options: nosniff`;
  - `Referrer-Policy: strict-origin-when-cross-origin`;
  - `Permissions-Policy`.
- Self-hosted HTMX asset.

### Posts and Pages

- Post/page types.
- Existing post/page edit-form autosave snapshots with restore and dismiss.
- Status workflow:
  - `draft`;
  - `published`;
  - `archived`;
  - soft-deleted via `deleted_at`.
- Admin listing with search/filter/sort/pagination.
- HTMX table refreshes.
- Create/edit forms.
- Quick-edit row swaps.
- Publish/unpublish actions.
- Soft delete.
- Slug validation and uniqueness enforcement.
- Validation error fragments.

### Content Safety

- Markdown stored in the database and rendered server-side.
- GFM support through Goldmark.
- Sanitized HTML output through Bluemonday.
- Public post/page templates use sanitized `template.HTML`.
- Untrusted template values remain escaped by Go `html/template`.

### Media

- Admin media upload and browse UI.
- Upload size limit.
- MIME allowlist for JPEG, PNG, and GIF.
- Magic-byte/content detection.
- Image decode verification.
- Randomized stored filenames.
- Original filename stored only as metadata.
- Per-user and global media quota enforcement.
- Auth-protected media preview endpoint.
- Static publishing copies allowlisted media into `dist/media`.

### Settings

- Key/value settings table.
- Admin settings UI for site-level values such as site name, base URL, and
  feature flags.
- Settings changes are audited.

### Search and Audit

- SQLite FTS5 `posts_fts` virtual table.
- Trigger-based FTS synchronization for post insert/update/delete.
- Admin post search uses FTS-backed queries.
- Audit log table and admin audit UI.
- Audit events for login/logout and post/media/settings mutations.

### Database and Store Hardening

- Local, remote, and replica DB modes.
- Local SQLite/libSQL uses one open connection and WAL/foreign-key/busy-timeout
  PRAGMAs.
- Remote Turso/libSQL connectivity using `DATABASE_URL` and
  `DATABASE_AUTH_TOKEN`.
- Embedded replica support with `REPLICA_SYNC_INTERVAL` and
  `REPLICA_READ_YOUR_WRITES`.
- Store retry helpers:
  - `ExecRetry`;
  - `QueryRetry`;
  - `TxRetry`.
- Exponential backoff and jitter for transient failures.
- Retry metrics counters for store-level retry activity.
- Idempotent conflict-as-success handling for selected replayed operation IDs.
- Transactional migration application.

### Static Publishing and Preview

- `cms publish` / `make publish` renders static output.
- Output paths:
  - `/` to `dist/index.html`;
  - `/p/{slug}/` to `dist/p/{slug}/index.html`;
  - `/page/{slug}/` to `dist/page/{slug}/index.html`;
  - static assets to `dist/assets/*`;
  - uploaded media to `dist/media/*`.
- RSS generation.
- `sitemap.xml` generation.
- `.publish-manifest.json` generation.
- Removed-file accounting from prior manifests.
- Temp-dir render followed by best-effort atomic output swap.
- `cms preview` / `make preview` serves generated static output locally.

### Deployment and Smoke Checks

- `GET /healthz` health probe.
- `make smoke` / `scripts/smoke.sh` support deployed smoke checks.
- Optional GitHub Pages publishing workflow exists.
- Optional admin deployment workflow exists.
- Deploy workflow uses pinned SSH known-hosts support.

### UI Refresh

- Semantic CSS tokens for background, surface, text, border, accent/focus, and
  status colors.
- Light/dark theme support with CSS custom properties.
- Default `system` theme behavior using `prefers-color-scheme`.
- Manual theme cycle:
  - `system`;
  - `light`;
  - `dark`.
- Theme preference stored in `localStorage` as `cms_theme_preference`.
- `data-theme` override on `<html>` for manual light/dark modes.
- Shared header theme toggle.
- Shared admin navigation partial.
- Responsive admin/public templates using shared component classes.
- README documents theme behavior.

## Required Remaining Work

No open required feature gaps remain after the R1 autosave implementation and
the passkey-only authentication cleanup were validated. The subsections below
record the required completion scope so future work does not confuse optional
backlog items with blockers.

### R1: Autosave Drafts

Autosave is implemented for existing post/page edit forms.

- `autosave_snapshots` stores one private latest snapshot per `(post_id,
  author_id)`.
- Store methods create/update, retrieve, and dismiss snapshots through the retry
  wrappers.
- `POST /admin/posts/{id}/autosave` validates the edit form, checks
  `base_updated_at`, and returns a save-status fragment without changing the
  canonical post.
- `POST /admin/posts/{id}/autosave/restore` repopulates the edit form from the
  snapshot.
- `POST /admin/posts/{id}/autosave/dismiss` removes the user's snapshot.
- The edit form autosaves only existing posts/pages. `/admin/posts/new` still
  creates content only through the normal manual submit path.
- Static publishing continues to read canonical posts/pages only.

### R2: Passkey-Only Authentication Cleanup

Passkey-only admin authentication is the active baseline.

- Active admin auth/onboarding routes are `/admin/setup`, `/admin/login`,
  `/admin/enroll`, `/admin/passkeys`, and admin-issued user invitations.
- Legacy `POST /admin/login` requests are rejected with a passkey-only message.
- `/admin/mfa` is unavailable.
- Password, TOTP, and recovery-code handler/template/test code has been removed
  from active workflows.
- First-admin setup is available only on an empty users table.
- Enrollment invitations are single-use and expire.
- Disabled users are rejected by session loading and passkey credential lookup
  preserves the disabled flag for login rejection.
- Passkey challenges are single-use and expire.
- A user cannot delete their last remaining passkey.

## Optional Backlog

These items are useful, but they are not required for the current documented
completion scope.

### O1: Passkey Recovery and Admin UX

- Operator recovery guidance for cases where every admin loses every passkey.
- Cleaner enrollment-link presentation and copy action in the users UI.
- Optional passkey attestation policy if the deployment needs hardware-key-only
  enrollment.

### O2: Media Processing Pipeline

- EXIF stripping for uploaded images.
- Thumbnail generation.
- Responsive image variants.
- Optional background job runner for media processing.
- Variant metadata in the media model.

### O3: Jobs Table and Runner

- `jobs` table with unique `job_key`.
- Internal runner with retry/attempt tracking.
- Use for stale autosave cleanup, media processing, publish workflows, or other
  deferred work.

### O4: Observability Expansion

- Request duration metrics.
- Login failure and lockout counters.
- Rate-limit hit counters.
- DB retry dashboards/log aggregation guidance.
- Job queue depth metrics if the job runner is added.

### O5: Media De-Duplication

- Decide whether `media.sha256` should become unique.
- If enabled, treat same-hash uploads as reuse or conflict based on UX needs.

### O6: Admin Exposure Extras

- Optional admin IP allowlist.
- Explicit `/robots.txt` rules blocking admin paths if static/public serving is
  expanded.

## Verification Plan

For documentation-only changes:

- No runtime checks are required.

For autosave or other code changes:

1. `make css`
2. `make lint`
3. `make build`
4. `make test`
5. `make smoke`

For UI-affecting changes, also manually check:

- `/`
- `/admin/login`
- `/admin`
- `/admin/posts`
- `/admin/posts/new`
- `/admin/media`
- `/admin/users`
- `/admin/settings`
- `/admin/audit`
- `/admin/passkeys`
- `/admin/setup` on an empty database
- `/admin/enroll` with an admin-created enrollment token
- add, rename, and delete passkeys
- last-passkey delete is blocked
- `/admin/mfa` is unavailable
- legacy password-form `POST /admin/login` cannot authenticate

For publishing changes, also check:

- `make publish`
- `make preview`
- generated `dist/index.html`;
- generated post/page routes;
- generated `rss.xml`;
- generated `sitemap.xml`;
- generated `.publish-manifest.json`;
- orphan cleanup after deleting or renaming content.

## Completion Criteria

Required scope is complete when:

- Autosave drafts work for post/page editing.
- Autosave respects RBAC, author ownership, CSRF, and validation rules.
- Autosave does not publish content or overwrite canonical state unexpectedly.
- Newer autosave snapshots can be restored or dismissed.
- Autosave has store and handler regression tests.
- Admin authentication remains passkey-only.
- First-admin setup, passkey login, user enrollment, passkey management,
  challenge expiry/single-use behavior, disabled-user rejection, and the
  last-passkey delete guard have focused regression coverage.
- Password login, password reset, TOTP, recovery-code login, and `/admin/mfa`
  remain absent from the routed public workflow.
- Existing required checks pass.
- README, `AGENTS.md`, and this implementation plan accurately reflect the
  completed state.

Optional scope is complete only when separately selected and implemented.

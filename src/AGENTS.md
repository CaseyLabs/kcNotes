# AGENTS.md

Instructions for coding agents working under `src/`.

## Scope

This subtree contains only the kcNotes Go + HTMX CMS source and app build
configuration. The repository root owns Make targets, Docker workflow scripts,
CI, scans, release artifacts, and generated output.

Do not add a `Makefile`, workflow scripts, generated `dist/`, local `data/`,
`.cache`, `.config`, or `node_modules` under this subtree.

## Read First

- Read `../AGENTS.md` for repository-wide rules and skill routing.
- Read `../docs/IMPLEMENTATION-PLAN.md` for current implementation status,
  remaining required work, and optional backlog.
- As of `../docs/IMPLEMENTATION-PLAN.md` dated 2026-04-24, autosave drafts are
  the only required feature gap. Do not describe autosave as complete until
  code, tests, and docs are updated.

## App Architecture

- `cmd/cms/`: application entrypoint and CLI modes.
- `internal/app/`: app wiring and configuration.
- `internal/http/`: routes, middleware, handlers, and view rendering.
- `internal/domain/`: core domain types.
- `internal/store/`: SQLite/libSQL persistence, migrations, retry logic, and
  store tests.
- `internal/content/`: Markdown rendering and sanitization.
- `internal/publish/`: static-site publishing pipeline.
- `web/templates/`: server-rendered Go templates and HTMX partials.
- `web/static/`: self-hosted CSS, JavaScript, and vendor assets.

## Source Workflow

Run commands from the repository root:

```shell
make css
make lint
make build
make test
make migrate
make create-user
make publish
make preview
make run
make smoke
make clean
```

The normal app validation stack for code changes is:

```shell
make css
make lint
make build
make test
```

Also run `make publish` or `make preview` when static publishing changes, and
`make smoke` when route, deployment, or runtime behavior changes.

## Security Rules

- Preserve session auth, secure cookie settings, CSRF middleware, HTMX CSRF
  header injection, MFA, RBAC, author ownership checks, rate limits, and
  self-lockout guards.
- Keep state-changing admin routes protected by auth, CSRF, authorization, and
  validation.
- Do not allow autosave, drafts, publish/unpublish, or static publishing to
  overwrite canonical content state unexpectedly.
- Keep Markdown rendering server-side and sanitized before any use of
  `template.HTML`.
- Do not use trusted template types for untrusted content unless sanitization is
  explicit and covered by tests.
- Keep uploaded media validation strict: size limits, MIME allowlist,
  magic-byte/content checks, image decode verification, randomized stored
  names, and quota enforcement.
- Keep local SQLite single-writer settings, WAL, foreign keys, busy timeout,
  remote/replica behavior, and admin read-your-writes rules intact.
- Use retry wrappers only for transient failures and keep retried writes
  idempotent through stable IDs or conflict-as-success semantics.
- Do not perform external IO inside database transactions.
- Treat proxy-derived client IP, scheme, and host values as untrusted unless
  trusted proxy CIDRs are explicitly enforced.
- Keep secrets, tokens, private URLs, local databases, uploads, and generated
  runtime artifacts out of source, docs, examples, logs, and commits.

## Testing

- Use TDD by default for app features, bug fixes, and behavior changes.
- Add or update focused tests for changed code behavior.
- Do not write tests for prose-only documentation, filenames, or directory layout.
- For security-sensitive changes, cover successful behavior plus unauthorized,
  forbidden, validation-failure, and stale/conflict paths where applicable.
- For HTMX handlers, verify status codes, fragments, redirects, and target
  contract behavior.
- For publish changes, verify generated files, feeds, sitemap, manifest
  updates, and orphan cleanup.

## Templates And UI

- Keep the app server-rendered; do not turn admin flows into a SPA.
- Preserve the HTMX partial contract for `#flash`, `#modal`, `#posts-table`,
  and `#row-{id}` where those targets apply.
- Keep successful HTMX navigations using `HX-Redirect` when full-page
  navigation is intended.
- Return validation and middleware errors as fragments where existing handlers
  do that today.
- Keep HTMX and other runtime assets self-hosted unless the task explicitly
  changes the asset policy.

## Documentation

- Update the root `README.md` when app setup, commands, routes, configuration,
  security posture, or user-visible behavior changes.
- Update `../docs/IMPLEMENTATION-PLAN.md` when current status, required
  remaining work, optional backlog, or completion criteria change.
- Keep documentation grounded in root commands and routes that exist.

## Comments

Use teaching-oriented comments where they clarify non-obvious Go, HTTP, HTMX,
database, security, template, publishing, or Docker workflow behavior.

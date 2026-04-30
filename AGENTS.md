# AGENTS.md

Instructions for coding agents working in this repository.

## Project Overview

This repository is kcNotes: a Go + HTMX CMS application.

The application source lives under `src/`:

- Go server-rendered HTML with a long-running HTTP process.
- HTMX admin flows with minimal custom JavaScript.
- Tailwind CSS and self-hosted static assets.
- SQLite/libSQL storage, with local, remote Turso, and optional replica modes.
- Static publishing from CMS content into root-owned generated output.
- Docker and Make driven build, test, lint, run, publish, preview, and smoke workflows.

The root owns repository maintenance: Docker baseline, Makefile entrypoints,
GitHub Actions, security scans, release artifacts, dependency updates, and
agent-support files.

## Current Project Tracker

Before changing app behavior, read:

1. `src/AGENTS.md` for source-tree architecture, security rules, and coding expectations.
2. `docs/IMPLEMENTATION-PLAN.md` for current implementation status and
   remaining required work.

As of `docs/IMPLEMENTATION-PLAN.md` dated 2026-04-30, the required R1 autosave
scope is implemented for existing post/page edit forms.

## Public Interface

Use `make help` as the source of truth for root targets. Run project commands
from the repository root.

Core targets:

```shell
make build
make test
make run
make stop
make status
make logs
make clean
make shell
```

Application targets:

```shell
make css
make lint
make migrate
make create-user
make publish
make preview
make smoke
```

Maintenance targets:

```shell
make update
make renovate
make scan
make dist
```

Keep root workflow logic in `scripts/`. Do not add Makefiles, workflow scripts,
generated output, local databases, caches, or installed dependencies under `src/`.

## Skill Routing

Use repo-local skills only when the skill exists and the request matches its
description.

- Use `workflow-validation` for root Makefile, Docker, scripts, CI, scan,
  release, packaging, or docs/implementation alignment checks.
- Use `github-hardening` for GitHub-side hardening guidance, rulesets, scanning,
  review protections, or workflow permissions.
- Use `release-integrity` for SBOMs, checksums, attestations, artifact scanning,
  signing guidance, or release workflow safety.
- Use `pr-draft-summary` when drafting a PR handoff after substantive changes.
- Use `security-review` only when the user explicitly invokes `$security-review`.

If no matching skill exists, follow this file, the nearest `AGENTS.md`, and the
repository itself.

## Before Making Changes

- Read the relevant files before editing.
- Identify whether the change is app code under `src/`, root workflow, GitHub Actions, docs, or agent guidance.
- Prefer targeted changes over broad rewrites.
- Preserve backwards compatibility unless the task explicitly allows a breaking change.
- Do not introduce new dependencies unless necessary and justified.
- Keep secrets, tokens, credentials, private URLs, generated DB files, uploads,
  local runtime artifacts, and release outputs out of code, docs, examples, logs, and commits.

## Source Rules

- Follow `src/AGENTS.md` for app code.
- Use TDD by default for app features, bug fixes, and behavior changes.
- Add or update tests for code behavior; do not write tests for prose-only content, filenames, or directory layout.
- Keep auth, RBAC, CSRF, session, MFA, rate-limit, upload, content-sanitization,
  static-publish, and replica consistency behavior explicit and covered by focused tests when changed.
- Keep new and updated source comments teaching-oriented where the code is not obvious, especially in security, persistence, HTTP, template, and workflow paths.

## Root Workflow Rules

- `scripts/` contains implementation details for root build, test, scan, release, dependency, and app helper workflows.
- `config/project.cfg` is the main reviewed customization point.
- `Dockerfile` provides the root development and CI runtime baseline.
- `.github/workflows/` should call `make` targets instead of duplicating logic.
- Keep release manifests, scripts, and documentation aligned when packaging behavior changes.

## Review

When reviewing changes or using `/review`:

1. Read `.agents/code_review.md`.
2. Apply the closest active `AGENTS.md`, especially `src/AGENTS.md` for app changes.
3. Prioritize correctness, security controls, workflow drift, reproducibility,
   documentation accuracy, and missing tests.

## Verification

Run the smallest relevant verification stack that gives high confidence.

- Documentation-only guidance changes: verify commands, paths, skill names, and references against current files.
- App code changes: normally run `make css`, `make lint`, `make build`, and `make test`.
- Static publishing changes: also run `make publish` or `make preview`.
- Runtime/route changes: also run `make smoke`.
- Root workflow, CI, scan, packaging, or release changes: run the relevant root `make` targets.
- Shell script changes: run relevant targets plus `shellcheck` and `shfmt` when available.

Do not claim success without verification. If a check cannot be run, say why.

## Documentation

- Update documentation when code changes materially affect setup, usage, behavior, configuration, security posture, or developer workflows.
- Keep documentation tightly scoped and grounded in the actual workflow.
- Avoid changing the root `README.md` unless root behavior or a root-facing fact changes.
- Add or update subtree `AGENTS.md` files only where a subtree has durable local rules, hazards, or verification needs.

## Definition Of Done

A change is done when:

1. the requested change is implemented cleanly
2. affected docs and agent guidance are aligned with the current repository
3. relevant checks were run, or any verification gap is stated
4. app and root workflows remain container-first and reproducible
5. security-sensitive behavior remains explicit and reviewable

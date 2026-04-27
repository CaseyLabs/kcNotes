# code_review.md

Review changes for this repository as the kcNotes Go + HTMX CMS.

## Review Objective

Prioritize findings that could cause:

- incorrect behavior or user-visible regressions
- weakened auth, authorization, CSRF, MFA, session, upload, content-safety, or
  deployment defaults
- broken static publishing, preview, migrations, or SQLite/libSQL behavior
- local, CI, Docker, or release workflow drift
- missing tests for changed code behavior
- inaccurate documentation or agent guidance

Prefer high-signal findings over broad commentary.

## Local `/review` Scope

Treat `/review` runs as reviewer-only passes: inspect the selected diff, report
prioritized and actionable findings, and do not edit the working tree during the
review.

Choose the review target deliberately:

- use a base-branch review for PR-style feedback
- use an uncommitted-changes review for staged, unstaged, and untracked work
- use a commit review when a specific SHA is the review unit
- use custom review instructions for a narrow focus such as security, release
  integrity, accessibility, or deployment

## Severity Guidance

High priority findings include changes that:

- introduce a likely bug, data loss path, authorization bypass, or security
  control regression
- break login, session loading, CSRF enforcement, MFA, RBAC, author ownership,
  rate limiting, content sanitization, or upload validation
- make migrations, retries, replica read-your-writes, publish output, or manifest
  cleanup incorrect
- break documented Docker/Make workflows or cause local and CI behavior to
  diverge
- add a likely secret-handling, credential, permission, or supply-chain risk

Medium priority findings include changes that:

- add complexity without clear benefit
- create documentation, route, command, or implementation-plan drift
- weaken auditability, failure visibility, or reproducibility
- add dependencies, root targets, workflow steps, or exposed interfaces without
  clear need
- leave changed behavior without focused regression coverage

Low priority findings include minor clarity, consistency, or style issues that
do not materially affect correctness, security, or maintainability.

## App Review Rules

For changes under `src/`, apply `src/AGENTS.md` first.

Flag when a change:

- bypasses or weakens auth, RBAC, CSRF, MFA, rate limits, sessions, or self-lockout
  guards
- trusts user-controlled Markdown, HTML, filenames, MIME types, slugs, redirect
  targets, proxy headers, or template data without the existing validation model
- uses `template.HTML` or similar trusted types without proving prior sanitization
- performs external IO inside DB transactions
- retries non-idempotent writes without stable operation IDs or conflict handling
- violates author ownership on post/page/media operations
- lets autosave, drafts, publish/unpublish, or static generation overwrite
  canonical state unexpectedly
- breaks HTMX partial contracts, expected targets, or fragment error responses
- changes routes, config variables, or Make targets without updating docs

## Root Workflow Review Rules

For root workflow, CI, scan, or release changes, flag when a change:

- requires host-installed language toolchains for normal build/test/lint/release
  use
- duplicates `Makefile` or script logic inline in `.github/workflows/`
- expands root `Makefile` public targets without recurring need
- removes version pinning, lockfiles, checksums, SBOMs, scans, or integrity
  outputs without justification
- weakens GitHub Actions permissions, secret scanning, release controls, or
  repository hardening defaults
- puts generated artifacts, caches, state files, credentials, DBs, or local
  runtime data into tracked paths

## What To Verify During Review

When relevant, check alignment across:

- `src/AGENTS.md` and `docs/IMPLEMENTATION-PLAN.MD`
- app tests and source build inputs under `src/`
- root `Makefile`, `scripts/`, `Dockerfile`, and `config/project.cfg`
- `.github/workflows/`
- README and focused docs
- `.agents/skills/` routing and metadata

## Preferred Review Style

- Lead with findings, ordered by severity.
- Be concise, specific, and evidence-based.
- Explain the concrete risk or regression.
- Point to the exact file, behavior, route, command, or workflow involved.
- Prefer fewer high-confidence comments over many speculative ones.
- Do not ask for broad rewrites when a targeted fix is sufficient.

## Suggested Finding Structure

- Finding: what is wrong
- Why it matters: concrete risk, regression, or inconsistency
- Scope: where it appears
- Suggested direction: the smallest reasonable fix

## Approval Standard

A change is generally acceptable when:

- it satisfies the request
- changed code behavior has focused tests
- security-sensitive behavior remains explicit
- local, CI, Docker, and release workflows remain aligned
- documentation and implementation-plan state remain credible
- verification actually ran or gaps are clearly stated

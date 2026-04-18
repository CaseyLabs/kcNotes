# Release And Packaging Guide

`make dist` builds kcNotes release artifacts and integrity outputs under `dist/`.
The release workflow calls the same target after repeating tests and security
scans on the tagged commit.

## What Ships

The default release binary is:

- `dist/kcnotes-linux-amd64`

The root `dist/` directory is generated output and is ignored by Git.

## Integrity Outputs

`make dist` writes release evidence next to the binary:

- `dist/SECURITY-ANALYSIS.md`: human-readable summary of generated release evidence.
- `dist/SHA256SUMS`: checksums for release assets.
- `dist/kcnotes.spdx.json`: SBOM output when `ENABLE_SBOM=true`.
- `dist/grype-report.txt`: vulnerability scan output when `ENABLE_GRYPE=true`.

`ENABLE_GRYPE=true` requires `ENABLE_SBOM=true` because the vulnerability scan
runs against the generated SBOM. `GRYPE_FAIL_ON` controls the severity threshold
for the Grype scan and defaults to `critical`.

## GitHub Release Behavior

The release workflow runs on `v*` tags. Before publishing, it:

- verifies the tag commit is reachable from the repository default branch
- runs `make test`
- runs `make scan`
- runs `make dist`
- uploads the complete `dist/` directory as an Actions artifact
- creates GitHub artifact provenance attestations for generated release files
- creates a GitHub Release only when one does not already exist for the tag

Existing releases are not modified. This is intentional: release assets should be
treated as published evidence, not mutable build output.

## Reproducibility Checks

For release-related changes, validate the packaging path:

```sh
ENABLE_SBOM=false ENABLE_GRYPE=false make dist
```

## When To Update This Area

Update this guide when changing:

- `scripts/dist.sh`
- release workflow behavior
- release artifact names
- SBOM or vulnerability scan settings

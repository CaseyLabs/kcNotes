## Summary

Reviewed `codex/phase-20260502-223931` against local `main`. The branch adds synchronous media normalization, image dimensions, generated variants, variant persistence, quota accounting, and publish support. I did not edit files.

## Findings

1. **Medium: upload processing lacks decoded image dimension/pixel limits.**  
   [media.go](/home/user/sync/git/casey/kcnotes/src/internal/http/handlers/media.go:123) now sends accepted uploads into `Normalize`, and [process.go](/home/user/sync/git/casey/kcnotes/src/internal/media/process.go:34) only checks that dimensions are positive before [fully decoding](/home/user/sync/git/casey/kcnotes/src/internal/media/process.go:44) and re-encoding/resizing the image synchronously. The 10 MiB body cap does not bound decoded pixels, so an authenticated user can upload a small compressed image with very large dimensions and force large memory/CPU work in the request path. Add explicit max width/height or max pixel count checks immediately after `DecodeConfig`, before `image.Decode`, and cover the rejection path with a focused test.

2. **Medium: changed upload behavior is not covered at the handler level.**  
   The branch changes the real `/admin/media` write path: normalization, variant file writes, quota size calculation, DB metadata, cleanup on partial failure, and HTMX table refresh. Existing handler tests still only cover `detectAndValidateMedia` and filename cleanup in [media_test.go](/home/user/sync/git/casey/kcnotes/src/internal/http/handlers/media_test.go:18), while the new tests cover the processing helper and store persistence separately. A regression in wiring, quota accounting, variant file cleanup, or rendered upload response could pass. Add at least one authenticated `UploadMedia` test that verifies original plus variant files, DB rows, dimensions, quota input, and an error/cleanup case.

## Validation gaps

`go test ./...` could not run in this environment because the filesystem is read-only and Go could not create `/tmp/kcnotes-gocache`.

I did not run `make css`, `make lint`, `make build`, `make test`, `make publish`, or `make smoke`; those are still needed for this route/media/publish branch.

## No-action notes

The new `media_variants` migration is transactional under the existing migration runner.

GIFs are conservatively left unmodified and do not get variants, which matches the “where practical” metadata-stripping wording.

The tracked `.codex/runs/.../next-phase-plan.md` is unusual, but I did not treat it as a defect unless the repo intends to keep local agent run plans out of branches.


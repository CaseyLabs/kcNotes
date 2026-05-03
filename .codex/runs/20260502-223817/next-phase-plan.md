1. **Phase Name**

O2: Media Processing Pipeline, synchronous first slice

2. **Goal**

Make uploaded images safer and more useful by normalizing stored uploads, stripping metadata where practical, generating thumbnails/responsive variants, recording variant metadata, and publishing those variants alongside originals.

3. **Files Likely Affected**

- `src/internal/domain/media.go`
- `src/internal/http/handlers/media.go`
- `src/internal/store/sqlite/media_store.go`
- `src/internal/store/sqlite/migrations/0009_media_variants.sql`
- `src/internal/publish/publisher.go`
- `src/internal/http/handlers/media_test.go`
- `src/internal/store/sqlite/post_store_test.go` or a new media store test file
- `src/internal/publish/publisher_test.go`
- `src/web/templates/partials/media_table.tmpl`
- `README.md`
- `docs/IMPLEMENTATION-PLAN.md`

4. **Exact Implementation Tasks**

- Add a `media_variants` table keyed by media ID and variant name, with stored filename, MIME, size, width, height, and timestamps.
- Extend media domain/store APIs to create and list variants with the original media item.
- Refactor upload processing so the handler:
  - validates the uploaded image as it does today;
  - decodes image dimensions;
  - writes a normalized original file;
  - strips EXIF/metadata for JPEG/PNG by re-encoding from decoded pixels where feasible;
  - creates deterministic thumbnail/responsive variant filenames derived from the media ID;
  - records original and variant metadata transactionally enough to avoid DB/file drift where practical.
- Add a small internal image-processing helper instead of putting resizing logic directly in `media.go`.
- Update `/admin/media` to show image dimensions and variant/static paths, keeping the current HTMX upload/table contract.
- Update static publishing so `dist/media` includes original published media plus recorded variants, and the publish manifest tracks them.
- Update docs and tracker to mark the selected O2 slice implemented and leave async/background processing explicitly in backlog.

5. **Validation Plan**

- Run focused Go tests for media handler/store/publish behavior.
- Run normal app checks:
  - `make css`
  - `make lint`
  - `make build`
  - `make test`
- Run publishing checks:
  - `make publish`
  - inspect generated `dist/site/media/*`
  - inspect generated `.publish-manifest.json`
- Run `make smoke` because upload/media route behavior changes.
- Manual UI check: `/admin/media` upload, table refresh, preview link, static path display, and published media output.

6. **Non-Goals**

- Do not add passkey attestation policy in this phase.
- Do not broaden auth, recovery, password, TOTP, or `/admin/mfa` behavior.
- Do not implement arbitrary background job orchestration for all media processing yet.
- Do not add a full asset picker/editor for post bodies.
- Do not make `media.sha256` unique; that belongs to O5.
- Do not change static publishing semantics for posts/pages beyond media files.

7. **Risks**

- Image re-encoding can increase file size or alter animation behavior, especially GIFs. Keep GIF normalization conservative.
- Adding image resize dependencies may be justified, but it should be minimal and verified before coding.
- File/database consistency needs care because filesystem writes cannot be rolled back with SQLite transactions.
- Publishing variants can expose stale files if manifest tracking is incomplete.
- Metadata stripping must be tested with fixture images; otherwise it is easy to claim privacy improvement without proving it.
- Responsive variant policy needs clear size/name choices to avoid later migration churn.


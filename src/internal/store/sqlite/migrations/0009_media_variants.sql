-- TEACHING NOTES (SQLite migrations):
-- - Variants are tied to their original media row and removed with it.
-- - `(media_id, name)` keeps deterministic variants idempotent.

ALTER TABLE media ADD COLUMN width INTEGER NOT NULL DEFAULT 0;
ALTER TABLE media ADD COLUMN height INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS media_variants (
    media_id TEXT NOT NULL,
    name TEXT NOT NULL,
    stored_name TEXT NOT NULL,
    mime TEXT NOT NULL,
    size INTEGER NOT NULL,
    width INTEGER NOT NULL,
    height INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (media_id, name),
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_media_variants_media_id ON media_variants(media_id);

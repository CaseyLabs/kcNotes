-- O5 media de-duplication:
-- - media_assets is the canonical physical file record, unique by normalized SHA-256.
-- - media rows remain the admin library entries, one per uploader per asset.
-- - existing media rows become their own canonical assets unless another row has the
--   same SHA-256, in which case the earliest row owns the shared asset.

CREATE TABLE IF NOT EXISTS media_assets (
    id TEXT PRIMARY KEY,
    stored_name TEXT NOT NULL UNIQUE,
    mime TEXT NOT NULL,
    size INTEGER NOT NULL,
    sha256 TEXT NOT NULL UNIQUE,
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO media_assets(id, stored_name, mime, size, sha256, width, height, created_at)
SELECT media.id, media.stored_name, media.mime, media.size, media.sha256, media.width, media.height, media.created_at
FROM media
WHERE media.id = (
    SELECT canonical.id
    FROM media AS canonical
    WHERE canonical.sha256 = media.sha256
    ORDER BY canonical.created_at ASC, canonical.id ASC
    LIMIT 1
);

ALTER TABLE media ADD COLUMN asset_id TEXT;

UPDATE media
SET asset_id = (
    SELECT media_assets.id
    FROM media_assets
    WHERE media_assets.sha256 = media.sha256
    LIMIT 1
)
WHERE asset_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_media_assets_sha256 ON media_assets(sha256);
CREATE INDEX IF NOT EXISTS idx_media_asset_id ON media(asset_id);
CREATE INDEX IF NOT EXISTS idx_media_created_by_asset ON media(created_by, asset_id);

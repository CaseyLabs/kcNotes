-- Enforce O5's one library row per uploader per physical media asset invariant.
-- This follow-up keeps branch-local databases that already applied the first
-- O5 migration aligned with the final schema.

DELETE FROM media
WHERE asset_id IS NOT NULL
  AND id NOT IN (
      SELECT id
      FROM (
          SELECT
              id,
              ROW_NUMBER() OVER (
                  PARTITION BY created_by, asset_id
                  ORDER BY created_at ASC, id ASC
              ) AS rn
          FROM media
          WHERE asset_id IS NOT NULL
      )
      WHERE rn = 1
  );

DROP INDEX IF EXISTS idx_media_created_by_asset;

CREATE UNIQUE INDEX IF NOT EXISTS idx_media_created_by_asset ON media(created_by, asset_id);

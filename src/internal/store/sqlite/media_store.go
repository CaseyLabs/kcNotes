// TEACHING NOTES:
// SQLite store files implement persistence behind focused methods.
// In Go, `database/sql` provides a portable API over concrete drivers.
// Useful Go concepts to notice:
// 1. Use context-aware DB calls (`QueryContext`, `ExecContext`).
// 2. Scan DB rows into strongly-typed structs.
// 3. Keep SQL close to call sites for readability unless reuse demands abstraction.
// 4. Wrap low-level errors with operation context for better debugging.
// 5. Transactions should stay short and avoid external IO inside.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"kcnotes/internal/domain"
)

// CreateMedia explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) CreateMedia(ctx context.Context, media domain.Media) error {
	return s.CreateMediaWithVariants(ctx, media, nil)
}

// CreateMediaWithVariants explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) CreateMediaWithVariants(ctx context.Context, media domain.Media, variants []domain.MediaVariant) error {
	err := s.retry.ExecRetry(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		assetID := strings.TrimSpace(media.AssetID)
		if assetID == "" {
			assetID = media.ID
		}
		_, err = tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO media_assets(id, stored_name, mime, size, sha256, width, height)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, assetID, media.StoredName, media.MIME, media.Size, media.SHA256, media.Width, media.Height)
		if err != nil {
			return err
		}
		var canonicalAssetID string
		if err := tx.QueryRowContext(ctx, `
			SELECT id
			FROM media_assets
			WHERE sha256 = ?
			LIMIT 1
		`, media.SHA256).Scan(&canonicalAssetID); err != nil {
			return err
		}
		if canonicalAssetID != assetID {
			return fmt.Errorf("media asset sha256 already exists")
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO media(id, asset_id, stored_name, original_name, mime, size, sha256, width, height, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, media.ID, assetID, media.StoredName, media.OriginalName, media.MIME, media.Size, media.SHA256, media.Width, media.Height, media.CreatedBy)
		if isMediaIDConflict(err) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, variant := range variants {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO media_variants(media_id, name, stored_name, mime, size, width, height)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, media.ID, variant.Name, variant.StoredName, variant.MIME, variant.Size, variant.Width, variant.Height)
			if err != nil {
				return err
			}
		}
		return tx.Commit()
	})
	if err != nil {
		return fmt.Errorf("create media: %w", err)
	}
	return nil
}

// AttachMediaToAsset creates a user-visible media library row for an existing
// physical asset without writing another copy of the file.
func (s *AuthStore) AttachMediaToAsset(ctx context.Context, media domain.Media) error {
	if strings.TrimSpace(media.AssetID) == "" {
		return fmt.Errorf("attach media to asset: asset id is required")
	}
	err := s.retry.ExecRetry(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO media(id, asset_id, stored_name, original_name, mime, size, sha256, width, height, created_by)
			SELECT ?, media_assets.id, media_assets.stored_name, ?, media_assets.mime, media_assets.size,
				media_assets.sha256, media_assets.width, media_assets.height, ?
			FROM media_assets
			WHERE media_assets.id = ?
		`, media.ID, media.OriginalName, media.CreatedBy, media.AssetID)
		return err
	})
	if err != nil {
		return fmt.Errorf("attach media to asset: %w", err)
	}
	return nil
}

// ListMedia explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) ListMedia(ctx context.Context, limit int) ([]domain.Media, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var media []domain.Media
	err := s.retry.QueryRetry(ctx, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT
				media.id,
				COALESCE(media.asset_id, media.id) AS asset_id,
				COALESCE(media_assets.stored_name, media.stored_name) AS stored_name,
				media.original_name,
				COALESCE(media_assets.mime, media.mime) AS mime,
				COALESCE(media_assets.size, media.size) AS size,
				COALESCE(media_assets.sha256, media.sha256) AS sha256,
				COALESCE(media_assets.width, media.width) AS width,
				COALESCE(media_assets.height, media.height) AS height,
				media.created_by,
				media.created_at
			FROM media
			LEFT JOIN media_assets ON media_assets.id = media.asset_id
			ORDER BY media.created_at DESC, media.id DESC
			LIMIT ?
		`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		items := make([]domain.Media, 0, limit)
		for rows.Next() {
			item, scanErr := scanMedia(rows)
			if scanErr != nil {
				return scanErr
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if err := s.loadMediaVariants(ctx, items); err != nil {
			return err
		}
		media = items
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list media: %w", err)
	}
	return media, nil
}

// GetMediaByID explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) GetMediaByID(ctx context.Context, id string) (domain.Media, error) {
	var media domain.Media
	err := s.retry.QueryRetry(ctx, func() error {
		row := s.db.QueryRowContext(ctx, `
			SELECT
				media.id,
				COALESCE(media.asset_id, media.id) AS asset_id,
				COALESCE(media_assets.stored_name, media.stored_name) AS stored_name,
				media.original_name,
				COALESCE(media_assets.mime, media.mime) AS mime,
				COALESCE(media_assets.size, media.size) AS size,
				COALESCE(media_assets.sha256, media.sha256) AS sha256,
				COALESCE(media_assets.width, media.width) AS width,
				COALESCE(media_assets.height, media.height) AS height,
				media.created_by,
				media.created_at
			FROM media
			LEFT JOIN media_assets ON media_assets.id = media.asset_id
			WHERE media.id = ?
			LIMIT 1
		`, id)
		item, err := scanMedia(row)
		if err != nil {
			return err
		}
		items := []domain.Media{item}
		if err := s.loadMediaVariants(ctx, items); err != nil {
			return err
		}
		item = items[0]
		media = item
		return nil
	})
	if err != nil {
		return domain.Media{}, err
	}
	return media, nil
}

// ListPublishableMedia explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) ListPublishableMedia(ctx context.Context) ([]domain.Media, error) {
	return s.ListMedia(ctx, 1000)
}

// GetMediaAssetBySHA256 returns the canonical physical media asset for a
// normalized upload hash.
func (s *AuthStore) GetMediaAssetBySHA256(ctx context.Context, sha256 string) (domain.MediaAsset, error) {
	var asset domain.MediaAsset
	err := s.retry.QueryRetry(ctx, func() error {
		row := s.db.QueryRowContext(ctx, `
			SELECT id, stored_name, mime, size, sha256, width, height, created_at
			FROM media_assets
			WHERE sha256 = ?
			LIMIT 1
		`, sha256)
		item, err := scanMediaAsset(row)
		if err != nil {
			return err
		}
		if err := s.loadMediaAssetVariants(ctx, []*domain.MediaAsset{&item}); err != nil {
			return err
		}
		asset = item
		return nil
	})
	if err != nil {
		return domain.MediaAsset{}, err
	}
	return asset, nil
}

// GetMediaByAssetAndUser returns the user's media library row for a shared
// physical asset.
func (s *AuthStore) GetMediaByAssetAndUser(ctx context.Context, assetID, userID string) (domain.Media, error) {
	var media domain.Media
	err := s.retry.QueryRetry(ctx, func() error {
		row := s.db.QueryRowContext(ctx, `
			SELECT
				media.id,
				COALESCE(media.asset_id, media.id) AS asset_id,
				COALESCE(media_assets.stored_name, media.stored_name) AS stored_name,
				media.original_name,
				COALESCE(media_assets.mime, media.mime) AS mime,
				COALESCE(media_assets.size, media.size) AS size,
				COALESCE(media_assets.sha256, media.sha256) AS sha256,
				COALESCE(media_assets.width, media.width) AS width,
				COALESCE(media_assets.height, media.height) AS height,
				media.created_by,
				media.created_at
			FROM media
			LEFT JOIN media_assets ON media_assets.id = media.asset_id
			WHERE COALESCE(media.asset_id, media.id) = ? AND media.created_by = ?
			LIMIT 1
		`, assetID, userID)
		item, err := scanMedia(row)
		if err != nil {
			return err
		}
		items := []domain.Media{item}
		if err := s.loadMediaVariants(ctx, items); err != nil {
			return err
		}
		media = items[0]
		return nil
	})
	if err != nil {
		return domain.Media{}, err
	}
	return media, nil
}

// MediaUsage explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) MediaUsage(ctx context.Context, userID string) (userBytes, totalBytes int64, err error) {
	err = s.retry.QueryRetry(ctx, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT
				COALESCE((SELECT SUM(asset_bytes) FROM (
					SELECT
						media_assets.id,
						media_assets.size + COALESCE(SUM(media_variants.size), 0) AS asset_bytes
					FROM media_assets
					LEFT JOIN media_variants ON media_variants.media_id = media_assets.id
					GROUP BY media_assets.id
				)), 0) AS total_bytes,
				COALESCE(SUM(CASE WHEN media_rows.created_by = ? THEN media_rows.item_bytes ELSE 0 END), 0) AS user_bytes
			FROM (
				SELECT
					media.created_by,
					COALESCE(media.asset_id, media.id) AS asset_id,
					COALESCE(media_assets.size, media.size) + COALESCE(SUM(media_variants.size), 0) AS item_bytes
				FROM media
				LEFT JOIN media_assets ON media_assets.id = media.asset_id
				LEFT JOIN media_variants ON media_variants.media_id = COALESCE(media.asset_id, media.id)
				GROUP BY media.id
			) AS media_rows
		`, userID).Scan(&totalBytes, &userBytes)
	})
	if err != nil {
		return 0, 0, fmt.Errorf("media usage: %w", err)
	}
	return userBytes, totalBytes, nil
}

// scanMedia explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func scanMedia(scanner interface{ Scan(dest ...any) error }) (domain.Media, error) {
	var (
		media        domain.Media
		createdAtRaw string
	)
	err := scanner.Scan(
		&media.ID,
		&media.AssetID,
		&media.StoredName,
		&media.OriginalName,
		&media.MIME,
		&media.Size,
		&media.SHA256,
		&media.Width,
		&media.Height,
		&media.CreatedBy,
		&createdAtRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Media{}, ErrNotFound
		}
		return domain.Media{}, fmt.Errorf("scan media: %w", err)
	}
	createdAt, err := parseSQLiteTime(createdAtRaw)
	if err != nil {
		return domain.Media{}, fmt.Errorf("parse media created_at: %w", err)
	}
	media.CreatedAt = createdAt
	return media, nil
}

func scanMediaAsset(scanner interface{ Scan(dest ...any) error }) (domain.MediaAsset, error) {
	var (
		asset        domain.MediaAsset
		createdAtRaw string
	)
	err := scanner.Scan(
		&asset.ID,
		&asset.StoredName,
		&asset.MIME,
		&asset.Size,
		&asset.SHA256,
		&asset.Width,
		&asset.Height,
		&createdAtRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.MediaAsset{}, ErrNotFound
		}
		return domain.MediaAsset{}, fmt.Errorf("scan media asset: %w", err)
	}
	createdAt, err := parseSQLiteTime(createdAtRaw)
	if err != nil {
		return domain.MediaAsset{}, fmt.Errorf("parse media asset created_at: %w", err)
	}
	asset.CreatedAt = createdAt
	return asset, nil
}

func (s *AuthStore) loadMediaVariants(ctx context.Context, media []domain.Media) error {
	if len(media) == 0 {
		return nil
	}
	byID := make(map[string][]int, len(media))
	ids := make([]any, 0, len(media))
	placeholders := make([]string, 0, len(media))
	for i := range media {
		assetID := media[i].AssetID
		if assetID == "" {
			assetID = media[i].ID
		}
		byID[assetID] = append(byID[assetID], i)
		ids = append(ids, assetID)
		placeholders = append(placeholders, "?")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT media_id, name, stored_name, mime, size, width, height, created_at
		FROM media_variants
		WHERE media_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY media_id, name
	`, ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			variant      domain.MediaVariant
			createdAtRaw string
		)
		if err := rows.Scan(
			&variant.MediaID,
			&variant.Name,
			&variant.StoredName,
			&variant.MIME,
			&variant.Size,
			&variant.Width,
			&variant.Height,
			&createdAtRaw,
		); err != nil {
			return err
		}
		createdAt, err := parseSQLiteTime(createdAtRaw)
		if err != nil {
			return fmt.Errorf("parse media variant created_at: %w", err)
		}
		variant.CreatedAt = createdAt
		if indexes, ok := byID[variant.MediaID]; ok {
			for _, idx := range indexes {
				media[idx].Variants = append(media[idx].Variants, variant)
			}
		}
	}
	return rows.Err()
}

func (s *AuthStore) loadMediaAssetVariants(ctx context.Context, assets []*domain.MediaAsset) error {
	if len(assets) == 0 {
		return nil
	}
	byID := make(map[string]*domain.MediaAsset, len(assets))
	ids := make([]any, 0, len(assets))
	placeholders := make([]string, 0, len(assets))
	for _, asset := range assets {
		byID[asset.ID] = asset
		ids = append(ids, asset.ID)
		placeholders = append(placeholders, "?")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT media_id, name, stored_name, mime, size, width, height, created_at
		FROM media_variants
		WHERE media_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY media_id, name
	`, ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			variant      domain.MediaVariant
			createdAtRaw string
		)
		if err := rows.Scan(
			&variant.MediaID,
			&variant.Name,
			&variant.StoredName,
			&variant.MIME,
			&variant.Size,
			&variant.Width,
			&variant.Height,
			&createdAtRaw,
		); err != nil {
			return err
		}
		createdAt, err := parseSQLiteTime(createdAtRaw)
		if err != nil {
			return fmt.Errorf("parse media variant created_at: %w", err)
		}
		variant.CreatedAt = createdAt
		if asset, ok := byID[variant.MediaID]; ok {
			asset.Variants = append(asset.Variants, variant)
		}
	}
	return rows.Err()
}

// isMediaIDConflict explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func isMediaIDConflict(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: media.id")
}

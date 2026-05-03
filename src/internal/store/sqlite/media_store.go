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

		_, err = tx.ExecContext(ctx, `
			INSERT INTO media(id, stored_name, original_name, mime, size, sha256, width, height, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, media.ID, media.StoredName, media.OriginalName, media.MIME, media.Size, media.SHA256, media.Width, media.Height, media.CreatedBy)
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

// ListMedia explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) ListMedia(ctx context.Context, limit int) ([]domain.Media, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var media []domain.Media
	err := s.retry.QueryRetry(ctx, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, stored_name, original_name, mime, size, sha256, width, height, created_by, created_at
			FROM media
			ORDER BY created_at DESC, id DESC
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
			SELECT id, stored_name, original_name, mime, size, sha256, width, height, created_by, created_at
			FROM media
			WHERE id = ?
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

// MediaUsage explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) MediaUsage(ctx context.Context, userID string) (userBytes, totalBytes int64, err error) {
	err = s.retry.QueryRetry(ctx, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT
				COALESCE(SUM(item_bytes), 0) AS total_bytes,
				COALESCE(SUM(CASE WHEN created_by = ? THEN item_bytes ELSE 0 END), 0) AS user_bytes
			FROM (
				SELECT
					media.created_by,
					media.size + COALESCE(SUM(media_variants.size), 0) AS item_bytes
				FROM media
				LEFT JOIN media_variants ON media_variants.media_id = media.id
				GROUP BY media.id
			)
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

func (s *AuthStore) loadMediaVariants(ctx context.Context, media []domain.Media) error {
	if len(media) == 0 {
		return nil
	}
	byID := make(map[string]int, len(media))
	ids := make([]any, 0, len(media))
	placeholders := make([]string, 0, len(media))
	for i := range media {
		byID[media[i].ID] = i
		ids = append(ids, media[i].ID)
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
		if idx, ok := byID[variant.MediaID]; ok {
			media[idx].Variants = append(media[idx].Variants, variant)
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

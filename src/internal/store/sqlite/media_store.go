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
	err := s.retry.ExecRetry(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO media(id, stored_name, original_name, mime, size, sha256, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, media.ID, media.StoredName, media.OriginalName, media.MIME, media.Size, media.SHA256, media.CreatedBy)
		if isMediaIDConflict(err) {
			return nil
		}
		return err
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
			SELECT id, stored_name, original_name, mime, size, sha256, created_by, created_at
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
			SELECT id, stored_name, original_name, mime, size, sha256, created_by, created_at
			FROM media
			WHERE id = ?
			LIMIT 1
		`, id)
		item, err := scanMedia(row)
		if err != nil {
			return err
		}
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
				COALESCE(SUM(size), 0) AS total_bytes,
				COALESCE(SUM(CASE WHEN created_by = ? THEN size ELSE 0 END), 0) AS user_bytes
			FROM media
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

// isMediaIDConflict explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func isMediaIDConflict(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: media.id")
}

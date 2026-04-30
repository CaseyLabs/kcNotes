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
	"regexp"
	"strings"
	"time"

	"kcnotes/internal/domain"
)

var ErrConflict = errors.New("conflict")
var ftsTokenPattern = regexp.MustCompile(`[a-zA-Z0-9]+`)

type PostListFilter struct {
	Type     domain.PostType
	Status   domain.PostStatus
	Query    string
	Sort     string
	Page     int
	PageSize int
	AuthorID string
}

// ListPosts explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) ListPosts(ctx context.Context, filter PostListFilter) ([]domain.Post, int, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	where, args := buildPostFilterWhere(filter)

	countQuery := "SELECT COUNT(1) FROM posts " + where
	sortExpr := mapSort(filter.Sort)
	listQuery := `
		SELECT id, type, title, slug, body_md, status, author_id, created_at, updated_at, published_at, deleted_at
		FROM posts
	` + where + `
		ORDER BY ` + sortExpr + `
		LIMIT ? OFFSET ?`
	var (
		total int
		posts []domain.Post
	)
	err := s.retry.QueryRetry(ctx, func() error {
		if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		rows, err := s.db.QueryContext(ctx, listQuery, append(args, pageSize, offset)...)
		if err != nil {
			return err
		}
		defer rows.Close()

		result := make([]domain.Post, 0, pageSize)
		for rows.Next() {
			post, err := scanPost(rows)
			if err != nil {
				return err
			}
			result = append(result, post)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		posts = result
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list posts: %w", err)
	}
	return posts, total, nil
}

// GetPostByID explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) GetPostByID(ctx context.Context, id string) (domain.Post, error) {
	var post domain.Post
	err := s.retry.QueryRetry(ctx, func() error {
		row := s.db.QueryRowContext(ctx, `
			SELECT id, type, title, slug, body_md, status, author_id, created_at, updated_at, published_at, deleted_at
			FROM posts
			WHERE id = ?
		`, id)
		p, err := scanPost(row)
		if err != nil {
			return err
		}
		post = p
		return nil
	})
	if err != nil {
		return domain.Post{}, err
	}
	return post, nil
}

// GetPublishedPostBySlug explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) GetPublishedPostBySlug(ctx context.Context, slug string) (domain.Post, error) {
	return s.getPublishedPostByTypeAndSlug(ctx, domain.PostTypePost, slug)
}

// GetPublishedPageBySlug explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) GetPublishedPageBySlug(ctx context.Context, slug string) (domain.Post, error) {
	return s.getPublishedPostByTypeAndSlug(ctx, domain.PostTypePage, slug)
}

// ListPublishedPosts explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) ListPublishedPosts(ctx context.Context, limit int) ([]domain.Post, error) {
	return s.ListPublicPosts(ctx, false, limit)
}

// ListPublicPosts explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) ListPublicPosts(ctx context.Context, includeDrafts bool, limit int) ([]domain.Post, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}
	var posts []domain.Post
	query := `
		SELECT id, type, title, slug, body_md, status, author_id, created_at, updated_at, published_at, deleted_at
		FROM posts
		WHERE type = ? AND deleted_at IS NULL
	`
	args := []any{string(domain.PostTypePost)}
	if includeDrafts {
		query += " AND status IN (?, ?) "
		args = append(args, string(domain.PostStatusPublished), string(domain.PostStatusDraft))
	} else {
		query += " AND status = ? "
		args = append(args, string(domain.PostStatusPublished))
	}
	query += " ORDER BY COALESCE(published_at, updated_at) DESC, updated_at DESC, id DESC LIMIT ?"
	args = append(args, limit)
	err := s.retry.QueryRetry(ctx, func() error {
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		result := make([]domain.Post, 0, limit)
		for rows.Next() {
			post, scanErr := scanPost(rows)
			if scanErr != nil {
				return scanErr
			}
			result = append(result, post)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		posts = result
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list published posts: %w", err)
	}
	return posts, nil
}

// ListPublicPages explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) ListPublicPages(ctx context.Context, includeDrafts bool) ([]domain.Post, error) {
	var pages []domain.Post
	query := `
		SELECT id, type, title, slug, body_md, status, author_id, created_at, updated_at, published_at, deleted_at
		FROM posts
		WHERE type = ? AND deleted_at IS NULL
	`
	args := []any{string(domain.PostTypePage)}
	if includeDrafts {
		query += " AND status IN (?, ?) "
		args = append(args, string(domain.PostStatusPublished), string(domain.PostStatusDraft))
	} else {
		query += " AND status = ? "
		args = append(args, string(domain.PostStatusPublished))
	}
	query += " ORDER BY slug ASC, id ASC"
	err := s.retry.QueryRetry(ctx, func() error {
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		result := make([]domain.Post, 0, 64)
		for rows.Next() {
			page, scanErr := scanPost(rows)
			if scanErr != nil {
				return scanErr
			}
			result = append(result, page)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		pages = result
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list public pages: %w", err)
	}
	return pages, nil
}

// CreatePost explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) CreatePost(ctx context.Context, post domain.Post) error {
	if !domain.IsValidPostType(post.Type) {
		return fmt.Errorf("invalid type %q", post.Type)
	}
	if !domain.IsValidPostStatus(post.Status) {
		return fmt.Errorf("invalid status %q", post.Status)
	}

	err := s.retry.ExecRetry(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO posts(id, type, title, slug, body_md, status, author_id, published_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, post.ID, string(post.Type), post.Title, post.Slug, post.BodyMD, string(post.Status), post.AuthorID, nullableTime(post.PublishedAt))
		if isPostIDConflict(err) {
			return nil
		}
		return err
	})
	if err != nil {
		if isSlugConflict(err) {
			return ErrConflict
		}
		return fmt.Errorf("create post: %w", err)
	}
	return nil
}

// getPublishedPostByTypeAndSlug explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) getPublishedPostByTypeAndSlug(ctx context.Context, postType domain.PostType, slug string) (domain.Post, error) {
	var post domain.Post
	err := s.retry.QueryRetry(ctx, func() error {
		row := s.db.QueryRowContext(ctx, `
			SELECT id, type, title, slug, body_md, status, author_id, created_at, updated_at, published_at, deleted_at
			FROM posts
			WHERE slug = ? AND type = ? AND status = ? AND deleted_at IS NULL
			LIMIT 1
		`, slug, string(postType), string(domain.PostStatusPublished))
		p, err := scanPost(row)
		if err != nil {
			return err
		}
		post = p
		return nil
	})
	if err != nil {
		return domain.Post{}, err
	}
	return post, nil
}

// UpdatePost explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) UpdatePost(ctx context.Context, post domain.Post, actor domain.User) (bool, error) {
	if !domain.IsValidPostType(post.Type) {
		return false, fmt.Errorf("invalid type %q", post.Type)
	}
	if !domain.IsValidPostStatus(post.Status) {
		return false, fmt.Errorf("invalid status %q", post.Status)
	}

	query := `
		UPDATE posts
		SET type = ?,
			title = ?,
			slug = ?,
			body_md = ?,
			status = ?,
			published_at = CASE
				WHEN ? IS NOT NULL AND published_at IS NULL THEN ?
				ELSE published_at
			END,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	publishedAt := nullableTime(post.PublishedAt)
	args := []any{string(post.Type), post.Title, post.Slug, post.BodyMD, string(post.Status), publishedAt, publishedAt, post.ID}
	if actor.Role == domain.RoleAuthor {
		query += " AND author_id = ?"
		args = append(args, actor.ID)
	}
	var res sql.Result
	err := s.retry.ExecRetry(ctx, func() error {
		var err error
		res, err = s.db.ExecContext(ctx, query, args...)
		return err
	})
	if err != nil {
		if isSlugConflict(err) {
			return false, ErrConflict
		}
		return false, fmt.Errorf("update post: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update post rows: %w", err)
	}
	return rows > 0, nil
}

// UpsertAutosaveSnapshot stores one recoverable edit snapshot per user/post pair.
func (s *AuthStore) UpsertAutosaveSnapshot(ctx context.Context, snapshot domain.AutosaveSnapshot, actor domain.User) (bool, error) {
	if !domain.IsValidPostType(snapshot.Type) {
		return false, fmt.Errorf("invalid type %q", snapshot.Type)
	}
	if !domain.IsValidPostStatus(snapshot.Status) || snapshot.Status == domain.PostStatusDeleted {
		return false, fmt.Errorf("invalid status %q", snapshot.Status)
	}
	if snapshot.AuthorID != actor.ID {
		return false, nil
	}

	query := `
		INSERT INTO autosave_snapshots(post_id, author_id, type, title, slug, body_md, status, base_updated_at, created_at, updated_at)
		SELECT id, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		FROM posts
		WHERE id = ? AND deleted_at IS NULL AND datetime(updated_at) = datetime(?)
	`
	args := []any{snapshot.AuthorID, string(snapshot.Type), snapshot.Title, snapshot.Slug, snapshot.BodyMD, string(snapshot.Status), nullableTime(&snapshot.BaseUpdatedAt), snapshot.PostID, nullableTime(&snapshot.BaseUpdatedAt)}
	if actor.Role == domain.RoleAuthor {
		query += " AND author_id = ?"
		args = append(args, actor.ID)
	}
	query += `
		ON CONFLICT(post_id, author_id) DO UPDATE SET
			type = excluded.type,
			title = excluded.title,
			slug = excluded.slug,
			body_md = excluded.body_md,
			status = excluded.status,
			base_updated_at = excluded.base_updated_at,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`

	var res sql.Result
	err := s.retry.ExecRetry(ctx, func() error {
		var err error
		res, err = s.db.ExecContext(ctx, query, args...)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("upsert autosave snapshot: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("upsert autosave snapshot rows: %w", err)
	}
	return rows > 0, nil
}

// GetLatestAutosaveSnapshot loads the user's latest recoverable snapshot.
func (s *AuthStore) GetLatestAutosaveSnapshot(ctx context.Context, postID, authorID string) (domain.AutosaveSnapshot, error) {
	var snapshot domain.AutosaveSnapshot
	err := s.retry.QueryRetry(ctx, func() error {
		row := s.db.QueryRowContext(ctx, `
			SELECT post_id, author_id, type, title, slug, body_md, status, base_updated_at, created_at, updated_at
			FROM autosave_snapshots
			WHERE post_id = ? AND author_id = ?
		`, postID, authorID)
		got, err := scanAutosaveSnapshot(row)
		if err != nil {
			return err
		}
		snapshot = got
		return nil
	})
	if err != nil {
		return domain.AutosaveSnapshot{}, err
	}
	return snapshot, nil
}

// DismissAutosaveSnapshot deletes only the actor's private recovery snapshot.
func (s *AuthStore) DismissAutosaveSnapshot(ctx context.Context, postID, authorID string) (bool, error) {
	var res sql.Result
	err := s.retry.ExecRetry(ctx, func() error {
		var err error
		res, err = s.db.ExecContext(ctx, `
			DELETE FROM autosave_snapshots
			WHERE post_id = ? AND author_id = ?
		`, postID, authorID)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("dismiss autosave snapshot: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("dismiss autosave snapshot rows: %w", err)
	}
	return rows > 0, nil
}

// SetPostStatus explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) SetPostStatus(ctx context.Context, id string, status domain.PostStatus, publishedAt *time.Time, actor domain.User) (bool, error) {
	if !domain.IsValidPostStatus(status) {
		return false, fmt.Errorf("invalid status %q", status)
	}
	query := `
		UPDATE posts
		SET status = ?, published_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND deleted_at IS NULL
	`
	args := []any{string(status), nullableTime(publishedAt), id}
	if actor.Role == domain.RoleAuthor {
		query += " AND author_id = ?"
		args = append(args, actor.ID)
	}
	var res sql.Result
	err := s.retry.ExecRetry(ctx, func() error {
		var err error
		res, err = s.db.ExecContext(ctx, query, args...)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("set post status: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("set post status rows: %w", err)
	}
	return rows > 0, nil
}

// SoftDeletePost explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) SoftDeletePost(ctx context.Context, id string, actor domain.User) (bool, error) {
	query := `
		UPDATE posts
		SET status = ?, deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND deleted_at IS NULL
	`
	args := []any{string(domain.PostStatusDeleted), id}
	if actor.Role == domain.RoleAuthor {
		query += " AND author_id = ?"
		args = append(args, actor.ID)
	}
	var res sql.Result
	err := s.retry.ExecRetry(ctx, func() error {
		var err error
		res, err = s.db.ExecContext(ctx, query, args...)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("soft delete post: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("soft delete post rows: %w", err)
	}
	return rows > 0, nil
}

// buildPostFilterWhere explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func buildPostFilterWhere(filter PostListFilter) (string, []any) {
	clauses := []string{"1=1"}
	args := make([]any, 0, 8)

	if filter.AuthorID != "" {
		clauses = append(clauses, "author_id = ?")
		args = append(args, filter.AuthorID)
	}

	if filter.Type != "" && domain.IsValidPostType(filter.Type) {
		clauses = append(clauses, "type = ?")
		args = append(args, string(filter.Type))
	}

	if filter.Status == domain.PostStatusDeleted {
		clauses = append(clauses, "deleted_at IS NOT NULL")
	} else {
		clauses = append(clauses, "deleted_at IS NULL")
		if filter.Status != "" && domain.IsValidPostStatus(filter.Status) {
			clauses = append(clauses, "status = ?")
			args = append(args, string(filter.Status))
		}
	}

	q := strings.TrimSpace(filter.Query)
	if q != "" {
		if matchExpr, ok := buildFTSMatchQuery(q); ok {
			clauses = append(clauses, "rowid IN (SELECT rowid FROM posts_fts WHERE posts_fts MATCH ?)")
			args = append(args, matchExpr)
		} else {
			like := "%" + q + "%"
			clauses = append(clauses, "(title LIKE ? OR body_md LIKE ? OR slug LIKE ?)")
			args = append(args, like, like, like)
		}
	}

	return "WHERE " + strings.Join(clauses, " AND "), args
}

// buildFTSMatchQuery explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func buildFTSMatchQuery(raw string) (string, bool) {
	tokens := ftsTokenPattern.FindAllString(strings.ToLower(raw), -1)
	if len(tokens) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token == "" {
			continue
		}
		parts = append(parts, token+"*")
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, " AND "), true
}

// mapSort explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func mapSort(sort string) string {
	switch sort {
	case "updated_asc":
		return "updated_at ASC, created_at ASC"
	case "created_desc":
		return "created_at DESC"
	case "created_asc":
		return "created_at ASC"
	case "title_asc":
		return "title ASC"
	case "title_desc":
		return "title DESC"
	default:
		return "updated_at DESC, created_at DESC"
	}
}

// isSlugConflict explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func isSlugConflict(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: posts.slug")
}

// isPostIDConflict explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func isPostIDConflict(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: posts.id")
}

// scanPost explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func scanPost(scanner interface{ Scan(dest ...any) error }) (domain.Post, error) {
	var (
		post         domain.Post
		postType     string
		status       string
		createdAtRaw string
		updatedAtRaw string
		publishedRaw sql.NullString
		deletedRaw   sql.NullString
	)

	err := scanner.Scan(
		&post.ID,
		&postType,
		&post.Title,
		&post.Slug,
		&post.BodyMD,
		&status,
		&post.AuthorID,
		&createdAtRaw,
		&updatedAtRaw,
		&publishedRaw,
		&deletedRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Post{}, ErrNotFound
		}
		return domain.Post{}, fmt.Errorf("scan post: %w", err)
	}

	createdAt, err := parseSQLiteTime(createdAtRaw)
	if err != nil {
		return domain.Post{}, fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := parseSQLiteTime(updatedAtRaw)
	if err != nil {
		return domain.Post{}, fmt.Errorf("parse updated_at: %w", err)
	}

	post.Type = domain.PostType(postType)
	post.Status = domain.PostStatus(status)
	post.CreatedAt = createdAt
	post.UpdatedAt = updatedAt

	if publishedRaw.Valid {
		t, err := parseSQLiteTime(publishedRaw.String)
		if err != nil {
			return domain.Post{}, fmt.Errorf("parse published_at: %w", err)
		}
		post.PublishedAt = &t
	}
	if deletedRaw.Valid {
		t, err := parseSQLiteTime(deletedRaw.String)
		if err != nil {
			return domain.Post{}, fmt.Errorf("parse deleted_at: %w", err)
		}
		post.DeletedAt = &t
	}

	return post, nil
}

func scanAutosaveSnapshot(scanner interface{ Scan(dest ...any) error }) (domain.AutosaveSnapshot, error) {
	var (
		snapshot     domain.AutosaveSnapshot
		postType     string
		status       string
		baseRaw      string
		createdAtRaw string
		updatedAtRaw string
	)
	err := scanner.Scan(
		&snapshot.PostID,
		&snapshot.AuthorID,
		&postType,
		&snapshot.Title,
		&snapshot.Slug,
		&snapshot.BodyMD,
		&status,
		&baseRaw,
		&createdAtRaw,
		&updatedAtRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.AutosaveSnapshot{}, ErrNotFound
		}
		return domain.AutosaveSnapshot{}, fmt.Errorf("scan autosave snapshot: %w", err)
	}
	baseUpdatedAt, err := parseSQLiteTime(baseRaw)
	if err != nil {
		return domain.AutosaveSnapshot{}, fmt.Errorf("parse base_updated_at: %w", err)
	}
	createdAt, err := parseSQLiteTime(createdAtRaw)
	if err != nil {
		return domain.AutosaveSnapshot{}, fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := parseSQLiteTime(updatedAtRaw)
	if err != nil {
		return domain.AutosaveSnapshot{}, fmt.Errorf("parse updated_at: %w", err)
	}
	snapshot.Type = domain.PostType(postType)
	snapshot.Status = domain.PostStatus(status)
	snapshot.BaseUpdatedAt = baseUpdatedAt
	snapshot.CreatedAt = createdAt
	snapshot.UpdatedAt = updatedAt
	return snapshot, nil
}

// parseSQLiteTime explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func parseSQLiteTime(v string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q", v)
}

// nullableTime explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func nullableTime(v *time.Time) any {
	if v == nil {
		return nil
	}
	return v.UTC().Format(time.RFC3339Nano)
}

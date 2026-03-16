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
	"time"

	"kcnotes/internal/domain"
)

var ErrNotFound = errors.New("not found")

type AuthStore struct {
	db      *sql.DB
	retry   *Retryer
	metrics *RetryMetrics
}

// NewAuthStore explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func NewAuthStore(db *sql.DB) *AuthStore {
	metrics := &RetryMetrics{}
	return &AuthStore{
		db:      db,
		retry:   NewRetryer(RetryConfig{Jitter: 30 * time.Millisecond}, metrics),
		metrics: metrics,
	}
}

// CreateUser explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) CreateUser(ctx context.Context, user domain.User) error {
	if !domain.IsValidRole(user.Role) {
		return fmt.Errorf("invalid role %q", user.Role)
	}
	err := s.retry.ExecRetry(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `
		INSERT INTO users(id, email, password_hash, role, disabled)
		VALUES (?, ?, ?, ?, ?)
	`, user.ID, strings.ToLower(strings.TrimSpace(user.Email)), user.PasswordHash, string(user.Role), boolToInt(user.Disabled))
		return err
	})
	if err != nil {
		if isUserEmailConflict(err) {
			return ErrConflict
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// GetUserByEmail explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	return s.getUser(ctx, `SELECT id, email, password_hash, role, disabled, mfa_enabled, mfa_secret FROM users WHERE email = ?`, strings.ToLower(strings.TrimSpace(email)))
}

// GetUserByID explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) GetUserByID(ctx context.Context, id string) (domain.User, error) {
	return s.getUser(ctx, `SELECT id, email, password_hash, role, disabled, mfa_enabled, mfa_secret FROM users WHERE id = ?`, id)
}

// getUser explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) getUser(ctx context.Context, query string, arg any) (domain.User, error) {
	var user domain.User
	var role string
	var disabled int
	var mfaEnabled int
	err := s.retry.QueryRetry(ctx, func() error {
		return s.db.QueryRowContext(ctx, query, arg).Scan(&user.ID, &user.Email, &user.PasswordHash, &role, &disabled, &mfaEnabled, &user.MFASecret)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, ErrNotFound
		}
		return domain.User{}, fmt.Errorf("query user: %w", err)
	}
	user.Role = domain.Role(role)
	user.Disabled = disabled == 1
	user.MFAEnabled = mfaEnabled == 1
	return user, nil
}

// CreateSession explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) CreateSession(ctx context.Context, sessionID, userID, csrfToken string, expiresAt time.Time) error {
	err := s.retry.ExecRetry(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions(id, user_id, csrf_token, expires_at)
		VALUES (?, ?, ?, ?)
	`, sessionID, userID, csrfToken, expiresAt.UTC().Format(time.RFC3339Nano))
		if isSessionConflict(err) {
			return nil
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// DeleteSession explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) DeleteSession(ctx context.Context, sessionID string) error {
	err := s.retry.ExecRetry(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, sessionID)
		return err
	})
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// GetSessionUser explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) GetSessionUser(ctx context.Context, sessionID string) (domain.SessionUser, error) {
	var (
		result        domain.SessionUser
		role          string
		disabled      int
		mfaEnabled    int
		expiresRawUTC string
	)

	err := s.retry.QueryRetry(ctx, func() error {
		return s.db.QueryRowContext(ctx, `
		SELECT s.id, s.csrf_token, s.expires_at, u.id, u.email, u.password_hash, u.role, u.disabled, u.mfa_enabled, u.mfa_secret
		FROM sessions s
		INNER JOIN users u ON u.id = s.user_id
		WHERE s.id = ?
	`, sessionID).Scan(
			&result.SessionID,
			&result.CSRFToken,
			&expiresRawUTC,
			&result.User.ID,
			&result.User.Email,
			&result.User.PasswordHash,
			&role,
			&disabled,
			&mfaEnabled,
			&result.User.MFASecret,
		)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.SessionUser{}, ErrNotFound
		}
		return domain.SessionUser{}, fmt.Errorf("query session user: %w", err)
	}

	expiresAt, err := time.Parse(time.RFC3339Nano, expiresRawUTC)
	if err != nil {
		return domain.SessionUser{}, fmt.Errorf("parse session expiry: %w", err)
	}
	result.ExpiresAt = expiresAt
	result.User.Role = domain.Role(role)
	result.User.Disabled = disabled == 1
	result.User.MFAEnabled = mfaEnabled == 1
	return result, nil
}

// CreateAuditEvent explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) CreateAuditEvent(ctx context.Context, id, actorUserID, action, entityType, entityID, ip, userAgent string) error {
	err := s.retry.ExecRetry(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_log(id, actor_user_id, action, entity_type, entity_id, ip, user_agent)
		VALUES (?, NULLIF(?, ''), ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))
	`, id, actorUserID, action, entityType, entityID, ip, userAgent)
		if isAuditConflict(err) {
			return nil
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("create audit event: %w", err)
	}
	return nil
}

// RetryMetrics explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) RetryMetrics() RetryMetricsSnapshot {
	return s.metrics.Snapshot()
}

// boolToInt explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// isSessionConflict explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func isSessionConflict(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: sessions.id")
}

// isAuditConflict explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func isAuditConflict(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: audit_log.id")
}

// isUserEmailConflict explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func isUserEmailConflict(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: users.email")
}

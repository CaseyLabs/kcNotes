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
	"fmt"
	"strings"

	"kcnotes/internal/domain"
)

// ListUsers explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) ListUsers(ctx context.Context) ([]domain.User, error) {
	var users []domain.User
	err := s.retry.QueryRetry(ctx, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, email, password_hash, role, disabled, mfa_enabled
			FROM users
			ORDER BY email ASC, id ASC
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

		items := make([]domain.User, 0, 32)
		for rows.Next() {
			var (
				user       domain.User
				role       string
				disabled   int
				mfaEnabled int
			)
			if err := rows.Scan(&user.ID, &user.Email, &user.PasswordHash, &role, &disabled, &mfaEnabled); err != nil {
				return err
			}
			user.Role = domain.Role(role)
			user.Disabled = disabled == 1
			user.MFAEnabled = mfaEnabled == 1
			items = append(items, user)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		users = items
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

// UpdateUserRoleDisabled explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) UpdateUserRoleDisabled(ctx context.Context, id string, role domain.Role, disabled bool) (bool, error) {
	if !domain.IsValidRole(role) {
		return false, fmt.Errorf("invalid role %q", role)
	}
	var res sql.Result
	err := s.retry.ExecRetry(ctx, func() error {
		var err error
		res, err = s.db.ExecContext(ctx, `
			UPDATE users
			SET role = ?, disabled = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, string(role), boolToInt(disabled), strings.TrimSpace(id))
		return err
	})
	if err != nil {
		return false, fmt.Errorf("update user role/disabled: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update user rows affected: %w", err)
	}
	return rows > 0, nil
}

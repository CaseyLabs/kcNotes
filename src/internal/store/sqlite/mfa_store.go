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
)

// SetUserMFASecret explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) SetUserMFASecret(ctx context.Context, userID, secret string) (bool, error) {
	var res sql.Result
	err := s.retry.ExecRetry(ctx, func() error {
		var err error
		res, err = s.db.ExecContext(ctx, `
			UPDATE users
			SET mfa_secret = ?, mfa_enabled = 0, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, strings.ToUpper(strings.TrimSpace(secret)), strings.TrimSpace(userID))
		return err
	})
	if err != nil {
		return false, fmt.Errorf("set user mfa secret: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("set user mfa secret rows affected: %w", err)
	}
	return rows > 0, nil
}

// EnableUserMFA explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) EnableUserMFA(ctx context.Context, userID string) (bool, error) {
	var res sql.Result
	err := s.retry.ExecRetry(ctx, func() error {
		var err error
		res, err = s.db.ExecContext(ctx, `
			UPDATE users
			SET mfa_enabled = 1, updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND mfa_secret != ''
		`, strings.TrimSpace(userID))
		return err
	})
	if err != nil {
		return false, fmt.Errorf("enable user mfa: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("enable user mfa rows affected: %w", err)
	}
	return rows > 0, nil
}

// DisableUserMFA explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) DisableUserMFA(ctx context.Context, userID string) (bool, error) {
	var updated bool
	err := s.retry.TxRetry(ctx, s.db, nil, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE users
			SET mfa_enabled = 0, mfa_secret = '', updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, strings.TrimSpace(userID))
		if err != nil {
			return err
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return err
		}
		updated = rows > 0
		if !updated {
			return nil
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id = ?`, strings.TrimSpace(userID))
		return err
	})
	if err != nil {
		return false, fmt.Errorf("disable user mfa: %w", err)
	}
	return updated, nil
}

// ReplaceRecoveryCodeHashes explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) ReplaceRecoveryCodeHashes(ctx context.Context, userID string, hashes []string) error {
	userID = strings.TrimSpace(userID)
	err := s.retry.TxRetry(ctx, s.db, nil, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id = ?`, userID); err != nil {
			return err
		}
		for _, hash := range hashes {
			value := strings.TrimSpace(hash)
			if value == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO mfa_recovery_codes(user_id, code_hash)
				VALUES (?, ?)
			`, userID, value); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("replace recovery code hashes: %w", err)
	}
	return nil
}

// ConsumeRecoveryCodeHash explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) ConsumeRecoveryCodeHash(ctx context.Context, userID, hash string) (bool, error) {
	var res sql.Result
	err := s.retry.ExecRetry(ctx, func() error {
		var err error
		res, err = s.db.ExecContext(ctx, `
			UPDATE mfa_recovery_codes
			SET used_at = CURRENT_TIMESTAMP
			WHERE user_id = ? AND code_hash = ? AND used_at IS NULL
		`, strings.TrimSpace(userID), strings.TrimSpace(hash))
		return err
	})
	if err != nil {
		return false, fmt.Errorf("consume recovery code hash: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("consume recovery code hash rows affected: %w", err)
	}
	return rows > 0, nil
}

// CountUnusedRecoveryCodes explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) CountUnusedRecoveryCodes(ctx context.Context, userID string) (int, error) {
	var count int
	err := s.retry.QueryRetry(ctx, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT COUNT(1)
			FROM mfa_recovery_codes
			WHERE user_id = ? AND used_at IS NULL
		`, strings.TrimSpace(userID)).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("count unused recovery codes: %w", err)
	}
	return count, nil
}

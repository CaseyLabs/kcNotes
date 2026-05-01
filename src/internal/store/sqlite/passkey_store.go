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

func (s *AuthStore) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := s.retry.QueryRetry(ctx, func() error {
		return s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

func (s *AuthStore) CreateFirstAdminWithPasskey(ctx context.Context, user domain.User, credential domain.PasskeyCredential) error {
	if user.Role != domain.RoleAdmin {
		return fmt.Errorf("first user must be admin")
	}
	return s.retry.TxRetry(ctx, s.db, nil, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO users(id, email, password_hash, role, disabled)
			VALUES (?, ?, '', ?, 0)
		`, user.ID, strings.ToLower(strings.TrimSpace(user.Email)), string(domain.RoleAdmin)); err != nil {
			return err
		}
		return insertPasskeyCredential(ctx, tx, credential)
	})
}

func (s *AuthStore) CreatePasskeyCredential(ctx context.Context, credential domain.PasskeyCredential) error {
	err := s.retry.TxRetry(ctx, s.db, nil, func(tx *sql.Tx) error {
		return insertPasskeyCredential(ctx, tx, credential)
	})
	if err != nil {
		return fmt.Errorf("create passkey credential: %w", err)
	}
	return nil
}

func (s *AuthStore) UpdatePasskeyCredential(ctx context.Context, credential domain.PasskeyCredential) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := s.retry.ExecRetry(ctx, func() error {
		res, err := s.db.ExecContext(ctx, `
			UPDATE webauthn_credentials
			SET public_key = ?, sign_count = ?, transports = ?, backup_eligible = ?,
			    backup_state = ?, attestation_type = ?, nickname = COALESCE(NULLIF(?, ''), nickname), webauthn_credential_json = ?,
			    updated_at = ?, last_used_at = COALESCE(?, last_used_at)
			WHERE credential_id = ?
		`, credential.PublicKey, credential.SignCount, strings.Join(credential.Transports, ","),
			boolToInt(credential.BackupEligible), boolToInt(credential.BackupState),
			credential.AttestationType, credential.Nickname, credential.WebAuthnCredential,
			now, nullablePasskeyTime(credential.LastUsedAt), credential.CredentialID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("update passkey credential: %w", err)
	}
	return nil
}

func (s *AuthStore) ListPasskeyCredentials(ctx context.Context, userID string) ([]domain.PasskeyCredential, error) {
	var items []domain.PasskeyCredential
	err := s.retry.QueryRetry(ctx, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, user_id, credential_id, public_key, sign_count, transports,
			       backup_eligible, backup_state, attestation_type, nickname,
			       webauthn_credential_json, created_at, updated_at, last_used_at
			FROM webauthn_credentials
			WHERE user_id = ?
			ORDER BY created_at ASC
		`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		creds, err := scanPasskeyCredentials(rows)
		if err != nil {
			return err
		}
		items = creds
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("list passkey credentials: %w", err)
	}
	return items, nil
}

func (s *AuthStore) GetUserByCredentialID(ctx context.Context, credentialID []byte) (domain.User, error) {
	return s.getUser(ctx, `
		SELECT u.id, u.email, u.password_hash, u.role, u.disabled, u.mfa_enabled, u.mfa_secret
		FROM users u
		INNER JOIN webauthn_credentials c ON c.user_id = u.id
		WHERE c.credential_id = ?
	`, credentialID)
}

func (s *AuthStore) RenamePasskeyCredential(ctx context.Context, userID, credentialID, nickname string) (bool, error) {
	var updated bool
	err := s.retry.ExecRetry(ctx, func() error {
		res, err := s.db.ExecContext(ctx, `
			UPDATE webauthn_credentials
			SET nickname = ?, updated_at = ?
			WHERE id = ? AND user_id = ?
		`, strings.TrimSpace(nickname), time.Now().UTC().Format(time.RFC3339Nano), credentialID, userID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		updated = n > 0
		return err
	})
	return updated, err
}

func (s *AuthStore) DeletePasskeyCredential(ctx context.Context, userID, credentialID string) (bool, error) {
	var deleted bool
	err := s.retry.TxRetry(ctx, s.db, nil, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM webauthn_credentials WHERE user_id = ?`, userID).Scan(&count); err != nil {
			return err
		}
		if count <= 1 {
			return ErrConflict
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM webauthn_credentials WHERE id = ? AND user_id = ?`, credentialID, userID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		deleted = n > 0
		return err
	})
	return deleted, err
}

func (s *AuthStore) CreateWebAuthnChallenge(ctx context.Context, challenge domain.WebAuthnChallenge) error {
	err := s.retry.ExecRetry(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO webauthn_challenges(id, user_id, type, session_json, expires_at)
			VALUES (?, NULLIF(?, ''), ?, ?, ?)
		`, challenge.ID, challenge.UserID, challenge.Type, challenge.Session, challenge.ExpiresAt.UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		return fmt.Errorf("create webauthn challenge: %w", err)
	}
	return nil
}

func (s *AuthStore) ConsumeWebAuthnChallenge(ctx context.Context, id, challengeType string, now time.Time) (domain.WebAuthnChallenge, error) {
	var item domain.WebAuthnChallenge
	err := s.retry.TxRetry(ctx, s.db, nil, func(tx *sql.Tx) error {
		var userID sql.NullString
		var expiresRaw string
		var usedRaw sql.NullString
		err := tx.QueryRowContext(ctx, `
			SELECT id, user_id, type, session_json, expires_at, used_at
			FROM webauthn_challenges
			WHERE id = ? AND type = ?
		`, id, challengeType).Scan(&item.ID, &userID, &item.Type, &item.Session, &expiresRaw, &usedRaw)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if userID.Valid {
			item.UserID = userID.String
		}
		expiresAt, err := time.Parse(time.RFC3339Nano, expiresRaw)
		if err != nil {
			return err
		}
		item.ExpiresAt = expiresAt
		if usedRaw.Valid {
			usedAt, err := time.Parse(time.RFC3339Nano, usedRaw.String)
			if err != nil {
				return err
			}
			item.UsedAt = &usedAt
		}
		if item.UsedAt != nil || !now.UTC().Before(item.ExpiresAt) {
			return ErrNotFound
		}
		res, err := tx.ExecContext(ctx, `UPDATE webauthn_challenges SET used_at = ? WHERE id = ? AND used_at IS NULL`, now.UTC().Format(time.RFC3339Nano), id)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		usedAt := now.UTC()
		item.UsedAt = &usedAt
		return nil
	})
	if err != nil {
		return domain.WebAuthnChallenge{}, fmt.Errorf("consume webauthn challenge: %w", err)
	}
	return item, nil
}

func (s *AuthStore) CreateEnrollmentInvitation(ctx context.Context, user domain.User, invite domain.EnrollmentInvitation) error {
	err := s.retry.TxRetry(ctx, s.db, nil, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO users(id, email, password_hash, role, disabled)
			VALUES (?, ?, '', ?, 1)
		`, user.ID, strings.ToLower(strings.TrimSpace(user.Email)), string(user.Role)); err != nil {
			if isUserEmailConflict(err) {
				return ErrConflict
			}
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user_enrollment_invitations(id, user_id, token_hash, expires_at, created_by)
			VALUES (?, ?, ?, ?, ?)
		`, invite.ID, user.ID, invite.TokenHash, invite.ExpiresAt.UTC().Format(time.RFC3339Nano), invite.CreatedBy)
		return err
	})
	if err != nil {
		return fmt.Errorf("create enrollment invitation: %w", err)
	}
	return nil
}

func (s *AuthStore) GetEnrollmentInvitationByTokenHash(ctx context.Context, tokenHash string, now time.Time) (domain.EnrollmentInvitation, domain.User, error) {
	var invite domain.EnrollmentInvitation
	var user domain.User
	var expiresRaw string
	var usedRaw sql.NullString
	var role string
	var disabled int
	err := s.retry.QueryRetry(ctx, func() error {
		return s.db.QueryRowContext(ctx, `
			SELECT i.id, i.user_id, i.token_hash, i.expires_at, i.used_at, i.created_by, i.created_at,
			       u.id, u.email, u.password_hash, u.role, u.disabled, u.mfa_enabled, u.mfa_secret
			FROM user_enrollment_invitations i
			INNER JOIN users u ON u.id = i.user_id
			WHERE i.token_hash = ?
		`, tokenHash).Scan(&invite.ID, &invite.UserID, &invite.TokenHash, &expiresRaw, &usedRaw, &invite.CreatedBy, &invite.CreatedAt,
			&user.ID, &user.Email, &user.PasswordHash, &role, &disabled, &user.MFAEnabled, &user.MFASecret)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.EnrollmentInvitation{}, domain.User{}, ErrNotFound
		}
		return domain.EnrollmentInvitation{}, domain.User{}, fmt.Errorf("get enrollment invitation: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expiresRaw)
	if err != nil {
		return domain.EnrollmentInvitation{}, domain.User{}, err
	}
	invite.ExpiresAt = expiresAt
	if usedRaw.Valid {
		usedAt, err := time.Parse(time.RFC3339Nano, usedRaw.String)
		if err != nil {
			return domain.EnrollmentInvitation{}, domain.User{}, err
		}
		invite.UsedAt = &usedAt
	}
	if invite.UsedAt != nil || !now.UTC().Before(invite.ExpiresAt) {
		return domain.EnrollmentInvitation{}, domain.User{}, ErrNotFound
	}
	user.Role = domain.Role(role)
	user.Disabled = disabled == 1
	return invite, user, nil
}

func (s *AuthStore) CompleteEnrollmentInvitation(ctx context.Context, inviteID string, credential domain.PasskeyCredential) error {
	err := s.retry.TxRetry(ctx, s.db, nil, func(tx *sql.Tx) error {
		var userID string
		err := tx.QueryRowContext(ctx, `
			SELECT user_id FROM user_enrollment_invitations
			WHERE id = ? AND used_at IS NULL
		`, inviteID).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if credential.UserID != userID {
			return ErrNotFound
		}
		if err := insertPasskeyCredential(ctx, tx, credential); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE users SET disabled = 0, updated_at = ? WHERE id = ?`, now, userID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE user_enrollment_invitations SET used_at = ? WHERE id = ?`, now, inviteID)
		return err
	})
	if err != nil {
		return fmt.Errorf("complete enrollment invitation: %w", err)
	}
	return nil
}

func insertPasskeyCredential(ctx context.Context, tx *sql.Tx, credential domain.PasskeyCredential) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO webauthn_credentials(
			id, user_id, credential_id, public_key, sign_count, transports,
			backup_eligible, backup_state, attestation_type, nickname, webauthn_credential_json
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, credential.ID, credential.UserID, credential.CredentialID, credential.PublicKey,
		credential.SignCount, strings.Join(credential.Transports, ","),
		boolToInt(credential.BackupEligible), boolToInt(credential.BackupState),
		credential.AttestationType, credential.Nickname, credential.WebAuthnCredential)
	return err
}

func scanPasskeyCredentials(rows *sql.Rows) ([]domain.PasskeyCredential, error) {
	var items []domain.PasskeyCredential
	for rows.Next() {
		var item domain.PasskeyCredential
		var transports string
		var backupEligible int
		var backupState int
		var createdRaw string
		var updatedRaw string
		var lastUsedRaw sql.NullString
		if err := rows.Scan(&item.ID, &item.UserID, &item.CredentialID, &item.PublicKey, &item.SignCount,
			&transports, &backupEligible, &backupState, &item.AttestationType, &item.Nickname,
			&item.WebAuthnCredential, &createdRaw, &updatedRaw, &lastUsedRaw); err != nil {
			return nil, err
		}
		item.Transports = splitCSV(transports)
		item.BackupEligible = backupEligible == 1
		item.BackupState = backupState == 1
		createdAt, err := time.Parse(time.RFC3339Nano, createdRaw)
		if err == nil {
			item.CreatedAt = createdAt
		}
		updatedAt, err := time.Parse(time.RFC3339Nano, updatedRaw)
		if err == nil {
			item.UpdatedAt = updatedAt
		}
		if lastUsedRaw.Valid {
			lastUsedAt, err := time.Parse(time.RFC3339Nano, lastUsedRaw.String)
			if err != nil {
				return nil, err
			}
			item.LastUsedAt = &lastUsedAt
		}
		items = append(items, item)
	}
	return items, nil
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if entry := strings.TrimSpace(part); entry != "" {
			items = append(items, entry)
		}
	}
	return items
}

func nullablePasskeyTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

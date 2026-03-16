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
	"fmt"
	"strings"

	"kcnotes/internal/domain"
)

type AuditListFilter struct {
	Query    string
	Action   string
	Page     int
	PageSize int
}

// ListAuditEvents explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) ListAuditEvents(ctx context.Context, filter AuditListFilter) ([]domain.AuditEvent, int, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 || pageSize > 200 {
		pageSize = 25
	}
	offset := (page - 1) * pageSize

	where, args := buildAuditFilterWhere(filter)
	countQuery := "SELECT COUNT(1) FROM audit_log a LEFT JOIN users u ON u.id = a.actor_user_id " + where
	listQuery := `
		SELECT a.id, IFNULL(a.actor_user_id, ''), IFNULL(u.email, ''), a.action, a.entity_type, IFNULL(a.entity_id, ''), IFNULL(a.ip, ''), IFNULL(a.user_agent, ''), a.created_at
		FROM audit_log a
		LEFT JOIN users u ON u.id = a.actor_user_id
	` + where + `
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT ? OFFSET ?`

	var (
		total  int
		events []domain.AuditEvent
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

		items := make([]domain.AuditEvent, 0, pageSize)
		for rows.Next() {
			event, scanErr := scanAuditEvent(rows)
			if scanErr != nil {
				return scanErr
			}
			items = append(items, event)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		events = items
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list audit events: %w", err)
	}

	return events, total, nil
}

// buildAuditFilterWhere explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func buildAuditFilterWhere(filter AuditListFilter) (string, []any) {
	clauses := []string{"1=1"}
	args := make([]any, 0, 6)

	action := strings.TrimSpace(filter.Action)
	if action != "" {
		clauses = append(clauses, "a.action = ?")
		args = append(args, action)
	}

	q := strings.TrimSpace(filter.Query)
	if q != "" {
		like := "%" + q + "%"
		clauses = append(clauses, "(u.email LIKE ? OR a.action LIKE ? OR a.entity_type LIKE ? OR IFNULL(a.entity_id, '') LIKE ? OR IFNULL(a.ip, '') LIKE ?)")
		args = append(args, like, like, like, like, like)
	}

	return "WHERE " + strings.Join(clauses, " AND "), args
}

// scanAuditEvent explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func scanAuditEvent(scanner interface{ Scan(dest ...any) error }) (domain.AuditEvent, error) {
	var (
		event        domain.AuditEvent
		createdAtRaw string
	)
	if err := scanner.Scan(
		&event.ID,
		&event.ActorUserID,
		&event.ActorEmail,
		&event.Action,
		&event.EntityType,
		&event.EntityID,
		&event.IP,
		&event.UserAgent,
		&createdAtRaw,
	); err != nil {
		return domain.AuditEvent{}, fmt.Errorf("scan audit event: %w", err)
	}
	createdAt, err := parseSQLiteTime(createdAtRaw)
	if err != nil {
		return domain.AuditEvent{}, fmt.Errorf("parse audit created_at: %w", err)
	}
	event.CreatedAt = createdAt
	return event, nil
}

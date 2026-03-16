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
	"regexp"
	"sort"
	"strings"
)

var settingKeyPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

// GetSettings explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) GetSettings(ctx context.Context, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}
	unique := make(map[string]struct{}, len(keys))
	normalized := make([]string, 0, len(keys))
	for _, k := range keys {
		key := strings.TrimSpace(strings.ToLower(k))
		if key == "" {
			continue
		}
		if _, exists := unique[key]; exists {
			continue
		}
		unique[key] = struct{}{}
		normalized = append(normalized, key)
	}
	if len(normalized) == 0 {
		return map[string]string{}, nil
	}
	sort.Strings(normalized)

	placeholders := make([]string, len(normalized))
	args := make([]any, len(normalized))
	for i, key := range normalized {
		placeholders[i] = "?"
		args[i] = key
	}

	settings := make(map[string]string, len(normalized))
	err := s.retry.QueryRetry(ctx, func() error {
		rows, err := s.db.QueryContext(ctx, `
			SELECT key, value
			FROM settings
			WHERE key IN (`+strings.Join(placeholders, ",")+`)`,
			args...,
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key, value string
			if err := rows.Scan(&key, &value); err != nil {
				return err
			}
			settings[key] = value
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("get settings: %w", err)
	}
	return settings, nil
}

// UpsertSettings explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s *AuthStore) UpsertSettings(ctx context.Context, entries map[string]string) error {
	if len(entries) == 0 {
		return nil
	}
	normalizedEntries := make(map[string]string, len(entries))
	for key, value := range entries {
		normalizedKey := strings.TrimSpace(strings.ToLower(key))
		if normalizedKey == "" {
			continue
		}
		normalizedEntries[normalizedKey] = value
	}
	keys := make([]string, 0, len(normalizedEntries))
	for key := range normalizedEntries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return s.retry.TxRetry(ctx, s.db, nil, func(tx *sql.Tx) error {
		for _, key := range keys {
			if !settingKeyPattern.MatchString(key) {
				return fmt.Errorf("invalid settings key %q", key)
			}
			value := normalizedEntries[key]
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO settings(key, value)
				VALUES (?, ?)
				ON CONFLICT(key) DO UPDATE SET
					value = excluded.value
			`, key, value); err != nil {
				return err
			}
		}
		return nil
	})
}

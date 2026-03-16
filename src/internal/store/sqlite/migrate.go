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
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const migrationsDir = "internal/store/sqlite/migrations"
const localMigrationsDir = "migrations"

// RunMigrations explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func RunMigrations(ctx context.Context, db *sql.DB) error {
	retryer := NewRetryer(RetryConfig{Jitter: 30 * time.Millisecond}, &RetryMetrics{})

	if err := retryer.ExecRetry(ctx, func() error {
		_, err := db.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS schema_migrations (
				version TEXT PRIMARY KEY,
				applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
			)
		`)
		return err
	}); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	dir, err := resolveMigrationsDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	files := make([]fs.DirEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

	for _, f := range files {
		version := strings.TrimSuffix(f.Name(), ".sql")
		applied, err := isApplied(ctx, db, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		path := filepath.Join(dir, f.Name())
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", f.Name(), err)
		}
		if err := retryer.TxRetry(ctx, db, nil, func(tx *sql.Tx) error {
			if err := execSQLScript(ctx, tx, string(sqlBytes)); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES (?)`, version); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: schema_migrations.version") {
					return nil
				}
				return err
			}
			return nil
		}); err != nil {
			return fmt.Errorf("apply migration %s: %w", f.Name(), err)
		}
	}

	return nil
}

// resolveMigrationsDir explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func resolveMigrationsDir() (string, error) {
	if _, err := os.Stat(migrationsDir); err == nil {
		return migrationsDir, nil
	}
	if _, err := os.Stat(localMigrationsDir); err == nil {
		return localMigrationsDir, nil
	}
	return "", fmt.Errorf("read migrations dir: open %s: no such file or directory", migrationsDir)
}

// isApplied explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func isApplied(ctx context.Context, db *sql.DB, version string) (bool, error) {
	var count int
	retryer := NewRetryer(RetryConfig{Jitter: 30 * time.Millisecond}, &RetryMetrics{})
	if err := retryer.QueryRetry(ctx, func() error {
		return db.QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, version).Scan(&count)
	}); err != nil {
		return false, fmt.Errorf("check migration %s: %w", version, err)
	}
	return count > 0, nil
}

// execSQLScript explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func execSQLScript(ctx context.Context, tx *sql.Tx, script string) error {
	statements := splitSQLStatements(script)
	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// splitSQLStatements explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func splitSQLStatements(script string) []string {
	lines := strings.Split(script, "\n")
	stmts := make([]string, 0, 16)
	var b strings.Builder
	inTrigger := false

	flush := func() {
		stmt := strings.TrimSpace(b.String())
		if stmt != "" {
			stmts = append(stmts, stmt)
		}
		b.Reset()
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") || trimmed == "" {
			continue
		}

		b.WriteString(line)
		b.WriteString("\n")

		lower := strings.ToLower(trimmed)
		if !inTrigger && strings.HasPrefix(lower, "create trigger") {
			inTrigger = true
		}

		if inTrigger {
			if strings.EqualFold(trimmed, "end;") || strings.HasSuffix(strings.ToLower(trimmed), " end;") {
				inTrigger = false
				flush()
			}
			continue
		}

		if strings.HasSuffix(trimmed, ";") {
			flush()
		}
	}

	flush()
	return stmts
}

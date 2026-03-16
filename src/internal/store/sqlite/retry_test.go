// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestRetryerExecRetryRetriesTransientAndUpdatesMetrics explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestRetryerExecRetryRetriesTransientAndUpdatesMetrics(t *testing.T) {
	t.Parallel()
	metrics := &RetryMetrics{}
	retryer := NewRetryer(RetryConfig{
		MaxAttempts: 4,
		BaseBackoff: time.Millisecond,
		MaxBackoff:  time.Millisecond,
		MaxElapsed:  time.Second,
	}, metrics)

	attempts := 0
	err := retryer.ExecRetry(context.Background(), func() error {
		attempts++
		if attempts < 3 {
			return errors.New("database is locked")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	snapshot := metrics.Snapshot()
	if snapshot.ExecRetries != 2 {
		t.Fatalf("expected 2 retries, got %d", snapshot.ExecRetries)
	}
	if snapshot.RetryFailures != 0 {
		t.Fatalf("expected 0 retry failures, got %d", snapshot.RetryFailures)
	}
}

// TestRetryerExecRetryStopsOnPermanentError explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestRetryerExecRetryStopsOnPermanentError(t *testing.T) {
	t.Parallel()
	metrics := &RetryMetrics{}
	retryer := NewRetryer(RetryConfig{
		MaxAttempts: 4,
		BaseBackoff: time.Millisecond,
		MaxBackoff:  time.Millisecond,
		MaxElapsed:  time.Second,
	}, metrics)

	attempts := 0
	permanent := errors.New("permanent failure")
	err := retryer.ExecRetry(context.Background(), func() error {
		attempts++
		return permanent
	})
	if !errors.Is(err, permanent) {
		t.Fatalf("expected permanent error, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected a single attempt, got %d", attempts)
	}
	snapshot := metrics.Snapshot()
	if snapshot.ExecRetries != 0 {
		t.Fatalf("expected 0 retries, got %d", snapshot.ExecRetries)
	}
	if snapshot.RetryFailures != 0 {
		t.Fatalf("expected 0 retry failures, got %d", snapshot.RetryFailures)
	}
}

// TestRetryerTxRetryRetriesTransientFnError explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestRetryerTxRetryRetriesTransientFnError(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)

	metrics := &RetryMetrics{}
	retryer := NewRetryer(RetryConfig{
		MaxAttempts: 4,
		BaseBackoff: time.Millisecond,
		MaxBackoff:  time.Millisecond,
		MaxElapsed:  time.Second,
	}, metrics)

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE tx_retry_test(id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	attempts := 0
	err := retryer.TxRetry(ctx, db, nil, func(tx *sql.Tx) error {
		attempts++
		if attempts == 1 {
			return errors.New("database is busy")
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO tx_retry_test(id) VALUES ('ok')`)
		return err
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM tx_retry_test`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one row, got %d", count)
	}
	snapshot := metrics.Snapshot()
	if snapshot.TxRetries != 1 {
		t.Fatalf("expected 1 tx retry, got %d", snapshot.TxRetries)
	}
}

// TestRetryerRecordsRetryFailureWhenTransientExhausted explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestRetryerRecordsRetryFailureWhenTransientExhausted(t *testing.T) {
	t.Parallel()
	metrics := &RetryMetrics{}
	retryer := NewRetryer(RetryConfig{
		MaxAttempts: 2,
		BaseBackoff: time.Millisecond,
		MaxBackoff:  time.Millisecond,
		MaxElapsed:  time.Second,
	}, metrics)

	err := retryer.QueryRetry(context.Background(), func() error {
		return errors.New("database is locked")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	snapshot := metrics.Snapshot()
	if snapshot.QueryRetries != 1 {
		t.Fatalf("expected 1 query retry, got %d", snapshot.QueryRetries)
	}
	if snapshot.RetryFailures != 1 {
		t.Fatalf("expected 1 retry failure, got %d", snapshot.RetryFailures)
	}
}

// openTestDB explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "retry.db")
	conn, err := Open(OpenConfig{
		Mode:   DBModeLocal,
		DBPath: path,
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn.DB
}

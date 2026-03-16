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
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type RetryConfig struct {
	MaxAttempts int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	MaxElapsed  time.Duration
	Jitter      time.Duration
}

type RetryMetrics struct {
	execRetries   atomic.Uint64
	queryRetries  atomic.Uint64
	txRetries     atomic.Uint64
	retryFailures atomic.Uint64
}

type RetryMetricsSnapshot struct {
	ExecRetries   uint64
	QueryRetries  uint64
	TxRetries     uint64
	RetryFailures uint64
}

// Snapshot explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (m *RetryMetrics) Snapshot() RetryMetricsSnapshot {
	return RetryMetricsSnapshot{
		ExecRetries:   m.execRetries.Load(),
		QueryRetries:  m.queryRetries.Load(),
		TxRetries:     m.txRetries.Load(),
		RetryFailures: m.retryFailures.Load(),
	}
}

type Retryer struct {
	cfg     RetryConfig
	metrics *RetryMetrics
	rngMu   sync.Mutex
	rng     *rand.Rand
}

// NewRetryer explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func NewRetryer(cfg RetryConfig, metrics *RetryMetrics) *Retryer {
	if metrics == nil {
		metrics = &RetryMetrics{}
	}
	if cfg.MaxAttempts < 1 {
		cfg.MaxAttempts = 4
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 40 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 600 * time.Millisecond
	}
	if cfg.MaxElapsed <= 0 {
		cfg.MaxElapsed = 3 * time.Second
	}
	if cfg.Jitter < 0 {
		cfg.Jitter = 0
	}
	return &Retryer{
		cfg:     cfg,
		metrics: metrics,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ExecRetry explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (r *Retryer) ExecRetry(ctx context.Context, fn func() error) error {
	return r.retry(ctx, fn, &r.metrics.execRetries)
}

// QueryRetry explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (r *Retryer) QueryRetry(ctx context.Context, fn func() error) error {
	return r.retry(ctx, fn, &r.metrics.queryRetries)
}

// TxRetry explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (r *Retryer) TxRetry(ctx context.Context, db *sql.DB, txOpts *sql.TxOptions, fn func(*sql.Tx) error) error {
	return r.retry(ctx, func() error {
		tx, err := db.BeginTx(ctx, txOpts)
		if err != nil {
			return err
		}
		if err := fn(tx); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}, &r.metrics.txRetries)
}

// retry explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (r *Retryer) retry(ctx context.Context, fn func() error, counter *atomic.Uint64) error {
	start := time.Now()
	attempt := 0
	for {
		attempt++
		err := fn()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil || !isTransientRetryError(err) || attempt >= r.cfg.MaxAttempts || time.Since(start) >= r.cfg.MaxElapsed {
			if isTransientRetryError(err) {
				r.metrics.retryFailures.Add(1)
			}
			return err
		}

		counter.Add(1)
		wait := r.backoff(attempt)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// backoff explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (r *Retryer) backoff(attempt int) time.Duration {
	wait := r.cfg.BaseBackoff << (attempt - 1)
	if wait > r.cfg.MaxBackoff {
		wait = r.cfg.MaxBackoff
	}
	if r.cfg.Jitter <= 0 {
		return wait
	}
	r.rngMu.Lock()
	jitter := time.Duration(r.rng.Int63n(int64(r.cfg.Jitter) + 1))
	r.rngMu.Unlock()
	return wait + jitter
}

// isTransientRetryError explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func isTransientRetryError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	transientSubstrings := []string{
		"database is locked",
		"database is busy",
		"sqlite_busy",
		"sqlite_locked",
		"temporarily unavailable",
		"connection reset",
		"connection refused",
		"broken pipe",
		"timeout",
	}
	for _, token := range transientSubstrings {
		if strings.Contains(msg, token) {
			return true
		}
	}
	return false
}

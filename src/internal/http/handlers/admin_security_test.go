// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package handlers

import (
	"testing"
	"time"

	"kcnotes/internal/observability"
)

// TestLoginFailureTrackerLocksAfterThreshold explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestLoginFailureTrackerLocksAfterThreshold(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	tracker := newLoginFailureTracker(3, 10*time.Minute, 15*time.Minute)
	tracker.now = func() time.Time { return now }

	key := "admin@example.com"
	tracker.RecordFailure(key)
	tracker.RecordFailure(key)
	if tracker.IsLocked(key) {
		t.Fatalf("expected key unlocked before threshold")
	}

	if locked := tracker.RecordFailure(key); !locked {
		t.Fatalf("expected threshold failure to report a new lockout")
	}
	if !tracker.IsLocked(key) {
		t.Fatalf("expected key locked at threshold")
	}

	now = now.Add(16 * time.Minute)
	if tracker.IsLocked(key) {
		t.Fatalf("expected lock to expire")
	}
}

func TestAdminRecordsLoginFailureMetrics(t *testing.T) {
	t.Parallel()

	metrics := observability.NewMetrics()
	admin := &Admin{
		loginFailures: newLoginFailureTracker(2, time.Minute, time.Minute),
		metrics:       metrics,
	}

	admin.recordPasskeyLoginFailure("10.0.0.1")
	admin.recordPasskeyLoginFailure("10.0.0.1")

	snapshot := metrics.Snapshot()
	if snapshot.LoginFailure != 2 {
		t.Fatalf("login failures = %d, want 2", snapshot.LoginFailure)
	}
	if snapshot.LoginLockoutTransitions != 1 {
		t.Fatalf("lockout transitions = %d, want 1", snapshot.LoginLockoutTransitions)
	}
}

// TestLoginFailureTrackerClearsOnSuccess explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestLoginFailureTrackerClearsOnSuccess(t *testing.T) {
	t.Parallel()

	tracker := newLoginFailureTracker(2, 10*time.Minute, 15*time.Minute)
	key := "admin@example.com"
	tracker.RecordFailure(key)
	tracker.RecordFailure(key)
	if !tracker.IsLocked(key) {
		t.Fatalf("expected key locked")
	}
	tracker.RecordSuccess(key)
	if tracker.IsLocked(key) {
		t.Fatalf("expected success to clear lock state")
	}
}

// TestLoginFailureKey explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestLoginFailureKey(t *testing.T) {
	t.Parallel()

	if got := loginFailureKey("  ADMIN@EXAMPLE.COM "); got != "admin@example.com" {
		t.Fatalf("unexpected normalized key: %q", got)
	}
	if got := loginFailureKey("  "); got != "_empty" {
		t.Fatalf("unexpected empty fallback key: %q", got)
	}
}

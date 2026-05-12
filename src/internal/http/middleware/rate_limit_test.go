// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kcnotes/internal/observability"
)

// TestFixedWindowLimiter explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestFixedWindowLimiter(t *testing.T) {
	l := NewFixedWindowLimiter(2, 1*time.Minute)
	if !l.Allow("key") {
		t.Fatal("first allow should pass")
	}
	if !l.Allow("key") {
		t.Fatal("second allow should pass")
	}
	if l.Allow("key") {
		t.Fatal("third allow should fail")
	}
}

// TestLoginRateLimitBlocksAccount explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestLoginRateLimitBlocksAccount(t *testing.T) {
	l := NewFixedWindowLimiter(1, 1*time.Minute)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := LoginRateLimit(l, nil, nil)(next)

	req1 := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader("email=test@example.com"))
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req1.RemoteAddr = "10.0.0.1:1234"
	res1 := httptest.NewRecorder()
	h.ServeHTTP(res1, req1)
	if res1.Code != http.StatusNoContent {
		t.Fatalf("req1 status = %d", res1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader("email=test@example.com"))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.RemoteAddr = "10.0.0.1:1234"
	res2 := httptest.NewRecorder()
	h.ServeHTTP(res2, req2)
	if res2.Code != http.StatusTooManyRequests {
		t.Fatalf("req2 status = %d", res2.Code)
	}
}

func TestRateLimitDenialsUpdateMetrics(t *testing.T) {
	metrics := observability.NewMetrics()
	loginLimiter := NewFixedWindowLimiter(1, time.Minute)
	sensitiveLimiter := NewFixedWindowLimiter(1, time.Minute)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	loginHandler := LoginRateLimit(loginLimiter, nil, metrics)(next)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader("email=test@example.com"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "10.0.0.1:1234"
		loginHandler.ServeHTTP(httptest.NewRecorder(), req)
	}

	sensitiveHandler := SensitiveRateLimit(sensitiveLimiter, nil, metrics)(next)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/admin/users", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		sensitiveHandler.ServeHTTP(httptest.NewRecorder(), req)
	}

	snapshot := metrics.Snapshot()
	if snapshot.RateLimitLoginIP != 1 {
		t.Fatalf("login IP denials = %d, want 1", snapshot.RateLimitLoginIP)
	}
	if snapshot.RateLimitSensitive != 1 {
		t.Fatalf("sensitive denials = %d, want 1", snapshot.RateLimitSensitive)
	}
}

// TestFixedWindowLimiterPrunesExpiredEntries explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestFixedWindowLimiterPrunesExpiredEntries(t *testing.T) {
	base := time.Date(2026, 2, 10, 8, 0, 0, 0, time.UTC)
	l := NewFixedWindowLimiter(1, 1*time.Minute)
	l.nowFunc = func() time.Time { return base }

	if !l.Allow("one") || !l.Allow("two") {
		t.Fatal("initial keys should be accepted")
	}
	if got := len(l.entries); got != 2 {
		t.Fatalf("entries = %d, want 2", got)
	}

	l.nowFunc = func() time.Time { return base.Add(2 * time.Minute) }
	if !l.Allow("three") {
		t.Fatal("new key after expiry should be accepted")
	}
	if got := len(l.entries); got != 1 {
		t.Fatalf("entries = %d, want 1 after prune", got)
	}
}

// TestFixedWindowLimiterMaxKeysBound explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestFixedWindowLimiterMaxKeysBound(t *testing.T) {
	l := NewFixedWindowLimiter(1, 1*time.Minute)
	l.maxKeys = 2

	if !l.Allow("one") || !l.Allow("two") {
		t.Fatal("expected first two keys to pass")
	}
	if l.Allow("three") {
		t.Fatal("expected third unique key to be rejected at cap")
	}
}

// TEACHING NOTES:
// Middleware wraps handlers to apply cross-cutting behavior (auth, CSRF, etc.).
// In Go, middleware is usually implemented as higher-order functions:
// `func(next http.Handler) http.Handler`.
// Useful Go concepts to notice:
// 1. Wrapping order matters and affects security behavior.
// 2. Request context can carry values down the chain safely.
// 3. Keep middleware focused on one concern for composability.
// 4. Tests should validate both allow and deny paths.
package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"kcnotes/internal/observability"
)

type FixedWindowLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	nowFunc func() time.Time
	entries map[string]limitEntry
	maxKeys int
	lastGC  time.Time
}

type limitEntry struct {
	count     int
	resetTime time.Time
}

// NewFixedWindowLimiter explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func NewFixedWindowLimiter(limit int, window time.Duration) *FixedWindowLimiter {
	return &FixedWindowLimiter{
		limit:   limit,
		window:  window,
		nowFunc: time.Now,
		entries: make(map[string]limitEntry),
		maxKeys: 10000,
	}
}

// Allow explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (l *FixedWindowLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.nowFunc()
	if l.lastGC.IsZero() || now.Sub(l.lastGC) >= l.window {
		l.pruneExpired(now)
		l.lastGC = now
	}

	entry, found := l.entries[key]
	if !found || now.After(entry.resetTime) {
		if !found && len(l.entries) >= l.maxKeys {
			return false
		}
		l.entries[key] = limitEntry{
			count:     1,
			resetTime: now.Add(l.window),
		}
		return true
	}
	if entry.count >= l.limit {
		return false
	}
	entry.count++
	l.entries[key] = entry
	return true
}

// pruneExpired explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (l *FixedWindowLimiter) pruneExpired(now time.Time) {
	for key, entry := range l.entries {
		if now.After(entry.resetTime) {
			delete(l.entries, key)
		}
	}
}

// LoginRateLimit explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func LoginRateLimit(limiter *FixedWindowLimiter, ipResolver *ClientIPResolver, metrics *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ipResolver.ClientIP(r)
			if !limiter.Allow("login-ip:" + ip) {
				if metrics != nil {
					metrics.RecordRateLimitDenied(observability.RateLimitLoginIP)
				}
				WriteError(w, r, http.StatusTooManyRequests, "too many login attempts")
				return
			}
			email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
			if email != "" && !limiter.Allow("login-account:"+email) {
				if metrics != nil {
					metrics.RecordRateLimitDenied(observability.RateLimitLoginAccount)
				}
				WriteError(w, r, http.StatusTooManyRequests, "too many login attempts")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SensitiveRateLimit explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func SensitiveRateLimit(limiter *FixedWindowLimiter, ipResolver *ClientIPResolver, metrics *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := "sensitive:" + r.Method + ":" + r.URL.Path + ":" + ipResolver.ClientIP(r)
			if !limiter.Allow(key) {
				if metrics != nil {
					metrics.RecordRateLimitDenied(observability.RateLimitSensitive)
				}
				WriteError(w, r, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

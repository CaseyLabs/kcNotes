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
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

// RequestID explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			var err error
			id, err = randomID()
			if err != nil {
				WriteError(w, r, http.StatusServiceUnavailable, "request id unavailable")
				return
			}
		}
		w.Header().Set("X-Request-Id", id)
		r = r.WithContext(context.WithValue(r.Context(), RequestIDKey, id))
		next.ServeHTTP(w, r)
	})
}

// randomID explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

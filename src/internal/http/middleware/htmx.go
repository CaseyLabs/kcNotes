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
	"net/http"
)

type htmxContextKey string

const HTMXRequestKey htmxContextKey = "is_htmx"

// HTMX explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func HTMX(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isHTMX := r.Header.Get("HX-Request") == "true"
		ctx := context.WithValue(r.Context(), HTMXRequestKey, isHTMX)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// IsHTMX explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func IsHTMX(r *http.Request) bool {
	v, ok := r.Context().Value(HTMXRequestKey).(bool)
	return ok && v
}

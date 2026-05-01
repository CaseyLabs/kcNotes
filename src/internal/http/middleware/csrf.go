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

type csrfContextKey string

const csrfTokenKey csrfContextKey = "csrf_token"

type CSRFConfig struct {
	CookieName string
	CookiePath string
	Secure     bool
}

// CSRF explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func CSRF(cfg CSRFConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := ensureCSRFCookie(w, r, cfg)
			if err != nil {
				WriteError(w, r, http.StatusServiceUnavailable, "csrf token unavailable")
				return
			}
			ctx := context.WithValue(r.Context(), csrfTokenKey, token)
			r = r.WithContext(ctx)

			if isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			headerToken := r.Header.Get("X-CSRF-Token")
			if headerToken == "" {
				headerToken = r.Header.Get("X-CSRFToken")
			}
			formToken := ""
			if headerToken == "" {
				formToken = r.PostFormValue("_csrf")
			}
			if sessionToken, ok := SessionCSRFToken(r); ok {
				if formToken != sessionToken && headerToken != sessionToken {
					WriteError(w, r, http.StatusForbidden, "csrf token invalid")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if formToken != token && headerToken != token {
				WriteError(w, r, http.StatusForbidden, "csrf token invalid")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CSRFToken explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func CSRFToken(r *http.Request) string {
	token, _ := r.Context().Value(csrfTokenKey).(string)
	return token
}

// ensureCSRFCookie explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func ensureCSRFCookie(w http.ResponseWriter, r *http.Request, cfg CSRFConfig) (string, error) {
	cookie, err := r.Cookie(cfg.CookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value, nil
	}
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.CookieName,
		Value:    token,
		Path:     cfg.CookiePath,
		HttpOnly: false,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	return token, nil
}

// isSafeMethod explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions || method == http.MethodTrace
}

// randomToken explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

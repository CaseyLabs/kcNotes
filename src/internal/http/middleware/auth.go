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
	"errors"
	"net/http"
	"time"

	"kcnotes/internal/domain"
	sqlitestore "kcnotes/internal/store/sqlite"
)

type authContextKey string

const currentUserKey authContextKey = "current_user"
const sessionCSRFTokenKey authContextKey = "session_csrf_token"

type SessionLookup interface {
	GetSessionUser(ctx context.Context, sessionID string) (domain.SessionUser, error)
	DeleteSession(ctx context.Context, sessionID string) error
}

type SessionConfig struct {
	CookieName string
	CookiePath string
	Secure     bool
}

// SessionLoader explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func SessionLoader(store SessionLookup, cfg SessionConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cfg.CookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}

			session, err := store.GetSessionUser(r.Context(), cookie.Value)
			if err != nil {
				if errors.Is(err, sqlitestore.ErrNotFound) {
					clearSessionCookie(w, cfg)
					next.ServeHTTP(w, r)
					return
				}
				WriteError(w, r, http.StatusInternalServerError, "internal server error")
				return
			}

			if session.ExpiresAt.Before(time.Now().UTC()) || session.User.Disabled {
				_ = store.DeleteSession(r.Context(), session.SessionID)
				clearSessionCookie(w, cfg)
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), currentUserKey, session.User)
			ctx = context.WithValue(ctx, sessionCSRFTokenKey, session.CSRFToken)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuth explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func RequireAuth(loginPath string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := CurrentUser(r); ok {
				next.ServeHTTP(w, r)
				return
			}

			if IsHTMX(r) {
				w.Header().Set("HX-Redirect", loginPath)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, loginPath, http.StatusSeeOther)
		})
	}
}

// RequireRoles explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func RequireRoles(roles ...domain.Role) func(http.Handler) http.Handler {
	allowed := map[domain.Role]struct{}{}
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := CurrentUser(r)
			if !ok {
				WriteError(w, r, http.StatusUnauthorized, "unauthorized")
				return
			}
			if _, allowedRole := allowed[user.Role]; !allowedRole {
				WriteError(w, r, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CurrentUser explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func CurrentUser(r *http.Request) (domain.User, bool) {
	user, ok := r.Context().Value(currentUserKey).(domain.User)
	return user, ok
}

// SessionCSRFToken explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func SessionCSRFToken(r *http.Request) (string, bool) {
	token, ok := r.Context().Value(sessionCSRFTokenKey).(string)
	return token, ok && token != ""
}

// clearSessionCookie explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func clearSessionCookie(w http.ResponseWriter, cfg SessionConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.CookieName,
		Value:    "",
		Path:     cfg.CookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

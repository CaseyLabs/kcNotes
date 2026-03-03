// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kcnotes/internal/domain"
	sqlitestore "kcnotes/internal/store/sqlite"
)

type fakeSessionStore struct {
	session domain.SessionUser
	err     error
}

// GetSessionUser explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (f *fakeSessionStore) GetSessionUser(_ context.Context, _ string) (domain.SessionUser, error) {
	if f.err != nil {
		return domain.SessionUser{}, f.err
	}
	return f.session, nil
}

// DeleteSession explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (f *fakeSessionStore) DeleteSession(_ context.Context, _ string) error { return nil }

// TestSessionLoaderSetsCurrentUser explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestSessionLoaderSetsCurrentUser(t *testing.T) {
	store := &fakeSessionStore{session: domain.SessionUser{
		SessionID: "s1",
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour),
		User:      domain.User{ID: "u1", Email: "a@example.com", Role: domain.RoleAdmin},
	}}
	h := SessionLoader(store, SessionConfig{CookieName: "cms_session", CookiePath: "/admin"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := CurrentUser(r)
		if !ok || u.ID != "u1" {
			t.Fatalf("expected current user in context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: "cms_session", Value: "s1"})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d", res.Code)
	}
}

// TestRequireAuthRedirectsWhenMissing explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestRequireAuthRedirectsWhenMissing(t *testing.T) {
	h := RequireAuth("/admin/login")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", res.Code)
	}
	if got := res.Header().Get("Location"); got != "/admin/login" {
		t.Fatalf("location = %q", got)
	}
}

// TestRequireRolesForbidsNonAdmin explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestRequireRolesForbidsNonAdmin(t *testing.T) {
	h := RequireRoles(domain.RoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := context.WithValue(context.Background(), currentUserKey, domain.User{ID: "u2", Role: domain.RoleAuthor})
	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil).WithContext(ctx)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d", res.Code)
	}
}

// TestSessionLoaderHandlesMissingSession explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestSessionLoaderHandlesMissingSession(t *testing.T) {
	store := &fakeSessionStore{err: sqlitestore.ErrNotFound}
	h := SessionLoader(store, SessionConfig{CookieName: "cms_session", CookiePath: "/admin"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: "cms_session", Value: "missing"})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d", res.Code)
	}
	if len(res.Result().Cookies()) == 0 {
		t.Fatal("expected cookie clear")
	}
}

// TestSessionLoaderInternalError explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestSessionLoaderInternalError(t *testing.T) {
	store := &fakeSessionStore{err: errors.New("db down")}
	h := SessionLoader(store, SessionConfig{CookieName: "cms_session", CookiePath: "/admin"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: "cms_session", Value: "s1"})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", res.Code)
	}
}

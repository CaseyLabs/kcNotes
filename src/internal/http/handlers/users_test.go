// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package handlers

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"kcnotes/internal/domain"
	"kcnotes/internal/http/middleware"
	"kcnotes/internal/http/views"
	storesqlite "kcnotes/internal/store/sqlite"
)

// TestParseAndValidateCreateUserForm explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestParseAndValidateCreateUserForm(t *testing.T) {
	t.Parallel()
	values := url.Values{}
	values.Set("email", "Admin@Example.com")
	values.Set("role", "admin")

	r, _ := http.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}

	form, errs := parseAndValidateCreateUserForm(r)
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
	if form.Email != "admin@example.com" {
		t.Fatalf("expected lowercase email, got %q", form.Email)
	}
}

// TestParseAndValidateCreateUserFormRejectsInvalid explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestParseAndValidateCreateUserFormRejectsInvalid(t *testing.T) {
	t.Parallel()
	values := url.Values{}
	values.Set("email", "bad")
	values.Set("role", "bad")

	r, _ := http.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}

	_, errs := parseAndValidateCreateUserForm(r)
	if len(errs) < 2 {
		t.Fatalf("expected validation errors, got %v", errs)
	}
}

func TestCreateUserRendersCopyableEnrollmentLink(t *testing.T) {
	admin, store := newUsersHandlerTestAdmin(t)

	res := httptest.NewRecorder()
	req := newUsersHandlerRequest(t, "editor@example.com", "editor")
	usersHandlerStack(store, http.HandlerFunc(admin.CreateUser)).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, `data-enrollment-link`) {
		t.Fatalf("expected rendered enrollment link block, got:\n%s", body)
	}
	if !strings.Contains(body, `<code class="copy-field-value">/admin/enroll?token=`) {
		t.Fatalf("expected enrollment link as distinct code value, got:\n%s", body)
	}
	if !strings.Contains(body, `data-copy-value="/admin/enroll?token=`) {
		t.Fatalf("expected copy button with enrollment link value, got:\n%s", body)
	}
	if strings.Contains(body, "Enrollment link: /admin/enroll") {
		t.Fatalf("expected link outside the flash message, got:\n%s", body)
	}
}

func TestCreateUserValidationErrorDoesNotRenderStaleEnrollmentLink(t *testing.T) {
	admin, store := newUsersHandlerTestAdmin(t)

	res := httptest.NewRecorder()
	req := newUsersHandlerRequest(t, "bad", "bad")
	usersHandlerStack(store, http.HandlerFunc(admin.CreateUser)).ServeHTTP(res, req)

	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422, got %d: %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if strings.Contains(body, `data-enrollment-link`) || strings.Contains(body, `/admin/enroll?token=`) {
		t.Fatalf("expected no stale enrollment link after validation failure, got:\n%s", body)
	}
	if !strings.Contains(body, "Email is required and must be valid") {
		t.Fatalf("expected validation error, got:\n%s", body)
	}
}

func newUsersHandlerTestAdmin(t *testing.T) (*Admin, *storesqlite.AuthStore) {
	t.Helper()
	store := newUsersHandlerTestStore(t)
	renderer, err := views.NewRenderer("../../../web/templates/*/*.tmpl")
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}
	admin := NewAdmin(renderer, slog.New(slog.NewTextHandler(io.Discard, nil)), store, AdminConfig{
		CookieName: "cms_session",
		CookiePath: "/admin",
		SessionTTL: 24 * time.Hour,
	})
	return admin, store
}

func newUsersHandlerTestStore(t *testing.T) *storesqlite.AuthStore {
	t.Helper()
	conn, err := storesqlite.Open(storesqlite.OpenConfig{
		Mode:   storesqlite.DBModeLocal,
		DBPath: t.TempDir() + "/cms.db",
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	schema := []string{
		`CREATE TABLE users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL,
			disabled INTEGER NOT NULL DEFAULT 0,
			mfa_enabled INTEGER NOT NULL DEFAULT 0,
			mfa_secret TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			csrf_token TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE audit_log (
			id TEXT PRIMARY KEY,
			actor_user_id TEXT,
			action TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			entity_id TEXT,
			ip TEXT,
			user_agent TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(actor_user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE user_enrollment_invitations (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TEXT NOT NULL,
			used_at TEXT,
			created_by TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY(created_by) REFERENCES users(id)
		)`,
	}
	for _, stmt := range schema {
		if _, err := conn.DB.Exec(stmt); err != nil {
			t.Fatalf("create test schema: %v", err)
		}
	}

	store := storesqlite.NewAuthStore(conn.DB)
	ctx := context.Background()
	if err := store.CreateUser(ctx, domain.User{
		ID:           "admin-1",
		Email:        "admin@example.com",
		PasswordHash: "",
		Role:         domain.RoleAdmin,
	}); err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	if err := store.CreateSession(ctx, "session-1", "admin-1", "csrf-token", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return store
}

func newUsersHandlerRequest(t *testing.T, email, role string) *http.Request {
	t.Helper()
	values := url.Values{}
	values.Set("email", email)
	values.Set("role", role)
	values.Set("_csrf", "csrf-token")

	req := httptest.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: "cms_session", Value: "session-1", Path: "/admin"})
	return req
}

func usersHandlerStack(store *storesqlite.AuthStore, next http.Handler) http.Handler {
	return middleware.HTMX(middleware.SessionLoader(store, middleware.SessionConfig{
		CookieName: "cms_session",
		CookiePath: "/admin",
	})(middleware.CSRF(middleware.CSRFConfig{
		CookieName: "cms_csrf",
		CookiePath: "/admin",
	})(next)))
}

func TestEnrollmentPathFromQueryAllowsOnlyServerEnrollmentPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "relative enrollment token path",
			value: "/admin/enroll?token=abc",
			want:  "/admin/enroll?token=abc",
		},
		{
			name:  "encoded token path",
			value: "/admin/enroll?token=abc%2B123",
			want:  "/admin/enroll?token=abc%2B123",
		},
		{
			name:  "absolute external URL",
			value: "https://evil.example/admin/enroll?token=abc",
		},
		{
			name:  "scheme relative external URL",
			value: "//evil.example/admin/enroll?token=abc",
		},
		{
			name:  "wrong path",
			value: "/login?token=abc",
		},
		{
			name:  "missing token",
			value: "/admin/enroll",
		},
		{
			name:  "extra query parameter",
			value: "/admin/enroll?token=abc&next=https%3A%2F%2Fevil.example",
		},
		{
			name:  "fragment",
			value: "/admin/enroll?token=abc#copy",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := enrollmentPathFromQuery(tt.value); got != tt.want {
				t.Fatalf("enrollmentPathFromQuery(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

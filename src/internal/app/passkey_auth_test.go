package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kcnotes/internal/store/sqlite"
)

func TestPasskeyOnlySetupRoutes(t *testing.T) {
	application := newTestApp(t)
	router := application.Router()

	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/admin/setup", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("empty-db setup status = %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "Create the first admin account and passkey") {
		t.Fatalf("expected setup page to describe passkey setup")
	}

	if err := application.CreateUser(context.Background(), "pending@example.com", "editor"); err != nil {
		t.Fatalf("create disabled placeholder: %v", err)
	}

	res = httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/admin/setup", nil))
	if res.Code != http.StatusNotFound {
		t.Fatalf("non-empty-db setup status = %d", res.Code)
	}
}

func TestLegacyPasswordLoginRejected(t *testing.T) {
	application := newTestApp(t)
	router := application.Router()

	getRes := httptest.NewRecorder()
	router.ServeHTTP(getRes, httptest.NewRequest(http.MethodGet, "/admin/login", nil))
	csrfCookie := findCookie(t, getRes.Result().Cookies(), "cms_csrf")

	form := url.Values{}
	form.Set("_csrf", csrfCookie.Value)
	form.Set("email", "admin@example.com")
	form.Set("password", "not-used")
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)

	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("legacy login status = %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "Use your passkey to sign in") {
		t.Fatalf("expected passkey-only login rejection")
	}
}

func TestMFARouteUnavailable(t *testing.T) {
	application := newTestApp(t)
	router := application.Router()

	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/admin/mfa", nil))
	if res.Code != http.StatusNotFound {
		t.Fatalf("/admin/mfa status = %d", res.Code)
	}
}

func TestCreateUserCreatesDisabledPlaceholder(t *testing.T) {
	application := newTestApp(t)

	if err := application.CreateUser(context.Background(), "placeholder@example.com", "author"); err != nil {
		t.Fatalf("create user placeholder: %v", err)
	}

	store := sqlite.NewAuthStore(application.db)
	user, err := store.GetUserByEmail(context.Background(), "placeholder@example.com")
	if err != nil {
		t.Fatalf("load placeholder user: %v", err)
	}
	if !user.Disabled {
		t.Fatal("expected create-user placeholder to be disabled")
	}
	if user.PasswordHash != "" {
		t.Fatal("expected create-user placeholder to have no password hash")
	}
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	t.Chdir(filepath.Join("..", ".."))
	application, err := New(Config{
		AppEnv:                "dev",
		HTTPAddr:              ":8080",
		PreviewHTTPAddr:       ":8081",
		DBMode:                "local",
		DBPath:                filepath.Join(t.TempDir(), "cms.db"),
		UploadDir:             filepath.Join(t.TempDir(), "uploads"),
		TemplateGlob:          filepath.Join("web", "templates", "*", "*.tmpl"),
		StaticDir:             filepath.Join("web", "static"),
		PublishOutDir:         filepath.Join(t.TempDir(), "site"),
		SessionCookieName:     "cms_session",
		CSRFCookieName:        "cms_csrf",
		AdminCookiePath:       "/admin",
		SessionTTL:            24 * time.Hour,
		MediaUserQuotaBytes:   200 << 20,
		MediaTotalQuotaBytes:  2 << 30,
		LoginLockoutThreshold: 8,
		LoginLockoutWindow:    15 * time.Minute,
		LoginLockoutDuration:  15 * time.Minute,
		WebAuthnRPName:        "kcNotes",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(application.Close)
	if err := application.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return application
}

func findCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("missing cookie %s", name)
	return nil
}

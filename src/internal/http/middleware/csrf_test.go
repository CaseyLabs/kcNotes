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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCSRFMiddlewareSetsTokenAndValidates explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestCSRFMiddlewareSetsTokenAndValidates(t *testing.T) {
	cfg := CSRFConfig{CookieName: "cms_csrf", CookiePath: "/admin", Secure: false}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := HTMX(CSRF(cfg)(next))

	getReq := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	getRes := httptest.NewRecorder()
	h.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusNoContent {
		t.Fatalf("GET status = %d", getRes.Code)
	}
	cookies := getRes.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Value == "" {
		t.Fatal("expected csrf cookie")
	}
	token := cookies[0].Value

	postReq := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader("email=a@b.com&password=x&_csrf="+token))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(cookies[0])
	postRes := httptest.NewRecorder()
	h.ServeHTTP(postRes, postReq)
	if postRes.Code != http.StatusNoContent {
		t.Fatalf("POST status = %d", postRes.Code)
	}

	badReq := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader("_csrf=bad"))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badReq.AddCookie(cookies[0])
	badRes := httptest.NewRecorder()
	h.ServeHTTP(badRes, badReq)
	if badRes.Code != http.StatusForbidden {
		t.Fatalf("bad POST status = %d", badRes.Code)
	}
}

// TestCSRFMiddlewareHTMXErrorFragment explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestCSRFMiddlewareHTMXErrorFragment(t *testing.T) {
	cfg := CSRFConfig{CookieName: "cms_csrf", CookiePath: "/admin", Secure: false}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := HTMX(CSRF(cfg)(next))

	getReq := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	getRes := httptest.NewRecorder()
	h.ServeHTTP(getRes, getReq)
	cookie := getRes.Result().Cookies()[0]

	badReq := httptest.NewRequest(http.MethodPost, "/admin/posts", strings.NewReader("_csrf=bad"))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badReq.Header.Set("HX-Request", "true")
	badReq.AddCookie(cookie)
	badRes := httptest.NewRecorder()
	h.ServeHTTP(badRes, badReq)

	if badRes.Code != http.StatusForbidden {
		t.Fatalf("bad POST status = %d", badRes.Code)
	}
	if got := badRes.Header().Get("HX-Retarget"); got != "#flash" {
		t.Fatalf("expected HX-Retarget #flash, got %q", got)
	}
}

// TestCSRFMiddlewareUsesSessionTokenWhenPresent explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestCSRFMiddlewareUsesSessionTokenWhenPresent(t *testing.T) {
	cfg := CSRFConfig{CookieName: "cms_csrf", CookiePath: "/admin", Secure: false}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	base := HTMX(CSRF(cfg)(next))
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), sessionCSRFTokenKey, "session-token")
		base.ServeHTTP(w, r.WithContext(ctx))
	})

	getReq := httptest.NewRequest(http.MethodGet, "/admin", nil)
	getRes := httptest.NewRecorder()
	h.ServeHTTP(getRes, getReq)
	cookie := getRes.Result().Cookies()[0]

	badReq := httptest.NewRequest(http.MethodPost, "/admin/posts", strings.NewReader("_csrf="+cookie.Value))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badReq.AddCookie(cookie)
	badRes := httptest.NewRecorder()
	h.ServeHTTP(badRes, badReq)
	if badRes.Code != http.StatusForbidden {
		t.Fatalf("bad POST status = %d", badRes.Code)
	}

	goodReq := httptest.NewRequest(http.MethodPost, "/admin/posts", strings.NewReader("_csrf=session-token"))
	goodReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	goodReq.AddCookie(cookie)
	goodRes := httptest.NewRecorder()
	h.ServeHTTP(goodRes, goodReq)
	if goodRes.Code != http.StatusNoContent {
		t.Fatalf("good POST status = %d", goodRes.Code)
	}
}

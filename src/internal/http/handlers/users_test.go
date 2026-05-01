// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package handlers

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
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

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

// TestParseAndValidateSettingsForm explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestParseAndValidateSettingsForm(t *testing.T) {
	t.Parallel()
	values := url.Values{}
	values.Set("site_name", "Example")
	values.Set("site_base_url", "https://example.com")
	values.Set("feature_flags", "a,b,c")

	r, _ := http.NewRequest(http.MethodPost, "/admin/settings", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}

	form, errs := parseAndValidateSettingsForm(r)
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
	if form.SiteName != "Example" {
		t.Fatalf("expected site name to parse")
	}
}

// TestParseAndValidateSettingsFormRejectsEmptyName explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestParseAndValidateSettingsFormRejectsEmptyName(t *testing.T) {
	t.Parallel()
	values := url.Values{}
	values.Set("site_name", "")

	r, _ := http.NewRequest(http.MethodPost, "/admin/settings", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}

	_, errs := parseAndValidateSettingsForm(r)
	if len(errs) == 0 {
		t.Fatalf("expected validation errors")
	}
}

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

	"kcnotes/internal/domain"
)

// TestParseAndValidatePostForm explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestParseAndValidatePostForm(t *testing.T) {
	t.Parallel()
	values := url.Values{}
	values.Set("type", "post")
	values.Set("title", "Hello World")
	values.Set("slug", "")
	values.Set("body_md", "body")
	values.Set("status", "draft")

	r, _ := http.NewRequest(http.MethodPost, "/admin/posts", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}

	form, errs := parseAndValidatePostForm(r, true)
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}
	if form.Slug != "hello-world" {
		t.Fatalf("expected slugify fallback, got %q", form.Slug)
	}
}

// TestParseAndValidatePostFormRejectsInvalidSlug explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestParseAndValidatePostFormRejectsInvalidSlug(t *testing.T) {
	t.Parallel()
	values := url.Values{}
	values.Set("type", "post")
	values.Set("title", "Hello")
	values.Set("slug", "bad_slug")
	values.Set("status", "published")

	r, _ := http.NewRequest(http.MethodPost, "/admin/posts", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}

	_, errs := parseAndValidatePostForm(r, false)
	if len(errs) == 0 {
		t.Fatalf("expected validation error")
	}
}

// TestCanAccessPost explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestCanAccessPost(t *testing.T) {
	t.Parallel()
	post := domain.Post{AuthorID: "u1"}
	if canAccessPost(domain.User{ID: "u2", Role: domain.RoleAuthor}, post) {
		t.Fatalf("author should not access non-owned post")
	}
	if !canAccessPost(domain.User{ID: "u2", Role: domain.RoleEditor}, post) {
		t.Fatalf("editor should access post")
	}
}

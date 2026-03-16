// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package content

import (
	"strings"
	"testing"
)

// TestMarkdownRenderer_StripsUnsafeContent explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestMarkdownRenderer_StripsUnsafeContent(t *testing.T) {
	t.Parallel()

	r := NewMarkdownRenderer()
	got, err := r.Render(`# Hello

<script>alert("xss")</script>

[bad](javascript:alert(1))
`)
	if err != nil {
		t.Fatalf("render markdown: %v", err)
	}
	out := string(got)
	if strings.Contains(out, "<script") {
		t.Fatalf("expected script tags to be removed, got: %s", out)
	}
	if strings.Contains(strings.ToLower(out), "javascript:") {
		t.Fatalf("expected javascript URLs to be removed, got: %s", out)
	}
	if !strings.Contains(out, "<h1>Hello</h1>") {
		t.Fatalf("expected markdown header to render, got: %s", out)
	}
}

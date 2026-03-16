// TEACHING NOTES:
// This file handles content transformation (Markdown -> safe HTML).
// In Go web apps, keep rendering and sanitization responsibilities clear.
// Useful Go concepts to notice:
// 1. Return errors instead of panicking for malformed input.
// 2. Explicit types communicate trust boundaries (`string` vs `template.HTML`).
// 3. Composition via small helpers keeps renderer logic easy to reason about.
// 4. Tests validate both functional output and safety guarantees.
package content

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// MarkdownRenderer converts Markdown to sanitized HTML safe for template.HTML.
type MarkdownRenderer struct {
	parser goldmark.Markdown
	policy *bluemonday.Policy
}

// NewMarkdownRenderer explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func NewMarkdownRenderer() *MarkdownRenderer {
	return &MarkdownRenderer{
		parser: goldmark.New(
			goldmark.WithExtensions(extension.GFM),
		),
		policy: bluemonday.UGCPolicy(),
	}
}

// Render explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (r *MarkdownRenderer) Render(markdown string) (template.HTML, error) {
	var out bytes.Buffer
	if err := r.parser.Convert([]byte(markdown), &out); err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	safe := r.policy.SanitizeBytes(out.Bytes())
	return template.HTML(safe), nil
}

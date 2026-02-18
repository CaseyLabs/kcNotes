// TEACHING NOTES:
// View-layer helpers centralize template parsing/rendering behavior.
// Go templates are server-side and auto-escape by default.
// Useful Go concepts to notice:
// 1. Parse templates once, execute many times (performance + correctness).
// 2. Template funcs are explicit dependencies passed at parse time.
// 3. Keep renderer API tiny to avoid leaking transport details everywhere.
package views

import (
	"fmt"
	"html/template"
	"io"
)

type Renderer struct {
	tpl *template.Template
}

// NewRenderer explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func NewRenderer(glob string) (*Renderer, error) {
	tpl, err := template.ParseGlob(glob)
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Renderer{tpl: tpl}, nil
}

// Render explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (r *Renderer) Render(w io.Writer, name string, data any) error {
	if err := r.tpl.ExecuteTemplate(w, name, data); err != nil {
		return fmt.Errorf("execute template %s: %w", name, err)
	}
	return nil
}

// TEACHING NOTES:
// Handler files translate HTTP requests into application/store operations.
// In Go's net/http model, handlers are ordinary functions with signature:
// `func(http.ResponseWriter, *http.Request)`.
// Useful Go concepts to notice:
// 1. Request parsing/validation is separate from persistence logic.
// 2. `context.Context` from the request is passed into store calls.
// 3. Errors are mapped to HTTP status codes + HTML fragment/full-page responses.
// 4. HTMX requests are just HTTP with extra headers; server logic stays explicit.
// 5. Handlers often return early on errors to keep happy-path code readable.
package handlers

import (
	"net/http"

	"kcnotes/internal/http/middleware"
)

// renderError explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) renderError(w http.ResponseWriter, r *http.Request, status int, message string) {
	if middleware.IsHTMX(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("HX-Retarget", "#flash")
		w.Header().Set("HX-Reswap", "outerHTML")
		w.WriteHeader(status)
		if err := h.renderer.Render(w, "partial-flash-error", map[string]any{"Message": message}); err == nil {
			return
		}
	}
	http.Error(w, message, status)
}

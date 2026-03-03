// TEACHING NOTES:
// Middleware wraps handlers to apply cross-cutting behavior (auth, CSRF, etc.).
// In Go, middleware is usually implemented as higher-order functions:
// `func(next http.Handler) http.Handler`.
// Useful Go concepts to notice:
// 1. Wrapping order matters and affects security behavior.
// 2. Request context can carry values down the chain safely.
// 3. Keep middleware focused on one concern for composability.
// 4. Tests should validate both allow and deny paths.
package middleware

import (
	"html/template"
	"net/http"
)

// WriteError explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func WriteError(w http.ResponseWriter, r *http.Request, status int, message string) {
	if IsHTMX(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("HX-Retarget", "#flash")
		w.Header().Set("HX-Reswap", "outerHTML")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`<div id="flash" class="rounded border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-700">` + template.HTMLEscapeString(message) + `</div>`))
		return
	}
	http.Error(w, message, status)
}

// TEACHING NOTES:
// Route registration maps URL patterns to handlers and middleware stacks.
// In Go, routing is usually explicit code, which makes request flow discoverable.
// Useful Go concepts to notice:
// 1. Group related endpoints by responsibility (public/admin/api).
// 2. Apply middleware near route declarations for readability.
// 3. Keep route names and URL shapes stable for maintainability.
package routes

import (
	"net/http"

	"kcnotes/internal/http/handlers"
)

type MiddlewareSet struct {
	RequestID       func(http.Handler) http.Handler
	SecurityHeaders func(http.Handler) http.Handler
	HTMX            func(http.Handler) http.Handler
	RequestMetrics  func(http.Handler) http.Handler
	Session         func(http.Handler) http.Handler
	CSRF            func(http.Handler) http.Handler
	RequireAuth     func(http.Handler) http.Handler
	RequireAdmin    func(http.Handler) http.Handler
	LoginRateLimit  func(http.Handler) http.Handler
	SensitiveLimit  func(http.Handler) http.Handler
}

type Router struct {
	mux *http.ServeMux
}

// New explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func New(public *handlers.Public, admin *handlers.Admin, staticDir string, mw MiddlewareSet) *Router {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", public.Healthz)
	mux.HandleFunc("GET /{$}", public.Home)
	mux.HandleFunc("GET /p/{slug}", redirectToTrailingSlash)
	mux.HandleFunc("GET /page/{slug}", redirectToTrailingSlash)
	mux.HandleFunc("GET /p/{slug}/", public.Post)
	mux.HandleFunc("GET /page/{slug}/", public.Page)

	mux.Handle("GET /admin/setup", mw.CSRF(http.HandlerFunc(admin.SetupPage)))
	mux.Handle("POST /admin/setup/passkeys/start", mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.SetupRegistrationStart))))
	mux.Handle("POST /admin/setup/passkeys/finish", mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.SetupRegistrationFinish))))
	mux.Handle("GET /admin/enroll", mw.CSRF(http.HandlerFunc(admin.EnrollmentPage)))
	mux.Handle("POST /admin/enroll/passkeys/start", mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.EnrollmentRegistrationStart))))
	mux.Handle("POST /admin/enroll/passkeys/finish", mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.EnrollmentRegistrationFinish))))
	mux.Handle("GET /admin/login", mw.CSRF(http.HandlerFunc(admin.LoginPage)))
	mux.Handle("POST /admin/login", mw.CSRF(mw.LoginRateLimit(http.HandlerFunc(admin.Login))))
	mux.Handle("POST /admin/login/passkeys/start", mw.CSRF(mw.LoginRateLimit(http.HandlerFunc(admin.PasskeyLoginStart))))
	mux.Handle("POST /admin/login/passkeys/finish", mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.PasskeyLoginFinish))))
	mux.Handle("POST /admin/logout", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.Logout)))))
	mux.Handle("GET /admin", mw.RequireAuth(mw.CSRF(http.HandlerFunc(admin.Dashboard))))
	mux.Handle("GET /admin/passkeys", mw.RequireAuth(mw.CSRF(http.HandlerFunc(admin.PasskeysPage))))
	mux.Handle("POST /admin/passkeys/start", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.PasskeyRegistrationStart)))))
	mux.Handle("POST /admin/passkeys/finish", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.PasskeyRegistrationFinish)))))
	mux.Handle("POST /admin/passkeys/{id}/rename", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.RenamePasskey)))))
	mux.Handle("POST /admin/passkeys/{id}/delete", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.DeletePasskey)))))
	mux.Handle("GET /admin/users", mw.RequireAuth(mw.RequireAdmin(mw.CSRF(http.HandlerFunc(admin.Users)))))
	mux.Handle("GET /admin/users/table", mw.RequireAuth(mw.RequireAdmin(mw.CSRF(http.HandlerFunc(admin.UsersTable)))))
	mux.Handle("POST /admin/users", mw.RequireAuth(mw.RequireAdmin(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.CreateUser))))))
	mux.Handle("POST /admin/users/{id}/update", mw.RequireAuth(mw.RequireAdmin(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.UpdateUser))))))
	mux.Handle("GET /admin/settings", mw.RequireAuth(mw.RequireAdmin(mw.CSRF(http.HandlerFunc(admin.SettingsPage)))))
	mux.Handle("POST /admin/settings", mw.RequireAuth(mw.RequireAdmin(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.UpdateSettings))))))
	mux.Handle("GET /admin/audit", mw.RequireAuth(mw.RequireAdmin(mw.CSRF(http.HandlerFunc(admin.AuditPage)))))
	mux.Handle("GET /admin/audit/table", mw.RequireAuth(mw.RequireAdmin(mw.CSRF(http.HandlerFunc(admin.AuditTable)))))
	mux.Handle("GET /admin/posts", mw.RequireAuth(mw.CSRF(http.HandlerFunc(admin.PostsPage))))
	mux.Handle("GET /admin/posts/table", mw.RequireAuth(mw.CSRF(http.HandlerFunc(admin.PostsTable))))
	mux.Handle("GET /admin/posts/new", mw.RequireAuth(mw.CSRF(http.HandlerFunc(admin.NewPostForm))))
	mux.Handle("POST /admin/posts", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.CreatePost)))))
	mux.Handle("GET /admin/posts/{id}/edit", mw.RequireAuth(mw.CSRF(http.HandlerFunc(admin.EditPostForm))))
	mux.Handle("POST /admin/posts/{id}/edit", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.UpdatePost)))))
	mux.Handle("POST /admin/posts/{id}/autosave", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.AutosavePost)))))
	mux.Handle("POST /admin/posts/{id}/autosave/restore", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.RestoreAutosavePost)))))
	mux.Handle("POST /admin/posts/{id}/autosave/dismiss", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.DismissAutosavePost)))))
	mux.Handle("GET /admin/posts/{id}/quick-edit", mw.RequireAuth(mw.CSRF(http.HandlerFunc(admin.QuickEditPost))))
	mux.Handle("POST /admin/posts/{id}/quick-edit", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.QuickEditPostSave)))))
	mux.Handle("POST /admin/posts/{id}/publish", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.PublishPost)))))
	mux.Handle("POST /admin/posts/{id}/unpublish", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.UnpublishPost)))))
	mux.Handle("POST /admin/posts/{id}/delete", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.DeletePost)))))
	mux.Handle("GET /admin/media", mw.RequireAuth(mw.CSRF(http.HandlerFunc(admin.MediaPage))))
	mux.Handle("GET /admin/media/table", mw.RequireAuth(mw.CSRF(http.HandlerFunc(admin.MediaTable))))
	mux.Handle("POST /admin/media", mw.RequireAuth(mw.CSRF(mw.SensitiveLimit(http.HandlerFunc(admin.UploadMedia)))))
	mux.Handle("GET /admin/media/files/{id}/variants/{name}", mw.RequireAuth(mw.CSRF(http.HandlerFunc(admin.MediaVariantFile))))
	mux.Handle("GET /admin/media/files/{id}", mw.RequireAuth(mw.CSRF(http.HandlerFunc(admin.MediaFile))))

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	handler := http.Handler(mux)
	handler = mw.Session(handler)
	handler = mw.HTMX(handler)
	handler = mw.SecurityHeaders(handler)
	if mw.RequestMetrics != nil {
		handler = mw.RequestMetrics(handler)
	}
	handler = mw.RequestID(handler)

	root := http.NewServeMux()
	root.Handle("/", handler)
	return &Router{mux: root}
}

// ServeHTTP explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

// redirectToTrailingSlash explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func redirectToTrailingSlash(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
}

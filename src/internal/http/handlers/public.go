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
	"context"
	"html/template"
	"net/http"
	"strings"

	"kcnotes/internal/domain"
	"kcnotes/internal/http/views"
	storesqlite "kcnotes/internal/store/sqlite"
)

type Public struct {
	renderer *views.Renderer
	store    publicStore
	md       markdownRenderer
}

type publicStore interface {
	ListPublicPosts(ctx context.Context, includeDrafts bool, limit int) ([]domain.Post, error)
	GetPublishedPostBySlug(ctx context.Context, slug string) (domain.Post, error)
	GetPublishedPageBySlug(ctx context.Context, slug string) (domain.Post, error)
}

type markdownRenderer interface {
	Render(markdown string) (template.HTML, error)
}

// NewPublic explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func NewPublic(renderer *views.Renderer, store publicStore, md markdownRenderer) *Public {
	return &Public{
		renderer: renderer,
		store:    store,
		md:       md,
	}
}

// Home explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Public) Home(w http.ResponseWriter, r *http.Request) {
	posts, err := h.store.ListPublicPosts(r.Context(), false, 10)
	if err != nil {
		http.Error(w, "failed to load published posts", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "home", map[string]any{
		"Title":         "kcNotes",
		"Posts":         posts,
		"AssetBase":     "/static",
		"ShowAdminLink": true,
	})
}

// Healthz explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Public) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// Post explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Public) Post(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.PathValue("slug"))
	if slug == "" {
		http.NotFound(w, r)
		return
	}
	post, err := h.store.GetPublishedPostBySlug(r.Context(), slug)
	if err != nil {
		if err == storesqlite.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to load post", http.StatusInternalServerError)
		return
	}

	bodyHTML, err := h.md.Render(post.BodyMD)
	if err != nil {
		http.Error(w, "failed to render content", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "public-post", map[string]any{
		"Title":     post.Title,
		"Post":      post,
		"BodyHTML":  bodyHTML,
		"AssetBase": "/static",
	})
}

// Page explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Public) Page(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.PathValue("slug"))
	if slug == "" {
		http.NotFound(w, r)
		return
	}
	page, err := h.store.GetPublishedPageBySlug(r.Context(), slug)
	if err != nil {
		if err == storesqlite.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to load page", http.StatusInternalServerError)
		return
	}

	bodyHTML, err := h.md.Render(page.BodyMD)
	if err != nil {
		http.Error(w, "failed to render content", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "public-page", map[string]any{
		"Title":     page.Title,
		"Page":      page,
		"BodyHTML":  bodyHTML,
		"AssetBase": "/static",
	})
}

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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"kcnotes/internal/domain"
	"kcnotes/internal/http/middleware"
	storesqlite "kcnotes/internal/store/sqlite"
)

const defaultPostPageSize = 10

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type postFormData struct {
	ID            string
	Type          domain.PostType
	Title         string
	Slug          string
	BodyMD        string
	Status        domain.PostStatus
	BaseUpdatedAt string
}

// PostsPage explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) PostsPage(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}

	filters := parsePostListFilter(r, user)
	posts, total, err := h.store.ListPosts(r.Context(), filters)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load posts")
		return
	}

	data := h.postsListData(r, user, filters, posts, total)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "admin-posts", data)
}

// PostsTable explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) PostsTable(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}

	filters := parsePostListFilter(r, user)
	posts, total, err := h.store.ListPosts(r.Context(), filters)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load posts")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "partial-posts-table", h.postsListData(r, user, filters, posts, total))
}

// NewPostForm explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) NewPostForm(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}

	data := h.postFormTemplateData(r, user, postFormData{Type: domain.PostTypePost, Status: domain.PostStatusDraft}, nil, false)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if middleware.IsHTMX(r) {
		_ = h.renderer.Render(w, "partial-post-form", data)
		return
	}
	_ = h.renderer.Render(w, "admin-post-form", data)
}

// CreatePost explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) CreatePost(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "invalid form")
		return
	}

	form, validationErrors := parseAndValidatePostForm(r, true)
	if len(validationErrors) > 0 {
		h.renderPostFormError(w, r, user, form, validationErrors, false)
		return
	}

	var publishedAt *time.Time
	if form.Status == domain.PostStatusPublished {
		now := time.Now().UTC()
		publishedAt = &now
	}

	postID, err := randomID()
	if err != nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "service unavailable")
		return
	}
	err = h.store.CreatePost(r.Context(), domain.Post{
		ID:          postID,
		Type:        form.Type,
		Title:       form.Title,
		Slug:        form.Slug,
		BodyMD:      form.BodyMD,
		Status:      form.Status,
		AuthorID:    user.ID,
		PublishedAt: publishedAt,
	})
	if err != nil {
		if errors.Is(err, storesqlite.ErrConflict) {
			validationErrors = append(validationErrors, "Slug already exists")
			h.renderPostFormError(w, r, user, form, validationErrors, false)
			return
		}
		h.renderError(w, r, http.StatusInternalServerError, "failed to create post")
		return
	}
	h.auditEvent(r, user.ID, "post_create", "post", postID)

	h.redirectAfterWrite(w, r, "/admin/posts", "Post created")
}

// EditPostForm explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) EditPostForm(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	post, err := h.store.GetPostByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, storesqlite.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		h.renderError(w, r, http.StatusInternalServerError, "failed to load post")
		return
	}
	if !canAccessPost(user, post) || post.DeletedAt != nil {
		http.NotFound(w, r)
		return
	}

	form := postFormData{ID: post.ID, Type: post.Type, Title: post.Title, Slug: post.Slug, BodyMD: post.BodyMD, Status: post.Status, BaseUpdatedAt: formatFormTime(post.UpdatedAt)}
	data := h.postFormTemplateData(r, user, form, nil, true)
	if snapshot, ok := h.newerAutosaveSnapshot(r, post, user); ok {
		data["AutosaveSnapshot"] = snapshot
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if middleware.IsHTMX(r) {
		_ = h.renderer.Render(w, "partial-post-form", data)
		return
	}
	_ = h.renderer.Render(w, "admin-post-form", data)
}

// UpdatePost explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) UpdatePost(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	form, validationErrors := parseAndValidatePostForm(r, false)
	form.ID = id
	if len(validationErrors) > 0 {
		h.renderPostFormError(w, r, user, form, validationErrors, true)
		return
	}

	var publishedAt *time.Time
	if form.Status == domain.PostStatusPublished {
		now := time.Now().UTC()
		publishedAt = &now
	}

	updated, err := h.store.UpdatePost(r.Context(), domain.Post{
		ID:          id,
		Type:        form.Type,
		Title:       form.Title,
		Slug:        form.Slug,
		BodyMD:      form.BodyMD,
		Status:      form.Status,
		PublishedAt: publishedAt,
	}, user)
	if err != nil {
		if errors.Is(err, storesqlite.ErrConflict) {
			validationErrors = append(validationErrors, "Slug already exists")
			h.renderPostFormError(w, r, user, form, validationErrors, true)
			return
		}
		h.renderError(w, r, http.StatusInternalServerError, "failed to update post")
		return
	}
	if !updated {
		http.NotFound(w, r)
		return
	}
	h.auditEvent(r, user.ID, "post_update", "post", id)

	h.redirectAfterWrite(w, r, "/admin/posts", "Post updated")
}

// AutosavePost stores a private recovery snapshot without changing canonical content.
func (h *Admin) AutosavePost(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	post, err := h.loadWritablePost(r, w, id, user)
	if err != nil {
		return
	}

	form, validationErrors := parseAndValidatePostForm(r, false)
	form.ID = id
	form.BaseUpdatedAt = strings.TrimSpace(r.FormValue("base_updated_at"))
	if len(validationErrors) > 0 {
		h.renderAutosaveStatus(w, http.StatusUnprocessableEntity, "Failed")
		return
	}
	baseUpdatedAt, err := parseFormTime(form.BaseUpdatedAt)
	if err != nil {
		h.renderAutosaveStatus(w, http.StatusConflict, "Conflict")
		return
	}
	if !post.UpdatedAt.Equal(baseUpdatedAt) {
		h.renderAutosaveStatus(w, http.StatusConflict, "Conflict")
		return
	}

	ok, err = h.store.UpsertAutosaveSnapshot(r.Context(), domain.AutosaveSnapshot{
		PostID:        id,
		AuthorID:      user.ID,
		Type:          form.Type,
		Title:         form.Title,
		Slug:          form.Slug,
		BodyMD:        form.BodyMD,
		Status:        form.Status,
		BaseUpdatedAt: baseUpdatedAt,
	}, user)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to autosave post")
		return
	}
	if !ok {
		h.renderAutosaveStatus(w, http.StatusConflict, "Conflict")
		return
	}
	h.renderAutosaveStatus(w, http.StatusOK, "Saved")
}

// RestoreAutosavePost returns the edit form populated from the user's snapshot.
func (h *Admin) RestoreAutosavePost(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	post, err := h.loadWritablePost(r, w, id, user)
	if err != nil {
		return
	}
	snapshot, err := h.store.GetLatestAutosaveSnapshot(r.Context(), id, user.ID)
	if err != nil {
		if errors.Is(err, storesqlite.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		h.renderError(w, r, http.StatusInternalServerError, "failed to load autosave")
		return
	}
	form := postFormData{ID: post.ID, Type: snapshot.Type, Title: snapshot.Title, Slug: snapshot.Slug, BodyMD: snapshot.BodyMD, Status: snapshot.Status, BaseUpdatedAt: formatFormTime(post.UpdatedAt)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "partial-post-form", h.postFormTemplateData(r, user, form, nil, true))
}

// DismissAutosavePost removes the recovery snapshot and returns the canonical form.
func (h *Admin) DismissAutosavePost(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	post, err := h.loadWritablePost(r, w, id, user)
	if err != nil {
		return
	}
	_, err = h.store.DismissAutosaveSnapshot(r.Context(), id, user.ID)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to dismiss autosave")
		return
	}
	form := postFormData{ID: post.ID, Type: post.Type, Title: post.Title, Slug: post.Slug, BodyMD: post.BodyMD, Status: post.Status, BaseUpdatedAt: formatFormTime(post.UpdatedAt)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "partial-post-form", h.postFormTemplateData(r, user, form, nil, true))
}

// QuickEditPost explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) QuickEditPost(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	post, err := h.store.GetPostByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, storesqlite.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		h.renderError(w, r, http.StatusInternalServerError, "failed to load post")
		return
	}
	if !canAccessPost(user, post) || post.DeletedAt != nil {
		http.NotFound(w, r)
		return
	}

	data := map[string]any{"Post": post, "CSRFToken": middleware.CSRFToken(r)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "partial-post-row-edit", data)
}

// QuickEditPostSave explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) QuickEditPostSave(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	post, err := h.store.GetPostByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, storesqlite.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		h.renderError(w, r, http.StatusInternalServerError, "failed to load post")
		return
	}
	if !canAccessPost(user, post) || post.DeletedAt != nil {
		http.NotFound(w, r)
		return
	}

	form := postFormData{
		ID:     id,
		Type:   post.Type,
		BodyMD: post.BodyMD,
		Status: post.Status,
		Title:  strings.TrimSpace(r.FormValue("title")),
		Slug:   strings.TrimSpace(r.FormValue("slug")),
	}
	if form.Slug == "" {
		form.Slug = slugify(form.Title)
	}

	validationErrors := validateQuickEdit(form)
	if len(validationErrors) > 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = h.renderer.Render(w, "partial-post-row-edit", map[string]any{
			"Post":      domain.Post{ID: id, Title: form.Title, Slug: form.Slug, Type: post.Type, Status: post.Status, AuthorID: post.AuthorID, UpdatedAt: post.UpdatedAt},
			"Errors":    validationErrors,
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	updated, err := h.store.UpdatePost(r.Context(), domain.Post{
		ID:     id,
		Type:   post.Type,
		Title:  form.Title,
		Slug:   form.Slug,
		BodyMD: post.BodyMD,
		Status: post.Status,
	}, user)
	if err != nil {
		if errors.Is(err, storesqlite.ErrConflict) {
			validationErrors = []string{"Slug already exists"}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = h.renderer.Render(w, "partial-post-row-edit", map[string]any{
				"Post":      domain.Post{ID: id, Title: form.Title, Slug: form.Slug, Type: post.Type, Status: post.Status, AuthorID: post.AuthorID, UpdatedAt: post.UpdatedAt},
				"Errors":    validationErrors,
				"CSRFToken": middleware.CSRFToken(r),
			})
			return
		}
		h.renderError(w, r, http.StatusInternalServerError, "failed to update post")
		return
	}
	if !updated {
		http.NotFound(w, r)
		return
	}
	h.auditEvent(r, user.ID, "post_quick_edit", "post", id)

	updatedPost, err := h.store.GetPostByID(r.Context(), id)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load post")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "partial-post-row", updatedPost)
}

// PublishPost explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) PublishPost(w http.ResponseWriter, r *http.Request) {
	h.updatePostStatus(w, r, domain.PostStatusPublished, "Post published")
}

// UnpublishPost explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) UnpublishPost(w http.ResponseWriter, r *http.Request) {
	h.updatePostStatus(w, r, domain.PostStatusDraft, "Post set to draft")
}

// DeletePost explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) DeletePost(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	updated, err := h.store.SoftDeletePost(r.Context(), id, user)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to delete post")
		return
	}
	if !updated {
		http.NotFound(w, r)
		return
	}
	h.auditEvent(r, user.ID, "post_delete", "post", id)

	h.renderPostsTableAfterAction(w, r, user, "Post deleted")
}

// updatePostStatus explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) updatePostStatus(w http.ResponseWriter, r *http.Request, status domain.PostStatus, message string) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	var publishedAt *time.Time
	if status == domain.PostStatusPublished {
		now := time.Now().UTC()
		publishedAt = &now
	}

	updated, err := h.store.SetPostStatus(r.Context(), id, status, publishedAt, user)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to update post")
		return
	}
	if !updated {
		http.NotFound(w, r)
		return
	}
	action := "post_unpublish"
	if status == domain.PostStatusPublished {
		action = "post_publish"
	}
	h.auditEvent(r, user.ID, action, "post", id)

	h.renderPostsTableAfterAction(w, r, user, message)
}

// renderPostsTableAfterAction explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) renderPostsTableAfterAction(w http.ResponseWriter, r *http.Request, user domain.User, message string) {
	if middleware.IsHTMX(r) {
		filters := parsePostListFilter(r, user)
		posts, total, err := h.store.ListPosts(r.Context(), filters)
		if err != nil {
			h.renderError(w, r, http.StatusInternalServerError, "failed to load posts")
			return
		}
		data := h.postsListData(r, user, filters, posts, total)
		data["FlashMessage"] = message
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = h.renderer.Render(w, "partial-posts-table", data)
		return
	}
	http.Redirect(w, r, "/admin/posts?msg="+message, http.StatusSeeOther)
}

// redirectAfterWrite explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) redirectAfterWrite(w http.ResponseWriter, r *http.Request, to, message string) {
	if middleware.IsHTMX(r) {
		w.Header().Set("HX-Redirect", to+"?msg="+message)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, to+"?msg="+message, http.StatusSeeOther)
}

// renderPostFormError explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) renderPostFormError(w http.ResponseWriter, r *http.Request, user domain.User, form postFormData, validationErrors []string, isEdit bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	data := h.postFormTemplateData(r, user, form, validationErrors, isEdit)
	if middleware.IsHTMX(r) {
		_ = h.renderer.Render(w, "partial-post-form", data)
		return
	}
	_ = h.renderer.Render(w, "admin-post-form", data)
}

func (h *Admin) renderAutosaveStatus(w http.ResponseWriter, status int, label string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = h.renderer.Render(w, "partial-autosave-status", map[string]any{"AutosaveStatus": label})
}

// postsListData explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) postsListData(r *http.Request, user domain.User, filter storesqlite.PostListFilter, posts []domain.Post, total int) map[string]any {
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = defaultPostPageSize
	}
	currentPage := filter.Page
	if currentPage <= 0 {
		currentPage = 1
	}
	totalPages := total / pageSize
	if total%pageSize != 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}
	if currentPage > totalPages {
		currentPage = totalPages
	}

	queryBase := fmt.Sprintf(
		"q=%s&type=%s&status=%s&sort=%s&page_size=%d",
		url.QueryEscape(filter.Query),
		url.QueryEscape(string(filter.Type)),
		url.QueryEscape(string(filter.Status)),
		url.QueryEscape(filter.Sort),
		pageSize,
	)

	return map[string]any{
		"Title":        "Posts",
		"CSRFToken":    middleware.CSRFToken(r),
		"UserEmail":    user.Email,
		"UserRole":     string(user.Role),
		"Posts":        posts,
		"Total":        total,
		"Page":         currentPage,
		"PageSize":     pageSize,
		"TotalPages":   totalPages,
		"HasPrevPage":  currentPage > 1,
		"HasNextPage":  currentPage < totalPages,
		"PrevPage":     currentPage - 1,
		"NextPage":     currentPage + 1,
		"Query":        filter.Query,
		"FilterType":   string(filter.Type),
		"FilterStatus": string(filter.Status),
		"FilterSort":   filter.Sort,
		"QueryBase":    queryBase,
		"FlashMessage": strings.TrimSpace(r.URL.Query().Get("msg")),
	}
}

// postFormTemplateData explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) postFormTemplateData(r *http.Request, user domain.User, form postFormData, errs []string, isEdit bool) map[string]any {
	action := "/admin/posts"
	title := "New Post"
	if isEdit {
		action = "/admin/posts/" + form.ID + "/edit"
		title = "Edit Post"
	}
	return map[string]any{
		"Title":       title,
		"CSRFToken":   middleware.CSRFToken(r),
		"UserEmail":   user.Email,
		"UserRole":    string(user.Role),
		"Form":        form,
		"FormErrors":  errs,
		"FormAction":  action,
		"IsEdit":      isEdit,
		"SubmitLabel": map[bool]string{true: "Save Changes", false: "Create Post"}[isEdit],
	}
}

func (h *Admin) newerAutosaveSnapshot(r *http.Request, post domain.Post, user domain.User) (domain.AutosaveSnapshot, bool) {
	snapshot, err := h.store.GetLatestAutosaveSnapshot(r.Context(), post.ID, user.ID)
	if err != nil {
		return domain.AutosaveSnapshot{}, false
	}
	return snapshot, snapshot.UpdatedAt.After(post.UpdatedAt)
}

func (h *Admin) loadWritablePost(r *http.Request, w http.ResponseWriter, id string, user domain.User) (domain.Post, error) {
	post, err := h.store.GetPostByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, storesqlite.ErrNotFound) {
			http.NotFound(w, r)
			return domain.Post{}, err
		}
		h.renderError(w, r, http.StatusInternalServerError, "failed to load post")
		return domain.Post{}, err
	}
	if !canAccessPost(user, post) || post.DeletedAt != nil {
		http.NotFound(w, r)
		return domain.Post{}, storesqlite.ErrNotFound
	}
	return post, nil
}

// parsePostListFilter explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func parsePostListFilter(r *http.Request, user domain.User) storesqlite.PostListFilter {
	query := r.URL.Query()
	page, _ := strconv.Atoi(strings.TrimSpace(query.Get("page")))
	pageSize, _ := strconv.Atoi(strings.TrimSpace(query.Get("page_size")))
	if pageSize <= 0 {
		pageSize = defaultPostPageSize
	}

	filter := storesqlite.PostListFilter{
		Type:     domain.PostType(strings.TrimSpace(query.Get("type"))),
		Status:   domain.PostStatus(strings.TrimSpace(query.Get("status"))),
		Query:    strings.TrimSpace(query.Get("q")),
		Sort:     strings.TrimSpace(query.Get("sort")),
		Page:     page,
		PageSize: pageSize,
	}
	if !domain.IsValidPostType(filter.Type) {
		filter.Type = ""
	}
	if filter.Status != "" && !domain.IsValidPostStatus(filter.Status) {
		filter.Status = ""
	}
	if user.Role == domain.RoleAuthor {
		filter.AuthorID = user.ID
	}
	return filter
}

// parseAndValidatePostForm explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func parseAndValidatePostForm(r *http.Request, creating bool) (postFormData, []string) {
	form := postFormData{
		Type:   domain.PostType(strings.TrimSpace(r.FormValue("type"))),
		Title:  strings.TrimSpace(r.FormValue("title")),
		Slug:   strings.TrimSpace(r.FormValue("slug")),
		BodyMD: strings.TrimSpace(r.FormValue("body_md")),
		Status: domain.PostStatus(strings.TrimSpace(r.FormValue("status"))),
	}
	if form.Slug == "" {
		form.Slug = slugify(form.Title)
	}
	if creating && form.Status == "" {
		form.Status = domain.PostStatusDraft
	}

	errs := make([]string, 0, 4)
	if !domain.IsValidPostType(form.Type) {
		errs = append(errs, "Type must be post or page")
	}
	if form.Title == "" || len(form.Title) > 160 {
		errs = append(errs, "Title is required and must be <= 160 characters")
	}
	if !slugPattern.MatchString(form.Slug) {
		errs = append(errs, "Slug must use lowercase letters, numbers, and single hyphens")
	}
	if !domain.IsValidPostStatus(form.Status) || form.Status == domain.PostStatusDeleted {
		errs = append(errs, "Status must be draft, published, or archived")
	}
	return form, errs
}

func parseFormTime(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
}

func formatFormTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// validateQuickEdit explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func validateQuickEdit(form postFormData) []string {
	errs := make([]string, 0, 2)
	if form.Title == "" || len(form.Title) > 160 {
		errs = append(errs, "Title is required and must be <= 160 characters")
	}
	if !slugPattern.MatchString(form.Slug) {
		errs = append(errs, "Slug must use lowercase letters, numbers, and single hyphens")
	}
	return errs
}

// slugify explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func slugify(title string) string {
	lower := strings.ToLower(strings.TrimSpace(title))
	if lower == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_' || r == '/':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	return slug
}

// canAccessPost explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func canAccessPost(user domain.User, post domain.Post) bool {
	if user.Role == domain.RoleAuthor {
		return post.AuthorID == user.ID
	}
	return true
}

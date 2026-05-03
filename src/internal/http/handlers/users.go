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
	"net/url"
	"strings"
	"time"

	"kcnotes/internal/auth"
	"kcnotes/internal/domain"
	"kcnotes/internal/http/middleware"
	storesqlite "kcnotes/internal/store/sqlite"
)

type userCreateForm struct {
	Email string
	Role  domain.Role
}

// Users explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) Users(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}

	users, err := h.store.ListUsers(r.Context())
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load users")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "admin-users", h.usersData(r, user, users, userCreateForm{}, nil))
}

// UsersTable explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) UsersTable(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}

	users, err := h.store.ListUsers(r.Context())
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load users")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "partial-users-table", h.usersData(r, user, users, userCreateForm{}, nil))
}

// CreateUser explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) CreateUser(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "invalid form")
		return
	}

	form, errs := parseAndValidateCreateUserForm(r)
	if len(errs) > 0 {
		h.renderUsersTableError(w, r, currentUser, form, errs)
		return
	}

	newUserID, err := randomID()
	if err != nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "service unavailable")
		return
	}
	token, err := newEnrollmentToken()
	if err != nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "service unavailable")
		return
	}
	inviteID, err := randomID()
	if err != nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "service unavailable")
		return
	}
	err = h.store.CreateEnrollmentInvitation(r.Context(), domain.User{
		ID:           newUserID,
		Email:        form.Email,
		PasswordHash: "",
		Role:         form.Role,
		Disabled:     true,
	}, domain.EnrollmentInvitation{
		ID:        inviteID,
		UserID:    newUserID,
		TokenHash: authTokenHash(token),
		ExpiresAt: time.Now().UTC().Add(7 * 24 * time.Hour),
		CreatedBy: currentUser.ID,
	})
	if err != nil {
		if err == storesqlite.ErrConflict {
			h.renderUsersTableError(w, r, currentUser, form, []string{"Email already exists"})
			return
		}
		h.renderError(w, r, http.StatusInternalServerError, "failed to create user")
		return
	}
	h.auditEvent(r, currentUser.ID, "user_invite", "user", newUserID)

	enrollPath := "/admin/enroll?token=" + url.QueryEscape(token)
	h.renderUsersTableAfterAction(w, r, currentUser, "Enrollment link created. Copy it now; it is single-use and expires.", enrollPath)
}

// UpdateUser explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) UpdateUser(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "invalid form")
		return
	}

	targetID := strings.TrimSpace(r.PathValue("id"))
	if targetID == "" {
		http.NotFound(w, r)
		return
	}

	role := domain.Role(strings.TrimSpace(r.FormValue("role")))
	disabled := strings.TrimSpace(r.FormValue("disabled")) == "on"
	if !domain.IsValidRole(role) {
		h.renderError(w, r, http.StatusUnprocessableEntity, "invalid role")
		return
	}
	if targetID == currentUser.ID && disabled {
		h.renderError(w, r, http.StatusUnprocessableEntity, "cannot disable your own account")
		return
	}
	if targetID == currentUser.ID && role != domain.RoleAdmin {
		h.renderError(w, r, http.StatusUnprocessableEntity, "cannot remove your own admin role")
		return
	}

	updated, err := h.store.UpdateUserRoleDisabled(r.Context(), targetID, role, disabled)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to update user")
		return
	}
	if !updated {
		http.NotFound(w, r)
		return
	}
	h.auditEvent(r, currentUser.ID, "user_update", "user", targetID)

	h.renderUsersTableAfterAction(w, r, currentUser, "User updated", "")
}

// usersData explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) usersData(r *http.Request, currentUser domain.User, users []domain.User, form userCreateForm, errs []string) map[string]any {
	return map[string]any{
		"Title":            "Users",
		"CSRFToken":        middleware.CSRFToken(r),
		"UserEmail":        currentUser.Email,
		"UserRole":         string(currentUser.Role),
		"Users":            users,
		"CreateForm":       form,
		"CreateFormErrors": errs,
		"CurrentUserID":    currentUser.ID,
		"FlashMessage":     strings.TrimSpace(r.URL.Query().Get("msg")),
		"EnrollmentLink":   enrollmentPathFromQuery(r.URL.Query().Get("enrollment_link")),
	}
}

func enrollmentPathFromQuery(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	if parsed.IsAbs() || parsed.Host != "" || parsed.Path != "/admin/enroll" || parsed.Fragment != "" {
		return ""
	}
	query := parsed.Query()
	token := strings.TrimSpace(query.Get("token"))
	if token == "" || len(query) != 1 || len(query["token"]) != 1 {
		return ""
	}
	return parsed.String()
}

// renderUsersTableError explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) renderUsersTableError(w http.ResponseWriter, r *http.Request, currentUser domain.User, form userCreateForm, errs []string) {
	users, err := h.store.ListUsers(r.Context())
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load users")
		return
	}
	data := h.usersData(r, currentUser, users, form, errs)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	if middleware.IsHTMX(r) {
		_ = h.renderer.Render(w, "partial-users-table", data)
		return
	}
	_ = h.renderer.Render(w, "admin-users", data)
}

// renderUsersTableAfterAction explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) renderUsersTableAfterAction(w http.ResponseWriter, r *http.Request, currentUser domain.User, message, enrollmentLink string) {
	if middleware.IsHTMX(r) {
		users, err := h.store.ListUsers(r.Context())
		if err != nil {
			h.renderError(w, r, http.StatusInternalServerError, "failed to load users")
			return
		}
		data := h.usersData(r, currentUser, users, userCreateForm{}, nil)
		data["FlashMessage"] = message
		data["EnrollmentLink"] = enrollmentLink
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = h.renderer.Render(w, "partial-users-table", data)
		return
	}
	redirectURL := "/admin/users?msg=" + url.QueryEscape(message)
	if enrollmentLink != "" {
		redirectURL += "&enrollment_link=" + url.QueryEscape(enrollmentLink)
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// parseAndValidateCreateUserForm explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func parseAndValidateCreateUserForm(r *http.Request) (userCreateForm, []string) {
	form := userCreateForm{
		Email: strings.ToLower(strings.TrimSpace(r.FormValue("email"))),
		Role:  domain.Role(strings.TrimSpace(r.FormValue("role"))),
	}
	errs := make([]string, 0, 2)
	if form.Email == "" || len(form.Email) > 254 || !strings.Contains(form.Email, "@") {
		errs = append(errs, "Email is required and must be valid")
	}
	if !domain.IsValidRole(form.Role) {
		errs = append(errs, "Role must be admin, editor, or author")
	}
	return form, errs
}

func authTokenHash(token string) string {
	return auth.TokenHash(token)
}

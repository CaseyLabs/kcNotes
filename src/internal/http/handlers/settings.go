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

	"kcnotes/internal/domain"
	"kcnotes/internal/http/middleware"
)

var managedSettingKeys = []string{
	"site_name",
	"site_base_url",
	"feature_flags",
}

type settingsForm struct {
	SiteName     string
	SiteBaseURL  string
	FeatureFlags string
}

// SettingsPage explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) SettingsPage(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}

	settings, err := h.store.GetSettings(r.Context(), managedSettingKeys)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load settings")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "admin-settings", h.settingsData(r, user, settingsForm{
		SiteName:     settings["site_name"],
		SiteBaseURL:  settings["site_base_url"],
		FeatureFlags: settings["feature_flags"],
	}, nil))
}

// UpdateSettings explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "invalid form")
		return
	}

	form, errs := parseAndValidateSettingsForm(r)
	if len(errs) > 0 {
		h.renderSettingsError(w, r, user, form, errs)
		return
	}

	entries := map[string]string{
		"site_name":     form.SiteName,
		"site_base_url": form.SiteBaseURL,
		"feature_flags": form.FeatureFlags,
	}
	if err := h.store.UpsertSettings(r.Context(), entries); err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to update settings")
		return
	}
	h.auditEvent(r, user.ID, "settings_update", "settings", "global")

	if middleware.IsHTMX(r) {
		data := h.settingsData(r, user, form, nil)
		data["FlashMessage"] = "Settings saved"
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = h.renderer.Render(w, "partial-settings-form", data)
		return
	}
	http.Redirect(w, r, "/admin/settings?msg="+url.QueryEscape("Settings saved"), http.StatusSeeOther)
}

// settingsData explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) settingsData(r *http.Request, user domain.User, form settingsForm, errs []string) map[string]any {
	return map[string]any{
		"Title":          "Settings",
		"CSRFToken":      middleware.CSRFToken(r),
		"UserEmail":      user.Email,
		"UserRole":       string(user.Role),
		"Form":           form,
		"FormErrors":     errs,
		"FlashMessage":   strings.TrimSpace(r.URL.Query().Get("msg")),
		"SettingsAction": "/admin/settings",
	}
}

// renderSettingsError explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) renderSettingsError(w http.ResponseWriter, r *http.Request, user domain.User, form settingsForm, errs []string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	data := h.settingsData(r, user, form, errs)
	if middleware.IsHTMX(r) {
		_ = h.renderer.Render(w, "partial-settings-form", data)
		return
	}
	_ = h.renderer.Render(w, "admin-settings", data)
}

// parseAndValidateSettingsForm explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func parseAndValidateSettingsForm(r *http.Request) (settingsForm, []string) {
	form := settingsForm{
		SiteName:     strings.TrimSpace(r.FormValue("site_name")),
		SiteBaseURL:  strings.TrimSpace(r.FormValue("site_base_url")),
		FeatureFlags: strings.TrimSpace(r.FormValue("feature_flags")),
	}
	errs := make([]string, 0, 2)
	if form.SiteName == "" || len(form.SiteName) > 120 {
		errs = append(errs, "Site name is required and must be <= 120 characters")
	}
	if len(form.SiteBaseURL) > 255 {
		errs = append(errs, "Site base URL must be <= 255 characters")
	}
	return form, errs
}

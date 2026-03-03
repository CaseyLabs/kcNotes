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
)

const recoveryCodeCount = 10

// MFAPage explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) MFAPage(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	mfaUser, err := h.store.GetUserByID(r.Context(), currentUser.ID)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load mfa settings")
		return
	}
	h.renderMFAPage(w, r, currentUser, mfaUser, strings.TrimSpace(r.URL.Query().Get("msg")), nil)
}

// MFASetup explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) MFASetup(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "service unavailable")
		return
	}
	updated, err := h.store.SetUserMFASecret(r.Context(), currentUser.ID, secret)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to store mfa secret")
		return
	}
	if !updated {
		http.NotFound(w, r)
		return
	}
	h.auditEvent(r, currentUser.ID, "mfa_setup", "user", currentUser.ID)
	h.renderMFAAfterAction(w, r, currentUser, "Authenticator secret created. Verify a code to enable MFA.", nil)
}

// MFAEnable explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) MFAEnable(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	code := strings.TrimSpace(r.FormValue("otp"))
	mfaUser, err := h.store.GetUserByID(r.Context(), currentUser.ID)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load mfa settings")
		return
	}
	if strings.TrimSpace(mfaUser.MFASecret) == "" {
		h.renderMFAError(w, r, currentUser, "Set up an authenticator secret first")
		return
	}
	if !auth.VerifyTOTP(mfaUser.MFASecret, code, nowUTC()) {
		h.renderMFAError(w, r, currentUser, "Invalid authenticator code")
		return
	}
	recoveryCodes, recoveryHashes, err := auth.GenerateRecoveryCodes(recoveryCodeCount)
	if err != nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "service unavailable")
		return
	}
	updated, err := h.store.EnableUserMFA(r.Context(), currentUser.ID)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to enable mfa")
		return
	}
	if !updated {
		http.NotFound(w, r)
		return
	}
	if err := h.store.ReplaceRecoveryCodeHashes(r.Context(), currentUser.ID, recoveryHashes); err != nil {
		_, _ = h.store.DisableUserMFA(r.Context(), currentUser.ID)
		h.renderError(w, r, http.StatusInternalServerError, "failed to create recovery codes")
		return
	}
	h.auditEvent(r, currentUser.ID, "mfa_enabled", "user", currentUser.ID)
	h.renderMFAAfterAction(w, r, currentUser, "MFA enabled. Save your recovery codes now.", recoveryCodes)
}

// MFADisable explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) MFADisable(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	password := r.FormValue("password")
	code := strings.TrimSpace(r.FormValue("otp"))
	mfaUser, err := h.store.GetUserByID(r.Context(), currentUser.ID)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load mfa settings")
		return
	}
	if !mfaUser.MFAEnabled {
		h.renderMFAError(w, r, currentUser, "MFA is not enabled")
		return
	}
	passwordOK, err := auth.VerifyPassword(password, mfaUser.PasswordHash)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to verify password")
		return
	}
	if !passwordOK {
		h.renderMFAError(w, r, currentUser, "Invalid password or verification code")
		return
	}
	validCode, usedRecovery, err := h.verifyMFAOrRecovery(r, mfaUser, code)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to verify code")
		return
	}
	if !validCode {
		h.renderMFAError(w, r, currentUser, "Invalid password or verification code")
		return
	}
	updated, err := h.store.DisableUserMFA(r.Context(), currentUser.ID)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to disable mfa")
		return
	}
	if !updated {
		http.NotFound(w, r)
		return
	}
	if usedRecovery {
		h.auditEvent(r, currentUser.ID, "mfa_recovery_code_used", "user", currentUser.ID)
	}
	h.auditEvent(r, currentUser.ID, "mfa_disabled", "user", currentUser.ID)
	h.renderMFAAfterAction(w, r, currentUser, "MFA disabled", nil)
}

// MFARegenerateRecovery explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) MFARegenerateRecovery(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	code := strings.TrimSpace(r.FormValue("otp"))
	mfaUser, err := h.store.GetUserByID(r.Context(), currentUser.ID)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load mfa settings")
		return
	}
	if !mfaUser.MFAEnabled {
		h.renderMFAError(w, r, currentUser, "MFA is not enabled")
		return
	}
	if !auth.VerifyTOTP(mfaUser.MFASecret, code, nowUTC()) {
		h.renderMFAError(w, r, currentUser, "Invalid authenticator code")
		return
	}
	recoveryCodes, recoveryHashes, err := auth.GenerateRecoveryCodes(recoveryCodeCount)
	if err != nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "service unavailable")
		return
	}
	if err := h.store.ReplaceRecoveryCodeHashes(r.Context(), currentUser.ID, recoveryHashes); err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to regenerate recovery codes")
		return
	}
	h.auditEvent(r, currentUser.ID, "mfa_recovery_regenerated", "user", currentUser.ID)
	h.renderMFAAfterAction(w, r, currentUser, "Recovery codes regenerated. Save the new set now.", recoveryCodes)
}

// verifyMFAOrRecovery explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) verifyMFAOrRecovery(r *http.Request, mfaUser domain.User, code string) (ok, usedRecovery bool, err error) {
	if auth.VerifyTOTP(mfaUser.MFASecret, code, nowUTC()) {
		return true, false, nil
	}
	used, err := h.store.ConsumeRecoveryCodeHash(r.Context(), mfaUser.ID, auth.RecoveryCodeHash(code))
	if err != nil {
		return false, false, err
	}
	return used, used, nil
}

// renderMFAError explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) renderMFAError(w http.ResponseWriter, r *http.Request, user domain.User, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	mfaUser, err := h.store.GetUserByID(r.Context(), user.ID)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load mfa settings")
		return
	}
	data := h.mfaData(r, user, mfaUser, message, nil)
	_ = h.renderer.Render(w, "admin-mfa", data)
}

// renderMFAAfterAction explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) renderMFAAfterAction(w http.ResponseWriter, r *http.Request, user domain.User, message string, recoveryCodes []string) {
	if middleware.IsHTMX(r) || len(recoveryCodes) > 0 {
		mfaUser, err := h.store.GetUserByID(r.Context(), user.ID)
		if err != nil {
			h.renderError(w, r, http.StatusInternalServerError, "failed to load mfa settings")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = h.renderer.Render(w, "admin-mfa", h.mfaData(r, user, mfaUser, message, recoveryCodes))
		return
	}
	http.Redirect(w, r, "/admin/mfa?msg="+url.QueryEscape(message), http.StatusSeeOther)
}

// renderMFAPage explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) renderMFAPage(w http.ResponseWriter, r *http.Request, user domain.User, mfaUser domain.User, message string, recoveryCodes []string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "admin-mfa", h.mfaData(r, user, mfaUser, message, recoveryCodes))
}

// mfaData explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) mfaData(r *http.Request, user domain.User, mfaUser domain.User, message string, recoveryCodes []string) map[string]any {
	recoveryCount := 0
	if mfaUser.MFAEnabled {
		if count, err := h.store.CountUnusedRecoveryCodes(r.Context(), user.ID); err == nil {
			recoveryCount = count
		}
	}
	setupURI := ""
	if strings.TrimSpace(mfaUser.MFASecret) != "" {
		setupURI = auth.OTPAuthURL("kcNotes", user.Email, mfaUser.MFASecret)
	}
	return map[string]any{
		"Title":             "MFA Security",
		"CSRFToken":         middleware.CSRFToken(r),
		"UserEmail":         user.Email,
		"UserRole":          string(user.Role),
		"MFAEnabled":        mfaUser.MFAEnabled,
		"MFASecret":         mfaUser.MFASecret,
		"MFASetupURI":       setupURI,
		"RecoveryCodeCount": recoveryCount,
		"NewRecoveryCodes":  recoveryCodes,
		"FlashMessage":      message,
	}
}

// nowUTC explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func nowUTC() time.Time {
	return time.Now().UTC()
}

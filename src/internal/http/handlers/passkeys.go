package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"kcnotes/internal/auth"
	"kcnotes/internal/domain"
	"kcnotes/internal/http/middleware"
	storesqlite "kcnotes/internal/store/sqlite"
)

const passkeyChallengeTTL = 5 * time.Minute

func (h *Admin) SetupRegistrationStart(w http.ResponseWriter, r *http.Request) {
	if h.webAuthn == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "passkey setup is not configured")
		return
	}
	count, err := h.store.CountUsers(r.Context())
	if err != nil || count != 0 {
		writeJSONError(w, http.StatusNotFound, "setup is unavailable")
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	nickname := strings.TrimSpace(r.FormValue("nickname"))
	if email == "" || !strings.Contains(email, "@") {
		writeJSONError(w, http.StatusUnprocessableEntity, "valid email is required")
		return
	}
	userID, err := randomID()
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "setup is unavailable")
		return
	}
	user := domain.User{ID: userID, Email: email, Role: domain.RoleAdmin}
	creation, session, err := h.webAuthn.BeginRegistration(auth.WebAuthnUser{User: user}, webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired))
	if err != nil {
		h.logger.Warn("start first-admin passkey registration", "error", err)
		writeJSONError(w, http.StatusServiceUnavailable, "setup is unavailable")
		return
	}
	challengeID, err := h.saveChallenge(r, user.ID+"|"+email+"|"+nickname, "setup_registration", session)
	if err != nil {
		h.logger.Warn("save first-admin passkey challenge", "error", err)
		writeJSONError(w, http.StatusServiceUnavailable, "setup is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"challenge_id": challengeID, "user_id": user.ID, "email": email, "nickname": nickname, "publicKey": creation.Response})
}

func (h *Admin) SetupRegistrationFinish(w http.ResponseWriter, r *http.Request) {
	challengeID := strings.TrimSpace(r.URL.Query().Get("challenge_id"))
	challenge, session, ok := h.consumeChallenge(w, r, challengeID, "setup_registration")
	if !ok {
		return
	}
	userID, rest, _ := strings.Cut(challenge.UserID, "|")
	email, nickname, _ := strings.Cut(rest, "|")
	if email == "" || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "setup failed")
		return
	}
	user := domain.User{ID: userID, Email: email, Role: domain.RoleAdmin}
	credential, err := h.webAuthn.FinishRegistration(auth.WebAuthnUser{User: user}, session, r)
	if err != nil {
		h.logger.Warn("finish first-admin passkey registration", "error", err)
		writeJSONError(w, http.StatusUnauthorized, "setup failed")
		return
	}
	credentialID, err := randomID()
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "setup failed")
		return
	}
	domainCredential, err := auth.CredentialToDomain(credentialID, user.ID, nickname, credential)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "setup failed")
		return
	}
	if err := h.store.CreateFirstAdminWithPasskey(r.Context(), user, domainCredential); err != nil {
		writeJSONError(w, http.StatusConflict, "setup is unavailable")
		return
	}
	if err := h.createAdminSession(w, r, user); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "setup failed")
		return
	}
	h.auditEvent(r, user.ID, "first_admin_setup", "user", user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"redirect": "/admin"})
}

func (h *Admin) PasskeysPage(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	credentials, err := h.store.ListPasskeyCredentials(r.Context(), user.ID)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load passkeys")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "admin-passkeys", map[string]any{
		"Title":        "Passkeys",
		"CSRFToken":    middleware.CSRFToken(r),
		"UserEmail":    user.Email,
		"UserRole":     string(user.Role),
		"Passkeys":     credentials,
		"FlashMessage": strings.TrimSpace(r.URL.Query().Get("msg")),
	})
}

func (h *Admin) PasskeyRegistrationStart(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	h.startRegistrationForUser(w, r, user, "passkey_registration")
}

func (h *Admin) PasskeyRegistrationFinish(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	h.finishRegistrationForUser(w, r, user, "passkey_registration", func(credential domain.PasskeyCredential) error {
		return h.store.CreatePasskeyCredential(r.Context(), credential)
	})
}

func (h *Admin) EnrollmentPage(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	_, user, err := h.store.GetEnrollmentInvitationByTokenHash(r.Context(), auth.TokenHash(token), time.Now().UTC())
	if token == "" || err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "admin-enroll", map[string]any{
		"Title":     "Passkey Enrollment",
		"CSRFToken": middleware.CSRFToken(r),
		"Token":     token,
		"Email":     user.Email,
	})
}

func (h *Admin) EnrollmentRegistrationStart(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.FormValue("token"))
	invite, user, err := h.store.GetEnrollmentInvitationByTokenHash(r.Context(), auth.TokenHash(token), time.Now().UTC())
	if token == "" || err != nil {
		writeJSONError(w, http.StatusNotFound, "enrollment is unavailable")
		return
	}
	_ = invite
	h.startRegistrationForUser(w, r, user, "enrollment_registration")
}

func (h *Admin) EnrollmentRegistrationFinish(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	invite, user, err := h.store.GetEnrollmentInvitationByTokenHash(r.Context(), auth.TokenHash(token), time.Now().UTC())
	if token == "" || err != nil {
		writeJSONError(w, http.StatusNotFound, "enrollment is unavailable")
		return
	}
	h.finishRegistrationForUser(w, r, user, "enrollment_registration", func(credential domain.PasskeyCredential) error {
		return h.store.CompleteEnrollmentInvitation(r.Context(), invite.ID, credential)
	})
}

func (h *Admin) RenamePasskey(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	updated, err := h.store.RenamePasskeyCredential(r.Context(), user.ID, id, r.FormValue("nickname"))
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to update passkey")
		return
	}
	if !updated {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/admin/passkeys?msg=Passkey+updated", http.StatusSeeOther)
}

func (h *Admin) DeletePasskey(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	deleted, err := h.store.DeletePasskeyCredential(r.Context(), user.ID, strings.TrimSpace(r.PathValue("id")))
	if err == storesqlite.ErrConflict {
		http.Redirect(w, r, "/admin/passkeys?msg=Keep+at+least+one+passkey", http.StatusSeeOther)
		return
	}
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to delete passkey")
		return
	}
	if !deleted {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/admin/passkeys?msg=Passkey+deleted", http.StatusSeeOther)
}

func (h *Admin) startRegistrationForUser(w http.ResponseWriter, r *http.Request, user domain.User, challengeType string) {
	if h.webAuthn == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "passkey registration is not configured")
		return
	}
	credentials, err := h.store.ListPasskeyCredentials(r.Context(), user.ID)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "passkey registration is unavailable")
		return
	}
	webAuthnCredentials, err := auth.DomainCredentialsToWebAuthn(credentials)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "passkey registration is unavailable")
		return
	}
	nickname := strings.TrimSpace(r.FormValue("nickname"))
	creation, session, err := h.webAuthn.BeginRegistration(auth.WebAuthnUser{User: user, Credential: webAuthnCredentials}, webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired))
	if err != nil {
		h.logger.Warn("start passkey registration", "error", err, "user_id", user.ID, "challenge_type", challengeType)
		writeJSONError(w, http.StatusServiceUnavailable, "passkey registration is unavailable")
		return
	}
	challengeID, err := h.saveChallenge(r, user.ID+"|"+nickname, challengeType, session)
	if err != nil {
		h.logger.Warn("save passkey challenge", "error", err, "user_id", user.ID, "challenge_type", challengeType)
		writeJSONError(w, http.StatusServiceUnavailable, "passkey registration is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"challenge_id": challengeID, "nickname": nickname, "publicKey": creation.Response})
}

func (h *Admin) finishRegistrationForUser(w http.ResponseWriter, r *http.Request, user domain.User, challengeType string, persist func(domain.PasskeyCredential) error) {
	challengeID := strings.TrimSpace(r.URL.Query().Get("challenge_id"))
	challenge, session, ok := h.consumeChallenge(w, r, challengeID, challengeType)
	if !ok {
		return
	}
	challengeUserID, nickname, _ := strings.Cut(challenge.UserID, "|")
	if challengeUserID != user.ID {
		writeJSONError(w, http.StatusUnauthorized, "passkey registration failed")
		return
	}
	credentials, err := h.store.ListPasskeyCredentials(r.Context(), user.ID)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "passkey registration failed")
		return
	}
	webAuthnCredentials, err := auth.DomainCredentialsToWebAuthn(credentials)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "passkey registration failed")
		return
	}
	credential, err := h.webAuthn.FinishRegistration(auth.WebAuthnUser{User: user, Credential: webAuthnCredentials}, session, r)
	if err != nil {
		h.logger.Warn("finish passkey registration", "error", err, "user_id", user.ID, "challenge_type", challengeType)
		writeJSONError(w, http.StatusUnauthorized, "passkey registration failed")
		return
	}
	id, err := randomID()
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "passkey registration failed")
		return
	}
	domainCredential, err := auth.CredentialToDomain(id, user.ID, nickname, credential)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "passkey registration failed")
		return
	}
	if err := persist(domainCredential); err != nil {
		writeJSONError(w, http.StatusConflict, "passkey registration failed")
		return
	}
	h.auditEvent(r, user.ID, "passkey_registered", "user", user.ID)
	redirect := "/admin/passkeys"
	if challengeType == "enrollment_registration" {
		if err := h.createAdminSession(w, r, user); err != nil {
			writeJSONError(w, http.StatusServiceUnavailable, "passkey registration failed")
			return
		}
		redirect = "/admin"
	}
	writeJSON(w, http.StatusOK, map[string]any{"redirect": redirect})
}

func (h *Admin) saveChallenge(r *http.Request, userID, challengeType string, session *webauthn.SessionData) (string, error) {
	challengeID, err := randomID()
	if err != nil {
		return "", err
	}
	rawSession, err := auth.EncodeSession(session)
	if err != nil {
		return "", err
	}
	return challengeID, h.store.CreateWebAuthnChallenge(r.Context(), domain.WebAuthnChallenge{
		ID:        challengeID,
		UserID:    userID,
		Type:      challengeType,
		Session:   rawSession,
		ExpiresAt: passkeyChallengeExpiresAt(session),
	})
}

func passkeyChallengeExpiresAt(session *webauthn.SessionData) time.Time {
	if session != nil && !session.Expires.IsZero() {
		return session.Expires
	}
	return time.Now().UTC().Add(passkeyChallengeTTL)
}

func (h *Admin) consumeChallenge(w http.ResponseWriter, r *http.Request, id, challengeType string) (domain.WebAuthnChallenge, webauthn.SessionData, bool) {
	if h.webAuthn == nil || id == "" {
		writeJSONError(w, http.StatusUnauthorized, "passkey challenge failed")
		return domain.WebAuthnChallenge{}, webauthn.SessionData{}, false
	}
	challenge, err := h.store.ConsumeWebAuthnChallenge(r.Context(), id, challengeType, time.Now().UTC())
	if err != nil {
		h.logger.Warn("consume passkey challenge", "error", err, "challenge_id", id, "challenge_type", challengeType)
		writeJSONError(w, http.StatusUnauthorized, "passkey challenge failed")
		return domain.WebAuthnChallenge{}, webauthn.SessionData{}, false
	}
	session, err := auth.DecodeSession(challenge.Session)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "passkey challenge failed")
		return domain.WebAuthnChallenge{}, webauthn.SessionData{}, false
	}
	return challenge, session, true
}

func newEnrollmentToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("read random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

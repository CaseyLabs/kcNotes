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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"kcnotes/internal/auth"
	"kcnotes/internal/domain"
	"kcnotes/internal/http/middleware"
	"kcnotes/internal/http/views"
	"kcnotes/internal/observability"
	storesqlite "kcnotes/internal/store/sqlite"

	"github.com/go-webauthn/webauthn/webauthn"
)

type Admin struct {
	renderer        *views.Renderer
	logger          *slog.Logger
	store           adminStore
	uploadDir       string
	sessionTTL      time.Duration
	cookieName      string
	cookiePath      string
	cookieSecure    bool
	ipResolver      *middleware.ClientIPResolver
	failedAuditLogs *failedAuditLimiter
	loginFailures   *loginFailureTracker
	maxMediaUserB   int64
	maxMediaTotalB  int64
	webAuthn        *webauthn.WebAuthn
	metrics         *observability.Metrics
}

type adminStore interface {
	CountUsers(ctx context.Context) (int, error)
	CreateFirstAdminWithPasskey(ctx context.Context, user domain.User, credential domain.PasskeyCredential) error
	CreatePasskeyCredential(ctx context.Context, credential domain.PasskeyCredential) error
	UpdatePasskeyCredential(ctx context.Context, credential domain.PasskeyCredential) error
	ListPasskeyCredentials(ctx context.Context, userID string) ([]domain.PasskeyCredential, error)
	GetUserByCredentialID(ctx context.Context, credentialID []byte) (domain.User, error)
	RenamePasskeyCredential(ctx context.Context, userID, credentialID, nickname string) (bool, error)
	DeletePasskeyCredential(ctx context.Context, userID, credentialID string) (bool, error)
	CreateWebAuthnChallenge(ctx context.Context, challenge domain.WebAuthnChallenge) error
	ConsumeWebAuthnChallenge(ctx context.Context, id, challengeType string, now time.Time) (domain.WebAuthnChallenge, error)
	CreateEnrollmentInvitation(ctx context.Context, user domain.User, invite domain.EnrollmentInvitation) error
	GetEnrollmentInvitationByTokenHash(ctx context.Context, tokenHash string, now time.Time) (domain.EnrollmentInvitation, domain.User, error)
	CompleteEnrollmentInvitation(ctx context.Context, inviteID string, credential domain.PasskeyCredential) error
	ListUsers(ctx context.Context) ([]domain.User, error)
	UpdateUserRoleDisabled(ctx context.Context, id string, role domain.Role, disabled bool) (bool, error)
	GetSettings(ctx context.Context, keys []string) (map[string]string, error)
	UpsertSettings(ctx context.Context, entries map[string]string) error
	CreateSession(ctx context.Context, sessionID, userID, csrfToken string, expiresAt time.Time) error
	DeleteSession(ctx context.Context, sessionID string) error
	CreateAuditEvent(ctx context.Context, id, actorUserID, action, entityType, entityID, ip, userAgent string) error
	ListAuditEvents(ctx context.Context, filter storesqlite.AuditListFilter) ([]domain.AuditEvent, int, error)
	ListPosts(ctx context.Context, filter storesqlite.PostListFilter) ([]domain.Post, int, error)
	GetPostByID(ctx context.Context, id string) (domain.Post, error)
	CreatePost(ctx context.Context, post domain.Post) error
	UpdatePost(ctx context.Context, post domain.Post, actor domain.User) (bool, error)
	UpsertAutosaveSnapshot(ctx context.Context, snapshot domain.AutosaveSnapshot, actor domain.User) (bool, error)
	GetLatestAutosaveSnapshot(ctx context.Context, postID, authorID string) (domain.AutosaveSnapshot, error)
	DismissAutosaveSnapshot(ctx context.Context, postID, authorID string) (bool, error)
	SetPostStatus(ctx context.Context, id string, status domain.PostStatus, publishedAt *time.Time, actor domain.User) (bool, error)
	SoftDeletePost(ctx context.Context, id string, actor domain.User) (bool, error)
	CreateMedia(ctx context.Context, media domain.Media) error
	CreateMediaWithVariants(ctx context.Context, media domain.Media, variants []domain.MediaVariant) error
	AttachMediaToAsset(ctx context.Context, media domain.Media) error
	GetMediaAssetBySHA256(ctx context.Context, sha256 string) (domain.MediaAsset, error)
	GetMediaByAssetAndUser(ctx context.Context, assetID, userID string) (domain.Media, error)
	MediaUsage(ctx context.Context, userID string) (userBytes, totalBytes int64, err error)
	ListMediaForUser(ctx context.Context, limit int, user domain.User) ([]domain.Media, error)
	GetMediaByIDForUser(ctx context.Context, id string, user domain.User) (domain.Media, error)
}

type AdminConfig struct {
	SessionTTL            time.Duration
	CookieName            string
	CookiePath            string
	CookieSecure          bool
	UploadDir             string
	IPResolver            *middleware.ClientIPResolver
	MediaUserQuotaBytes   int64
	MediaTotalQuotaBytes  int64
	LoginLockoutThreshold int
	LoginLockoutWindow    time.Duration
	LoginLockoutDuration  time.Duration
	WebAuthn              *webauthn.WebAuthn
	Metrics               *observability.Metrics
}

// NewAdmin explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func NewAdmin(renderer *views.Renderer, logger *slog.Logger, store adminStore, cfg AdminConfig) *Admin {
	lockoutThreshold := cfg.LoginLockoutThreshold
	if lockoutThreshold <= 0 {
		lockoutThreshold = 8
	}
	lockoutWindow := cfg.LoginLockoutWindow
	if lockoutWindow <= 0 {
		lockoutWindow = 15 * time.Minute
	}
	lockoutDuration := cfg.LoginLockoutDuration
	if lockoutDuration <= 0 {
		lockoutDuration = 15 * time.Minute
	}
	return &Admin{
		renderer:        renderer,
		logger:          logger,
		store:           store,
		uploadDir:       cfg.UploadDir,
		sessionTTL:      cfg.SessionTTL,
		cookieName:      cfg.CookieName,
		cookiePath:      cfg.CookiePath,
		cookieSecure:    cfg.CookieSecure,
		ipResolver:      cfg.IPResolver,
		failedAuditLogs: newFailedAuditLimiter(1 * time.Minute),
		loginFailures:   newLoginFailureTracker(lockoutThreshold, lockoutWindow, lockoutDuration),
		maxMediaUserB:   cfg.MediaUserQuotaBytes,
		maxMediaTotalB:  cfg.MediaTotalQuotaBytes,
		webAuthn:        cfg.WebAuthn,
		metrics:         cfg.Metrics,
	}
}

// Dashboard explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) Dashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	user, _ := middleware.CurrentUser(r)
	postFilter := storesqlite.PostListFilter{Page: 1, PageSize: 5}
	if user.Role == domain.RoleAuthor {
		postFilter.AuthorID = user.ID
	}
	recentPosts, contentTotal, err := h.store.ListPosts(r.Context(), postFilter)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load dashboard")
		return
	}
	_, publishedTotal, err := h.store.ListPosts(r.Context(), storesqlite.PostListFilter{
		Status:   domain.PostStatusPublished,
		Page:     1,
		PageSize: 1,
		AuthorID: postFilter.AuthorID,
	})
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load dashboard")
		return
	}
	_, draftTotal, err := h.store.ListPosts(r.Context(), storesqlite.PostListFilter{
		Status:   domain.PostStatusDraft,
		Page:     1,
		PageSize: 1,
		AuthorID: postFilter.AuthorID,
	})
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load dashboard")
		return
	}
	media, err := h.store.ListMediaForUser(r.Context(), 4, user)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load dashboard")
		return
	}
	userTotal := 0
	if user.Role == domain.RoleAdmin {
		userTotal, err = h.store.CountUsers(r.Context())
		if err != nil {
			h.renderError(w, r, http.StatusInternalServerError, "failed to load dashboard")
			return
		}
	}
	_ = h.renderer.Render(w, "admin-dashboard", map[string]any{
		"Title":          "Admin Dashboard",
		"CSRFToken":      middleware.CSRFToken(r),
		"UserEmail":      user.Email,
		"UserRole":       string(user.Role),
		"IsAdmin":        user.Role == domain.RoleAdmin,
		"RecentPosts":    recentPosts,
		"RecentMedia":    media,
		"ContentTotal":   contentTotal,
		"PublishedTotal": publishedTotal,
		"DraftTotal":     draftTotal,
		"MediaTotal":     len(media),
		"UserTotal":      userTotal,
	})
}

// LoginPage explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) LoginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.CurrentUser(r); ok {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "admin-login", map[string]any{
		"Title":     "Login",
		"CSRFToken": middleware.CSRFToken(r),
	})
}

// Login explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) Login(w http.ResponseWriter, r *http.Request) {
	h.renderLoginError(w, r, "Use your passkey to sign in")
}

func (h *Admin) SetupPage(w http.ResponseWriter, r *http.Request) {
	count, err := h.store.CountUsers(r.Context())
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load setup")
		return
	}
	if count != 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "admin-setup", map[string]any{
		"Title":     "First Admin Setup",
		"CSRFToken": middleware.CSRFToken(r),
	})
}

func (h *Admin) PasskeyLoginStart(w http.ResponseWriter, r *http.Request) {
	if h.webAuthn == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "passkey login is not configured")
		return
	}
	assertion, session, err := h.webAuthn.BeginDiscoverableLogin(passkeyLoginOptions()...)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "passkey login is unavailable")
		return
	}
	challengeID, err := randomID()
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "passkey login is unavailable")
		return
	}
	rawSession, err := auth.EncodeSession(session)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "passkey login is unavailable")
		return
	}
	if err := h.store.CreateWebAuthnChallenge(r.Context(), domain.WebAuthnChallenge{
		ID:        challengeID,
		Type:      "login",
		Session:   rawSession,
		ExpiresAt: passkeyChallengeExpiresAt(session),
	}); err != nil {
		h.logger.Error("create passkey login challenge", "error", err)
		writeJSONError(w, http.StatusServiceUnavailable, "passkey login is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"challenge_id": challengeID, "publicKey": assertion.Response})
}

func (h *Admin) PasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	if h.webAuthn == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "passkey login is not configured")
		return
	}
	challengeID := strings.TrimSpace(r.URL.Query().Get("challenge_id"))
	if challengeID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing challenge")
		return
	}
	loginKey := loginFailureKey(h.clientIP(r))
	if h.loginFailures.IsLocked(loginKey) {
		if h.metrics != nil {
			h.metrics.RecordLoginLockoutHit()
			h.metrics.RecordLoginFailure()
		}
		h.auditFailedLogin(r, "", "")
		writeJSONError(w, http.StatusUnauthorized, "passkey login failed")
		return
	}
	challenge, err := h.store.ConsumeWebAuthnChallenge(r.Context(), challengeID, "login", time.Now().UTC())
	if err != nil {
		h.recordPasskeyLoginFailure(loginKey)
		h.auditFailedLogin(r, "", "")
		writeJSONError(w, http.StatusUnauthorized, "passkey login failed")
		return
	}
	session, err := auth.DecodeSession(challenge.Session)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "passkey login failed")
		return
	}
	discoveredUser, credential, err := h.webAuthn.FinishPasskeyLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		user, err := h.store.GetUserByCredentialID(r.Context(), rawID)
		if err != nil {
			return nil, err
		}
		credentials, err := h.store.ListPasskeyCredentials(r.Context(), user.ID)
		if err != nil {
			return nil, err
		}
		webAuthnCredentials, err := auth.DomainCredentialsToWebAuthn(credentials)
		if err != nil {
			return nil, err
		}
		return auth.WebAuthnUser{User: user, Credential: webAuthnCredentials}, nil
	}, session, r)
	if err != nil {
		h.recordPasskeyLoginFailure(loginKey)
		h.auditFailedLogin(r, "", "")
		writeJSONError(w, http.StatusUnauthorized, "passkey login failed")
		return
	}
	webAuthnUser, ok := discoveredUser.(auth.WebAuthnUser)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "passkey login failed")
		return
	}
	user := webAuthnUser.User
	if user.Disabled {
		h.recordPasskeyLoginFailure(loginKey)
		h.auditFailedLogin(r, user.Email, user.ID)
		writeJSONError(w, http.StatusUnauthorized, "passkey login failed")
		return
	}
	domainCredential, err := auth.CredentialToDomain("", user.ID, "", credential)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "passkey login failed")
		return
	}
	now := time.Now().UTC()
	domainCredential.LastUsedAt = &now
	if err := h.store.UpdatePasskeyCredential(r.Context(), domainCredential); err != nil {
		h.logger.Warn("update passkey credential", "error", err, "user_id", user.ID)
	}
	h.loginFailures.RecordSuccess(loginKey)
	if h.metrics != nil {
		h.metrics.RecordLoginSuccess()
	}
	if err := h.createAdminSession(w, r, user); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "passkey login is unavailable")
		return
	}
	h.auditEvent(r, user.ID, "login_success", "user", user.ID)
	writeJSON(w, http.StatusOK, map[string]any{"redirect": "/admin"})
}

func (h *Admin) createAdminSession(w http.ResponseWriter, r *http.Request, user domain.User) error {
	sessionID, err := randomID()
	if err != nil {
		h.logger.Error("generate session id", "error", err)
		return err
	}
	csrfToken := middleware.CSRFToken(r)
	expiresAt := time.Now().UTC().Add(h.sessionTTL)
	if err := h.store.CreateSession(r.Context(), sessionID, user.ID, csrfToken, expiresAt); err != nil {
		h.logger.Error("create session", "error", err, "user_id", user.ID)
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    sessionID,
		Path:     h.cookiePath,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (h *Admin) recordPasskeyLoginFailure(loginKey string) {
	locked := h.loginFailures.RecordFailure(loginKey)
	if h.metrics == nil {
		return
	}
	h.metrics.RecordLoginFailure()
	if locked {
		h.metrics.RecordLoginLockoutTransition()
	}
}

// Logout explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) Logout(w http.ResponseWriter, r *http.Request) {
	if user, ok := middleware.CurrentUser(r); ok {
		h.auditEvent(r, user.ID, "logout", "user", user.ID)
	}
	cookie, err := r.Cookie(h.cookieName)
	if err == nil && cookie.Value != "" {
		_ = h.store.DeleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    "",
		Path:     h.cookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// renderLoginError explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) renderLoginError(w http.ResponseWriter, r *http.Request, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_ = h.renderer.Render(w, "admin-login", map[string]any{
		"Title":     "Login",
		"CSRFToken": middleware.CSRFToken(r),
		"Error":     message,
	})
}

// auditFailedLogin explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) auditFailedLogin(r *http.Request, email, actorID string) {
	key := strings.ToLower(strings.TrimSpace(email)) + "|" + h.clientIP(r)
	if !h.failedAuditLogs.Allow(key) {
		return
	}
	h.auditEvent(r, actorID, "login_failed", "user", "")
}

// auditEvent explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) auditEvent(r *http.Request, actorID, action, entityType, entityID string) {
	eventID, err := randomID()
	if err != nil {
		h.logger.Warn("audit event id generation failed", "error", err, "action", action)
		return
	}
	if err := h.store.CreateAuditEvent(
		r.Context(),
		eventID,
		actorID,
		action,
		entityType,
		entityID,
		h.clientIP(r),
		r.UserAgent(),
	); err != nil {
		h.logger.Warn("audit event failed", "error", err, "action", action)
	}
}

// clientIP explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) clientIP(r *http.Request) string {
	if h.ipResolver == nil {
		return r.RemoteAddr
	}
	return h.ipResolver.ClientIP(r)
}

// randomID explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// loginFailureKey explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func loginFailureKey(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "_empty"
	}
	return email
}

type failedAuditLimiter struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]time.Time
}

// newFailedAuditLimiter explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func newFailedAuditLimiter(ttl time.Duration) *failedAuditLimiter {
	return &failedAuditLimiter{
		ttl:     ttl,
		entries: make(map[string]time.Time),
	}
}

// Allow explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (l *failedAuditLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if expiresAt, found := l.entries[key]; found && now.Before(expiresAt) {
		return false
	}
	l.entries[key] = now.Add(l.ttl)
	return true
}

type loginFailureTracker struct {
	mu        sync.Mutex
	threshold int
	window    time.Duration
	duration  time.Duration
	now       func() time.Time
	entries   map[string]loginFailureEntry
}

type loginFailureEntry struct {
	failures    int
	firstFailed time.Time
	lockedUntil time.Time
}

// newLoginFailureTracker explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func newLoginFailureTracker(threshold int, window, duration time.Duration) *loginFailureTracker {
	return &loginFailureTracker{
		threshold: threshold,
		window:    window,
		duration:  duration,
		now:       time.Now,
		entries:   make(map[string]loginFailureEntry),
	}
}

// IsLocked explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (t *loginFailureTracker) IsLocked(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	entry, ok := t.entries[key]
	if !ok {
		return false
	}
	now := t.now()
	if now.Before(entry.lockedUntil) {
		return true
	}
	if !entry.lockedUntil.IsZero() {
		delete(t.entries, key)
	}
	return false
}

// RecordFailure explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (t *loginFailureTracker) RecordFailure(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	entry := t.entries[key]
	if now.Before(entry.lockedUntil) {
		return false
	}
	if entry.firstFailed.IsZero() || now.Sub(entry.firstFailed) > t.window {
		entry.firstFailed = now
		entry.failures = 1
		entry.lockedUntil = time.Time{}
		t.entries[key] = entry
		return false
	}
	entry.failures++
	locked := false
	if entry.failures >= t.threshold {
		entry.lockedUntil = now.Add(t.duration)
		locked = true
	}
	t.entries[key] = entry
	return locked
}

// RecordSuccess explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (t *loginFailureTracker) RecordSuccess(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
}

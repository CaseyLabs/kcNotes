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
	storesqlite "kcnotes/internal/store/sqlite"
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
}

type adminStore interface {
	GetUserByEmail(ctx context.Context, email string) (domain.User, error)
	GetUserByID(ctx context.Context, id string) (domain.User, error)
	CreateUser(ctx context.Context, user domain.User) error
	ListUsers(ctx context.Context) ([]domain.User, error)
	UpdateUserRoleDisabled(ctx context.Context, id string, role domain.Role, disabled bool) (bool, error)
	GetSettings(ctx context.Context, keys []string) (map[string]string, error)
	UpsertSettings(ctx context.Context, entries map[string]string) error
	SetUserMFASecret(ctx context.Context, userID, secret string) (bool, error)
	EnableUserMFA(ctx context.Context, userID string) (bool, error)
	DisableUserMFA(ctx context.Context, userID string) (bool, error)
	ReplaceRecoveryCodeHashes(ctx context.Context, userID string, hashes []string) error
	ConsumeRecoveryCodeHash(ctx context.Context, userID, hash string) (bool, error)
	CountUnusedRecoveryCodes(ctx context.Context, userID string) (int, error)
	CreateSession(ctx context.Context, sessionID, userID, csrfToken string, expiresAt time.Time) error
	DeleteSession(ctx context.Context, sessionID string) error
	CreateAuditEvent(ctx context.Context, id, actorUserID, action, entityType, entityID, ip, userAgent string) error
	ListAuditEvents(ctx context.Context, filter storesqlite.AuditListFilter) ([]domain.AuditEvent, int, error)
	ListPosts(ctx context.Context, filter storesqlite.PostListFilter) ([]domain.Post, int, error)
	GetPostByID(ctx context.Context, id string) (domain.Post, error)
	CreatePost(ctx context.Context, post domain.Post) error
	UpdatePost(ctx context.Context, post domain.Post, actor domain.User) (bool, error)
	SetPostStatus(ctx context.Context, id string, status domain.PostStatus, publishedAt *time.Time, actor domain.User) (bool, error)
	SoftDeletePost(ctx context.Context, id string, actor domain.User) (bool, error)
	CreateMedia(ctx context.Context, media domain.Media) error
	MediaUsage(ctx context.Context, userID string) (userBytes, totalBytes int64, err error)
	ListMedia(ctx context.Context, limit int) ([]domain.Media, error)
	GetMediaByID(ctx context.Context, id string) (domain.Media, error)
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
	}
}

// Dashboard explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) Dashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	user, _ := middleware.CurrentUser(r)
	_ = h.renderer.Render(w, "admin-dashboard", map[string]any{
		"Title":     "Admin Dashboard",
		"CSRFToken": middleware.CSRFToken(r),
		"UserEmail": user.Email,
		"UserRole":  string(user.Role),
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
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "invalid form")
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	password := r.FormValue("password")
	loginKey := loginFailureKey(email)
	if h.loginFailures.IsLocked(loginKey) {
		h.auditFailedLogin(r, email, "")
		h.renderLoginError(w, r, "Invalid email or password")
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), email)
	if err != nil {
		h.loginFailures.RecordFailure(loginKey)
		h.auditFailedLogin(r, email, "")
		h.renderLoginError(w, r, "Invalid email or password")
		return
	}
	if user.Disabled {
		h.loginFailures.RecordFailure(loginKey)
		h.auditFailedLogin(r, email, user.ID)
		h.renderLoginError(w, r, "Invalid email or password")
		return
	}

	ok, err := auth.VerifyPassword(password, user.PasswordHash)
	if err != nil {
		h.logger.Error("verify password", "error", err, "email", email)
		h.renderLoginError(w, r, "Login unavailable")
		return
	}
	if !ok {
		h.loginFailures.RecordFailure(loginKey)
		h.auditFailedLogin(r, email, user.ID)
		h.renderLoginError(w, r, "Invalid email or password")
		return
	}
	if user.MFAEnabled {
		code := strings.TrimSpace(r.FormValue("otp"))
		mfaOK := auth.VerifyTOTP(user.MFASecret, code, time.Now().UTC())
		if !mfaOK {
			used, err := h.store.ConsumeRecoveryCodeHash(r.Context(), user.ID, auth.RecoveryCodeHash(code))
			if err != nil {
				h.logger.Error("consume recovery code", "error", err, "user_id", user.ID)
				h.renderLoginError(w, r, "Login unavailable")
				return
			}
			mfaOK = used
			if used {
				h.auditEvent(r, user.ID, "mfa_recovery_code_used", "user", user.ID)
			}
		}
		if !mfaOK {
			h.loginFailures.RecordFailure(loginKey)
			h.auditFailedLogin(r, email, user.ID)
			h.renderLoginError(w, r, "Invalid email or password")
			return
		}
	}
	h.loginFailures.RecordSuccess(loginKey)

	sessionID, err := randomID()
	if err != nil {
		h.logger.Error("generate session id", "error", err)
		h.renderLoginError(w, r, "Login unavailable")
		return
	}
	csrfToken := middleware.CSRFToken(r)
	expiresAt := time.Now().UTC().Add(h.sessionTTL)
	if err := h.store.CreateSession(r.Context(), sessionID, user.ID, csrfToken, expiresAt); err != nil {
		h.logger.Error("create session", "error", err, "user_id", user.ID)
		h.renderLoginError(w, r, "Login unavailable")
		return
	}
	h.auditEvent(r, user.ID, "login_success", "user", user.ID)

	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    sessionID,
		Path:     h.cookiePath,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
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
func (t *loginFailureTracker) RecordFailure(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	entry := t.entries[key]
	if now.Before(entry.lockedUntil) {
		return
	}
	if entry.firstFailed.IsZero() || now.Sub(entry.firstFailed) > t.window {
		entry.firstFailed = now
		entry.failures = 1
		entry.lockedUntil = time.Time{}
		t.entries[key] = entry
		return
	}
	entry.failures++
	if entry.failures >= t.threshold {
		entry.lockedUntil = now.Add(t.duration)
	}
	t.entries[key] = entry
}

// RecordSuccess explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (t *loginFailureTracker) RecordSuccess(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
}

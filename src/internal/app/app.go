// TEACHING NOTES:
// This file is part of the app wiring layer (dependency construction + config).
// In idiomatic Go, wiring is usually explicit: dependencies are passed as values
// rather than hidden behind global state.
// Useful Go concepts to notice:
// 1. Structs group related dependencies into a single composable type.
// 2. Constructors return `(value, error)` so callers must handle failures.
// 3. Package boundaries (`internal/...`) enforce architecture constraints.
// 4. Unexported fields/functions keep implementation details private.
// 5. Small pure helpers make configuration parsing safer and easier to test.
package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"kcnotes/internal/auth"
	"kcnotes/internal/content"
	"kcnotes/internal/domain"
	"kcnotes/internal/http/handlers"
	"kcnotes/internal/http/middleware"
	"kcnotes/internal/http/routes"
	"kcnotes/internal/http/views"
	"kcnotes/internal/publish"
	storesqlite "kcnotes/internal/store/sqlite"
)

type App struct {
	cfg      Config
	logger   *slog.Logger
	db       *sql.DB
	dbConn   *storesqlite.Connection
	renderer *views.Renderer
	ipRes    *middleware.ClientIPResolver
	jobStore *storesqlite.AuthStore

	jobsPollInterval     time.Duration
	autosaveRetention    time.Duration
	autosaveCleanupEvery time.Duration
	jobsMetrics          *jobsMetrics
}

// New explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func New(cfg Config, logger *slog.Logger) (*App, error) {
	conn, err := storesqlite.Open(storesqlite.OpenConfig{
		Mode:                  storesqlite.DBMode(cfg.DBMode),
		DBPath:                cfg.DBPath,
		DatabaseURL:           cfg.DatabaseURL,
		DatabaseAuthToken:     cfg.DatabaseAuthToken,
		ReplicaSyncInterval:   cfg.ReplicaSyncInterval,
		ReplicaReadYourWrites: cfg.ReplicaReadYourWrites,
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	renderer, err := views.NewRenderer(cfg.TemplateGlob)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("init renderer: %w", err)
	}
	ipRes, err := middleware.NewClientIPResolver(cfg.TrustedProxyCIDRs)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("init client ip resolver: %w", err)
	}
	if cfg.EnforceTrustedProxies && cfg.AppEnv != "dev" && len(cfg.TrustedProxyCIDRs) == 0 {
		_ = conn.Close()
		return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS must be set when ENFORCE_TRUSTED_PROXY_CIDRS=true in %s", cfg.AppEnv)
	}

	return &App{
		cfg:                  cfg,
		logger:               logger,
		db:                   conn.DB,
		dbConn:               conn,
		renderer:             renderer,
		ipRes:                ipRes,
		jobsPollInterval:     cfg.JobsPollInterval,
		autosaveRetention:    cfg.AutosaveRetention,
		autosaveCleanupEvery: cfg.AutosaveCleanupEvery,
	}, nil
}

// Close explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (a *App) Close() {
	if a.dbConn != nil {
		_ = a.dbConn.Close()
	}
}

// Migrate explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (a *App) Migrate(ctx context.Context) error {
	return storesqlite.RunMigrations(ctx, a.db)
}

// Publish explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (a *App) Publish(ctx context.Context) (publish.Result, error) {
	store := storesqlite.NewAuthStore(a.db)
	md := content.NewMarkdownRenderer()
	p := publish.New(publish.Config{
		OutDir:        a.cfg.PublishOutDir,
		StaticDir:     a.cfg.StaticDir,
		UploadDir:     a.cfg.UploadDir,
		SiteBaseURL:   a.cfg.SiteBaseURL,
		IncludeDrafts: a.cfg.PublishIncludeDrafts,
	}, a.renderer, store, md)
	return p.Publish(ctx)
}

// Router explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (a *App) Router() *routes.Router {
	authStore := a.jobStore
	if authStore == nil {
		authStore = storesqlite.NewAuthStore(a.db)
		a.jobStore = authStore
	}
	webAuthn, err := auth.NewWebAuthn(auth.WebAuthnConfig{
		RPID:        a.cfg.WebAuthnRPID,
		RPName:      a.cfg.WebAuthnRPName,
		RPOrigins:   a.cfg.WebAuthnOrigins,
		SiteBaseURL: a.cfg.SiteBaseURL,
		AppEnv:      a.cfg.AppEnv,
	})
	if err != nil {
		a.logger.Error("webauthn unavailable", "error", err)
	}
	markdown := content.NewMarkdownRenderer()
	publicHandlers := handlers.NewPublic(a.renderer, authStore, markdown)
	adminHandlers := handlers.NewAdmin(a.renderer, a.logger, authStore, handlers.AdminConfig{
		SessionTTL:            a.cfg.SessionTTL,
		CookieName:            a.cfg.SessionCookieName,
		CookiePath:            a.cfg.AdminCookiePath,
		CookieSecure:          a.cfg.CookieSecure,
		UploadDir:             a.cfg.UploadDir,
		IPResolver:            a.ipRes,
		MediaUserQuotaBytes:   a.cfg.MediaUserQuotaBytes,
		MediaTotalQuotaBytes:  a.cfg.MediaTotalQuotaBytes,
		LoginLockoutThreshold: a.cfg.LoginLockoutThreshold,
		LoginLockoutWindow:    a.cfg.LoginLockoutWindow,
		LoginLockoutDuration:  a.cfg.LoginLockoutDuration,
		WebAuthn:              webAuthn,
	})

	loginLimiter := middleware.NewFixedWindowLimiter(10, 10*time.Minute)
	sensitiveLimiter := middleware.NewFixedWindowLimiter(30, 1*time.Minute)

	mw := routes.MiddlewareSet{
		RequestID:       middleware.RequestID,
		SecurityHeaders: middleware.SecurityHeaders,
		HTMX:            middleware.HTMX,
		Session: middleware.SessionLoader(authStore, middleware.SessionConfig{
			CookieName: a.cfg.SessionCookieName,
			CookiePath: a.cfg.AdminCookiePath,
			Secure:     a.cfg.CookieSecure,
		}),
		CSRF: middleware.CSRF(middleware.CSRFConfig{
			CookieName: a.cfg.CSRFCookieName,
			CookiePath: a.cfg.AdminCookiePath,
			Secure:     a.cfg.CookieSecure,
		}),
		RequireAuth:    middleware.RequireAuth("/admin/login"),
		RequireAdmin:   middleware.RequireRoles(domain.RoleAdmin),
		LoginRateLimit: middleware.LoginRateLimit(loginLimiter, a.ipRes),
		SensitiveLimit: middleware.SensitiveRateLimit(sensitiveLimiter, a.ipRes),
	}

	return routes.New(publicHandlers, adminHandlers, a.cfg.StaticDir, mw)
}

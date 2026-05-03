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
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv                string
	HTTPAddr              string
	PreviewHTTPAddr       string
	DBMode                string
	DBPath                string
	DatabaseURL           string
	DatabaseAuthToken     string
	ReplicaSyncInterval   time.Duration
	ReplicaReadYourWrites bool
	UploadDir             string
	TemplateGlob          string
	StaticDir             string
	SiteBaseURL           string
	PublishOutDir         string
	PublishIncludeDrafts  bool
	SessionCookieName     string
	CSRFCookieName        string
	AdminCookiePath       string
	SessionTTL            time.Duration
	CookieSecure          bool
	TrustedProxyCIDRs     []string
	EnforceTrustedProxies bool
	MediaUserQuotaBytes   int64
	MediaTotalQuotaBytes  int64
	LoginLockoutThreshold int
	LoginLockoutWindow    time.Duration
	LoginLockoutDuration  time.Duration
	WebAuthnRPID          string
	WebAuthnRPName        string
	WebAuthnOrigins       []string
	JobsPollInterval      time.Duration
	AutosaveRetention     time.Duration
	AutosaveCleanupEvery  time.Duration
}

// LoadConfig explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func LoadConfig() Config {
	appEnv := getEnv("APP_ENV", "dev")
	cfg := Config{
		AppEnv:                appEnv,
		HTTPAddr:              getEnv("HTTP_ADDR", ":8080"),
		PreviewHTTPAddr:       getEnv("PREVIEW_HTTP_ADDR", ":8081"),
		DBMode:                strings.ToLower(getEnv("DB_MODE", "local")),
		DBPath:                getEnv("DB_PATH", "./data/cms.db"),
		DatabaseURL:           getEnv("DATABASE_URL", ""),
		DatabaseAuthToken:     getEnv("DATABASE_AUTH_TOKEN", ""),
		ReplicaSyncInterval:   getDurationEnv("REPLICA_SYNC_INTERVAL", 2*time.Second),
		ReplicaReadYourWrites: getBoolEnv("REPLICA_READ_YOUR_WRITES", true),
		UploadDir:             getEnv("UPLOAD_DIR", "./data/uploads"),
		TemplateGlob:          getEnv("TEMPLATE_GLOB", "web/templates/**/*.tmpl"),
		StaticDir:             getEnv("STATIC_DIR", "web/static"),
		SiteBaseURL:           strings.TrimRight(getEnv("SITE_BASE_URL", ""), "/"),
		PublishOutDir:         getEnv("PUBLISH_OUT_DIR", "../dist/site"),
		PublishIncludeDrafts:  getBoolEnv("PUBLISH_INCLUDE_DRAFTS", false),
		SessionCookieName:     getEnv("SESSION_COOKIE_NAME", "cms_session"),
		CSRFCookieName:        getEnv("CSRF_COOKIE_NAME", "cms_csrf"),
		AdminCookiePath:       getEnv("ADMIN_COOKIE_PATH", "/admin"),
		SessionTTL:            getDurationEnv("SESSION_TTL", 24*time.Hour),
		CookieSecure:          getBoolEnv("COOKIE_SECURE", appEnv != "dev"),
		TrustedProxyCIDRs:     getListEnv("TRUSTED_PROXY_CIDRS"),
		EnforceTrustedProxies: getBoolEnv("ENFORCE_TRUSTED_PROXY_CIDRS", appEnv != "dev"),
		MediaUserQuotaBytes:   getInt64Env("MEDIA_USER_QUOTA_BYTES", 200<<20),
		MediaTotalQuotaBytes:  getInt64Env("MEDIA_TOTAL_QUOTA_BYTES", 2<<30),
		LoginLockoutThreshold: getIntEnv("LOGIN_LOCKOUT_THRESHOLD", 8),
		LoginLockoutWindow:    getDurationEnv("LOGIN_LOCKOUT_WINDOW", 15*time.Minute),
		LoginLockoutDuration:  getDurationEnv("LOGIN_LOCKOUT_DURATION", 15*time.Minute),
		WebAuthnRPID:          getEnv("WEBAUTHN_RP_ID", ""),
		WebAuthnRPName:        getEnv("WEBAUTHN_RP_NAME", "kcNotes"),
		WebAuthnOrigins:       getListEnv("WEBAUTHN_ORIGINS"),
		JobsPollInterval:      getDurationEnv("JOBS_POLL_INTERVAL", 15*time.Second),
		AutosaveRetention:     getDurationEnv("AUTOSAVE_RETENTION_DURATION", 720*time.Hour),
		AutosaveCleanupEvery:  getDurationEnv("AUTOSAVE_CLEANUP_INTERVAL", 1*time.Hour),
	}
	return cfg
}

// getEnv explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

// getDurationEnv explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func getDurationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

// getBoolEnv explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func getBoolEnv(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

// getListEnv explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func getListEnv(key string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		entry := strings.TrimSpace(part)
		if entry != "" {
			items = append(items, entry)
		}
	}
	return items
}

// getIntEnv explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func getIntEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// getInt64Env explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func getInt64Env(key string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

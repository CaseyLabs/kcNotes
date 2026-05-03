// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kcnotes/internal/domain"
	"kcnotes/internal/http/views"
	storesqlite "kcnotes/internal/store/sqlite"
)

// TestDetectAndValidateMediaAcceptsPNG explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestDetectAndValidateMediaAcceptsPNG(t *testing.T) {
	t.Parallel()
	// 1x1 transparent PNG
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO2X6pYAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}

	mimeType, ext, err := detectAndValidateMedia(data)
	if err != nil {
		t.Fatalf("expected valid png, got error: %v", err)
	}
	if mimeType != "image/png" {
		t.Fatalf("expected image/png, got %s", mimeType)
	}
	if ext != ".png" {
		t.Fatalf("expected .png extension, got %s", ext)
	}
}

// TestDetectAndValidateMediaRejectsText explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestDetectAndValidateMediaRejectsText(t *testing.T) {
	t.Parallel()
	_, _, err := detectAndValidateMedia([]byte("hello world"))
	if err == nil {
		t.Fatalf("expected rejection for text upload")
	}
}

// TestCleanOriginalName explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestCleanOriginalName(t *testing.T) {
	t.Parallel()
	got := cleanOriginalName("../../my-image.png")
	if got != "my-image.png" {
		t.Fatalf("expected basename only, got %s", got)
	}

	long := strings.Repeat("a", 300)
	if len(cleanOriginalName(long)) != 180 {
		t.Fatalf("expected truncated filename")
	}
}

func TestUploadMediaStoresOriginalVariantsAndMetadata(t *testing.T) {
	admin, store, uploadDir := newMediaHandlerTestAdmin(t, 200<<20, 2<<30)
	req := newMediaUploadRequest(t, "hero.png", testPNG(t, 640, 320))
	res := httptest.NewRecorder()

	usersHandlerStack(store, http.HandlerFunc(admin.UploadMedia)).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, "Media uploaded") || !strings.Contains(body, "hero.png") {
		t.Fatalf("expected uploaded media table response, got:\n%s", body)
	}

	items, err := store.ListMedia(context.Background(), 10)
	if err != nil {
		t.Fatalf("list media: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one media row, got %d", len(items))
	}
	item := items[0]
	if item.OriginalName != "hero.png" || item.MIME != "image/png" || item.Width != 640 || item.Height != 320 {
		t.Fatalf("unexpected media metadata: %#v", item)
	}
	if len(item.Variants) != 1 {
		t.Fatalf("expected one stored variant, got %d", len(item.Variants))
	}
	if item.Variants[0].Name != "thumb" || item.Variants[0].Width != 320 || item.Variants[0].Height != 160 {
		t.Fatalf("unexpected variant metadata: %#v", item.Variants[0])
	}
	if _, err := os.Stat(filepath.Join(uploadDir, item.StoredName)); err != nil {
		t.Fatalf("expected original file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, item.Variants[0].StoredName)); err != nil {
		t.Fatalf("expected variant file: %v", err)
	}

	userBytes, totalBytes, err := store.MediaUsage(context.Background(), "admin-1")
	if err != nil {
		t.Fatalf("media usage: %v", err)
	}
	wantBytes := item.Size + item.Variants[0].Size
	if userBytes != wantBytes || totalBytes != wantBytes {
		t.Fatalf("expected quota usage %d, got user=%d total=%d", wantBytes, userBytes, totalBytes)
	}
}

func TestUploadMediaQuotaErrorDoesNotWriteFilesOrMetadata(t *testing.T) {
	admin, store, uploadDir := newMediaHandlerTestAdmin(t, 1, 2<<30)
	req := newMediaUploadRequest(t, "hero.png", testPNG(t, 640, 320))
	res := httptest.NewRecorder()

	usersHandlerStack(store, http.HandlerFunc(admin.UploadMedia)).ServeHTTP(res, req)

	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "user media quota exceeded") {
		t.Fatalf("expected quota error, got:\n%s", res.Body.String())
	}
	items, err := store.ListMedia(context.Background(), 10)
	if err != nil {
		t.Fatalf("list media: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no media rows, got %d", len(items))
	}
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no uploaded files, got %d", len(entries))
	}
}

func newMediaHandlerTestAdmin(t *testing.T, userQuota, totalQuota int64) (*Admin, *storesqlite.AuthStore, string) {
	t.Helper()
	store := newMediaHandlerTestStore(t)
	uploadDir := t.TempDir()
	renderer, err := views.NewRenderer("../../../web/templates/*/*.tmpl")
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}
	admin := NewAdmin(renderer, slog.New(slog.NewTextHandler(io.Discard, nil)), store, AdminConfig{
		CookieName:           "cms_session",
		CookiePath:           "/admin",
		SessionTTL:           24 * time.Hour,
		UploadDir:            uploadDir,
		MediaUserQuotaBytes:  userQuota,
		MediaTotalQuotaBytes: totalQuota,
	})
	return admin, store, uploadDir
}

func newMediaHandlerTestStore(t *testing.T) *storesqlite.AuthStore {
	t.Helper()
	conn, err := storesqlite.Open(storesqlite.OpenConfig{
		Mode:   storesqlite.DBModeLocal,
		DBPath: filepath.Join(t.TempDir(), "cms.db"),
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	mediaHandlerSchema := []string{
		`CREATE TABLE users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL,
			disabled INTEGER NOT NULL DEFAULT 0,
			mfa_enabled INTEGER NOT NULL DEFAULT 0,
			mfa_secret TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			csrf_token TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE media (
			id TEXT PRIMARY KEY,
			stored_name TEXT NOT NULL,
			original_name TEXT NOT NULL,
			mime TEXT NOT NULL,
			size INTEGER NOT NULL,
			sha256 TEXT NOT NULL,
			width INTEGER NOT NULL DEFAULT 0,
			height INTEGER NOT NULL DEFAULT 0,
			created_by TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(created_by) REFERENCES users(id)
		)`,
		`CREATE TABLE media_variants (
			media_id TEXT NOT NULL,
			name TEXT NOT NULL,
			stored_name TEXT NOT NULL,
			mime TEXT NOT NULL,
			size INTEGER NOT NULL,
			width INTEGER NOT NULL,
			height INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			PRIMARY KEY (media_id, name),
			FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE audit_log (
			id TEXT PRIMARY KEY,
			actor_user_id TEXT,
			action TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			entity_id TEXT,
			ip TEXT,
			user_agent TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(actor_user_id) REFERENCES users(id)
		)`,
	}
	for _, stmt := range mediaHandlerSchema {
		if _, err := conn.DB.Exec(stmt); err != nil {
			t.Fatalf("create media handler schema: %v", err)
		}
	}
	store := storesqlite.NewAuthStore(conn.DB)
	if err := store.CreateUser(context.Background(), domain.User{
		ID:    "admin-1",
		Email: "admin@example.com",
		Role:  domain.RoleAdmin,
	}); err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	if err := store.CreateSession(context.Background(), "session-1", "admin-1", "csrf-token", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return store
}

func newMediaUploadRequest(t *testing.T, name string, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("_csrf", "csrf-token"); err != nil {
		t.Fatalf("write csrf field: %v", err)
	}
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/media", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: "cms_session", Value: "session-1", Path: "/admin"})
	return req
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 0x66, G: 0x33, B: 0x99, A: 0xff})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return out.Bytes()
}

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
	if !strings.Contains(body, `src="/admin/media/files/`+item.ID+`/variants/thumb"`) {
		t.Fatalf("expected media grid to use thumbnail variant, got:\n%s", body)
	}
	if strings.Contains(body, `src="/admin/media/files/`+item.ID+`"`) {
		t.Fatalf("expected media grid image not to load original upload, got:\n%s", body)
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

func TestMediaVariantFileServesGeneratedThumbnail(t *testing.T) {
	admin, store, uploadDir := newMediaHandlerTestAdmin(t, 200<<20, 2<<30)
	upload := httptest.NewRecorder()
	usersHandlerStack(store, http.HandlerFunc(admin.UploadMedia)).ServeHTTP(upload, newMediaUploadRequest(t, "hero.png", testPNG(t, 640, 320)))
	if upload.Code != http.StatusOK {
		t.Fatalf("expected upload status 200, got %d: %s", upload.Code, upload.Body.String())
	}
	items, err := store.ListMedia(context.Background(), 10)
	if err != nil {
		t.Fatalf("list media: %v", err)
	}
	if len(items) != 1 || len(items[0].Variants) != 1 {
		t.Fatalf("expected one media item with one variant, got %#v", items)
	}
	item := items[0]
	variant := item.Variants[0]

	req := httptest.NewRequest(http.MethodGet, "/admin/media/files/"+item.ID+"/variants/thumb", nil)
	req.SetPathValue("id", item.ID)
	req.SetPathValue("name", "thumb")
	req.AddCookie(&http.Cookie{Name: "cms_session", Value: "session-1", Path: "/admin"})
	res := httptest.NewRecorder()
	usersHandlerStack(store, http.HandlerFunc(admin.MediaVariantFile)).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("expected image/png content type, got %s", got)
	}
	wantVariant, err := os.ReadFile(filepath.Join(uploadDir, variant.StoredName))
	if err != nil {
		t.Fatalf("read variant: %v", err)
	}
	if !bytes.Equal(res.Body.Bytes(), wantVariant) {
		t.Fatalf("expected response body to be thumbnail variant bytes")
	}
	original, err := os.ReadFile(filepath.Join(uploadDir, item.StoredName))
	if err != nil {
		t.Fatalf("read original: %v", err)
	}
	if bytes.Equal(res.Body.Bytes(), original) {
		t.Fatalf("expected thumbnail response to differ from original upload bytes")
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

func TestUploadMediaSameUserDuplicateDoesNotWriteFilesOrMetadata(t *testing.T) {
	admin, store, uploadDir := newMediaHandlerTestAdmin(t, 200<<20, 2<<30)
	data := testPNG(t, 640, 320)
	first := httptest.NewRecorder()
	usersHandlerStack(store, http.HandlerFunc(admin.UploadMedia)).ServeHTTP(first, newMediaUploadRequest(t, "hero.png", data))
	if first.Code != http.StatusOK {
		t.Fatalf("expected first upload status 200, got %d: %s", first.Code, first.Body.String())
	}
	before := mustReadDirCount(t, uploadDir)

	second := httptest.NewRecorder()
	usersHandlerStack(store, http.HandlerFunc(admin.UploadMedia)).ServeHTTP(second, newMediaUploadRequest(t, "hero-copy.png", data))
	if second.Code != http.StatusOK {
		t.Fatalf("expected duplicate upload status 200, got %d: %s", second.Code, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), "Media already exists in your library") {
		t.Fatalf("expected duplicate feedback, got:\n%s", second.Body.String())
	}
	if got := mustReadDirCount(t, uploadDir); got != before {
		t.Fatalf("expected duplicate upload to write no files, before=%d after=%d", before, got)
	}
	items, err := store.ListMedia(context.Background(), 10)
	if err != nil {
		t.Fatalf("list media: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one media row, got %d", len(items))
	}
}

func TestUploadMediaSameHashRaceReusesCanonicalAsset(t *testing.T) {
	admin, store, uploadDir := newMediaHandlerTestAdmin(t, 200<<20, 2<<30)
	admin.store = &raceCreateMediaStore{AuthStore: store}
	data := testPNG(t, 640, 320)

	res := httptest.NewRecorder()
	usersHandlerStack(store, http.HandlerFunc(admin.UploadMedia)).ServeHTTP(res, newMediaUploadRequest(t, "hero.png", data))
	if res.Code != http.StatusOK {
		t.Fatalf("expected upload race status 200, got %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "Media already exists in your library") {
		t.Fatalf("expected race feedback, got:\n%s", res.Body.String())
	}
	if got := mustReadDirCount(t, uploadDir); got != 0 {
		t.Fatalf("expected losing upload files to be removed, got %d files", got)
	}
	items, err := store.ListMedia(context.Background(), 10)
	if err != nil {
		t.Fatalf("list media: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one canonical media row, got %d", len(items))
	}
}

func TestUploadMediaCrossUserDuplicateCreatesLibraryRowWithoutWritingFiles(t *testing.T) {
	admin, store, uploadDir := newMediaHandlerTestAdmin(t, 200<<20, 2<<30)
	data := testPNG(t, 640, 320)
	first := httptest.NewRecorder()
	usersHandlerStack(store, http.HandlerFunc(admin.UploadMedia)).ServeHTTP(first, newMediaUploadRequest(t, "hero.png", data))
	if first.Code != http.StatusOK {
		t.Fatalf("expected first upload status 200, got %d: %s", first.Code, first.Body.String())
	}
	before := mustReadDirCount(t, uploadDir)
	if err := store.CreateUser(context.Background(), domain.User{
		ID:    "admin-2",
		Email: "admin2@example.com",
		Role:  domain.RoleAdmin,
	}); err != nil {
		t.Fatalf("create second user: %v", err)
	}
	if err := store.CreateSession(context.Background(), "session-2", "admin-2", "csrf-token", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create second session: %v", err)
	}

	req := newMediaUploadRequest(t, "hero-shared.png", data)
	req.Header.Del("Cookie")
	req.AddCookie(&http.Cookie{Name: "cms_session", Value: "session-2", Path: "/admin"})
	second := httptest.NewRecorder()
	usersHandlerStack(store, http.HandlerFunc(admin.UploadMedia)).ServeHTTP(second, req)
	if second.Code != http.StatusOK {
		t.Fatalf("expected duplicate upload status 200, got %d: %s", second.Code, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), "Media added from existing upload") {
		t.Fatalf("expected attach feedback, got:\n%s", second.Body.String())
	}
	if got := mustReadDirCount(t, uploadDir); got != before {
		t.Fatalf("expected cross-user duplicate to write no files, before=%d after=%d", before, got)
	}
	items, err := store.ListMedia(context.Background(), 10)
	if err != nil {
		t.Fatalf("list media: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two media rows, got %d", len(items))
	}
	if items[0].StoredName != items[1].StoredName || items[0].AssetID != items[1].AssetID {
		t.Fatalf("expected rows to share physical asset, got %#v", items)
	}
	userBytes, totalBytes, err := store.MediaUsage(context.Background(), "admin-2")
	if err != nil {
		t.Fatalf("media usage: %v", err)
	}
	wantUser := items[0].Size + items[0].Variants[0].Size
	if userBytes != wantUser || totalBytes != wantUser {
		t.Fatalf("expected logical user and physical total bytes %d, got user=%d total=%d", wantUser, userBytes, totalBytes)
	}
}

type raceCreateMediaStore struct {
	*storesqlite.AuthStore
}

func (s *raceCreateMediaStore) CreateMediaWithVariants(ctx context.Context, media domain.Media, variants []domain.MediaVariant) error {
	winner := media
	winner.ID = "race-winner"
	winner.AssetID = winner.ID
	winner.StoredName = winner.ID + filepath.Ext(media.StoredName)
	winnerVariants := make([]domain.MediaVariant, 0, len(variants))
	for _, variant := range variants {
		variant.MediaID = winner.ID
		variant.StoredName = winner.ID + "-" + variant.Name + filepath.Ext(variant.StoredName)
		winnerVariants = append(winnerVariants, variant)
	}
	if err := s.AuthStore.CreateMediaWithVariants(ctx, winner, winnerVariants); err != nil {
		return err
	}
	return storesqlite.ErrMediaAssetExists
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
			asset_id TEXT,
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
		`CREATE TABLE media_assets (
			id TEXT PRIMARY KEY,
			stored_name TEXT NOT NULL UNIQUE,
			mime TEXT NOT NULL,
			size INTEGER NOT NULL,
			sha256 TEXT NOT NULL UNIQUE,
			width INTEGER NOT NULL DEFAULT 0,
			height INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
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
		`CREATE UNIQUE INDEX idx_media_created_by_asset ON media(created_by, asset_id)`,
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

func mustReadDirCount(t *testing.T, path string) int {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}
	return len(entries)
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

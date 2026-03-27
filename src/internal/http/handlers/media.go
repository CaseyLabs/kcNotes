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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"kcnotes/internal/domain"
	"kcnotes/internal/http/middleware"
	storesqlite "kcnotes/internal/store/sqlite"
)

const (
	maxUploadSizeBytes = 10 << 20 // 10 MiB
	defaultMediaLimit  = 200
)

var allowedMediaTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
}

// MediaPage explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) MediaPage(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	media, err := h.store.ListMedia(r.Context(), defaultMediaLimit)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load media")
		return
	}

	data := h.mediaListData(r, user, media)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "admin-media", data)
}

// MediaTable explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) MediaTable(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	media, err := h.store.ListMedia(r.Context(), defaultMediaLimit)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to load media")
		return
	}

	data := h.mediaListData(r, user, media)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "partial-media-table", data)
}

// UploadMedia explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) UploadMedia(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r)
	if !ok {
		h.renderError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSizeBytes+1024)
	if err := r.ParseMultipartForm(maxUploadSizeBytes); err != nil {
		h.renderMediaUploadError(w, r, user, "Upload exceeds maximum size (10 MiB)")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.renderMediaUploadError(w, r, user, "File is required")
		return
	}
	defer file.Close()

	data, err := readUpload(file, maxUploadSizeBytes)
	if err != nil {
		h.renderMediaUploadError(w, r, user, err.Error())
		return
	}
	mimeType, ext, err := detectAndValidateMedia(data)
	if err != nil {
		h.renderMediaUploadError(w, r, user, err.Error())
		return
	}
	if err := h.enforceMediaQuota(r.Context(), user.ID, int64(len(data))); err != nil {
		h.renderMediaUploadError(w, r, user, err.Error())
		return
	}

	storedID, err := randomID()
	if err != nil {
		h.renderError(w, r, http.StatusServiceUnavailable, "service unavailable")
		return
	}
	storedName := storedID + ext
	if err := os.MkdirAll(h.uploadDir, 0o755); err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to prepare upload directory")
		return
	}
	targetPath := filepath.Join(h.uploadDir, storedName)
	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "failed to store upload")
		return
	}

	mediaID, err := randomID()
	if err != nil {
		_ = os.Remove(targetPath)
		h.renderError(w, r, http.StatusServiceUnavailable, "service unavailable")
		return
	}
	sum := sha256.Sum256(data)
	item := domain.Media{
		ID:           mediaID,
		StoredName:   storedName,
		OriginalName: cleanOriginalName(header.Filename),
		MIME:         mimeType,
		Size:         int64(len(data)),
		SHA256:       hex.EncodeToString(sum[:]),
		CreatedBy:    user.ID,
	}
	if err := h.store.CreateMedia(r.Context(), item); err != nil {
		_ = os.Remove(targetPath)
		h.renderError(w, r, http.StatusInternalServerError, "failed to save media metadata")
		return
	}

	h.auditEvent(r, user.ID, "media_upload", "media", item.ID)

	h.renderMediaTableAfterAction(w, r, user, "Media uploaded")
}

// MediaFile explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) MediaFile(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	media, err := h.store.GetMediaByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, storesqlite.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		h.renderError(w, r, http.StatusInternalServerError, "failed to load media")
		return
	}
	if !isAllowedMediaType(media.MIME) {
		h.renderError(w, r, http.StatusForbidden, "unsupported media type")
		return
	}
	if filepath.Base(media.StoredName) != media.StoredName {
		h.renderError(w, r, http.StatusBadRequest, "invalid media path")
		return
	}

	targetPath := filepath.Join(h.uploadDir, media.StoredName)
	data, err := os.ReadFile(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		h.renderError(w, r, http.StatusInternalServerError, "failed to read media")
		return
	}

	w.Header().Set("Content-Type", media.MIME)
	w.Header().Set("Cache-Control", "private, max-age=0, no-store")
	w.Header().Set("Content-Disposition", contentDispositionInline(media.OriginalName))
	http.ServeContent(w, r, media.OriginalName, media.CreatedAt, bytes.NewReader(data))
}

// renderMediaUploadError explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) renderMediaUploadError(w http.ResponseWriter, r *http.Request, user domain.User, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	media, err := h.store.ListMedia(r.Context(), defaultMediaLimit)
	if err != nil {
		media = nil
	}
	data := h.mediaListData(r, user, media)
	data["FlashMessage"] = message
	if middleware.IsHTMX(r) {
		_ = h.renderer.Render(w, "partial-media-table", data)
		return
	}
	_ = h.renderer.Render(w, "admin-media", data)
}

// renderMediaTableAfterAction explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) renderMediaTableAfterAction(w http.ResponseWriter, r *http.Request, user domain.User, message string) {
	if middleware.IsHTMX(r) {
		media, err := h.store.ListMedia(r.Context(), defaultMediaLimit)
		if err != nil {
			h.renderError(w, r, http.StatusInternalServerError, "failed to load media")
			return
		}
		data := h.mediaListData(r, user, media)
		data["FlashMessage"] = message
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = h.renderer.Render(w, "partial-media-table", data)
		return
	}
	http.Redirect(w, r, "/admin/media?msg="+url.QueryEscape(message), http.StatusSeeOther)
}

// mediaListData explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) mediaListData(r *http.Request, user domain.User, media []domain.Media) map[string]any {
	return map[string]any{
		"Title":        "Media",
		"CSRFToken":    middleware.CSRFToken(r),
		"UserEmail":    user.Email,
		"UserRole":     string(user.Role),
		"Media":        media,
		"FlashMessage": strings.TrimSpace(r.URL.Query().Get("msg")),
	}
}

// readUpload explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func readUpload(file io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read upload")
	}
	if int64(len(data)) == 0 {
		return nil, fmt.Errorf("file is empty")
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("upload exceeds maximum size (10 MiB)")
	}
	return data, nil
}

// detectAndValidateMedia explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func detectAndValidateMedia(data []byte) (mimeType, ext string, err error) {
	sniffLen := len(data)
	if sniffLen > 512 {
		sniffLen = 512
	}
	mimeType = http.DetectContentType(data[:sniffLen])
	ext, ok := allowedMediaTypes[mimeType]
	if !ok {
		return "", "", fmt.Errorf("unsupported file type")
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
		return "", "", fmt.Errorf("invalid image data")
	}
	return mimeType, ext, nil
}

// isAllowedMediaType explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func isAllowedMediaType(mimeType string) bool {
	_, ok := allowedMediaTypes[mimeType]
	return ok
}

// enforceMediaQuota explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (h *Admin) enforceMediaQuota(ctx context.Context, userID string, incomingBytes int64) error {
	if incomingBytes <= 0 {
		return nil
	}
	if h.maxMediaUserB <= 0 && h.maxMediaTotalB <= 0 {
		return nil
	}
	userBytes, totalBytes, err := h.store.MediaUsage(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to check media quota")
	}
	if h.maxMediaUserB > 0 && userBytes+incomingBytes > h.maxMediaUserB {
		return fmt.Errorf("user media quota exceeded")
	}
	if h.maxMediaTotalB > 0 && totalBytes+incomingBytes > h.maxMediaTotalB {
		return fmt.Errorf("global media quota exceeded")
	}
	return nil
}

// cleanOriginalName explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func cleanOriginalName(name string) string {
	clean := filepath.Base(strings.TrimSpace(name))
	clean = strings.ReplaceAll(clean, "\x00", "")
	if clean == "." || clean == "/" || clean == "" {
		return "upload"
	}
	if len(clean) > 180 {
		clean = clean[:180]
	}
	return clean
}

// contentDispositionInline explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func contentDispositionInline(fileName string) string {
	escaped := strings.ReplaceAll(fileName, `"`, "")
	return "inline; filename=\"" + escaped + "\""
}

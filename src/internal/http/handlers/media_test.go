// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package handlers

import (
	"encoding/base64"
	"strings"
	"testing"
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

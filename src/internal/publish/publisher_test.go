// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package publish

import (
	"context"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kcnotes/internal/domain"
	"kcnotes/internal/http/views"
)

type fakeStore struct {
	posts []domain.Post
	pages []domain.Post
	media []domain.Media
}

// ListPublicPosts explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s fakeStore) ListPublicPosts(_ context.Context, _ bool, _ int) ([]domain.Post, error) {
	return s.posts, nil
}

// ListPublicPages explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s fakeStore) ListPublicPages(_ context.Context, _ bool) ([]domain.Post, error) {
	return s.pages, nil
}

// ListPublishableMedia explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (s fakeStore) ListPublishableMedia(_ context.Context) ([]domain.Media, error) {
	return s.media, nil
}

type fakeMarkdown struct{}

// Render explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (fakeMarkdown) Render(markdown string) (template.HTML, error) {
	return template.HTML("<p>" + template.HTMLEscapeString(markdown) + "</p>"), nil
}

// TestPublisherBuildsStaticSiteAndReplacesOldOutput explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestPublisherBuildsStaticSiteAndReplacesOldOutput(t *testing.T) {
	root := t.TempDir()
	templatePath := filepath.Join(root, "templates", "all.tmpl")
	if err := os.MkdirAll(filepath.Dir(templatePath), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}

	const tpl = `
{{ define "shell-start" }}<!doctype html><html><head><title>{{ .Title }}</title><link rel="stylesheet" href="{{ if .AssetBase }}{{ .AssetBase }}{{ else }}/static{{ end }}/css/app.css" /></head><body>{{ end }}
{{ define "shell-end" }}</body></html>{{ end }}
{{ define "home" }}{{ template "shell-start" . }}{{ range .Posts }}<a href="/p/{{ .Slug }}/">{{ .Title }}</a>{{ end }}{{ template "shell-end" . }}{{ end }}
{{ define "public-post" }}{{ template "shell-start" . }}{{ .BodyHTML }}{{ template "shell-end" . }}{{ end }}
{{ define "public-page" }}{{ template "shell-start" . }}{{ .BodyHTML }}{{ template "shell-end" . }}{{ end }}
`
	if err := os.WriteFile(templatePath, []byte(strings.TrimSpace(tpl)), 0o644); err != nil {
		t.Fatalf("write templates: %v", err)
	}

	renderer, err := views.NewRenderer(filepath.Join(root, "templates", "*.tmpl"))
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}

	staticDir := filepath.Join(root, "web-static")
	uploadDir := filepath.Join(root, "uploads")
	if err := os.MkdirAll(filepath.Join(staticDir, "css"), 0o755); err != nil {
		t.Fatalf("mkdir css: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(staticDir, "js"), 0o755); err != nil {
		t.Fatalf("mkdir js: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "css", "app.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "js", "app.js"), []byte("console.log('x')"), 0o644); err != nil {
		t.Fatalf("write js: %v", err)
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatalf("mkdir uploads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "img-1.png"), []byte{0x89, 0x50, 0x4e, 0x47}, 0o644); err != nil {
		t.Fatalf("write upload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "img-1-thumb.png"), []byte{0x89, 0x50, 0x4e, 0x47}, 0o644); err != nil {
		t.Fatalf("write media variant: %v", err)
	}

	outDir := filepath.Join(root, "dist")
	if err := os.MkdirAll(filepath.Join(outDir, "old"), 0o755); err != nil {
		t.Fatalf("mkdir old output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "old", "orphan.html"), []byte("orphan"), 0o644); err != nil {
		t.Fatalf("write orphan: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, manifestName), []byte("{\"files\":[\"old/orphan.html\"]}\n"), 0o644); err != nil {
		t.Fatalf("write old manifest: %v", err)
	}

	now := time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)
	store := fakeStore{
		posts: []domain.Post{{
			ID:          "p1",
			Type:        domain.PostTypePost,
			Title:       "Hello",
			Slug:        "hello",
			BodyMD:      "world",
			Status:      domain.PostStatusPublished,
			UpdatedAt:   now,
			PublishedAt: &now,
		}},
		pages: []domain.Post{{
			ID:        "pg1",
			Type:      domain.PostTypePage,
			Title:     "About",
			Slug:      "about",
			BodyMD:    "about page",
			Status:    domain.PostStatusPublished,
			UpdatedAt: now,
		}},
		media: []domain.Media{{
			ID:         "m1",
			StoredName: "img-1.png",
			MIME:       "image/png",
			Variants: []domain.MediaVariant{{
				Name:       "thumb",
				StoredName: "img-1-thumb.png",
				MIME:       "image/png",
			}},
		}},
	}

	pub := New(Config{
		OutDir:      outDir,
		StaticDir:   staticDir,
		UploadDir:   uploadDir,
		SiteBaseURL: "https://example.com",
	}, renderer, store, fakeMarkdown{})

	result, err := pub.Publish(context.Background())
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if result.RemovedFiles != 1 {
		t.Fatalf("expected one removed file, got %d", result.RemovedFiles)
	}

	expectFiles := []string{
		filepath.Join(outDir, "index.html"),
		filepath.Join(outDir, "p", "hello", "index.html"),
		filepath.Join(outDir, "page", "about", "index.html"),
		filepath.Join(outDir, "sitemap.xml"),
		filepath.Join(outDir, "rss.xml"),
		filepath.Join(outDir, "assets", "css", "app.css"),
		filepath.Join(outDir, "assets", "js", "app.js"),
		filepath.Join(outDir, "media", "img-1.png"),
		filepath.Join(outDir, "media", "img-1-thumb.png"),
		filepath.Join(outDir, manifestName),
	}
	for _, f := range expectFiles {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("expected file %s to exist: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "old", "orphan.html")); !os.IsNotExist(err) {
		t.Fatalf("expected orphan file removed, err=%v", err)
	}

	sitemap, err := os.ReadFile(filepath.Join(outDir, "sitemap.xml"))
	if err != nil {
		t.Fatalf("read sitemap: %v", err)
	}
	if !strings.Contains(string(sitemap), "https://example.com/p/hello/") {
		t.Fatalf("sitemap missing post URL: %s", sitemap)
	}
}

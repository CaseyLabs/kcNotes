// TEACHING NOTES:
// Publisher files implement static-site generation from DB content.
// This code demonstrates Go's strength at file IO + deterministic batch jobs.
// Useful Go concepts to notice:
// 1. Build into a temp directory, then swap atomically for safety.
// 2. Prefer pure data transforms before writing files.
// 3. Normalize ordering/time usage for reproducible output.
// 4. Keep filesystem operations explicit and check every returned error.
package publish

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kcnotes/internal/domain"
	"kcnotes/internal/http/views"
)

const manifestName = ".publish-manifest.json"

type Store interface {
	ListPublicPosts(ctx context.Context, includeDrafts bool, limit int) ([]domain.Post, error)
	ListPublicPages(ctx context.Context, includeDrafts bool) ([]domain.Post, error)
	ListPublishableMedia(ctx context.Context) ([]domain.Media, error)
}

type MarkdownRenderer interface {
	Render(markdown string) (template.HTML, error)
}

type Config struct {
	OutDir        string
	StaticDir     string
	UploadDir     string
	SiteBaseURL   string
	IncludeDrafts bool
}

type Publisher struct {
	cfg      Config
	renderer *views.Renderer
	store    Store
	md       MarkdownRenderer
}

type Result struct {
	GeneratedFiles int
	RemovedFiles   int
}

type publishManifest struct {
	Files []string `json:"files"`
}

// New explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func New(cfg Config, renderer *views.Renderer, store Store, md MarkdownRenderer) *Publisher {
	cfg.OutDir = strings.TrimSpace(cfg.OutDir)
	if cfg.OutDir == "" {
		cfg.OutDir = "./dist"
	}
	cfg.StaticDir = strings.TrimSpace(cfg.StaticDir)
	if cfg.StaticDir == "" {
		cfg.StaticDir = "web/static"
	}
	cfg.UploadDir = strings.TrimSpace(cfg.UploadDir)
	if cfg.UploadDir == "" {
		cfg.UploadDir = "./data/uploads"
	}
	cfg.SiteBaseURL = strings.TrimRight(strings.TrimSpace(cfg.SiteBaseURL), "/")
	return &Publisher{
		cfg:      cfg,
		renderer: renderer,
		store:    store,
		md:       md,
	}
}

// Publish explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (p *Publisher) Publish(ctx context.Context) (Result, error) {
	oldManifest, _ := readManifest(filepath.Join(p.cfg.OutDir, manifestName))
	oldSet := make(map[string]struct{}, len(oldManifest.Files))
	for _, f := range oldManifest.Files {
		oldSet[f] = struct{}{}
	}

	tmpDir := p.cfg.OutDir + ".tmp"
	backupDir := p.cfg.OutDir + ".old"
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create temp publish dir: %w", err)
	}

	files, err := p.renderAll(ctx, tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return Result{}, err
	}
	slices.Sort(files)

	manifest := publishManifest{Files: files}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return Result{}, fmt.Errorf("marshal publish manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, manifestName), append(manifestBytes, '\n'), 0o644); err != nil {
		_ = os.RemoveAll(tmpDir)
		return Result{}, fmt.Errorf("write publish manifest: %w", err)
	}

	_ = os.RemoveAll(backupDir)
	if _, err := os.Stat(p.cfg.OutDir); err == nil {
		if err := os.Rename(p.cfg.OutDir, backupDir); err != nil {
			_ = os.RemoveAll(tmpDir)
			return Result{}, fmt.Errorf("prepare output dir swap: %w", err)
		}
	}
	if err := os.Rename(tmpDir, p.cfg.OutDir); err != nil {
		if _, statErr := os.Stat(backupDir); statErr == nil {
			_ = os.Rename(backupDir, p.cfg.OutDir)
		}
		_ = os.RemoveAll(tmpDir)
		return Result{}, fmt.Errorf("activate published output: %w", err)
	}
	_ = os.RemoveAll(backupDir)

	newSet := make(map[string]struct{}, len(files))
	for _, f := range files {
		newSet[f] = struct{}{}
	}
	removed := 0
	for f := range oldSet {
		if _, ok := newSet[f]; !ok {
			removed++
		}
	}

	return Result{
		GeneratedFiles: len(files),
		RemovedFiles:   removed,
	}, nil
}

// renderAll explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (p *Publisher) renderAll(ctx context.Context, outDir string) ([]string, error) {
	renderedFiles := make([]string, 0, 32)

	posts, err := p.store.ListPublicPosts(ctx, p.cfg.IncludeDrafts, 0)
	if err != nil {
		return nil, fmt.Errorf("load posts for publish: %w", err)
	}
	pages, err := p.store.ListPublicPages(ctx, p.cfg.IncludeDrafts)
	if err != nil {
		return nil, fmt.Errorf("load pages for publish: %w", err)
	}

	if err := p.copyStaticAssets(outDir); err != nil {
		return nil, err
	}
	if assets, err := listFiles(filepath.Join(outDir, "assets")); err == nil {
		renderedFiles = append(renderedFiles, assets...)
	}
	mediaFiles, err := p.copyUploadedMedia(ctx, outDir)
	if err != nil {
		return nil, err
	}
	renderedFiles = append(renderedFiles, mediaFiles...)

	if err := p.renderTemplateToPath(filepath.Join(outDir, "index.html"), "home", map[string]any{
		"Title":         "Home",
		"Posts":         posts,
		"AssetBase":     "/assets",
		"CanonicalURL":  p.absoluteURL("/"),
		"ShowAdminLink": false,
	}); err != nil {
		return nil, fmt.Errorf("render home page: %w", err)
	}
	renderedFiles = append(renderedFiles, "index.html")

	urls := []string{"/"}
	for _, post := range posts {
		bodyHTML, err := p.md.Render(post.BodyMD)
		if err != nil {
			return nil, fmt.Errorf("render markdown for post %s: %w", post.ID, err)
		}
		route := fmt.Sprintf("/p/%s/", post.Slug)
		target := filepath.Join(outDir, "p", post.Slug, "index.html")
		if err := p.renderTemplateToPath(target, "public-post", map[string]any{
			"Title":        post.Title,
			"Post":         post,
			"BodyHTML":     bodyHTML,
			"AssetBase":    "/assets",
			"CanonicalURL": p.absoluteURL(route),
		}); err != nil {
			return nil, fmt.Errorf("render post page: %w", err)
		}
		rel, err := filepath.Rel(outDir, target)
		if err != nil {
			return nil, fmt.Errorf("resolve published post path: %w", err)
		}
		renderedFiles = append(renderedFiles, toSlashPath(rel))
		urls = append(urls, route)
	}

	for _, page := range pages {
		bodyHTML, err := p.md.Render(page.BodyMD)
		if err != nil {
			return nil, fmt.Errorf("render markdown for page %s: %w", page.ID, err)
		}
		route := fmt.Sprintf("/page/%s/", page.Slug)
		target := filepath.Join(outDir, "page", page.Slug, "index.html")
		if err := p.renderTemplateToPath(target, "public-page", map[string]any{
			"Title":        page.Title,
			"Page":         page,
			"BodyHTML":     bodyHTML,
			"AssetBase":    "/assets",
			"CanonicalURL": p.absoluteURL(route),
		}); err != nil {
			return nil, fmt.Errorf("render page page: %w", err)
		}
		rel, err := filepath.Rel(outDir, target)
		if err != nil {
			return nil, fmt.Errorf("resolve published page path: %w", err)
		}
		renderedFiles = append(renderedFiles, toSlashPath(rel))
		urls = append(urls, route)
	}

	if err := p.writeRSS(outDir, posts); err != nil {
		return nil, err
	}
	renderedFiles = append(renderedFiles, "rss.xml")

	if err := p.writeSitemap(outDir, urls); err != nil {
		return nil, err
	}
	renderedFiles = append(renderedFiles, "sitemap.xml")

	return renderedFiles, nil
}

// copyUploadedMedia explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (p *Publisher) copyUploadedMedia(ctx context.Context, outDir string) ([]string, error) {
	items, err := p.store.ListPublishableMedia(ctx)
	if err != nil {
		return nil, fmt.Errorf("load media for publish: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}

	mediaDir := filepath.Join(outDir, "media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		return nil, fmt.Errorf("create media dir: %w", err)
	}

	files := make([]string, 0, len(items))
	for _, item := range items {
		if !isPublishableMediaType(item.MIME) {
			continue
		}
		if filepath.Base(item.StoredName) != item.StoredName {
			continue
		}
		src := filepath.Join(p.cfg.UploadDir, item.StoredName)
		dst := filepath.Join(mediaDir, item.StoredName)
		if err := copyPath(src, dst); err != nil {
			return nil, fmt.Errorf("copy media file %s: %w", item.StoredName, err)
		}
		files = append(files, filepath.ToSlash(filepath.Join("media", item.StoredName)))
	}
	return files, nil
}

// renderTemplateToPath explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (p *Publisher) renderTemplateToPath(targetPath, name string, data map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create template output dir: %w", err)
	}
	f, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("create template output: %w", err)
	}
	defer f.Close()
	if err := p.renderer.Render(f, name, data); err != nil {
		return fmt.Errorf("render template %s: %w", name, err)
	}
	return nil
}

// copyStaticAssets explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (p *Publisher) copyStaticAssets(outDir string) error {
	assetsDir := filepath.Join(outDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return fmt.Errorf("create assets dir: %w", err)
	}
	entries, err := os.ReadDir(p.cfg.StaticDir)
	if err != nil {
		return fmt.Errorf("read static dir %s: %w", p.cfg.StaticDir, err)
	}
	for _, entry := range entries {
		src := filepath.Join(p.cfg.StaticDir, entry.Name())
		dst := filepath.Join(assetsDir, entry.Name())
		if err := copyPath(src, dst); err != nil {
			return fmt.Errorf("copy static asset path %s: %w", src, err)
		}
	}
	return nil
}

// writeRSS explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (p *Publisher) writeRSS(outDir string, posts []domain.Post) error {
	type item struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		GUID        string `xml:"guid"`
		Description string `xml:"description"`
		PubDate     string `xml:"pubDate"`
	}
	type channel struct {
		Title         string `xml:"title"`
		Link          string `xml:"link"`
		Description   string `xml:"description"`
		LastBuildDate string `xml:"lastBuildDate"`
		Items         []item `xml:"item"`
	}
	type rss struct {
		XMLName xml.Name `xml:"rss"`
		Version string   `xml:"version,attr"`
		Channel channel  `xml:"channel"`
	}

	items := make([]item, 0, len(posts))
	lastBuild := time.Time{}
	for _, post := range posts {
		if post.Status != domain.PostStatusPublished {
			continue
		}
		postURL := p.absoluteURL(fmt.Sprintf("/p/%s/", post.Slug))
		pub := coalesceTime(post.PublishedAt, post.UpdatedAt)
		if pub.After(lastBuild) {
			lastBuild = pub
		}
		items = append(items, item{
			Title:       post.Title,
			Link:        postURL,
			GUID:        postURL,
			Description: post.Title,
			PubDate:     pub.UTC().Format(time.RFC1123Z),
		})
	}
	if lastBuild.IsZero() {
		lastBuild = time.Unix(0, 0).UTC()
	}
	doc := rss{
		Version: "2.0",
		Channel: channel{
			Title:         "Go + HTMX CMS",
			Link:          p.absoluteURL("/"),
			Description:   "Published posts",
			LastBuildDate: lastBuild.Format(time.RFC1123Z),
			Items:         items,
		},
	}
	b, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal rss: %w", err)
	}
	content := append([]byte(xml.Header), b...)
	content = append(content, '\n')
	return os.WriteFile(filepath.Join(outDir, "rss.xml"), content, 0o644)
}

// writeSitemap explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (p *Publisher) writeSitemap(outDir string, urls []string) error {
	type urlEntry struct {
		Loc string `xml:"loc"`
	}
	type urlSet struct {
		XMLName xml.Name   `xml:"urlset"`
		XMLNS   string     `xml:"xmlns,attr"`
		URLs    []urlEntry `xml:"url"`
	}

	entries := make([]urlEntry, 0, len(urls))
	for _, u := range urls {
		entries = append(entries, urlEntry{Loc: p.absoluteURL(u)})
	}
	doc := urlSet{
		XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  entries,
	}
	b, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sitemap: %w", err)
	}
	content := append([]byte(xml.Header), b...)
	content = append(content, '\n')
	return os.WriteFile(filepath.Join(outDir, "sitemap.xml"), content, 0o644)
}

// absoluteURL explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (p *Publisher) absoluteURL(path string) string {
	if p.cfg.SiteBaseURL == "" {
		return path
	}
	return p.cfg.SiteBaseURL + path
}

// copyPath explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPath(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// listFiles explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func listFiles(root string) ([]string, error) {
	_, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, 32)
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(filepath.Dir(root), path)
		if err != nil {
			return err
		}
		files = append(files, toSlashPath(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// toSlashPath explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func toSlashPath(path string) string {
	return filepath.ToSlash(path)
}

// readManifest explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func readManifest(path string) (publishManifest, error) {
	var m publishManifest
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	return m, nil
}

// coalesceTime explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func coalesceTime(primary *time.Time, fallback time.Time) time.Time {
	if primary != nil {
		return primary.UTC()
	}
	return fallback.UTC()
}

// isPublishableMediaType explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func isPublishableMediaType(mime string) bool {
	switch mime {
	case "image/jpeg", "image/png", "image/gif":
		return true
	default:
		return false
	}
}

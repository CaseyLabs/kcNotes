// TEACHING NOTES:
// This is a Go test file. Tests are executable documentation for behavior.
// Useful Go testing concepts to notice:
// 1. Functions named `TestXxx(*testing.T)` are auto-discovered by `go test`.
// 2. Table-driven tests reduce duplication and improve coverage readability.
// 3. `t.Helper()` marks helper functions so failure lines point to call sites.
// 4. Subtests (`t.Run`) isolate scenarios while sharing setup.
package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"kcnotes/internal/domain"
)

// TestPostStoreAuthorScopeAndOwnership explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestPostStoreAuthorScopeAndOwnership(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newTestAuthStore(t)

	author := domain.User{ID: "u-author", Email: "author@example.com", PasswordHash: "x", Role: domain.RoleAuthor}
	other := domain.User{ID: "u-other", Email: "other@example.com", PasswordHash: "x", Role: domain.RoleEditor}
	mustNoErr(t, store.CreateUser(ctx, author))
	mustNoErr(t, store.CreateUser(ctx, other))

	mustNoErr(t, store.CreatePost(ctx, domain.Post{ID: "p-a", Type: domain.PostTypePost, Title: "Author Post", Slug: "author-post", BodyMD: "x", Status: domain.PostStatusDraft, AuthorID: author.ID}))
	mustNoErr(t, store.CreatePost(ctx, domain.Post{ID: "p-o", Type: domain.PostTypePost, Title: "Other Post", Slug: "other-post", BodyMD: "x", Status: domain.PostStatusDraft, AuthorID: other.ID}))

	deleted, err := store.SoftDeletePost(ctx, "p-a", author)
	mustNoErr(t, err)
	if !deleted {
		t.Fatalf("expected soft delete to affect row")
	}

	posts, total, err := store.ListPosts(ctx, PostListFilter{AuthorID: author.ID, Page: 1, PageSize: 10})
	mustNoErr(t, err)
	if total != 0 || len(posts) != 0 {
		t.Fatalf("expected deleted post to be excluded, got total=%d len=%d", total, len(posts))
	}

	deletedPosts, total, err := store.ListPosts(ctx, PostListFilter{AuthorID: author.ID, Status: domain.PostStatusDeleted, Page: 1, PageSize: 10})
	mustNoErr(t, err)
	if total != 1 || len(deletedPosts) != 1 || deletedPosts[0].ID != "p-a" {
		t.Fatalf("expected author deleted post only, got total=%d len=%d", total, len(deletedPosts))
	}

	updated, err := store.UpdatePost(ctx, domain.Post{ID: "p-o", Type: domain.PostTypePost, Title: "Nope", Slug: "other-post", BodyMD: "x", Status: domain.PostStatusDraft}, author)
	mustNoErr(t, err)
	if updated {
		t.Fatalf("expected author update on non-owned post to be blocked")
	}
}

// TestPostStoreSlugConflict explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestPostStoreSlugConflict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newTestAuthStore(t)

	user := domain.User{ID: "u-1", Email: "a@example.com", PasswordHash: "x", Role: domain.RoleAdmin}
	mustNoErr(t, store.CreateUser(ctx, user))

	mustNoErr(t, store.CreatePost(ctx, domain.Post{ID: "p-1", Type: domain.PostTypePost, Title: "One", Slug: "same-slug", BodyMD: "x", Status: domain.PostStatusDraft, AuthorID: user.ID}))
	err := store.CreatePost(ctx, domain.Post{ID: "p-2", Type: domain.PostTypePost, Title: "Two", Slug: "same-slug", BodyMD: "x", Status: domain.PostStatusDraft, AuthorID: user.ID})
	if err != ErrConflict {
		t.Fatalf("expected ErrConflict, got %v", err)
	}

	mustNoErr(t, store.CreatePost(ctx, domain.Post{ID: "p-3", Type: domain.PostTypePost, Title: "Three", Slug: "three", BodyMD: "x", Status: domain.PostStatusDraft, AuthorID: user.ID}))
	_, err = store.UpdatePost(ctx, domain.Post{ID: "p-3", Type: domain.PostTypePost, Title: "Three", Slug: "same-slug", BodyMD: "x", Status: domain.PostStatusDraft}, user)
	if err != ErrConflict {
		t.Fatalf("expected ErrConflict on update, got %v", err)
	}
}

// TestPostStoreUpdatePostPublishesDraftOnce explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestPostStoreUpdatePostPublishesDraftOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newTestAuthStore(t)

	user := domain.User{ID: "u-publish-edit", Email: "publish-edit@example.com", PasswordHash: "x", Role: domain.RoleAdmin}
	mustNoErr(t, store.CreateUser(ctx, user))

	mustNoErr(t, store.CreatePost(ctx, domain.Post{
		ID:       "p-publish-edit",
		Type:     domain.PostTypePost,
		Title:    "Draft",
		Slug:     "draft-to-published",
		BodyMD:   "draft",
		Status:   domain.PostStatusDraft,
		AuthorID: user.ID,
	}))

	publishedAt := time.Now().UTC().Add(-time.Hour)
	updated, err := store.UpdatePost(ctx, domain.Post{
		ID:          "p-publish-edit",
		Type:        domain.PostTypePost,
		Title:       "Published",
		Slug:        "draft-to-published",
		BodyMD:      "published",
		Status:      domain.PostStatusPublished,
		PublishedAt: &publishedAt,
	}, user)
	mustNoErr(t, err)
	if !updated {
		t.Fatalf("expected update to affect row")
	}

	post, err := store.GetPostByID(ctx, "p-publish-edit")
	mustNoErr(t, err)
	if post.PublishedAt == nil || !post.PublishedAt.Equal(publishedAt) {
		t.Fatalf("expected first publish time %v, got %v", publishedAt, post.PublishedAt)
	}

	republishAt := time.Now().UTC()
	updated, err = store.UpdatePost(ctx, domain.Post{
		ID:          "p-publish-edit",
		Type:        domain.PostTypePost,
		Title:       "Still Published",
		Slug:        "draft-to-published",
		BodyMD:      "edited",
		Status:      domain.PostStatusPublished,
		PublishedAt: &republishAt,
	}, user)
	mustNoErr(t, err)
	if !updated {
		t.Fatalf("expected second update to affect row")
	}

	post, err = store.GetPostByID(ctx, "p-publish-edit")
	mustNoErr(t, err)
	if post.PublishedAt == nil || !post.PublishedAt.Equal(publishedAt) {
		t.Fatalf("expected existing publish time %v, got %v", publishedAt, post.PublishedAt)
	}
}

// TestPostStoreCreatePostIDConflictTreatedAsSuccess explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestPostStoreCreatePostIDConflictTreatedAsSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newTestAuthStore(t)

	user := domain.User{ID: "u-idempotent", Email: "idempotent@example.com", PasswordHash: "x", Role: domain.RoleAdmin}
	mustNoErr(t, store.CreateUser(ctx, user))

	post := domain.Post{
		ID:       "p-idempotent",
		Type:     domain.PostTypePost,
		Title:    "Idempotent",
		Slug:     "idempotent",
		BodyMD:   "same content",
		Status:   domain.PostStatusDraft,
		AuthorID: user.ID,
	}
	mustNoErr(t, store.CreatePost(ctx, post))

	replayed := post
	replayed.Slug = "idempotent-replayed"
	mustNoErr(t, store.CreatePost(ctx, replayed))
}

// TestAuthStoreSessionAndAuditIDConflictTreatedAsSuccess explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestAuthStoreSessionAndAuditIDConflictTreatedAsSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newTestAuthStore(t)

	user := domain.User{ID: "u-session", Email: "session@example.com", PasswordHash: "x", Role: domain.RoleAdmin}
	mustNoErr(t, store.CreateUser(ctx, user))

	expires := time.Now().UTC().Add(time.Hour)
	mustNoErr(t, store.CreateSession(ctx, "s-1", user.ID, "csrf-1", expires))
	mustNoErr(t, store.CreateSession(ctx, "s-1", user.ID, "csrf-1", expires))

	mustNoErr(t, store.CreateAuditEvent(ctx, "a-1", user.ID, "test", "user", user.ID, "127.0.0.1", "ua"))
	mustNoErr(t, store.CreateAuditEvent(ctx, "a-1", user.ID, "test", "user", user.ID, "127.0.0.1", "ua"))
}

// TestPostStorePublishedQueries explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestPostStorePublishedQueries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newTestAuthStore(t)

	user := domain.User{ID: "u-pub", Email: "pub@example.com", PasswordHash: "x", Role: domain.RoleAdmin}
	mustNoErr(t, store.CreateUser(ctx, user))

	publishedAtOld := time.Now().UTC().Add(-2 * time.Hour)
	publishedAtNew := time.Now().UTC().Add(-1 * time.Hour)

	mustNoErr(t, store.CreatePost(ctx, domain.Post{
		ID:          "post-old",
		Type:        domain.PostTypePost,
		Title:       "Old",
		Slug:        "old",
		BodyMD:      "old",
		Status:      domain.PostStatusPublished,
		AuthorID:    user.ID,
		PublishedAt: &publishedAtOld,
	}))
	mustNoErr(t, store.CreatePost(ctx, domain.Post{
		ID:          "post-new",
		Type:        domain.PostTypePost,
		Title:       "New",
		Slug:        "new",
		BodyMD:      "new",
		Status:      domain.PostStatusPublished,
		AuthorID:    user.ID,
		PublishedAt: &publishedAtNew,
	}))
	mustNoErr(t, store.CreatePost(ctx, domain.Post{
		ID:       "post-draft",
		Type:     domain.PostTypePost,
		Title:    "Draft",
		Slug:     "draft",
		BodyMD:   "draft",
		Status:   domain.PostStatusDraft,
		AuthorID: user.ID,
	}))
	mustNoErr(t, store.CreatePost(ctx, domain.Post{
		ID:          "page-about",
		Type:        domain.PostTypePage,
		Title:       "About",
		Slug:        "about",
		BodyMD:      "about",
		Status:      domain.PostStatusPublished,
		AuthorID:    user.ID,
		PublishedAt: &publishedAtNew,
	}))

	posts, err := store.ListPublishedPosts(ctx, 10)
	mustNoErr(t, err)
	if len(posts) != 2 {
		t.Fatalf("expected two published posts, got %d", len(posts))
	}
	if posts[0].ID != "post-new" || posts[1].ID != "post-old" {
		t.Fatalf("expected published posts sorted newest first, got %s then %s", posts[0].ID, posts[1].ID)
	}

	post, err := store.GetPublishedPostBySlug(ctx, "new")
	mustNoErr(t, err)
	if post.ID != "post-new" {
		t.Fatalf("expected published post by slug, got %s", post.ID)
	}

	_, err = store.GetPublishedPostBySlug(ctx, "draft")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for draft slug, got %v", err)
	}

	page, err := store.GetPublishedPageBySlug(ctx, "about")
	mustNoErr(t, err)
	if page.ID != "page-about" {
		t.Fatalf("expected published page by slug, got %s", page.ID)
	}
}

// TestPostStoreFTSSearch explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestPostStoreFTSSearch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newTestAuthStore(t)

	user := domain.User{ID: "u-search", Email: "search@example.com", PasswordHash: "x", Role: domain.RoleAdmin}
	mustNoErr(t, store.CreateUser(ctx, user))
	mustNoErr(t, store.CreatePost(ctx, domain.Post{
		ID:       "p-1",
		Type:     domain.PostTypePost,
		Title:    "Galactic Update",
		Slug:     "galactic-update",
		BodyMD:   "New nebula detected in sector seven",
		Status:   domain.PostStatusDraft,
		AuthorID: user.ID,
	}))
	mustNoErr(t, store.CreatePost(ctx, domain.Post{
		ID:       "p-2",
		Type:     domain.PostTypePost,
		Title:    "Kitchen Notes",
		Slug:     "kitchen-notes",
		BodyMD:   "Baking sourdough this weekend",
		Status:   domain.PostStatusDraft,
		AuthorID: user.ID,
	}))

	posts, total, err := store.ListPosts(ctx, PostListFilter{Query: "nebula", Page: 1, PageSize: 10})
	mustNoErr(t, err)
	if total != 1 || len(posts) != 1 || posts[0].ID != "p-1" {
		t.Fatalf("expected one FTS result for nebula, got total=%d len=%d", total, len(posts))
	}

	posts, total, err = store.ListPosts(ctx, PostListFilter{Query: "galact", Page: 1, PageSize: 10})
	mustNoErr(t, err)
	if total != 1 || len(posts) != 1 || posts[0].ID != "p-1" {
		t.Fatalf("expected prefix FTS match for galact, got total=%d len=%d", total, len(posts))
	}
}

// TestAuditStoreListEvents explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestAuditStoreListEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newTestAuthStore(t)

	user := domain.User{ID: "u-audit", Email: "audit@example.com", PasswordHash: "x", Role: domain.RoleAdmin}
	mustNoErr(t, store.CreateUser(ctx, user))
	mustNoErr(t, store.CreateAuditEvent(ctx, "a-1", user.ID, "post_create", "post", "p-1", "127.0.0.1", "test"))
	mustNoErr(t, store.CreateAuditEvent(ctx, "a-2", user.ID, "login_success", "user", user.ID, "127.0.0.1", "test"))

	events, total, err := store.ListAuditEvents(ctx, AuditListFilter{Page: 1, PageSize: 10})
	mustNoErr(t, err)
	if total != 2 || len(events) != 2 {
		t.Fatalf("expected two audit events, got total=%d len=%d", total, len(events))
	}
	if events[0].ActorEmail != user.Email {
		t.Fatalf("expected actor email %s, got %s", user.Email, events[0].ActorEmail)
	}

	filtered, total, err := store.ListAuditEvents(ctx, AuditListFilter{Action: "post_create", Page: 1, PageSize: 10})
	mustNoErr(t, err)
	if total != 1 || len(filtered) != 1 || filtered[0].Action != "post_create" {
		t.Fatalf("expected action filter to return post_create only, got total=%d len=%d", total, len(filtered))
	}
}

// TestMediaStoreCreateListAndGet explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestMediaStoreCreateListAndGet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newTestAuthStore(t)

	user := domain.User{ID: "u-media", Email: "media@example.com", PasswordHash: "x", Role: domain.RoleEditor}
	mustNoErr(t, store.CreateUser(ctx, user))

	mustNoErr(t, store.CreateMedia(ctx, domain.Media{
		ID:           "m-1",
		StoredName:   "abc.jpg",
		OriginalName: "photo.jpg",
		MIME:         "image/jpeg",
		Size:         123,
		SHA256:       "deadbeef",
		CreatedBy:    user.ID,
	}))

	items, err := store.ListMedia(ctx, 10)
	mustNoErr(t, err)
	if len(items) != 1 {
		t.Fatalf("expected one media row, got %d", len(items))
	}
	if items[0].StoredName != "abc.jpg" {
		t.Fatalf("expected stored name abc.jpg, got %s", items[0].StoredName)
	}

	item, err := store.GetMediaByID(ctx, "m-1")
	mustNoErr(t, err)
	if item.OriginalName != "photo.jpg" {
		t.Fatalf("expected original name photo.jpg, got %s", item.OriginalName)
	}
}

// TestMediaStoreUsage explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestMediaStoreUsage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newTestAuthStore(t)

	u1 := domain.User{ID: "u-media-1", Email: "media1@example.com", PasswordHash: "x", Role: domain.RoleEditor}
	u2 := domain.User{ID: "u-media-2", Email: "media2@example.com", PasswordHash: "x", Role: domain.RoleEditor}
	mustNoErr(t, store.CreateUser(ctx, u1))
	mustNoErr(t, store.CreateUser(ctx, u2))

	mustNoErr(t, store.CreateMedia(ctx, domain.Media{
		ID:           "m-1",
		StoredName:   "one.jpg",
		OriginalName: "one.jpg",
		MIME:         "image/jpeg",
		Size:         100,
		SHA256:       "a",
		CreatedBy:    u1.ID,
	}))
	mustNoErr(t, store.CreateMedia(ctx, domain.Media{
		ID:           "m-2",
		StoredName:   "two.jpg",
		OriginalName: "two.jpg",
		MIME:         "image/jpeg",
		Size:         50,
		SHA256:       "b",
		CreatedBy:    u2.ID,
	}))

	userBytes, totalBytes, err := store.MediaUsage(ctx, u1.ID)
	mustNoErr(t, err)
	if userBytes != 100 {
		t.Fatalf("expected user bytes 100, got %d", userBytes)
	}
	if totalBytes != 150 {
		t.Fatalf("expected total bytes 150, got %d", totalBytes)
	}
}

// TestUserStoreListAndUpdate explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestUserStoreListAndUpdate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newTestAuthStore(t)

	admin := domain.User{ID: "u-admin", Email: "admin@example.com", PasswordHash: "x", Role: domain.RoleAdmin}
	editor := domain.User{ID: "u-editor", Email: "editor@example.com", PasswordHash: "x", Role: domain.RoleEditor}
	mustNoErr(t, store.CreateUser(ctx, admin))
	mustNoErr(t, store.CreateUser(ctx, editor))

	users, err := store.ListUsers(ctx)
	mustNoErr(t, err)
	if len(users) != 2 {
		t.Fatalf("expected two users, got %d", len(users))
	}

	updated, err := store.UpdateUserRoleDisabled(ctx, editor.ID, domain.RoleAuthor, true)
	mustNoErr(t, err)
	if !updated {
		t.Fatalf("expected user update")
	}
	updatedUser, err := store.GetUserByID(ctx, editor.ID)
	mustNoErr(t, err)
	if updatedUser.Role != domain.RoleAuthor || !updatedUser.Disabled {
		t.Fatalf("unexpected updated user: %+v", updatedUser)
	}
}

// TestSettingsStoreGetAndUpsert explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestSettingsStoreGetAndUpsert(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newTestAuthStore(t)

	mustNoErr(t, store.UpsertSettings(ctx, map[string]string{
		"site_name":     "Example CMS",
		"site_base_url": "https://example.com",
	}))

	values, err := store.GetSettings(ctx, []string{"site_name", "site_base_url", "missing_key"})
	mustNoErr(t, err)
	if values["site_name"] != "Example CMS" {
		t.Fatalf("unexpected site_name: %q", values["site_name"])
	}
	if values["site_base_url"] != "https://example.com" {
		t.Fatalf("unexpected site_base_url: %q", values["site_base_url"])
	}
	if _, ok := values["missing_key"]; ok {
		t.Fatalf("missing key should not be populated")
	}
}

// TestMFAStoreLifecycle explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func TestMFAStoreLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newTestAuthStore(t)

	user := domain.User{ID: "u-mfa", Email: "mfa@example.com", PasswordHash: "x", Role: domain.RoleAdmin}
	mustNoErr(t, store.CreateUser(ctx, user))

	updated, err := store.SetUserMFASecret(ctx, user.ID, "ABC123")
	mustNoErr(t, err)
	if !updated {
		t.Fatalf("expected mfa secret update")
	}

	enabled, err := store.EnableUserMFA(ctx, user.ID)
	mustNoErr(t, err)
	if !enabled {
		t.Fatalf("expected mfa enabled")
	}

	mustNoErr(t, store.ReplaceRecoveryCodeHashes(ctx, user.ID, []string{"h1", "h2"}))
	count, err := store.CountUnusedRecoveryCodes(ctx, user.ID)
	mustNoErr(t, err)
	if count != 2 {
		t.Fatalf("expected 2 recovery codes, got %d", count)
	}

	used, err := store.ConsumeRecoveryCodeHash(ctx, user.ID, "h1")
	mustNoErr(t, err)
	if !used {
		t.Fatalf("expected recovery code consumed")
	}
	count, err = store.CountUnusedRecoveryCodes(ctx, user.ID)
	mustNoErr(t, err)
	if count != 1 {
		t.Fatalf("expected 1 unused recovery code, got %d", count)
	}

	disabled, err := store.DisableUserMFA(ctx, user.ID)
	mustNoErr(t, err)
	if !disabled {
		t.Fatalf("expected mfa disabled")
	}
	reloaded, err := store.GetUserByID(ctx, user.ID)
	mustNoErr(t, err)
	if reloaded.MFAEnabled || reloaded.MFASecret != "" {
		t.Fatalf("expected mfa fields reset, got enabled=%v secret=%q", reloaded.MFAEnabled, reloaded.MFASecret)
	}
	count, err = store.CountUnusedRecoveryCodes(ctx, user.ID)
	mustNoErr(t, err)
	if count != 0 {
		t.Fatalf("expected recovery codes cleared, got %d", count)
	}
}

// newTestAuthStore explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func newTestAuthStore(t *testing.T) *AuthStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cms.db")
	conn, err := Open(OpenConfig{
		Mode:   DBModeLocal,
		DBPath: path,
	})
	mustNoErr(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mustNoErr(t, RunMigrations(ctx, conn.DB))
	return NewAuthStore(conn.DB)
}

// mustNoErr explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

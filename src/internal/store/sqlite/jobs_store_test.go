package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"kcnotes/internal/domain"
)

func TestJobsStoreEnsureClaimAndComplete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newJobsTestAuthStore(t)

	inserted, err := store.EnsureJob(ctx, Job{
		ID:          "job-1",
		Type:        "autosave_cleanup",
		Key:         "autosave-cleanup",
		PayloadJSON: `{}`,
		MaxAttempts: 3,
		RunAt:       time.Now().UTC().Add(-time.Minute),
	})
	mustNoErr(t, err)
	if !inserted {
		t.Fatalf("expected first ensure job to insert")
	}

	inserted, err = store.EnsureJob(ctx, Job{
		ID:          "job-2",
		Type:        "autosave_cleanup",
		Key:         "autosave-cleanup",
		PayloadJSON: `{}`,
		MaxAttempts: 3,
		RunAt:       time.Now().UTC(),
	})
	mustNoErr(t, err)
	if inserted {
		t.Fatalf("expected duplicate key ensure to no-op")
	}

	job, ok, err := store.ClaimDueJob(ctx, time.Now().UTC())
	mustNoErr(t, err)
	if !ok {
		t.Fatalf("expected due job to be claimed")
	}
	if job.ID != "job-1" || job.Status != JobStatusRunning {
		t.Fatalf("unexpected claimed job: %+v", job)
	}

	nextRun := time.Now().UTC().Add(time.Hour)
	mustNoErr(t, store.CompleteJob(ctx, job.ID, nextRun))

	pending, running, err := store.JobQueueCounts(ctx)
	mustNoErr(t, err)
	if pending != 1 || running != 0 {
		t.Fatalf("unexpected queue counts pending=%d running=%d", pending, running)
	}
}

func TestDeleteAutosaveSnapshotsOlderThan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newJobsTestAuthStore(t)

	author := domain.User{ID: "u-jobs-autosave", Email: "jobs-autosave@example.com", PasswordHash: "x", Role: domain.RoleAuthor}
	mustNoErr(t, store.CreateUser(ctx, author))
	mustNoErr(t, store.CreatePost(ctx, domain.Post{ID: "p-jobs-autosave", Type: domain.PostTypePost, Title: "Canonical", Slug: "jobs-canonical", BodyMD: "body", Status: domain.PostStatusDraft, AuthorID: author.ID}))

	post, err := store.GetPostByID(ctx, "p-jobs-autosave")
	mustNoErr(t, err)
	_, err = store.UpsertAutosaveSnapshot(ctx, domain.AutosaveSnapshot{
		PostID:        post.ID,
		AuthorID:      author.ID,
		Type:          domain.PostTypePost,
		Title:         "Old draft",
		Slug:          "jobs-old-draft",
		BodyMD:        "old",
		Status:        domain.PostStatusDraft,
		BaseUpdatedAt: post.UpdatedAt,
	}, author)
	mustNoErr(t, err)

	_, err = store.db.ExecContext(ctx, `
		UPDATE autosave_snapshots
		SET updated_at = ?
		WHERE post_id = ? AND author_id = ?
	`, time.Now().UTC().Add(-48*time.Hour).Format(time.RFC3339Nano), post.ID, author.ID)
	mustNoErr(t, err)

	deleted, err := store.DeleteAutosaveSnapshotsOlderThan(ctx, time.Now().UTC().Add(-24*time.Hour))
	mustNoErr(t, err)
	if deleted != 1 {
		t.Fatalf("expected one stale snapshot deleted, got %d", deleted)
	}

	_, err = store.GetLatestAutosaveSnapshot(ctx, post.ID, author.ID)
	if err != ErrNotFound {
		t.Fatalf("expected deleted autosave snapshot to be gone, got %v", err)
	}

	canonical, err := store.GetPostByID(ctx, post.ID)
	mustNoErr(t, err)
	if canonical.Title != "Canonical" {
		t.Fatalf("expected canonical post unchanged, got %+v", canonical)
	}
}

func newJobsTestAuthStore(t *testing.T) *AuthStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	conn, err := Open(OpenConfig{Mode: DBModeLocal, DBPath: dbPath})
	mustNoErr(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	mustNoErr(t, RunMigrations(ctx, conn.DB))
	return NewAuthStore(conn.DB)
}

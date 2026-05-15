package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"kcnotes/internal/domain"
	"kcnotes/internal/store/sqlite"
)

func TestRunJobsOnceExecutesAutosaveCleanupHandler(t *testing.T) {
	ctx := context.Background()
	application := newTestApp(t)
	application.jobStore = sqlite.NewAuthStore(application.db)
	application.autosaveRetention = 24 * time.Hour
	application.autosaveCleanupEvery = 2 * time.Hour

	author := domain.User{ID: "u-app-jobs-autosave", Email: "app-jobs-autosave@example.com", Role: domain.RoleAuthor}
	if err := application.jobStore.CreateUser(ctx, author); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := application.jobStore.CreatePost(ctx, domain.Post{ID: "p-app-jobs-autosave", Type: domain.PostTypePost, Title: "Canonical", Slug: "app-jobs-canonical", BodyMD: "body", Status: domain.PostStatusDraft, AuthorID: author.ID}); err != nil {
		t.Fatalf("create post: %v", err)
	}
	post, err := application.jobStore.GetPostByID(ctx, "p-app-jobs-autosave")
	if err != nil {
		t.Fatalf("get post: %v", err)
	}
	if _, err := application.jobStore.UpsertAutosaveSnapshot(ctx, domain.AutosaveSnapshot{
		PostID:        post.ID,
		AuthorID:      author.ID,
		Type:          domain.PostTypePost,
		Title:         "Old draft",
		Slug:          "app-jobs-old-draft",
		BodyMD:        "old",
		Status:        domain.PostStatusDraft,
		BaseUpdatedAt: post.UpdatedAt,
	}, author); err != nil {
		t.Fatalf("upsert autosave snapshot: %v", err)
	}

	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	if _, err := application.db.ExecContext(ctx, `
		UPDATE autosave_snapshots
		SET updated_at = ?
		WHERE post_id = ? AND author_id = ?
	`, now.Add(-48*time.Hour).Format(time.RFC3339Nano), post.ID, author.ID); err != nil {
		t.Fatalf("age autosave snapshot: %v", err)
	}
	ensureTestJob(t, application, sqlite.Job{
		ID:          "job-app-autosave",
		Type:        autosaveCleanupJobType,
		Key:         "app-autosave-cleanup",
		PayloadJSON: `{}`,
		MaxAttempts: 3,
		RunAt:       now.Add(-time.Minute),
	})

	application.runJobsOnceAt(ctx, now)

	if _, err := application.jobStore.GetLatestAutosaveSnapshot(ctx, post.ID, author.ID); err != sqlite.ErrNotFound {
		t.Fatalf("expected stale autosave snapshot to be deleted, got %v", err)
	}
	status, runAt, lastError := readJobState(t, application, "job-app-autosave")
	if status != sqlite.JobStatusPending {
		t.Fatalf("expected autosave cleanup to be rescheduled pending, got %q", status)
	}
	if !runAt.Equal(now.Add(2 * time.Hour)) {
		t.Fatalf("expected next run at %s, got %s", now.Add(2*time.Hour), runAt)
	}
	if lastError != "" {
		t.Fatalf("expected last error to be cleared, got %q", lastError)
	}
	snapshot := application.JobsMetricsSnapshot(ctx)
	if got := snapshot.ByType[autosaveCleanupJobType].SuccessTotal; got != 1 {
		t.Fatalf("expected autosave success metric 1, got %d", got)
	}
}

func TestRunJobsOnceExecutesRegisteredNonAutosaveHandler(t *testing.T) {
	ctx := context.Background()
	application := newTestApp(t)
	application.jobStore = sqlite.NewAuthStore(application.db)

	var called int
	application.registerJobHandler("test_job", func(ctx context.Context, job sqlite.Job, now time.Time) (jobRunResult, error) {
		payload, err := decodeJobPayload[struct {
			Name string `json:"name"`
		}](job)
		if err != nil {
			return jobRunResult{}, err
		}
		if payload.Name != "ok" {
			return jobRunResult{}, fmt.Errorf("unexpected payload name %q", payload.Name)
		}
		called++
		return jobRunResult{nextRunAt: now.Add(10 * time.Minute), reschedule: true}, nil
	})

	now := time.Date(2026, 5, 15, 13, 0, 0, 0, time.UTC)
	ensureTestJob(t, application, sqlite.Job{
		ID:          "job-app-test",
		Type:        "test_job",
		Key:         "app-test-job",
		PayloadJSON: `{"name":"ok"}`,
		MaxAttempts: 3,
		RunAt:       now.Add(-time.Minute),
	})

	application.runJobsOnceAt(ctx, now)

	if called != 1 {
		t.Fatalf("expected test handler to be called once, got %d", called)
	}
	status, runAt, lastError := readJobState(t, application, "job-app-test")
	if status != sqlite.JobStatusPending {
		t.Fatalf("expected test job to be rescheduled pending, got %q", status)
	}
	if !runAt.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("expected test job next run at %s, got %s", now.Add(10*time.Minute), runAt)
	}
	if lastError != "" {
		t.Fatalf("expected test job last error cleared, got %q", lastError)
	}
	snapshot := application.JobsMetricsSnapshot(ctx)
	if snapshot.RunsTotal != 1 || snapshot.SuccessTotal != 1 || snapshot.FailureTotal != 0 {
		t.Fatalf("unexpected total metrics: %+v", snapshot)
	}
	if got := snapshot.ByType["test_job"].SuccessTotal; got != 1 {
		t.Fatalf("expected test job success metric 1, got %d", got)
	}
}

func TestRunJobsOnceCompletesOneShotHandler(t *testing.T) {
	ctx := context.Background()
	application := newTestApp(t)
	application.jobStore = sqlite.NewAuthStore(application.db)

	var called int
	application.registerJobHandler("one_shot_job", func(ctx context.Context, job sqlite.Job, now time.Time) (jobRunResult, error) {
		called++
		return jobRunResult{}, nil
	})

	now := time.Date(2026, 5, 15, 13, 30, 0, 0, time.UTC)
	ensureTestJob(t, application, sqlite.Job{
		ID:          "job-app-one-shot",
		Type:        "one_shot_job",
		Key:         "app-one-shot-job",
		PayloadJSON: `{}`,
		MaxAttempts: 3,
		RunAt:       now.Add(-time.Minute),
	})

	application.runJobsOnceAt(ctx, now)
	application.runJobsOnceAt(ctx, now.Add(time.Minute))

	if called != 1 {
		t.Fatalf("expected one-shot handler to be called once, got %d", called)
	}
	status, _, lastError := readJobState(t, application, "job-app-one-shot")
	if status != sqlite.JobStatusSucceeded {
		t.Fatalf("expected one-shot job to complete successfully, got %q", status)
	}
	if lastError != "" {
		t.Fatalf("expected one-shot job last error cleared, got %q", lastError)
	}
	snapshot := application.JobsMetricsSnapshot(ctx)
	if snapshot.RunsTotal != 1 || snapshot.SuccessTotal != 1 || snapshot.FailureTotal != 0 {
		t.Fatalf("unexpected total metrics: %+v", snapshot)
	}
	if got := snapshot.ByType["one_shot_job"].SuccessTotal; got != 1 {
		t.Fatalf("expected one-shot job success metric 1, got %d", got)
	}
}

func TestRunJobsOnceFailsUnsupportedJobType(t *testing.T) {
	ctx := context.Background()
	application := newTestApp(t)
	application.jobStore = sqlite.NewAuthStore(application.db)

	now := time.Date(2026, 5, 15, 14, 0, 0, 0, time.UTC)
	ensureTestJob(t, application, sqlite.Job{
		ID:          "job-app-unsupported",
		Type:        "unsupported_job",
		Key:         "app-unsupported-job",
		PayloadJSON: `{}`,
		MaxAttempts: 1,
		RunAt:       now.Add(-time.Minute),
	})

	application.runJobsOnceAt(ctx, now)

	status, _, lastError := readJobState(t, application, "job-app-unsupported")
	if status != sqlite.JobStatusFailed {
		t.Fatalf("expected unsupported job to fail terminally, got %q", status)
	}
	if !strings.Contains(lastError, `unsupported job type "unsupported_job"`) {
		t.Fatalf("expected unsupported job error, got %q", lastError)
	}
	snapshot := application.JobsMetricsSnapshot(ctx)
	if got := snapshot.ByType["unsupported_job"].FailureTotal; got != 1 {
		t.Fatalf("expected unsupported job failure metric 1, got %d", got)
	}
}

func TestRunJobsOnceFailsInvalidPayload(t *testing.T) {
	ctx := context.Background()
	application := newTestApp(t)
	application.jobStore = sqlite.NewAuthStore(application.db)

	now := time.Date(2026, 5, 15, 15, 0, 0, 0, time.UTC)
	ensureTestJob(t, application, sqlite.Job{
		ID:          "job-app-invalid-payload",
		Type:        autosaveCleanupJobType,
		Key:         "app-invalid-payload",
		PayloadJSON: `{"unexpected":true}`,
		MaxAttempts: 1,
		RunAt:       now.Add(-time.Minute),
	})

	application.runJobsOnceAt(ctx, now)

	status, _, lastError := readJobState(t, application, "job-app-invalid-payload")
	if status != sqlite.JobStatusFailed {
		t.Fatalf("expected invalid payload job to fail terminally, got %q", status)
	}
	if !strings.Contains(lastError, `unknown field "unexpected"`) {
		t.Fatalf("expected invalid payload error, got %q", lastError)
	}
}

func TestRunJobsOnceRetriesHandlerErrorBeforeTerminalFailure(t *testing.T) {
	ctx := context.Background()
	application := newTestApp(t)
	application.jobStore = sqlite.NewAuthStore(application.db)
	application.registerJobHandler("flaky_job", func(ctx context.Context, job sqlite.Job, now time.Time) (jobRunResult, error) {
		return jobRunResult{}, errors.New("temporary boom")
	})

	now := time.Date(2026, 5, 15, 16, 0, 0, 0, time.UTC)
	ensureTestJob(t, application, sqlite.Job{
		ID:          "job-app-flaky",
		Type:        "flaky_job",
		Key:         "app-flaky-job",
		PayloadJSON: `{}`,
		MaxAttempts: 2,
		RunAt:       now.Add(-time.Minute),
	})

	application.runJobsOnceAt(ctx, now)

	status, runAt, lastError := readJobState(t, application, "job-app-flaky")
	if status != sqlite.JobStatusPending {
		t.Fatalf("expected first failure to reschedule pending, got %q", status)
	}
	if !runAt.Equal(now.Add(30 * time.Second)) {
		t.Fatalf("expected retry at %s, got %s", now.Add(30*time.Second), runAt)
	}
	if lastError != "temporary boom" {
		t.Fatalf("expected retry error recorded, got %q", lastError)
	}

	if _, err := application.db.ExecContext(ctx, `UPDATE jobs SET run_at = ? WHERE id = ?`, now.Add(-time.Minute).Format(time.RFC3339Nano), "job-app-flaky"); err != nil {
		t.Fatalf("make flaky job due again: %v", err)
	}
	application.runJobsOnceAt(ctx, now.Add(time.Minute))

	status, _, lastError = readJobState(t, application, "job-app-flaky")
	if status != sqlite.JobStatusFailed {
		t.Fatalf("expected second failure to be terminal, got %q", status)
	}
	if lastError != "temporary boom" {
		t.Fatalf("expected terminal error recorded, got %q", lastError)
	}
	snapshot := application.JobsMetricsSnapshot(ctx)
	if got := snapshot.ByType["flaky_job"].FailureTotal; got != 2 {
		t.Fatalf("expected two flaky failure metrics, got %d", got)
	}
}

func ensureTestJob(t *testing.T, application *App, job sqlite.Job) {
	t.Helper()
	changed, err := application.jobStore.EnsureJob(context.Background(), job)
	if err != nil {
		t.Fatalf("ensure job: %v", err)
	}
	if !changed {
		t.Fatalf("expected job %q to be inserted or rearmed", job.ID)
	}
}

func readJobState(t *testing.T, application *App, id string) (status string, runAt time.Time, lastError string) {
	t.Helper()
	var runAtRaw string
	err := application.db.QueryRowContext(context.Background(), `
		SELECT status, run_at, COALESCE(last_error, '')
		FROM jobs
		WHERE id = ?
	`, id).Scan(&status, &runAtRaw, &lastError)
	if err != nil {
		t.Fatalf("read job state: %v", err)
	}
	runAt, err = time.Parse(time.RFC3339Nano, runAtRaw)
	if err != nil {
		t.Fatalf("parse job run_at: %v", err)
	}
	return status, runAt, lastError
}

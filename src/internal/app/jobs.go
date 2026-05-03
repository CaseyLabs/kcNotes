package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	storesqlite "kcnotes/internal/store/sqlite"
)

const autosaveCleanupJobType = "autosave_cleanup"
const autosaveCleanupJobKey = "autosave-cleanup"

type JobsMetricsSnapshot struct {
	RunsTotal    uint64
	SuccessTotal uint64
	FailureTotal uint64
	Pending      int
	Running      int
}

type jobsMetrics struct {
	runsTotal    atomic.Uint64
	successTotal atomic.Uint64
	failureTotal atomic.Uint64
}

func (m *jobsMetrics) snapshot() JobsMetricsSnapshot {
	return JobsMetricsSnapshot{
		RunsTotal:    m.runsTotal.Load(),
		SuccessTotal: m.successTotal.Load(),
		FailureTotal: m.failureTotal.Load(),
	}
}

// StartBackgroundJobs schedules the recurring autosave cleanup and starts polling.
func (a *App) StartBackgroundJobs(ctx context.Context) error {
	if a.jobStore == nil {
		a.jobStore = storesqlite.NewAuthStore(a.db)
	}
	if a.jobsPollInterval <= 0 {
		a.jobsPollInterval = 15 * time.Second
	}
	if a.autosaveRetention <= 0 {
		a.autosaveRetention = 720 * time.Hour
	}
	if a.autosaveCleanupEvery <= 0 {
		a.autosaveCleanupEvery = time.Hour
	}
	if a.jobsMetrics == nil {
		a.jobsMetrics = &jobsMetrics{}
	}

	_, err := a.jobStore.EnsureJob(ctx, storesqlite.Job{
		ID:          fmt.Sprintf("job-%d", time.Now().UTC().UnixNano()),
		Type:        autosaveCleanupJobType,
		Key:         autosaveCleanupJobKey,
		PayloadJSON: "{}",
		MaxAttempts: 3,
		RunAt:       time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("ensure autosave cleanup job: %w", err)
	}

	go a.runJobs(ctx)
	go a.logJobsMetrics(ctx)
	return nil
}

func (a *App) runJobs(ctx context.Context) {
	ticker := time.NewTicker(a.jobsPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.runJobsOnce(ctx)
		}
	}
}

func (a *App) runJobsOnce(ctx context.Context) {
	now := time.Now().UTC()
	job, ok, err := a.jobStore.ClaimDueJob(ctx, now, 5*time.Minute)
	if err != nil {
		a.logger.Error("claim due job", "error", err)
		return
	}
	if !ok {
		return
	}

	a.jobsMetrics.runsTotal.Add(1)
	if err := a.executeJob(ctx, job, now); err != nil {
		a.jobsMetrics.failureTotal.Add(1)
		a.logger.Error("run job", "job_type", job.Type, "job_key", job.Key, "error", err)
		_ = a.jobStore.FailJob(ctx, job, now, 30*time.Second, err.Error())
		return
	}

	a.jobsMetrics.successTotal.Add(1)
	if err := a.jobStore.CompleteJob(ctx, job.ID, now.Add(a.autosaveCleanupEvery)); err != nil {
		a.logger.Error("complete job", "job_type", job.Type, "job_key", job.Key, "error", err)
	}
}

func (a *App) executeJob(ctx context.Context, job storesqlite.Job, now time.Time) error {
	switch job.Type {
	case autosaveCleanupJobType:
		cutoff := now.Add(-a.autosaveRetention)
		deleted, err := a.jobStore.DeleteAutosaveSnapshotsOlderThan(ctx, cutoff)
		if err != nil {
			return err
		}
		a.logger.Info("autosave cleanup completed", "deleted_snapshots", deleted, "retention", a.autosaveRetention.String())
		return nil
	default:
		return fmt.Errorf("unsupported job type %q", job.Type)
	}
}

func (a *App) logJobsMetrics(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snapshot := a.JobsMetricsSnapshot(ctx)
			a.logger.Info("jobs metrics", "runs_total", snapshot.RunsTotal, "success_total", snapshot.SuccessTotal, "failure_total", snapshot.FailureTotal, "pending", snapshot.Pending, "running", snapshot.Running)
		}
	}
}

func (a *App) JobsMetricsSnapshot(ctx context.Context) JobsMetricsSnapshot {
	base := JobsMetricsSnapshot{}
	if a.jobsMetrics != nil {
		base = a.jobsMetrics.snapshot()
	}
	if a.jobStore == nil {
		return base
	}
	pending, running, err := a.jobStore.JobQueueCounts(ctx)
	if err != nil {
		a.logger.Debug("jobs queue counts unavailable", "error", err)
		return base
	}
	base.Pending = pending
	base.Running = running
	return base
}

func (a *App) JobsLogger() *slog.Logger {
	return a.logger
}

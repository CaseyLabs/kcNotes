package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	storesqlite "kcnotes/internal/store/sqlite"
)

const autosaveCleanupJobType = "autosave_cleanup"
const autosaveCleanupJobKey = "autosave-cleanup"

type jobHandlerFunc func(ctx context.Context, job storesqlite.Job, now time.Time) (time.Time, error)

type autosaveCleanupPayload struct{}

type JobsMetricsSnapshot struct {
	RunsTotal    uint64
	SuccessTotal uint64
	FailureTotal uint64
	Pending      int
	Running      int
	ByType       map[string]JobTypeMetricsSnapshot
}

type JobTypeMetricsSnapshot struct {
	RunsTotal    uint64
	SuccessTotal uint64
	FailureTotal uint64
}

type jobsMetrics struct {
	runsTotal    atomic.Uint64
	successTotal atomic.Uint64
	failureTotal atomic.Uint64
	mu           sync.Mutex
	byType       map[string]*jobTypeMetrics
}

type jobTypeMetrics struct {
	runsTotal    atomic.Uint64
	successTotal atomic.Uint64
	failureTotal atomic.Uint64
}

func (m *jobsMetrics) snapshot() JobsMetricsSnapshot {
	if m == nil {
		return JobsMetricsSnapshot{ByType: map[string]JobTypeMetricsSnapshot{}}
	}
	byType := make(map[string]JobTypeMetricsSnapshot)
	m.mu.Lock()
	for jobType, counters := range m.byType {
		byType[jobType] = counters.snapshot()
	}
	m.mu.Unlock()
	return JobsMetricsSnapshot{
		RunsTotal:    m.runsTotal.Load(),
		SuccessTotal: m.successTotal.Load(),
		FailureTotal: m.failureTotal.Load(),
		ByType:       byType,
	}
}

func (m *jobTypeMetrics) snapshot() JobTypeMetricsSnapshot {
	if m == nil {
		return JobTypeMetricsSnapshot{}
	}
	return JobTypeMetricsSnapshot{
		RunsTotal:    m.runsTotal.Load(),
		SuccessTotal: m.successTotal.Load(),
		FailureTotal: m.failureTotal.Load(),
	}
}

func (m *jobsMetrics) recordRun(jobType string) {
	if m == nil {
		return
	}
	m.runsTotal.Add(1)
	m.metricsForType(jobType).runsTotal.Add(1)
}

func (m *jobsMetrics) recordSuccess(jobType string) {
	if m == nil {
		return
	}
	m.successTotal.Add(1)
	m.metricsForType(jobType).successTotal.Add(1)
}

func (m *jobsMetrics) recordFailure(jobType string) {
	if m == nil {
		return
	}
	m.failureTotal.Add(1)
	m.metricsForType(jobType).failureTotal.Add(1)
}

func (m *jobsMetrics) metricsForType(jobType string) *jobTypeMetrics {
	if jobType == "" {
		jobType = "unknown"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.byType == nil {
		m.byType = make(map[string]*jobTypeMetrics)
	}
	counters := m.byType[jobType]
	if counters == nil {
		counters = &jobTypeMetrics{}
		m.byType[jobType] = counters
	}
	return counters
}

// StartBackgroundJobs schedules the recurring autosave cleanup and starts polling.
func (a *App) StartBackgroundJobs(ctx context.Context) error {
	a.configureJobsRuntime()

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

func (a *App) configureJobsRuntime() {
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
	a.registerDefaultJobHandlers()
}

func (a *App) registerDefaultJobHandlers() {
	if a.jobHandlers == nil {
		a.jobHandlers = make(map[string]jobHandlerFunc)
	}
	if _, ok := a.jobHandlers[autosaveCleanupJobType]; !ok {
		a.jobHandlers[autosaveCleanupJobType] = a.runAutosaveCleanupJob
	}
}

func (a *App) registerJobHandler(jobType string, handler jobHandlerFunc) {
	if a.jobHandlers == nil {
		a.jobHandlers = make(map[string]jobHandlerFunc)
	}
	a.jobHandlers[jobType] = handler
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
	a.runJobsOnceAt(ctx, time.Now().UTC())
}

func (a *App) runJobsOnceAt(ctx context.Context, now time.Time) {
	a.configureJobsRuntime()
	job, ok, err := a.jobStore.ClaimDueJob(ctx, now, 5*time.Minute)
	if err != nil {
		a.logger.Error("claim due job", "error", err)
		return
	}
	if !ok {
		return
	}

	a.jobsMetrics.recordRun(job.Type)
	nextRunAt, err := a.executeJob(ctx, job, now)
	if err != nil {
		a.jobsMetrics.recordFailure(job.Type)
		a.logger.Error("run job", "job_type", job.Type, "job_key", job.Key, "error", err)
		_ = a.jobStore.FailJob(ctx, job, now, 30*time.Second, err.Error())
		return
	}

	a.jobsMetrics.recordSuccess(job.Type)
	if err := a.jobStore.CompleteJob(ctx, job, nextRunAt); err != nil {
		a.logger.Error("complete job", "job_type", job.Type, "job_key", job.Key, "error", err)
	}
}

func (a *App) executeJob(ctx context.Context, job storesqlite.Job, now time.Time) (time.Time, error) {
	handler := a.jobHandlers[job.Type]
	if handler == nil {
		return time.Time{}, fmt.Errorf("unsupported job type %q", job.Type)
	}
	return handler(ctx, job, now)
}

func (a *App) runAutosaveCleanupJob(ctx context.Context, job storesqlite.Job, now time.Time) (time.Time, error) {
	if _, err := decodeJobPayload[autosaveCleanupPayload](job); err != nil {
		return time.Time{}, err
	}
	cutoff := now.Add(-a.autosaveRetention)
	deleted, err := a.jobStore.DeleteAutosaveSnapshotsOlderThan(ctx, cutoff)
	if err != nil {
		return time.Time{}, err
	}
	a.logger.Info("autosave cleanup completed", "deleted_snapshots", deleted, "retention", a.autosaveRetention.String())
	return now.Add(a.autosaveCleanupEvery), nil
}

func decodeJobPayload[T any](job storesqlite.Job) (T, error) {
	var payload T
	raw := strings.TrimSpace(job.PayloadJSON)
	if raw == "" {
		raw = "{}"
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return payload, fmt.Errorf("decode %s payload: %w", job.Type, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return payload, fmt.Errorf("decode %s payload: trailing json content", job.Type)
	}
	return payload, nil
}

func (a *App) logJobsMetrics(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			jobs := a.JobsMetricsSnapshot(ctx)
			requests := a.metrics.Snapshot()
			retries := storesqlite.RetryMetricsSnapshot{}
			if a.jobStore != nil {
				retries = a.jobStore.RetryMetrics()
			}
			a.logger.Info("operational metrics",
				"jobs_runs_total", jobs.RunsTotal,
				"jobs_success_total", jobs.SuccessTotal,
				"jobs_failure_total", jobs.FailureTotal,
				"jobs_by_type", jobs.ByType,
				"jobs_pending", jobs.Pending,
				"jobs_running", jobs.Running,
				"http_requests_total", requests.RequestsTotal,
				"http_status_1xx", requests.Status1xx,
				"http_status_2xx", requests.Status2xx,
				"http_status_3xx", requests.Status3xx,
				"http_status_4xx", requests.Status4xx,
				"http_status_5xx", requests.Status5xx,
				"http_duration_buckets", requests.Duration,
				"login_success_total", requests.LoginSuccess,
				"login_failure_total", requests.LoginFailure,
				"login_lockout_hits_total", requests.LoginLockoutHits,
				"login_lockout_transitions_total", requests.LoginLockoutTransitions,
				"rate_limit_login_ip_total", requests.RateLimitLoginIP,
				"rate_limit_login_account_total", requests.RateLimitLoginAccount,
				"rate_limit_sensitive_total", requests.RateLimitSensitive,
				"db_exec_retries", retries.ExecRetries,
				"db_query_retries", retries.QueryRetries,
				"db_tx_retries", retries.TxRetries,
				"db_retry_failures", retries.RetryFailures,
			)
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

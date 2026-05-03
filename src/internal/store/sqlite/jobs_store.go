package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	JobStatusPending = "pending"
	JobStatusRunning = "running"
	JobStatusFailed  = "failed"
)

type Job struct {
	ID             string
	Type           string
	Key            string
	PayloadJSON    string
	Status         string
	Attempts       int
	MaxAttempts    int
	RunAt          time.Time
	LockedAt       *time.Time
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastFinishedAt *time.Time
}

type JobsMetricsSnapshot struct {
	RunsTotal    uint64
	SuccessTotal uint64
	FailureTotal uint64
	Pending      int
	Running      int
}

// EnsureJob creates a job if the key does not already exist.
func (s *AuthStore) EnsureJob(ctx context.Context, job Job) (bool, error) {
	if job.ID == "" || job.Type == "" || job.Key == "" {
		return false, fmt.Errorf("job id, type, and key are required")
	}
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = 3
	}
	if job.RunAt.IsZero() {
		job.RunAt = time.Now().UTC()
	}
	if job.PayloadJSON == "" {
		job.PayloadJSON = "{}"
	}
	if !json.Valid([]byte(job.PayloadJSON)) {
		return false, fmt.Errorf("job payload must be valid json")
	}

	var res sql.Result
	err := s.retry.ExecRetry(ctx, func() error {
		var err error
		res, err = s.db.ExecContext(ctx, `
			INSERT INTO jobs(id, job_type, job_key, payload_json, status, attempts, max_attempts, run_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 0, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
			ON CONFLICT(job_key) DO UPDATE SET
				status = excluded.status,
				attempts = 0,
				max_attempts = excluded.max_attempts,
				run_at = excluded.run_at,
				locked_at = NULL,
				last_error = NULL,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE jobs.status = ?
		`, job.ID, job.Type, job.Key, job.PayloadJSON, JobStatusPending, job.MaxAttempts, job.RunAt.UTC().Format(time.RFC3339Nano), JobStatusFailed)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("ensure job: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("ensure job rows: %w", err)
	}
	return rows > 0, nil
}

// ClaimDueJob atomically claims the next due pending job or a stale running job.
func (s *AuthStore) ClaimDueJob(ctx context.Context, now time.Time, staleAfter time.Duration) (Job, bool, error) {
	nowUTC := now.UTC().Format(time.RFC3339Nano)
	staleCutoffUTC := now.UTC().Add(-staleAfter).Format(time.RFC3339Nano)
	var claimed Job
	err := s.retry.TxRetry(ctx, s.db, nil, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			SELECT id, job_type, job_key, payload_json, attempts, max_attempts, run_at, locked_at, created_at, updated_at
			FROM jobs
			WHERE
				(status = ? AND datetime(run_at) <= datetime(?))
				OR
				(status = ? AND (locked_at IS NULL OR datetime(locked_at) <= datetime(?)))
			ORDER BY
				CASE status WHEN ? THEN 0 ELSE 1 END,
				datetime(run_at) ASC,
				created_at ASC
			LIMIT 1
		`, JobStatusPending, nowUTC, JobStatusRunning, staleCutoffUTC, JobStatusPending)

		var runAtRaw, createdAtRaw, updatedAtRaw string
		var lockedAtRaw sql.NullString
		err := row.Scan(&claimed.ID, &claimed.Type, &claimed.Key, &claimed.PayloadJSON, &claimed.Attempts, &claimed.MaxAttempts, &runAtRaw, &lockedAtRaw, &createdAtRaw, &updatedAtRaw)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}

		claimed.RunAt, err = time.Parse(time.RFC3339Nano, runAtRaw)
		if err != nil {
			return fmt.Errorf("parse run_at: %w", err)
		}
		claimed.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAtRaw)
		if err != nil {
			return fmt.Errorf("parse created_at: %w", err)
		}
		claimed.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAtRaw)
		if err != nil {
			return fmt.Errorf("parse updated_at: %w", err)
		}
		if lockedAtRaw.Valid {
			lockedAt, err := time.Parse(time.RFC3339Nano, lockedAtRaw.String)
			if err != nil {
				return fmt.Errorf("parse locked_at: %w", err)
			}
			claimed.LockedAt = &lockedAt
		}

		res, err := tx.ExecContext(ctx, `
			UPDATE jobs
			SET status = ?, attempts = attempts + 1, locked_at = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE
				id = ?
				AND (
					status = ?
					OR
					(status = ? AND (locked_at IS NULL OR datetime(locked_at) <= datetime(?)))
				)
		`, JobStatusRunning, nowUTC, claimed.ID, JobStatusPending, JobStatusRunning, staleCutoffUTC)
		if err != nil {
			return err
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			claimed = Job{}
			return nil
		}
		claimed.Status = JobStatusRunning
		lock := now.UTC()
		claimed.LockedAt = &lock
		claimed.Attempts++
		return nil
	})
	if err != nil {
		return Job{}, false, fmt.Errorf("claim due job: %w", err)
	}
	if claimed.ID == "" {
		return Job{}, false, nil
	}
	return claimed, true, nil
}

// CompleteJob marks a running job as successful and re-schedules it.
func (s *AuthStore) CompleteJob(ctx context.Context, id string, nextRunAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = ?, attempts = 0, run_at = ?, locked_at = NULL, last_error = NULL, last_finished_at = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, JobStatusPending, nextRunAt.UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	return nil
}

// FailJob re-schedules the job or marks it failed when max attempts is exceeded.
func (s *AuthStore) FailJob(ctx context.Context, job Job, now time.Time, retryAfter time.Duration, errMsg string) error {
	status := JobStatusPending
	nextRun := now.UTC().Add(retryAfter)
	if job.Attempts >= job.MaxAttempts {
		status = JobStatusFailed
		nextRun = now.UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = ?, run_at = ?, locked_at = NULL, last_error = ?, last_finished_at = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, status, nextRun.Format(time.RFC3339Nano), errMsg, now.UTC().Format(time.RFC3339Nano), job.ID)
	if err != nil {
		return fmt.Errorf("fail job: %w", err)
	}
	return nil
}

func (s *AuthStore) JobQueueCounts(ctx context.Context) (pending int, running int, err error) {
	err = s.retry.QueryRetry(ctx, func() error {
		return s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM jobs WHERE status = ?`, JobStatusPending).Scan(&pending)
	})
	if err != nil {
		return 0, 0, fmt.Errorf("count pending jobs: %w", err)
	}
	err = s.retry.QueryRetry(ctx, func() error {
		return s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM jobs WHERE status = ?`, JobStatusRunning).Scan(&running)
	})
	if err != nil {
		return 0, 0, fmt.Errorf("count running jobs: %w", err)
	}
	return pending, running, nil
}

func (s *AuthStore) DeleteAutosaveSnapshotsOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM autosave_snapshots
		WHERE datetime(updated_at) < datetime(?)
	`, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("delete stale autosave snapshots: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete stale autosave snapshots rows: %w", err)
	}
	return rows, nil
}

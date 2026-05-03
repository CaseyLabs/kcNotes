-- TEACHING NOTES (SQLite migrations):
-- - Jobs are persisted so background work survives process restarts.
-- - `job_key` is unique for idempotent enqueue semantics.
-- - Autosave cleanup uses `updated_at` lookups, so keep a direct index.

CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    job_type TEXT NOT NULL,
    job_key TEXT NOT NULL UNIQUE,
    payload_json TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    run_at TEXT NOT NULL,
    locked_at TEXT,
    last_error TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_finished_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_jobs_status_run_at ON jobs(status, run_at);
CREATE INDEX IF NOT EXISTS idx_jobs_status_locked_at ON jobs(status, locked_at);
CREATE INDEX IF NOT EXISTS idx_autosave_snapshots_updated_at ON autosave_snapshots(updated_at);

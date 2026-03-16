-- TEACHING NOTES (SQLite migrations):
-- - Migration files evolve schema incrementally so every environment can move forward safely.
-- - `CREATE TABLE IF NOT EXISTS` makes creation idempotent across repeated runs.
-- - `PRIMARY KEY`, `UNIQUE`, and `FOREIGN KEY` enforce correctness in the database layer.
-- - `CREATE INDEX` speeds up read patterns at the cost of slightly slower writes.
-- - In this project, migrations are applied in order by filename prefix (`0001_`, `0002_`, ...).

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    csrf_token TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

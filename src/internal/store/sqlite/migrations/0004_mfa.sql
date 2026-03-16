-- TEACHING NOTES (SQLite migrations):
-- - Migration files evolve schema incrementally so every environment can move forward safely.
-- - `CREATE TABLE IF NOT EXISTS` makes creation idempotent across repeated runs.
-- - `PRIMARY KEY`, `UNIQUE`, and `FOREIGN KEY` enforce correctness in the database layer.
-- - `CREATE INDEX` speeds up read patterns at the cost of slightly slower writes.
-- - In this project, migrations are applied in order by filename prefix (`0001_`, `0002_`, ...).

ALTER TABLE users ADD COLUMN mfa_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN mfa_secret TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS mfa_recovery_codes (
    user_id TEXT NOT NULL,
    code_hash TEXT NOT NULL,
    used_at TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(user_id, code_hash),
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mfa_recovery_unused
ON mfa_recovery_codes(user_id, used_at);

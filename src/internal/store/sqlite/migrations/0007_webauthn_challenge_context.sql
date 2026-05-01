-- TEACHING NOTES (SQLite migrations):
-- - First-admin setup needs to bind a WebAuthn challenge to an account that
--   does not exist until the ceremony finishes.
-- - Rebuilding the table removes the user foreign key while preserving the
--   existing challenge context column and consumed challenge history.

CREATE TABLE IF NOT EXISTS webauthn_challenges_next (
    id TEXT PRIMARY KEY,
    user_id TEXT,
    type TEXT NOT NULL,
    session_json BLOB NOT NULL,
    expires_at TEXT NOT NULL,
    used_at TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO webauthn_challenges_next(id, user_id, type, session_json, expires_at, used_at, created_at)
SELECT id, user_id, type, session_json, expires_at, used_at, created_at
FROM webauthn_challenges;

DROP TABLE webauthn_challenges;

ALTER TABLE webauthn_challenges_next RENAME TO webauthn_challenges;

CREATE INDEX IF NOT EXISTS idx_webauthn_challenges_lookup
ON webauthn_challenges(type, used_at, expires_at);

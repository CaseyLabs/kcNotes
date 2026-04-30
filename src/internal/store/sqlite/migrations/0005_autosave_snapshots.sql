-- Autosave snapshots are private recovery copies for existing canonical posts.
-- They are intentionally stored outside posts so autosave cannot publish or
-- mutate public content until the user submits the normal edit form.

CREATE TABLE IF NOT EXISTS autosave_snapshots (
    post_id TEXT NOT NULL,
    author_id TEXT NOT NULL,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    slug TEXT NOT NULL,
    body_md TEXT NOT NULL,
    status TEXT NOT NULL,
    base_updated_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY(post_id, author_id),
    FOREIGN KEY(post_id) REFERENCES posts(id) ON DELETE CASCADE,
    FOREIGN KEY(author_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_autosave_snapshots_post ON autosave_snapshots(post_id);
CREATE INDEX IF NOT EXISTS idx_autosave_snapshots_author_updated ON autosave_snapshots(author_id, updated_at);

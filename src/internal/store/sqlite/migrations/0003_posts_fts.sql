-- TEACHING NOTES (SQLite migrations):
-- - Migration files evolve schema incrementally so every environment can move forward safely.
-- - `CREATE TABLE IF NOT EXISTS` makes creation idempotent across repeated runs.
-- - `PRIMARY KEY`, `UNIQUE`, and `FOREIGN KEY` enforce correctness in the database layer.
-- - `CREATE INDEX` speeds up read patterns at the cost of slightly slower writes.
-- - In this project, migrations are applied in order by filename prefix (`0001_`, `0002_`, ...).

CREATE VIRTUAL TABLE IF NOT EXISTS posts_fts USING fts5(
    title,
    slug,
    body_md,
    content='posts',
    content_rowid='rowid'
);

INSERT INTO posts_fts(rowid, title, slug, body_md)
SELECT rowid, title, slug, body_md
FROM posts
WHERE rowid NOT IN (SELECT rowid FROM posts_fts);

CREATE TRIGGER IF NOT EXISTS posts_fts_ai AFTER INSERT ON posts BEGIN
    INSERT INTO posts_fts(rowid, title, slug, body_md)
    VALUES (new.rowid, new.title, new.slug, new.body_md);
END;

CREATE TRIGGER IF NOT EXISTS posts_fts_ad AFTER DELETE ON posts BEGIN
    INSERT INTO posts_fts(posts_fts, rowid, title, slug, body_md)
    VALUES('delete', old.rowid, old.title, old.slug, old.body_md);
END;

CREATE TRIGGER IF NOT EXISTS posts_fts_au AFTER UPDATE ON posts BEGIN
    INSERT INTO posts_fts(posts_fts, rowid, title, slug, body_md)
    VALUES('delete', old.rowid, old.title, old.slug, old.body_md);
    INSERT INTO posts_fts(rowid, title, slug, body_md)
    VALUES (new.rowid, new.title, new.slug, new.body_md);
END;


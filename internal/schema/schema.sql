CREATE TABLE IF NOT EXISTS blocked_domains (
    domain   TEXT NOT NULL PRIMARY KEY,
    note     TEXT NOT NULL DEFAULT '',
    added_by TEXT NOT NULL DEFAULT '',
    added_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS app_meta (
    key   TEXT NOT NULL PRIMARY KEY,
    value TEXT NOT NULL
);

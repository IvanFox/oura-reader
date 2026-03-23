package store

const schema = `
CREATE TABLE IF NOT EXISTS users (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name           TEXT NOT NULL UNIQUE,
    api_key_hash   TEXT NOT NULL,
    api_key_prefix TEXT NOT NULL,
    created_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS oauth_tokens (
    user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    encrypted  BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS oura_data (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint   TEXT NOT NULL,
    day        TEXT,
    oura_id    TEXT,
    data       TEXT NOT NULL,
    fetched_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(user_id, endpoint, day, oura_id)
);

CREATE INDEX IF NOT EXISTS idx_oura_data_lookup ON oura_data(user_id, endpoint, day);

CREATE TABLE IF NOT EXISTS sync_state (
    user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint       TEXT NOT NULL,
    last_sync_date TEXT NOT NULL,
    last_sync_at   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY(user_id, endpoint)
);
`

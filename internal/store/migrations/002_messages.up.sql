CREATE TABLE IF NOT EXISTS session_messages (
    session_id TEXT PRIMARY KEY,
    messages TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

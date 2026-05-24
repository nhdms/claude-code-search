CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  project_path TEXT,
  cwd TEXT,
  started_at TEXT,
  ended_at TEXT,
  message_count INTEGER DEFAULT 0,
  title TEXT
);

CREATE TABLE IF NOT EXISTS messages (
  uuid TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  parent_uuid TEXT,
  role TEXT NOT NULL,
  kind TEXT NOT NULL,
  model TEXT,
  ts TEXT NOT NULL,
  cwd TEXT,
  text TEXT,
  tool_name TEXT,
  tool_input TEXT,
  tool_output TEXT
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id);
CREATE INDEX IF NOT EXISTS idx_messages_ts ON messages(ts);

CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
  text,
  session_id UNINDEXED,
  uuid UNINDEXED,
  role UNINDEXED,
  ts UNINDEXED,
  tokenize = 'porter unicode61'
);

CREATE TABLE IF NOT EXISTS chunks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  message_uuid TEXT NOT NULL,
  session_id TEXT NOT NULL,
  project_path TEXT,
  role TEXT,
  ts TEXT,
  text TEXT NOT NULL,
  embedded INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_chunks_msg ON chunks(message_uuid);
CREATE INDEX IF NOT EXISTS idx_chunks_embedded ON chunks(embedded);

CREATE TABLE IF NOT EXISTS sync_state (
  file_path TEXT PRIMARY KEY,
  last_offset INTEGER NOT NULL DEFAULT 0,
  last_mtime INTEGER NOT NULL DEFAULT 0,
  last_synced_at TEXT
);

CREATE TABLE IF NOT EXISTS meta (
  k TEXT PRIMARY KEY,
  v TEXT
);

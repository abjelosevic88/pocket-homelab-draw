CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);

-- Folders group drawings ("Work", "House plans").
CREATE TABLE folders (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  sort       INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX folders_user_name ON folders(user_id, name COLLATE NOCASE);

-- One drawing: a sketch (Excalidraw scene JSON), a text diagram (Mermaid
-- source) or a draw.io diagram (its XML). The preview is an SVG the
-- browser draws on save; search_text is the words in the drawing.
CREATE TABLE drawings (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  uid         TEXT NOT NULL,                     -- stable id, survives export/import
  folder_id   INTEGER REFERENCES folders(id) ON DELETE SET NULL,
  kind        TEXT NOT NULL,                     -- sketch | mermaid | drawio
  title       TEXT NOT NULL DEFAULT '',
  data        TEXT NOT NULL DEFAULT '',
  preview     TEXT NOT NULL DEFAULT '',
  preview_rev INTEGER NOT NULL DEFAULT 0,        -- the rev the preview shows
  search_text TEXT NOT NULL DEFAULT '',
  starred     INTEGER NOT NULL DEFAULT 0,
  rev         INTEGER NOT NULL DEFAULT 1,        -- goes up with every change to data
  bytes       INTEGER NOT NULL DEFAULT 0,
  source      TEXT NOT NULL DEFAULT 'app',       -- app | import | <pocket app slug>
  files_path  TEXT NOT NULL DEFAULT '',          -- the copy in Pocket Files
  files_rev   INTEGER NOT NULL DEFAULT 0,        -- the rev that copy holds
  files_error TEXT NOT NULL DEFAULT '',
  opened_at   TEXT NOT NULL DEFAULT '',
  edited_at   TEXT NOT NULL DEFAULT '',          -- the last change to data
  trashed_at  TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX drawings_user_uid ON drawings(user_id, uid);
CREATE INDEX drawings_user_list ON drawings(user_id, trashed_at, edited_at);
CREATE INDEX drawings_user_folder ON drawings(user_id, folder_id);

-- Earlier states of a drawing, kept while someone works (at most one per
-- ten minutes of editing) and before a restore.
CREATE TABLE versions (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  drawing_id INTEGER NOT NULL REFERENCES drawings(id) ON DELETE CASCADE,
  rev        INTEGER NOT NULL,
  title      TEXT NOT NULL DEFAULT '',
  data       TEXT NOT NULL DEFAULT '',
  preview    TEXT NOT NULL DEFAULT '',
  bytes      INTEGER NOT NULL DEFAULT 0,
  reason     TEXT NOT NULL DEFAULT 'edit',       -- edit | restore | import
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX versions_drawing ON versions(user_id, drawing_id, id);

-- A person's sketch library (Excalidraw library items, JSON array).
CREATE TABLE libraries (
  user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  items      TEXT NOT NULL DEFAULT '[]',
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- A drawing shared with another person on this box: they find it under
-- "Shared with me" and open it live with the owner (edit or view).
CREATE TABLE shares (
  drawing_id INTEGER NOT NULL REFERENCES drawings(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role       TEXT NOT NULL DEFAULT 'edit',          -- edit | view
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (drawing_id, user_id)
);
CREATE INDEX shares_user ON shares(user_id);

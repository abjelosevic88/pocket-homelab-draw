# Backup

One SQLite file: `DATA_DIR/pocket-draw.db` (WAL mode). Take a consistent copy with `pocket-draw backup` (writes `DATA_DIR/backups/pocket-draw-<time>.db` with `VACUUM INTO`) or Settings → Backup & data → Download database (admin). Back up that copy, not the live file. `DATA_DIR/core.token*` is the Pocket Core registration.

Restore: stop the app, replace `pocket-draw.db` (remove `-wal`/`-shm` files), start.

Every person can also export their own drawings as JSON (`/api/v1/export.json`: content, pictures, folders and the sketch library; re-importable) or a list as CSV. With Pocket Files connected, a copy of each drawing (picture + file) can also live in your files.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrStale is returned when a save is based on an older rev than the stored one.
var ErrStale = errors.New("changed elsewhere")

const drawingCols = `d.id, d.user_id, d.uid, d.folder_id, d.kind, d.title, d.preview_rev, d.starred, d.rev, d.bytes, d.source,
	d.files_path, d.files_rev, d.files_error, d.opened_at, d.edited_at, d.trashed_at, d.created_at, d.updated_at,
	COALESCE(f.name, ''), d.preview != '', (SELECT COUNT(1) FROM versions v WHERE v.drawing_id = d.id)`

func scanDrawing(row interface{ Scan(...any) error }, extra ...any) (*Drawing, error) {
	var d Drawing
	var folder sql.NullInt64
	dest := []any{&d.ID, &d.UserID, &d.UID, &folder, &d.Kind, &d.Title, &d.PreviewRev, &d.Starred, &d.Rev, &d.Bytes, &d.Source,
		&d.FilesPath, &d.FilesRev, &d.FilesError, &d.OpenedAt, &d.EditedAt, &d.TrashedAt, &d.CreatedAt, &d.UpdatedAt,
		&d.FolderName, &d.HasPreview, &d.Versions}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return nil, notFound(err)
	}
	if folder.Valid {
		d.FolderID = &folder.Int64
	}
	return &d, nil
}

// DrawingFilter narrows a list.
type DrawingFilter struct {
	View     string // all | starred | recent | trash
	Kind     string
	FolderID int64 // -1 = not in a folder
	Query    string
	Sort     string // edited | created | title | opened
	Source   string
	Since    string // edited at or after (RFC3339)
	Limit    int
	Offset   int
}

// ListDrawings returns one person's drawings without their data, and the total.
func (s *Store) ListDrawings(ctx context.Context, uid int64, f DrawingFilter) ([]Drawing, int, error) {
	where := []string{"d.user_id = ?"}
	args := []any{uid}
	if f.View == "trash" {
		where = append(where, "d.trashed_at != ''")
	} else {
		where = append(where, "d.trashed_at = ''")
	}
	if f.View == "starred" {
		where = append(where, "d.starred = 1")
	}
	if f.View == "recent" {
		where = append(where, "d.opened_at != ''")
		f.Sort = "opened"
	}
	if ValidKind(f.Kind) {
		where = append(where, "d.kind = ?")
		args = append(args, f.Kind)
	}
	if f.FolderID > 0 {
		where = append(where, "d.folder_id = ?")
		args = append(args, f.FolderID)
	} else if f.FolderID == -1 {
		where = append(where, "d.folder_id IS NULL")
	}
	if f.Source != "" {
		where = append(where, "d.source = ?")
		args = append(args, f.Source)
	}
	if f.Since != "" {
		where = append(where, "d.edited_at >= ?")
		args = append(args, f.Since)
	}
	for _, w := range strings.Fields(strings.ToLower(f.Query)) {
		like := "%" + strings.NewReplacer("%", `\%`, "_", `\_`, `\`, `\\`).Replace(w) + "%"
		where = append(where, `(lower(d.title) LIKE ? ESCAPE '\' OR lower(d.search_text) LIKE ? ESCAPE '\' OR lower(COALESCE(f.name,'')) LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like)
	}
	order := "d.edited_at DESC, d.id DESC"
	switch f.Sort {
	case "created":
		order = "d.created_at DESC, d.id DESC"
	case "title":
		order = "lower(d.title) ASC, d.id ASC"
	case "opened":
		order = "d.opened_at DESC, d.id DESC"
	}
	if f.View == "trash" {
		order = "d.trashed_at DESC, d.id DESC"
	}
	from := ` FROM drawings d LEFT JOIN folders f ON f.id = d.folder_id AND f.user_id = d.user_id WHERE ` + strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1)`+from, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+drawingCols+from+` ORDER BY `+order+` LIMIT ? OFFSET ?`, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Drawing{}
	for rows.Next() {
		d, err := scanDrawing(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *d)
	}
	return out, total, rows.Err()
}

// GetDrawing returns one drawing with its data.
func (s *Store) GetDrawing(ctx context.Context, uid, id int64) (*Drawing, error) {
	var data string
	d, err := scanDrawing(s.DB.QueryRowContext(ctx, `SELECT `+drawingCols+`, d.data FROM drawings d
		LEFT JOIN folders f ON f.id = d.folder_id AND f.user_id = d.user_id WHERE d.user_id = ? AND d.id = ?`, uid, id), &data)
	if err != nil {
		return nil, err
	}
	d.Data = data
	return d, nil
}

// DrawingPreview returns the stored SVG preview and the rev it shows.
func (s *Store) DrawingPreview(ctx context.Context, uid, id int64) (string, int, error) {
	var svg string
	var rev int
	err := s.DB.QueryRowContext(ctx, `SELECT preview, preview_rev FROM drawings WHERE user_id = ? AND id = ?`, uid, id).Scan(&svg, &rev)
	return svg, rev, notFound(err)
}

// CreateDrawing stores a new drawing (UID, Kind required).
func (s *Store) CreateDrawing(ctx context.Context, uid int64, d *Drawing) error {
	if !ValidKind(d.Kind) {
		return fmt.Errorf("unknown kind %q", d.Kind)
	}
	if d.UID == "" {
		d.UID = NewUID()
	}
	if d.Source == "" {
		d.Source = "app"
	}
	now := Now()
	if d.CreatedAt == "" {
		d.CreatedAt = now
	}
	if d.EditedAt == "" {
		d.EditedAt = d.CreatedAt
	}
	if d.Rev < 1 {
		d.Rev = 1
	}
	if err := s.checkFolder(ctx, uid, d.FolderID); err != nil {
		return err
	}
	prevRev := 0
	if d.Preview != "" {
		prevRev = d.Rev
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO drawings (user_id, uid, folder_id, kind, title, data, preview, preview_rev, search_text, starred, rev, bytes, source, edited_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, d.UID, nullI(d.FolderID), d.Kind, d.Title, d.Data, d.Preview, prevRev, SearchText(d.Kind, d.Data), boolInt(d.Starred), d.Rev, len(d.Data), d.Source, d.EditedAt, d.CreatedAt, now)
	if err != nil {
		return conflict(err)
	}
	d.ID, _ = res.LastInsertId()
	got, err := s.GetDrawing(ctx, uid, d.ID)
	if err != nil {
		return err
	}
	*d = *got
	return nil
}

func (s *Store) checkFolder(ctx context.Context, uid int64, id *int64) error {
	if id == nil || *id == 0 {
		return nil
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM folders WHERE user_id = ? AND id = ?`, uid, *id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DrawingPatch is a change to a drawing; nil fields stay as they are.
type DrawingPatch struct {
	Title    *string
	FolderID **int64
	Starred  *bool
	Data     *string
	Preview  *string // with Data, or alone for the rev in PreviewRev
	BaseRev  int     // with Data: the rev the change was made on (0 = don't check)
	PrevRev  int     // with Preview alone: the rev it shows
}

// SnapshotEvery is the least time between two kept versions while editing.
const SnapshotEvery = 10 * time.Minute

// UpdateDrawing applies a patch. A data change on an older rev than the
// stored one fails with ErrStale; otherwise the rev goes up and, at most
// every ten minutes, the state before the change is kept as a version. The
// second value reports whether a version was kept.
func (s *Store) UpdateDrawing(ctx context.Context, uid, id int64, p DrawingPatch) (*Drawing, bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	var kind, title, data, preview, edited string
	var rev int
	if err := tx.QueryRowContext(ctx, `SELECT kind, title, data, preview, rev, edited_at FROM drawings WHERE user_id = ? AND id = ?`, uid, id).
		Scan(&kind, &title, &data, &preview, &rev, &edited); err != nil {
		return nil, false, notFound(err)
	}
	now := Now()
	sets := []string{"updated_at = ?"}
	args := []any{now}
	if p.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, strings.TrimSpace(*p.Title))
	}
	if p.FolderID != nil {
		if err := s.checkFolderTx(ctx, tx, uid, *p.FolderID); err != nil {
			return nil, false, err
		}
		sets = append(sets, "folder_id = ?")
		args = append(args, nullI(*p.FolderID))
	}
	if p.Starred != nil {
		sets = append(sets, "starred = ?")
		args = append(args, boolInt(*p.Starred))
	}
	kept := false
	if p.Data != nil && *p.Data != data {
		if p.BaseRev > 0 && p.BaseRev != rev {
			return nil, false, ErrStale
		}
		if keep, err := s.dueSnapshot(ctx, tx, id); err != nil {
			return nil, false, err
		} else if keep && data != "" {
			if _, err := tx.ExecContext(ctx, `INSERT INTO versions (user_id, drawing_id, rev, title, data, preview, bytes, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, 'edit', ?)`,
				uid, id, rev, title, data, preview, len(data), firstNonEmpty(edited, now)); err != nil {
				return nil, false, err
			}
			kept = true
		}
		rev++
		sets = append(sets, "data = ?", "bytes = ?", "search_text = ?", "rev = ?", "edited_at = ?")
		args = append(args, *p.Data, len(*p.Data), SearchText(kind, *p.Data), rev, now)
		if p.Preview != nil {
			sets = append(sets, "preview = ?", "preview_rev = ?")
			args = append(args, *p.Preview, rev)
		}
	} else if p.Preview != nil {
		pr := p.PrevRev
		if pr <= 0 || pr > rev {
			pr = rev
		}
		sets = append(sets, "preview = ?", "preview_rev = ?")
		args = append(args, *p.Preview, pr)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE drawings SET `+strings.Join(sets, ", ")+` WHERE user_id = ? AND id = ?`, append(args, uid, id)...); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	d, err := s.GetDrawing(ctx, uid, id)
	return d, kept, err
}

func (s *Store) checkFolderTx(ctx context.Context, tx *sql.Tx, uid int64, id *int64) error {
	if id == nil || *id == 0 {
		return nil
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM folders WHERE user_id = ? AND id = ?`, uid, *id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// dueSnapshot reports whether the last kept version is old enough for another.
func (s *Store) dueSnapshot(ctx context.Context, tx *sql.Tx, id int64) (bool, error) {
	var last sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT MAX(created_at) FROM versions WHERE drawing_id = ?`, id).Scan(&last); err != nil {
		return false, err
	}
	if !last.Valid || last.String == "" {
		return true, nil
	}
	t, err := time.Parse(time.RFC3339Nano, last.String)
	if err != nil {
		return true, nil
	}
	return time.Since(t) >= SnapshotEvery, nil
}

// MarkOpened notes that the person opened a drawing (for "Recent").
func (s *Store) MarkOpened(ctx context.Context, uid, id int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE drawings SET opened_at = ? WHERE user_id = ? AND id = ?`, Now(), uid, id)
}

// TrashDrawing moves a drawing to the trash (or back with restore).
func (s *Store) TrashDrawing(ctx context.Context, uid, id int64, restore bool) error {
	at := Now()
	if restore {
		at = ""
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE drawings SET trashed_at = ?, updated_at = ? WHERE user_id = ? AND id = ?`, at, Now(), uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteDrawing removes a drawing and its versions for good.
func (s *Store) DeleteDrawing(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM drawings WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// EmptyTrash removes trashed drawings, all of them or those trashed before.
func (s *Store) EmptyTrash(ctx context.Context, uid int64, before string) (int, error) {
	q, args := `DELETE FROM drawings WHERE user_id = ? AND trashed_at != ''`, []any{uid}
	if before != "" {
		q += ` AND trashed_at < ?`
		args = append(args, before)
	}
	res, err := s.DB.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ---- versions ----

// ListVersions returns a drawing's kept versions, newest first, without data.
func (s *Store) ListVersions(ctx context.Context, uid, drawingID int64) ([]Version, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, drawing_id, rev, title, bytes, reason, created_at, preview != '' FROM versions
		WHERE user_id = ? AND drawing_id = ? ORDER BY id DESC`, uid, drawingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Version{}
	for rows.Next() {
		var v Version
		var hasPrev bool
		if err := rows.Scan(&v.ID, &v.DrawingID, &v.Rev, &v.Title, &v.Bytes, &v.Reason, &v.CreatedAt, &hasPrev); err != nil {
			return nil, err
		}
		v.UserID = uid
		if hasPrev {
			v.Preview = "1"
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetVersion returns one version with its data and preview.
func (s *Store) GetVersion(ctx context.Context, uid, drawingID, id int64) (*Version, error) {
	var v Version
	err := s.DB.QueryRowContext(ctx, `SELECT id, drawing_id, rev, title, data, preview, bytes, reason, created_at FROM versions
		WHERE user_id = ? AND drawing_id = ? AND id = ?`, uid, drawingID, id).
		Scan(&v.ID, &v.DrawingID, &v.Rev, &v.Title, &v.Data, &v.Preview, &v.Bytes, &v.Reason, &v.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	v.UserID = uid
	return &v, nil
}

// RestoreVersion keeps the current state as a version, then puts the old
// one back as a new rev.
func (s *Store) RestoreVersion(ctx context.Context, uid, drawingID, versionID int64) (*Drawing, error) {
	v, err := s.GetVersion(ctx, uid, drawingID, versionID)
	if err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var kind, title, data, preview string
	var rev int
	if err := tx.QueryRowContext(ctx, `SELECT kind, title, data, preview, rev FROM drawings WHERE user_id = ? AND id = ?`, uid, drawingID).
		Scan(&kind, &title, &data, &preview, &rev); err != nil {
		return nil, notFound(err)
	}
	now := Now()
	if _, err := tx.ExecContext(ctx, `INSERT INTO versions (user_id, drawing_id, rev, title, data, preview, bytes, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, 'restore', ?)`,
		uid, drawingID, rev, title, data, preview, len(data), now); err != nil {
		return nil, err
	}
	rev++
	prevRev := 0
	if v.Preview != "" {
		prevRev = rev
	}
	if _, err := tx.ExecContext(ctx, `UPDATE drawings SET data = ?, preview = ?, preview_rev = ?, bytes = ?, search_text = ?, rev = ?, edited_at = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		v.Data, v.Preview, prevRev, len(v.Data), SearchText(kind, v.Data), rev, now, now, uid, drawingID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetDrawing(ctx, uid, drawingID)
}

// PruneVersions keeps the newest keep versions of each of a person's drawings.
func (s *Store) PruneVersions(ctx context.Context, uid int64, keep int) (int, error) {
	if keep <= 0 {
		return 0, nil
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM versions WHERE user_id = ? AND id IN (
		SELECT id FROM (SELECT id, ROW_NUMBER() OVER (PARTITION BY drawing_id ORDER BY id DESC) AS n FROM versions WHERE user_id = ?) WHERE n > ?)`, uid, uid, keep)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ---- folders ----

// ListFolders returns one person's folders with how many drawings each holds.
func (s *Store) ListFolders(ctx context.Context, uid int64) ([]Folder, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT f.id, f.name, f.sort, f.created_at,
		(SELECT COUNT(1) FROM drawings d WHERE d.folder_id = f.id AND d.trashed_at = '')
		FROM folders f WHERE f.user_id = ? ORDER BY f.sort, lower(f.name)`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Folder{}
	for rows.Next() {
		f := Folder{UserID: uid}
		if err := rows.Scan(&f.ID, &f.Name, &f.Sort, &f.CreatedAt, &f.Count); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FolderByName finds a person's folder by name (case-insensitive).
func (s *Store) FolderByName(ctx context.Context, uid int64, name string) (*Folder, error) {
	f := Folder{UserID: uid}
	err := s.DB.QueryRowContext(ctx, `SELECT id, name, sort, created_at FROM folders WHERE user_id = ? AND name = ? COLLATE NOCASE`, uid, strings.TrimSpace(name)).
		Scan(&f.ID, &f.Name, &f.Sort, &f.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	return &f, nil
}

// CreateFolder adds a folder.
func (s *Store) CreateFolder(ctx context.Context, uid int64, name string) (*Folder, error) {
	name = strings.TrimSpace(name)
	var sort int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort), 0) + 1 FROM folders WHERE user_id = ?`, uid).Scan(&sort)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO folders (user_id, name, sort) VALUES (?, ?, ?)`, uid, name, sort)
	if err != nil {
		return nil, conflict(err)
	}
	id, _ := res.LastInsertId()
	return &Folder{ID: id, UserID: uid, Name: name, Sort: sort, CreatedAt: Now()}, nil
}

// EnsureFolder returns the folder with this name, creating it when missing.
func (s *Store) EnsureFolder(ctx context.Context, uid int64, name string) (*Folder, error) {
	if f, err := s.FolderByName(ctx, uid, name); err == nil {
		return f, nil
	}
	return s.CreateFolder(ctx, uid, name)
}

// UpdateFolder renames or reorders a folder.
func (s *Store) UpdateFolder(ctx context.Context, uid, id int64, name *string, sort *int) error {
	sets, args := []string{}, []any{}
	if name != nil {
		sets = append(sets, "name = ?")
		args = append(args, strings.TrimSpace(*name))
	}
	if sort != nil {
		sets = append(sets, "sort = ?")
		args = append(args, *sort)
	}
	if len(sets) == 0 {
		return nil
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE folders SET `+strings.Join(sets, ", ")+` WHERE user_id = ? AND id = ?`, append(args, uid, id)...)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteFolder removes a folder; its drawings stay, outside any folder.
func (s *Store) DeleteFolder(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM folders WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- the sketch library ----

// Library returns a person's sketch library (a JSON array).
func (s *Store) Library(ctx context.Context, uid int64) (string, string, error) {
	var items, at string
	err := s.DB.QueryRowContext(ctx, `SELECT items, updated_at FROM libraries WHERE user_id = ?`, uid).Scan(&items, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return "[]", "", nil
	}
	return items, at, err
}

// SaveLibrary replaces a person's sketch library.
func (s *Store) SaveLibrary(ctx context.Context, uid int64, items string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO libraries (user_id, items, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET items = excluded.items, updated_at = excluded.updated_at`, uid, items, Now())
	return err
}

// ---- Pocket Files copies ----

// FilesDue lists a person's drawings whose copy in Pocket Files is behind,
// edited before quietBefore (so a drawing being worked on waits).
func (s *Store) FilesDue(ctx context.Context, uid int64, quietBefore string, limit int) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM drawings WHERE user_id = ? AND trashed_at = '' AND files_rev < rev AND edited_at < ?
		AND preview_rev = rev ORDER BY edited_at LIMIT ?`, uid, quietBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetFilesCopy records where a drawing's copy went (or why it did not).
func (s *Store) SetFilesCopy(ctx context.Context, uid, id int64, path string, rev int, errText string) error {
	if errText != "" {
		_, err := s.DB.ExecContext(ctx, `UPDATE drawings SET files_error = ? WHERE user_id = ? AND id = ?`, errText, uid, id)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE drawings SET files_path = ?, files_rev = ?, files_error = '' WHERE user_id = ? AND id = ?`, path, rev, uid, id)
	return err
}

// ---- overview ----

// Overview is the sidebar's numbers.
type Overview struct {
	All      int            `json:"all"`
	Starred  int            `json:"starred"`
	Recent   int            `json:"recent"`
	Trash    int            `json:"trash"`
	Kinds    map[string]int `json:"kinds"`
	NoFolder int            `json:"no_folder"`
	Bytes    int64          `json:"bytes"`
}

// Overview counts one person's drawings.
func (s *Store) Overview(ctx context.Context, uid int64) (Overview, error) {
	o := Overview{}
	var sk, mm, dio int
	err := s.DB.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(trashed_at = ''), 0), COALESCE(SUM(trashed_at = '' AND starred = 1), 0), COALESCE(SUM(trashed_at = '' AND opened_at != ''), 0),
		COALESCE(SUM(trashed_at != ''), 0), COALESCE(SUM(trashed_at = '' AND folder_id IS NULL), 0), COALESCE(SUM(bytes), 0),
		COALESCE(SUM(trashed_at = '' AND kind = 'sketch'), 0), COALESCE(SUM(trashed_at = '' AND kind = 'mermaid'), 0), COALESCE(SUM(trashed_at = '' AND kind = 'drawio'), 0)
		FROM drawings WHERE user_id = ?`, uid).Scan(&o.All, &o.Starred, &o.Recent, &o.Trash, &o.NoFolder, &o.Bytes,
		&sk, &mm, &dio)
	o.Kinds = map[string]int{KindSketch: sk, KindMermaid: mm, KindDrawio: dio}
	return o, err
}

// DrawingsOn lists the drawings a person made, and the ones they changed,
// on a local day (a change is the last edit or a version kept that day).
func (s *Store) DrawingsOn(ctx context.Context, uid int64, day string) (made, changed []Drawing, err error) {
	t, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		return nil, nil, err
	}
	from, to := t.UTC().Format(time.RFC3339Nano), t.AddDate(0, 0, 1).UTC().Format(time.RFC3339Nano)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+drawingCols+` FROM drawings d LEFT JOIN folders f ON f.id = d.folder_id AND f.user_id = d.user_id
		WHERE d.user_id = ? AND d.trashed_at = '' AND ((d.created_at >= ? AND d.created_at < ?) OR (d.edited_at >= ? AND d.edited_at < ?)
		OR EXISTS (SELECT 1 FROM versions v WHERE v.drawing_id = d.id AND v.created_at >= ? AND v.created_at < ?))
		ORDER BY d.created_at`, uid, from, to, from, to, from, to)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	made, changed = []Drawing{}, []Drawing{}
	for rows.Next() {
		d, err := scanDrawing(rows)
		if err != nil {
			return nil, nil, err
		}
		if d.CreatedAt >= from && d.CreatedAt < to {
			made = append(made, *d)
		} else {
			changed = append(changed, *d)
		}
	}
	return made, changed, rows.Err()
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// AddVersion keeps a version as given (imports and the demo).
func (s *Store) AddVersion(ctx context.Context, uid int64, v Version) error {
	if v.CreatedAt == "" {
		v.CreatedAt = Now()
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO versions (user_id, drawing_id, rev, title, data, preview, bytes, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, v.DrawingID, v.Rev, v.Title, v.Data, v.Preview, len(v.Data), firstNonEmpty(v.Reason, "edit"), v.CreatedAt)
	return err
}

// SetDates backdates a drawing (the demo).
func (s *Store) SetDates(ctx context.Context, uid, id int64, created, edited, opened string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE drawings SET created_at = ?, edited_at = ?, updated_at = ?, opened_at = ? WHERE user_id = ? AND id = ?`, created, edited, edited, opened, uid, id)
	return err
}

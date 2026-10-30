package store

import (
	"context"
	"strings"
)

// Access tells how a person may use a drawing: "owner", or the role it is
// shared with them in, and whose it is. A drawing in the trash is its
// owner's alone. ErrNotFound when it is neither theirs nor shared with them.
func (s *Store) Access(ctx context.Context, uid, id int64) (owner int64, role string, err error) {
	var trashed string
	var shared *string
	err = s.DB.QueryRowContext(ctx, `SELECT d.user_id, d.trashed_at, (SELECT role FROM shares WHERE drawing_id = d.id AND user_id = ?)
		FROM drawings d WHERE d.id = ?`, uid, id).Scan(&owner, &trashed, &shared)
	if err != nil {
		return 0, "", notFound(err)
	}
	if owner == uid {
		return owner, "owner", nil
	}
	if shared == nil || trashed != "" {
		return 0, "", ErrNotFound
	}
	return owner, *shared, nil
}

// ListShares returns who a drawing is shared with.
func (s *Store) ListShares(ctx context.Context, drawingID int64) ([]Share, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT u.id, u.name, u.email, sh.role, sh.created_at FROM shares sh JOIN users u ON u.id = sh.user_id
		WHERE sh.drawing_id = ? ORDER BY lower(COALESCE(NULLIF(u.name, ''), u.email))`, drawingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Share{}
	for rows.Next() {
		var sh Share
		if err := rows.Scan(&sh.UserID, &sh.Name, &sh.Email, &sh.Role, &sh.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// SetShare shares a drawing with a person (or changes their role).
func (s *Store) SetShare(ctx context.Context, drawingID, uid int64, role string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO shares (drawing_id, user_id, role) VALUES (?, ?, ?)
		ON CONFLICT(drawing_id, user_id) DO UPDATE SET role = excluded.role`, drawingID, uid, role)
	return err
}

// RemoveShare stops sharing a drawing with a person.
func (s *Store) RemoveShare(ctx context.Context, drawingID, uid int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM shares WHERE drawing_id = ? AND user_id = ?`, drawingID, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountShares returns with how many people a drawing is shared.
func (s *Store) CountShares(ctx context.Context, drawingID int64) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM shares WHERE drawing_id = ?`, drawingID).Scan(&n)
	return n
}

// CountSharedWith returns how many drawings others share with a person.
func (s *Store) CountSharedWith(ctx context.Context, uid int64) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM shares sh JOIN drawings d ON d.id = sh.drawing_id WHERE sh.user_id = ? AND d.trashed_at = ''`, uid).Scan(&n)
	return n
}

// People lists everyone on this box but one person, for sharing with.
func (s *Store) People(ctx context.Context, except int64) ([]Person, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, email FROM users WHERE id != ? ORDER BY lower(COALESCE(NULLIF(name, ''), email))`, except)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Person{}
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.Name, &p.Email); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DisplayName is how a person is shown to others.
func DisplayName(name, email string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return email
}

package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-draw/internal/store"
)

// ---- copies in Pocket Files ----
//
// A drawing's copy is two files in the person's chosen folder (and the
// drawing's own folder under it): the picture (.svg) and the file other
// tools open (.excalidraw, .mmd, .drawio). Draw keeps the drawing; the copy
// is for finding it among the rest of your files, sharing it, and backups.

// filesPaths are where a drawing's copy goes: the folder and the base name.
func filesPaths(st store.Settings, d *store.Drawing) (dir, base string) {
	dir = strings.Trim(strings.TrimSpace(st.FilesFolder), "/")
	if dir == "" {
		dir = "Drawings"
	}
	if d.FolderName != "" {
		dir += "/" + safeName(d.FolderName, "Folder")
	}
	return dir, safeName(d.Title, fmt.Sprintf("Drawing %d", d.ID))
}

// copyToFiles puts a drawing's picture and file into Pocket Files and
// records where. A copy left under an old name (renamed, moved) goes to the
// trash in Files.
func (s *Server) copyToFiles(ctx context.Context, cl *clients, id int64) (string, error) {
	s.filesMu.Lock()
	defer s.filesMu.Unlock()
	uid := cl.u.ID
	d, err := s.store.GetDrawing(ctx, uid, id)
	if err != nil {
		return "", err
	}
	sib, err := cl.conn(ctx, appFiles, scopeFilesWrite)
	if err != nil {
		return "", err
	}
	dir, base := filesPaths(cl.st, d)
	name, mime, body := fileFor(d)
	ext := name[strings.LastIndex(name, "."):]
	mtime, _ := time.Parse(time.RFC3339Nano, d.EditedAt)
	target := dir + "/" + base
	if _, err := sib.client.Upload(ctx, target+ext, body, mime, mtime); err != nil {
		_ = s.store.SetFilesCopy(ctx, uid, id, "", 0, err.Error())
		return "", err
	}
	if svg, rev, _ := s.store.DrawingPreview(ctx, uid, id); svg != "" && rev == d.Rev {
		if _, err := sib.client.Upload(ctx, target+".svg", []byte(svg), "image/svg+xml", mtime); err != nil {
			_ = s.store.SetFilesCopy(ctx, uid, id, "", 0, err.Error())
			return "", err
		}
	}
	if old := d.FilesPath; old != "" && old != target {
		_ = sib.client.Delete(ctx, old+ext, old+".svg")
	}
	if err := s.store.SetFilesCopy(ctx, uid, id, target, d.Rev, ""); err != nil {
		return "", err
	}
	return target, nil
}

// filesOpenURL is where the person sees a copy in Pocket Files.
func (s *Server) filesOpenURL(ctx context.Context, u *store.User, st store.Settings, target string) string {
	public := st.FilesPublicURL
	if public == "" {
		public = s.resolvePublic(ctx, appFiles.slug)
	}
	if public == "" || target == "" {
		return ""
	}
	dir := target
	if i := strings.LastIndex(dir, "/"); i >= 0 {
		dir = dir[:i]
	}
	parts := strings.Split(dir, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.TrimRight(public, "/") + "/f/home/" + strings.Join(parts, "/")
}

// handleCopyToFiles copies one drawing now.
func (s *Server) handleCopyToFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	cl := s.clients(ctx, uid)
	target, err := s.copyToFiles(ctx, cl, idParam(r, "id"))
	if err != nil {
		if isNotConnected(err) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if err == store.ErrNotFound {
			s.fail(w, err, "drawing")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "Pocket Files: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": target, "open_url": s.filesOpenURL(ctx, cl.u, cl.st, target)})
}

// syncFiles brings everybody's copies up to date (those who asked for it),
// waiting until a drawing has been left alone for two minutes.
func (s *Server) syncFiles(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	quiet := time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339Nano)
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil || !st.FilesAuto {
			continue
		}
		due, err := s.store.FilesDue(ctx, uid, quiet, 50)
		if err != nil || len(due) == 0 {
			continue
		}
		cl := s.clients(ctx, uid)
		for _, id := range due {
			if _, err := s.copyToFiles(ctx, cl, id); err != nil {
				s.log.Warn("copy to Pocket Files failed", "user", uid, "drawing", id, "err", err)
				s.notifyFilesError(ctx, uid, st, err.Error())
				break
			}
		}
	}
}

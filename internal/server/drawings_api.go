package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-draw/internal/store"
)

// maxPreview caps the SVG picture kept with a drawing (pictures pasted into
// a sketch travel inside it).
const maxPreview = 12 << 20

// blankScene is an empty Excalidraw scene, in its file format.
const blankScene = `{"type":"excalidraw","version":2,"source":"pocket-draw","elements":[],"appState":{"gridSize":20,"viewBackgroundColor":"#ffffff"},"files":{}}`

// starterMermaid is what a new text diagram starts with.
const starterMermaid = "flowchart LR\n    A[Idea] --> B{Worth doing?}\n    B -- Yes --> C[Plan it]\n    B -- Not now --> D[Park it]\n    C --> E[Do it]\n"

// kindNames are the kinds as people read them.
var kindNames = map[string]string{store.KindSketch: "sketch", store.KindMermaid: "text diagram", store.KindDrawio: "diagram"}

func untitled(kind string) string {
	switch kind {
	case store.KindMermaid:
		return "Untitled text diagram"
	case store.KindDrawio:
		return "Untitled diagram"
	}
	return "Untitled sketch"
}

// validData checks a drawing's content for its kind ("" is a blank drawing).
func validData(kind, data string) error {
	if data == "" {
		return nil
	}
	switch kind {
	case store.KindSketch:
		var scene struct {
			Elements []json.RawMessage `json:"elements"`
		}
		if err := json.Unmarshal([]byte(data), &scene); err != nil || scene.Elements == nil {
			return errors.New("a sketch is an Excalidraw scene (JSON with elements)")
		}
	case store.KindDrawio:
		if t := strings.TrimSpace(data); !strings.HasPrefix(t, "<") {
			return errors.New("a draw.io diagram is XML (<mxfile> or <mxGraphModel>)")
		}
	}
	return nil
}

// cleanPreview keeps a preview only if it is an SVG of a sensible size.
func cleanPreview(p string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", true
	}
	if len(p) > maxPreview {
		return "", false
	}
	head := strings.ToLower(p[:min(len(p), 400)])
	if !strings.HasPrefix(head, "<svg") && !strings.HasPrefix(head, "<?xml") {
		return "", false
	}
	if !strings.Contains(head, "<svg") && !strings.Contains(strings.ToLower(p[:min(len(p), 4000)]), "<svg") {
		return "", false
	}
	return p, true
}

func drawingURL(base string, id int64) string {
	if base == "" {
		return ""
	}
	return base + "/d/" + strconv.FormatInt(id, 10)
}

// ---- overview ----

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	o, err := s.store.Overview(ctx, uid)
	if err != nil {
		s.fail(w, err, "overview")
		return
	}
	folders, err := s.store.ListFolders(ctx, uid)
	if err != nil {
		s.fail(w, err, "folders")
		return
	}
	recent, _, err := s.store.ListDrawings(ctx, uid, store.DrawingFilter{View: "recent", Limit: 6})
	if err != nil {
		s.fail(w, err, "recent")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"counts": o, "folders": folders, "recent": recent})
}

// handleDay is one day for Pocket Day: what the person drew and changed.
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	day := chiParam(r, "date")
	made, changed, err := s.store.DrawingsOn(ctx, actorID(ctx), day)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "date is YYYY-MM-DD")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": day, "made": made, "changed": changed, "url": s.cfg.BaseURL})
}

// ---- drawings ----

func (s *Server) handleListDrawings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	f := store.DrawingFilter{View: q.Get("view"), Kind: q.Get("kind"), Query: q.Get("q"), Sort: q.Get("sort"), Source: q.Get("source"),
		Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset")}
	if v := q.Get("folder"); v == "none" {
		f.FolderID = -1
	} else {
		f.FolderID, _ = strconv.ParseInt(v, 10, 64)
	}
	if f.Sort == "" {
		st, _ := s.store.GetSettings(ctx, actorID(ctx))
		f.Sort = st.SortBy
	}
	list, total, err := s.store.ListDrawings(ctx, actorID(ctx), f)
	if err != nil {
		s.fail(w, err, "drawings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"drawings": list, "total": total})
}

type drawingIn struct {
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Data       string `json:"data"`
	Preview    string `json:"preview"`
	FolderID   *int64 `json:"folder_id"`
	Folder     string `json:"folder"` // by name, created when missing
	Starred    bool   `json:"starred"`
	Rev        int    `json:"rev"`         // the rev a change was made on
	PreviewRev int    `json:"preview_rev"` // the rev a preview alone shows
}

func (s *Server) decodeDrawing(r *http.Request, v any) (map[string]bool, error) {
	b, err := s.readBody(r)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys, json.Unmarshal(b, v)
}

func (s *Server) handleCreateDrawing(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in drawingIn
	if _, err := s.decodeDrawing(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json (or too large)")
		return
	}
	d, err := s.newDrawing(ctx, uid, in, firstNonEmpty(linkApp(ctx), "app"))
	if err != nil {
		var bad badInput
		if errors.As(err, &bad) {
			writeErr(w, http.StatusBadRequest, bad.Error())
			return
		}
		s.fail(w, err, "create")
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

type badInput string

func (b badInput) Error() string { return string(b) }

// newDrawing validates and stores a drawing, then tells the bus (and the
// person, when another app added it).
func (s *Server) newDrawing(ctx context.Context, uid int64, in drawingIn, source string) (*store.Drawing, error) {
	if in.Kind == "" {
		in.Kind = store.KindSketch
	}
	if !store.ValidKind(in.Kind) {
		return nil, badInput("kind is sketch, mermaid or drawio")
	}
	if err := validData(in.Kind, in.Data); err != nil {
		return nil, badInput(err.Error())
	}
	if in.Data == "" && in.Kind == store.KindSketch {
		in.Data = blankScene
	}
	if strings.TrimSpace(in.Data) == "" && in.Kind == store.KindMermaid {
		in.Data = starterMermaid // something to see (and a picture) from the start
	}
	prev, ok := cleanPreview(in.Preview)
	if !ok {
		prev = ""
	}
	d := &store.Drawing{Kind: in.Kind, Title: strings.TrimSpace(in.Title), Data: in.Data, Preview: prev, FolderID: in.FolderID, Starred: in.Starred, Source: source}
	if d.Title == "" {
		d.Title = untitled(in.Kind)
	}
	if len([]rune(d.Title)) > 200 {
		d.Title = string([]rune(d.Title)[:200])
	}
	if name := strings.TrimSpace(in.Folder); name != "" && d.FolderID == nil {
		if len(name) > 80 {
			return nil, badInput("that folder name is too long")
		}
		f, err := s.store.EnsureFolder(ctx, uid, name)
		if err != nil {
			return nil, err
		}
		d.FolderID = &f.ID
	}
	if err := s.store.CreateDrawing(ctx, uid, d); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, badInput("that folder does not exist")
		}
		return nil, err
	}
	s.publish(ctx, uid, "draw.created", fmt.Sprintf("drawing:%d", d.ID), map[string]any{"id": d.ID, "title": d.Title, "kind": d.Kind, "source": d.Source, "url": drawingURL(s.cfg.BaseURL, d.ID)})
	if source != "app" && source != "import" {
		go s.notifyLinkAdd(uid, d, source)
	}
	return d, nil
}

func (s *Server) handleGetDrawing(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	d, err := s.store.GetDrawing(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "drawing")
		return
	}
	if queryBool(r, "open") && linkApp(ctx) == "" {
		s.store.MarkOpened(ctx, uid, d.ID)
	}
	writeJSON(w, http.StatusOK, d)
}

// handleUpdateDrawing changes a drawing. A change to its content must say
// which rev it was made on; when the drawing moved on meanwhile (another tab,
// another device) the answer is 409 with the current rev and nothing is lost.
func (s *Server) handleUpdateDrawing(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := idParam(r, "id")
	cur, err := s.store.GetDrawing(ctx, uid, id)
	if err != nil {
		s.fail(w, err, "drawing")
		return
	}
	if app := linkApp(ctx); app != "" && cur.Source != app {
		writeErr(w, http.StatusForbidden, "this app may change only the drawings it added")
		return
	}
	var in drawingIn
	keys, err := s.decodeDrawing(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json (or too large)")
		return
	}
	var p store.DrawingPatch
	if keys["title"] {
		t := strings.TrimSpace(in.Title)
		if t == "" {
			t = untitled(cur.Kind)
		}
		if len([]rune(t)) > 200 {
			t = string([]rune(t)[:200])
		}
		p.Title = &t
	}
	if keys["folder_id"] {
		p.FolderID = &in.FolderID
	}
	if keys["starred"] {
		p.Starred = &in.Starred
	}
	if keys["data"] {
		if err := validData(cur.Kind, in.Data); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		p.Data = &in.Data
		p.BaseRev = in.Rev
	}
	if keys["preview"] {
		prev, ok := cleanPreview(in.Preview)
		if ok {
			p.Preview = &prev
			p.PrevRev = in.PreviewRev
		}
	}
	d, kept, err := s.store.UpdateDrawing(ctx, uid, id, p)
	if errors.Is(err, store.ErrStale) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "this drawing was changed somewhere else", "rev": cur.Rev})
		return
	}
	if errors.Is(err, store.ErrNotFound) && keys["folder_id"] {
		writeErr(w, http.StatusBadRequest, "that folder does not exist")
		return
	}
	if err != nil {
		s.fail(w, err, "save")
		return
	}
	if kept {
		s.publish(ctx, uid, "draw.edited", fmt.Sprintf("drawing:%d", d.ID), map[string]any{"id": d.ID, "title": d.Title, "kind": d.Kind, "rev": d.Rev, "url": drawingURL(s.cfg.BaseURL, d.ID)})
	}
	d.Data = ""
	writeJSON(w, http.StatusOK, d)
}

// handleDeleteDrawing moves a drawing to the trash; ?forever=1 removes a
// trashed one for good.
func (s *Server) handleDeleteDrawing(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	d, err := s.store.GetDrawing(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "drawing")
		return
	}
	if app := linkApp(ctx); app != "" && d.Source != app {
		writeErr(w, http.StatusForbidden, "this app may remove only the drawings it added")
		return
	}
	if queryBool(r, "forever") {
		if d.TrashedAt == "" {
			writeErr(w, http.StatusBadRequest, "move it to the trash first")
			return
		}
		err = s.store.DeleteDrawing(ctx, uid, d.ID)
	} else {
		err = s.store.TrashDrawing(ctx, uid, d.ID, false)
	}
	if err != nil {
		s.fail(w, err, "delete")
		return
	}
	s.publish(ctx, uid, "draw.deleted", fmt.Sprintf("drawing:%d", d.ID), map[string]any{"id": d.ID, "title": d.Title, "kind": d.Kind, "forever": queryBool(r, "forever")})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRestoreDrawing(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.TrashDrawing(ctx, actorID(ctx), idParam(r, "id"), true); err != nil {
		s.fail(w, err, "restore")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEmptyTrash(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, err := s.store.EmptyTrash(ctx, actorID(ctx), "")
	if err != nil {
		s.fail(w, err, "empty trash")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}

func (s *Server) handleDuplicate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	d, err := s.store.GetDrawing(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "drawing")
		return
	}
	prev, _, _ := s.store.DrawingPreview(ctx, uid, d.ID)
	cp := &store.Drawing{Kind: d.Kind, Title: d.Title + " (copy)", Data: d.Data, Preview: prev, FolderID: d.FolderID, Source: "app"}
	if err := s.store.CreateDrawing(ctx, uid, cp); err != nil {
		s.fail(w, err, "duplicate")
		return
	}
	s.publish(ctx, uid, "draw.created", fmt.Sprintf("drawing:%d", cp.ID), map[string]any{"id": cp.ID, "title": cp.Title, "kind": cp.Kind, "source": "app", "url": drawingURL(s.cfg.BaseURL, cp.ID)})
	cp.Data = ""
	writeJSON(w, http.StatusCreated, cp)
}

// serveSVG sends a stored picture so that, opened on its own, nothing in it
// can run (it is shown with <img> in the app).
func serveSVG(w http.ResponseWriter, r *http.Request, svg, etag string) {
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, no-cache")
	h.Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", "image/svg+xml")
	_, _ = w.Write([]byte(svg))
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := idParam(r, "id")
	svg, rev, err := s.store.DrawingPreview(ctx, uid, id)
	if err != nil {
		s.fail(w, err, "preview")
		return
	}
	if svg == "" {
		writeErr(w, http.StatusNotFound, "no picture yet: open the drawing once")
		return
	}
	serveSVG(w, r, svg, fmt.Sprintf(`"%d-%d"`, id, rev))
}

// fileFor is a drawing as a file other tools open: .excalidraw, .mmd, .drawio.
func fileFor(d *store.Drawing) (name, mime string, body []byte) {
	base := safeName(d.Title, "drawing")
	switch d.Kind {
	case store.KindMermaid:
		return base + ".mmd", "text/plain; charset=utf-8", []byte(d.Data)
	case store.KindDrawio:
		data := d.Data
		if strings.TrimSpace(data) == "" {
			data = `<mxfile><diagram name="Page-1"><mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/></root></mxGraphModel></diagram></mxfile>`
		}
		return base + ".drawio", "application/xml", []byte(data)
	}
	return base + ".excalidraw", "application/json", []byte(d.Data)
}

// safeName makes a title usable as a file name everywhere.
func safeName(t, fallback string) string {
	t = strings.Map(func(r rune) rune {
		switch {
		case r < 32, strings.ContainsRune(`/\:*?"<>|`, r):
			return '-'
		}
		return r
	}, strings.TrimSpace(t))
	t = strings.Trim(strings.Join(strings.Fields(t), " "), ". -")
	if r := []rune(t); len(r) > 100 {
		t = strings.TrimSpace(string(r[:100]))
	}
	if t == "" {
		return fallback
	}
	return t
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, err := s.store.GetDrawing(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "drawing")
		return
	}
	name, mime, body := fileFor(d)
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", contentDisposition(name))
	_, _ = w.Write(body)
}

func contentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r > 126 || r == '"' {
			return '_'
		}
		return r
	}, name)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, pathEscape(name))
}

func pathEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("-._~", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// ---- versions ----

func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := idParam(r, "id")
	if _, err := s.store.GetDrawing(ctx, uid, id); err != nil {
		s.fail(w, err, "drawing")
		return
	}
	list, err := s.store.ListVersions(ctx, uid, id)
	if err != nil {
		s.fail(w, err, "versions")
		return
	}
	type view struct {
		store.Version
		HasPreview bool `json:"has_preview"`
	}
	out := make([]view, 0, len(list))
	for _, v := range list {
		out = append(out, view{v, v.Preview != ""})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v, err := s.store.GetVersion(ctx, actorID(ctx), idParam(r, "id"), idParam(r, "vid"))
	if err != nil {
		s.fail(w, err, "version")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleVersionPreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v, err := s.store.GetVersion(ctx, actorID(ctx), idParam(r, "id"), idParam(r, "vid"))
	if err != nil {
		s.fail(w, err, "version")
		return
	}
	if v.Preview == "" {
		writeErr(w, http.StatusNotFound, "this version has no picture")
		return
	}
	serveSVG(w, r, v.Preview, fmt.Sprintf(`"v%d"`, v.ID))
}

func (s *Server) handleRestoreVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	d, err := s.store.RestoreVersion(ctx, uid, idParam(r, "id"), idParam(r, "vid"))
	if err != nil {
		s.fail(w, err, "restore")
		return
	}
	s.publish(ctx, uid, "draw.edited", fmt.Sprintf("drawing:%d", d.ID), map[string]any{"id": d.ID, "title": d.Title, "kind": d.Kind, "rev": d.Rev, "restored": true, "url": drawingURL(s.cfg.BaseURL, d.ID)})
	writeJSON(w, http.StatusOK, d)
}

// ---- folders ----

func (s *Server) handleListFolders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListFolders(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "folders")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func validFolderName(n string) (string, bool) {
	n = strings.TrimSpace(n)
	return n, n != "" && len(n) <= 80 && !strings.ContainsAny(n, "/\\")
}

func (s *Server) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	name, ok := validFolderName(in.Name)
	if !ok {
		writeErr(w, http.StatusBadRequest, "a folder needs a name (no slashes, up to 80 characters)")
		return
	}
	f, err := s.store.CreateFolder(ctx, actorID(ctx), name)
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, http.StatusConflict, "you already have a folder called "+name)
		return
	}
	if err != nil {
		s.fail(w, err, "folder")
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (s *Server) handleUpdateFolder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Name *string `json:"name"`
		Sort *int    `json:"sort"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Name != nil {
		name, ok := validFolderName(*in.Name)
		if !ok {
			writeErr(w, http.StatusBadRequest, "a folder needs a name (no slashes, up to 80 characters)")
			return
		}
		in.Name = &name
	}
	err := s.store.UpdateFolder(ctx, actorID(ctx), idParam(r, "id"), in.Name, in.Sort)
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, http.StatusConflict, "you already have a folder with that name")
		return
	}
	if err != nil {
		s.fail(w, err, "folder")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteFolder(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "folder")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- the sketch library ----

func (s *Server) handleGetLibrary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, at, err := s.store.Library(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "library")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, `{"items":%s,"updated_at":%q}`, items, at)
}

func (s *Server) handlePutLibrary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Items []json.RawMessage `json:"items"`
	}
	if _, err := s.decodeDrawing(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json (or too large)")
		return
	}
	if in.Items == nil {
		in.Items = []json.RawMessage{}
	}
	b, _ := json.Marshal(in.Items)
	if err := s.store.SaveLibrary(ctx, actorID(ctx), string(b)); err != nil {
		s.fail(w, err, "library")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(in.Items), "updated_at": time.Now().UTC().Format(time.RFC3339)})
}

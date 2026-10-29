package server

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-draw/internal/store"
)

// ---- export ----

// exportDoc is the JSON export of one person's drawings: every drawing with
// its content (trash included), the folders and the sketch library. Kept
// versions are not in it. Importing it again adds nothing twice (uid).
type exportDoc struct {
	App      string            `json:"app"`
	Version  string            `json:"version"`
	Exported string            `json:"exported_at"`
	Folders  []string          `json:"folders"`
	Drawings []exportDrawing   `json:"drawings"`
	Library  []json.RawMessage `json:"library"`
}

type exportDrawing struct {
	UID       string `json:"uid"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Folder    string `json:"folder"`
	Data      string `json:"data"`
	Preview   string `json:"preview,omitempty"`
	Starred   bool   `json:"starred"`
	Source    string `json:"source"`
	CreatedAt string `json:"created_at"`
	EditedAt  string `json:"edited_at"`
	TrashedAt string `json:"trashed_at,omitempty"`
}

// allDrawings lists every drawing of a person, trash included (no data).
func (s *Server) allDrawings(ctx context.Context, uid int64) ([]store.Drawing, error) {
	var out []store.Drawing
	for _, view := range []string{"all", "trash"} {
		for off := 0; ; off += 500 {
			list, _, err := s.store.ListDrawings(ctx, uid, store.DrawingFilter{View: view, Sort: "created", Limit: 500, Offset: off})
			if err != nil {
				return nil, err
			}
			out = append(out, list...)
			if len(list) < 500 {
				break
			}
		}
	}
	return out, nil
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	doc := exportDoc{App: "pocket-draw", Version: Version, Exported: store.Now(), Folders: []string{}, Drawings: []exportDrawing{}, Library: []json.RawMessage{}}
	fl, _ := s.store.ListFolders(ctx, uid)
	for _, f := range fl {
		doc.Folders = append(doc.Folders, f.Name)
	}
	list, err := s.allDrawings(ctx, uid)
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	for _, d := range list {
		full, err := s.store.GetDrawing(ctx, uid, d.ID)
		if err != nil {
			continue
		}
		prev, _, _ := s.store.DrawingPreview(ctx, uid, d.ID)
		doc.Drawings = append(doc.Drawings, exportDrawing{UID: full.UID, Kind: full.Kind, Title: full.Title, Folder: full.FolderName, Data: full.Data, Preview: prev,
			Starred: full.Starred, Source: full.Source, CreatedAt: full.CreatedAt, EditedAt: full.EditedAt, TrashedAt: full.TrashedAt})
	}
	if items, _, err := s.store.Library(ctx, uid); err == nil {
		_ = json.Unmarshal([]byte(items), &doc.Library)
	}
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-draw-export.json"`)
	writeJSON(w, http.StatusOK, doc)
}

// handleExportCSV lists the drawings (not their content) as CSV.
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.allDrawings(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-draw-drawings.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "title", "kind", "folder", "starred", "created_at", "edited_at", "in_trash", "bytes", "versions", "source", "url"})
	for _, d := range list {
		_ = cw.Write([]string{strconv.FormatInt(d.ID, 10), d.Title, d.Kind, d.FolderName, strconv.FormatBool(d.Starred), d.CreatedAt, d.EditedAt,
			strconv.FormatBool(d.TrashedAt != ""), strconv.Itoa(d.Bytes), strconv.Itoa(d.Versions), d.Source, drawingURL(s.cfg.BaseURL, d.ID)})
	}
	cw.Flush()
}

// ---- import ----

// importResult is what one import did.
type importResult struct {
	Kind     string         `json:"kind"`
	Counts   map[string]int `json:"counts"`
	Drawings []int64        `json:"drawings"`
}

// handleImport takes a file (multipart "file", or the raw body with ?name=)
// and works out what it is: a Pocket Draw export, an Excalidraw drawing
// (.excalidraw) or library (.excalidrawlib), a draw.io file (.drawio, .xml),
// a Mermaid file (.mmd, or Markdown with ```mermaid blocks) or a Mermaid
// Live link. ?folder_id= puts new drawings in a folder.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	name := r.URL.Query().Get("name")
	var body []byte
	var err error
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(s.cfg.MaxBodyBytes); err != nil {
			writeErr(w, http.StatusBadRequest, "that file is too large")
			return
		}
		f, h, ferr := r.FormFile("file")
		if ferr != nil {
			writeErr(w, http.StatusBadRequest, "choose a file")
			return
		}
		defer f.Close()
		if name == "" {
			name = h.Filename
		}
		body, err = io.ReadAll(io.LimitReader(f, s.cfg.MaxBodyBytes))
	} else {
		body, err = s.readBody(r)
	}
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		writeErr(w, http.StatusBadRequest, "the file is empty")
		return
	}
	var folder *int64
	if fid, _ := strconv.ParseInt(r.URL.Query().Get("folder_id"), 10, 64); fid > 0 {
		folder = &fid
	}
	res, err := s.importFile(ctx, uid, name, body, folder)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

var errUnknownFile = errors.New("this is not a file Pocket Draw can read: a Pocket Draw export, .excalidraw, .excalidrawlib, .drawio, .mmd, Markdown with a mermaid block, or a Mermaid Live link")

// sniffImport says what a file is from its name and first bytes.
func sniffImport(name string, b []byte) string {
	ext := strings.ToLower(path.Ext(name))
	t := bytes.TrimSpace(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")))
	head := strings.ToLower(string(t[:min(len(t), 600)]))
	switch {
	case strings.HasPrefix(head, "{") && strings.Contains(head, `"app"`) && strings.Contains(head, `"pocket-draw"`):
		return "pocket-draw"
	case strings.HasPrefix(head, "{") && strings.Contains(head, `"excalidrawlib"`), ext == ".excalidrawlib":
		return "excalidrawlib"
	case strings.HasPrefix(head, "{") && (strings.Contains(head, `"excalidraw"`) || strings.Contains(head, `"elements"`)), ext == ".excalidraw":
		return "excalidraw"
	case strings.Contains(head, "<mxfile") || strings.Contains(head, "<mxgraphmodel"), ext == ".drawio", ext == ".dio":
		return "drawio"
	case strings.HasPrefix(head, "http") && (strings.Contains(head, "#pako:") || strings.Contains(head, "#base64:")):
		return "mermaid-live"
	case ext == ".md" || ext == ".markdown" || strings.Contains(head, "```mermaid"):
		return "markdown"
	case ext == ".mmd" || ext == ".mermaid" || looksMermaid(head):
		return "mermaid"
	}
	return ""
}

var mermaidStart = regexp.MustCompile(`^(?:---[\s\S]*?---\s*)?(?:%%.*\n\s*)*(graph|flowchart|sequencediagram|classdiagram|statediagram(-v2)?|erdiagram|journey|gantt|pie|quadrantchart|requirementdiagram|gitgraph|mindmap|timeline|sankey(-beta)?|xychart(-beta)?|block(-beta)?|packet(-beta)?|kanban|architecture(-beta)?|c4context|radar(-beta)?|treemap(-beta)?)\b`)

func looksMermaid(lowerHead string) bool {
	return mermaidStart.MatchString(strings.TrimSpace(lowerHead))
}

func titleFromName(name, fallback string) string {
	base := path.Base(strings.ReplaceAll(name, "\\", "/"))
	for {
		ext := path.Ext(base)
		if ext == "" || len(ext) > 15 {
			break
		}
		base = strings.TrimSuffix(base, ext)
	}
	base = strings.TrimSpace(strings.NewReplacer("_", " ").Replace(base))
	if base == "" || base == "." {
		return fallback
	}
	return base
}

func (s *Server) importFile(ctx context.Context, uid int64, name string, body []byte, folder *int64) (*importResult, error) {
	kind := sniffImport(name, body)
	res := &importResult{Kind: kind, Counts: map[string]int{}, Drawings: []int64{}}
	add := func(in drawingIn) error {
		in.FolderID = folder
		d, err := s.newDrawing(ctx, uid, in, "import")
		if err != nil {
			return err
		}
		res.Drawings = append(res.Drawings, d.ID)
		res.Counts["drawings"]++
		return nil
	}
	text := strings.TrimPrefix(string(body), "\xef\xbb\xbf")
	switch kind {
	case "pocket-draw":
		counts, err := s.importOwn(ctx, uid, body)
		if err != nil {
			return nil, err
		}
		res.Counts = counts
		return res, nil
	case "excalidrawlib":
		n, err := s.mergeLibrary(ctx, uid, body)
		if err != nil {
			return nil, err
		}
		res.Counts["library_items"] = n
		return res, nil
	case "excalidraw":
		data, err := normalScene(body)
		if err != nil {
			return nil, err
		}
		return res, add(drawingIn{Kind: store.KindSketch, Title: titleFromName(name, "Imported sketch"), Data: data})
	case "drawio":
		return res, add(drawingIn{Kind: store.KindDrawio, Title: titleFromName(name, "Imported diagram"), Data: strings.TrimSpace(text)})
	case "mermaid":
		return res, add(drawingIn{Kind: store.KindMermaid, Title: titleFromName(name, "Imported text diagram"), Data: strings.TrimSpace(text) + "\n"})
	case "mermaid-live":
		code, err := mermaidLive(strings.TrimSpace(text))
		if err != nil {
			return nil, err
		}
		return res, add(drawingIn{Kind: store.KindMermaid, Title: "From Mermaid Live", Data: code})
	case "markdown":
		blocks := mermaidBlocks(text)
		if len(blocks) == 0 {
			return nil, errors.New("there is no ```mermaid block in this file")
		}
		title := titleFromName(name, "Imported text diagram")
		for i, b := range blocks {
			t := title
			if len(blocks) > 1 {
				t = fmt.Sprintf("%s (%d)", title, i+1)
			}
			if err := add(drawingIn{Kind: store.KindMermaid, Title: t, Data: b}); err != nil {
				return res, err
			}
		}
		return res, nil
	}
	return nil, errUnknownFile
}

// normalScene keeps what a sketch needs from an .excalidraw file.
func normalScene(body []byte) (string, error) {
	var scene map[string]json.RawMessage
	if err := json.Unmarshal(body, &scene); err != nil {
		return "", errors.New("this .excalidraw file is not valid JSON")
	}
	if _, ok := scene["elements"]; !ok {
		return "", errors.New("this .excalidraw file has no drawing in it")
	}
	out := map[string]json.RawMessage{"type": json.RawMessage(`"excalidraw"`), "version": json.RawMessage(`2`), "source": json.RawMessage(`"pocket-draw"`), "elements": scene["elements"]}
	if v, ok := scene["appState"]; ok {
		out["appState"] = v
	}
	if v, ok := scene["files"]; ok {
		out["files"] = v
	} else {
		out["files"] = json.RawMessage(`{}`)
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

var fenceRe = regexp.MustCompile("(?s)```mermaid[^\\n]*\\n(.*?)```")

func mermaidBlocks(md string) []string {
	var out []string
	for _, m := range fenceRe.FindAllStringSubmatch(md, -1) {
		if b := strings.TrimSpace(m[1]); b != "" {
			out = append(out, b+"\n")
		}
	}
	return out
}

// mermaidLive reads the diagram out of a Mermaid Live link (#pako: is zlib,
// #base64: is plain; both base64url of a JSON state with "code").
func mermaidLive(link string) (string, error) {
	bad := errors.New("this Mermaid Live link could not be read")
	i := strings.Index(link, "#")
	if i < 0 {
		return "", bad
	}
	frag := link[i+1:]
	var raw []byte
	var err error
	dec := func(s string) ([]byte, error) {
		s = strings.TrimRight(s, "=")
		return base64.RawURLEncoding.DecodeString(strings.NewReplacer("+", "-", "/", "_").Replace(s))
	}
	switch {
	case strings.HasPrefix(frag, "pako:"):
		var z []byte
		if z, err = dec(strings.TrimPrefix(frag, "pako:")); err != nil {
			return "", bad
		}
		zr, zerr := zlib.NewReader(bytes.NewReader(z))
		if zerr != nil {
			return "", bad
		}
		raw, err = io.ReadAll(io.LimitReader(zr, 4<<20))
	case strings.HasPrefix(frag, "base64:"):
		raw, err = dec(strings.TrimPrefix(frag, "base64:"))
	default:
		return "", bad
	}
	if err != nil {
		return "", bad
	}
	var st struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(raw, &st) != nil || strings.TrimSpace(st.Code) == "" {
		return "", bad
	}
	return strings.TrimSpace(st.Code) + "\n", nil
}

// mergeLibrary adds the items of an .excalidrawlib file to the person's
// library, skipping ones already there (by id).
func (s *Server) mergeLibrary(ctx context.Context, uid int64, body []byte) (int, error) {
	var lib struct {
		Library      []json.RawMessage `json:"library"`      // version 1: arrays of elements
		LibraryItems []json.RawMessage `json:"libraryItems"` // version 2
	}
	if err := json.Unmarshal(body, &lib); err != nil {
		return 0, errors.New("this library file is not valid JSON")
	}
	cur, _, err := s.store.Library(ctx, uid)
	if err != nil {
		return 0, err
	}
	var items []json.RawMessage
	_ = json.Unmarshal([]byte(cur), &items)
	seen := map[string]bool{}
	idOf := func(raw json.RawMessage) string {
		var it struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &it)
		return it.ID
	}
	for _, it := range items {
		seen[idOf(it)] = true
	}
	added := 0
	for _, it := range lib.LibraryItems {
		if id := idOf(it); id != "" && seen[id] {
			continue
		}
		items = append(items, it)
		added++
	}
	for _, els := range lib.Library {
		item, _ := json.Marshal(map[string]any{"id": store.NewUID(), "status": "unpublished", "elements": els, "created": 0})
		items = append(items, item)
		added++
	}
	b, _ := json.Marshal(items)
	return added, s.store.SaveLibrary(ctx, uid, string(b))
}

// importOwn reads a Pocket Draw export. Drawings already here (same uid) are skipped.
func (s *Server) importOwn(ctx context.Context, uid int64, body []byte) (map[string]int, error) {
	var doc exportDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, errors.New("this Pocket Draw export is not valid JSON")
	}
	counts := map[string]int{"drawings": 0, "skipped": 0, "folders": 0, "library_items": 0}
	folderID := map[string]int64{}
	ensure := func(name string) *int64 {
		if id, ok := folderID[strings.ToLower(name)]; ok {
			return &id
		}
		f, err := s.store.FolderByName(ctx, uid, name)
		if err != nil {
			if f, err = s.store.CreateFolder(ctx, uid, name); err != nil {
				return nil
			}
			counts["folders"]++
		}
		folderID[strings.ToLower(name)] = f.ID
		return &f.ID
	}
	for _, name := range doc.Folders {
		if n, ok := validFolderName(name); ok {
			ensure(n)
		}
	}
	for _, e := range doc.Drawings {
		if !store.ValidKind(e.Kind) || validData(e.Kind, e.Data) != nil {
			counts["skipped"]++
			continue
		}
		prev, _ := cleanPreview(e.Preview)
		d := &store.Drawing{UID: e.UID, Kind: e.Kind, Title: firstNonEmpty(strings.TrimSpace(e.Title), untitled(e.Kind)), Data: e.Data, Preview: prev,
			Starred: e.Starred, Source: firstNonEmpty(e.Source, "import"), CreatedAt: e.CreatedAt, EditedAt: e.EditedAt}
		if n, ok := validFolderName(e.Folder); ok {
			d.FolderID = ensure(n)
		}
		if err := s.store.CreateDrawing(ctx, uid, d); err != nil {
			if errors.Is(err, store.ErrConflict) {
				counts["skipped"]++
				continue
			}
			return counts, err
		}
		if e.TrashedAt != "" {
			_ = s.store.TrashDrawing(ctx, uid, d.ID, false)
		}
		counts["drawings"]++
	}
	if len(doc.Library) > 0 {
		b, _ := json.Marshal(map[string]any{"type": "excalidrawlib", "version": 2, "libraryItems": doc.Library})
		n, err := s.mergeLibrary(ctx, uid, b)
		if err != nil {
			return counts, err
		}
		counts["library_items"] = n
	}
	return counts, nil
}

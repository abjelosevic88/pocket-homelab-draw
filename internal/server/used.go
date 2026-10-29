package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-draw/internal/store"
)

// ---- Used in ----
//
// A drawing is in a note or an issue when that text links to the drawing's
// page here, or to its picture under it (![…](…/d/12/picture.svg)). Nothing
// is stored on either side: Draw asks Pocket Notes and Pocket Projects, as
// the person, which of their notes and issues link here (the /mentions call
// both offer to linked apps, docs/notes-from-apps.md in Pocket Notes).

// usedItem is one note, issue or project that shows a drawing.
type usedItem struct {
	Kind     string `json:"kind"` // note | issue | project
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	URL      string `json:"url"`
	Modified string `json:"modified"`
}

// usedSource is what one app answered.
type usedSource struct {
	App       string     `json:"app"`
	Name      string     `json:"name"`
	Connected bool       `json:"connected"`
	Detail    string     `json:"detail,omitempty"` // why not connected, or what failed
	Items     []usedItem `json:"items"`
	OpenURL   string     `json:"open_url,omitempty"`
}

// pageURL is a drawing's page here, the address notes and issues link to.
func (s *Server) pageURL(id int64) string {
	return drawingURL(s.cfg.BaseURL, id)
}

// pictureURL is the picture under the drawing's page, for Markdown elsewhere.
func (s *Server) pictureURL(id int64) string {
	if s.cfg.BaseURL == "" {
		return ""
	}
	return s.pageURL(id) + "/picture.svg"
}

// embedMarkdown is how a drawing goes into a note or an issue: its picture,
// with the title as the caption.
func embedMarkdown(title, picture string) string {
	t := strings.Join(strings.Fields(strings.NewReplacer("[", "", "]", "", `"`, "", "(", "", ")", "").Replace(title)), " ")
	if t == "" {
		t = "Drawing"
	}
	return "![" + t + "](" + picture + ` "` + t + `")`
}

func (s *Server) handleUsedIn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	d, err := s.store.GetDrawing(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "drawing")
		return
	}
	page, picture := s.pageURL(d.ID), s.pictureURL(d.ID)
	out := map[string]any{"page_url": page, "picture_url": picture, "markdown": "", "sources": []usedSource{}}
	if picture != "" {
		out["markdown"] = embedMarkdown(d.Title, picture)
	}
	if page == "" {
		out["detail"] = "Set BASE_URL so drawings have an address other apps can link to"
		writeJSON(w, http.StatusOK, out)
		return
	}
	cl := s.clients(ctx, uid)
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	sources := []usedSource{{App: appNotes.slug, Name: appNotes.name}, {App: appProjects.slug, Name: appProjects.name}}
	var wg sync.WaitGroup
	var mu sync.Mutex // the clients' connection cache is not safe for two at once
	for i := range sources {
		wg.Add(1)
		go func(src *usedSource) {
			defer wg.Done()
			a := appNotes
			if src.App == appProjects.slug {
				a = appProjects
			}
			mu.Lock()
			sib, err := cl.conn(cctx, a, a.scopes...)
			mu.Unlock()
			if err != nil {
				src.Detail = err.Error()
				return
			}
			src.Connected = true
			if a.slug == appNotes.slug {
				src.OpenURL = notesNewURL(sib.public, d, picture)
				src.Items, err = notesUsing(cctx, sib, page)
			} else {
				src.Items, err = projectsUsing(cctx, sib, page)
			}
			if err != nil {
				src.Detail = a.name + ": " + err.Error()
			}
		}(&sources[i])
	}
	wg.Wait()
	for i := range sources {
		if sources[i].Items == nil {
			sources[i].Items = []usedItem{}
		}
	}
	out["sources"] = sources
	writeJSON(w, http.StatusOK, out)
}

// notesNewURL opens Pocket Notes on a new note that shows the drawing.
func notesNewURL(public string, d *store.Drawing, picture string) string {
	if public == "" || picture == "" {
		return ""
	}
	q := url.Values{"title": {d.Title}, "url": {picture}, "label": {d.Title}, "image": {"1"}}
	return strings.TrimRight(public, "/") + "/new?" + q.Encode()
}

func notesUsing(ctx context.Context, sib *siblingConn, page string) ([]usedItem, error) {
	var notes []struct {
		ID       int64  `json:"id"`
		Title    string `json:"title"`
		Category string `json:"category"`
		Modified int64  `json:"modified"`
	}
	if err := sib.client.Get(ctx, "/api/v1/mentions", url.Values{"url": {page}, "limit": {"30"}}, &notes); err != nil {
		return nil, err
	}
	out := []usedItem{}
	for _, n := range notes {
		it := usedItem{Kind: "note", Title: firstNonEmpty(n.Title, fmt.Sprintf("Note %d", n.ID)), Detail: n.Category}
		if sib.public != "" {
			it.URL = fmt.Sprintf("%s/note/%d", strings.TrimRight(sib.public, "/"), n.ID)
		}
		if n.Modified > 0 {
			it.Modified = time.Unix(n.Modified, 0).UTC().Format(time.RFC3339)
		}
		out = append(out, it)
	}
	return out, nil
}

func projectsUsing(ctx context.Context, sib *siblingConn, page string) ([]usedItem, error) {
	var found []struct {
		Kind       string `json:"kind"`
		Identifier string `json:"identifier"`
		Title      string `json:"title"`
		URL        string `json:"url"`
		Team       string `json:"team"`
		UpdatedAt  string `json:"updated_at"`
	}
	if err := sib.client.Get(ctx, "/api/v1/mentions", url.Values{"url": {page}}, &found); err != nil {
		return nil, err
	}
	out := []usedItem{}
	for _, f := range found {
		kind := "issue"
		if f.Kind == "project" {
			kind = "project"
		}
		title := f.Title
		if f.Identifier != "" {
			title = f.Identifier + " " + f.Title
		}
		out = append(out, usedItem{Kind: kind, Title: strings.TrimSpace(title), Detail: f.Team, URL: f.URL, Modified: f.UpdatedAt})
	}
	return out, nil
}

package server

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-draw/internal/config"
	"github.com/abjelosevic88/pocket-homelab-draw/internal/store"
)

type app struct {
	t     *testing.T
	url   string
	s     *Server
	st    *store.Store
	alice string
	bob   string
	ids   map[string]int64
}

const scene = `{"type":"excalidraw","version":2,"elements":[{"id":"a","type":"rectangle","x":0,"y":0,"width":10,"height":10},{"id":"b","type":"text","text":"Kitchen sink","x":0,"y":0},{"id":"c","type":"text","text":"gone","isDeleted":true}],"appState":{},"files":{}}`

const preview = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10"/></svg>`

// newApp starts Draw with Alice (admin) and Bob (member).
func newApp(t *testing.T, tweak func(*config.Config)) *app {
	t.Helper()
	a := &app{t: t, ids: map[string]int64{}}
	cfg := testConfig(t.TempDir())
	cfg.BaseURL = "http://draw.test"
	if tweak != nil {
		tweak(cfg)
	}
	st := openTestStore(t, cfg)
	a.st = st
	ctx := context.Background()
	for _, u := range []*store.User{{Email: "alice@example.com", Name: "Alice", Role: "admin"}, {Email: "bob@example.com", Name: "Bob", Role: "member"}} {
		if err := st.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		tok, _, _ := st.CreateAPIToken(ctx, u.ID, "t")
		a.ids[u.Name] = u.ID
		if u.Name == "Alice" {
			a.alice = tok
		} else {
			a.bob = tok
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	a.s = New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err := a.s.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(a.s.Router())
	t.Cleanup(ts.Close)
	a.url = ts.URL
	return a
}

func (a *app) do(auth, method, path, ctype string, body io.Reader) (*http.Response, []byte) {
	a.t.Helper()
	req, _ := http.NewRequest(method, a.url+path, body)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res, raw
}

func (a *app) call(auth, method, path string, body any) (int, map[string]any, []any) {
	a.t.Helper()
	var rd io.Reader
	if body != nil {
		j, _ := json.Marshal(body)
		rd = bytes.NewReader(j)
	}
	res, raw := a.do(auth, method, path, "application/json", rd)
	var m map[string]any
	var l []any
	if json.Unmarshal(raw, &m) != nil {
		_ = json.Unmarshal(raw, &l)
	}
	return res.StatusCode, m, l
}

func (a *app) ok(auth, method, path string, body any) map[string]any {
	a.t.Helper()
	code, m, _ := a.call(auth, method, path, body)
	if code >= 300 {
		a.t.Fatalf("%s %s → %d %v", method, path, code, m)
	}
	return m
}

func (a *app) expect(want int, auth, method, path string, body any) map[string]any {
	a.t.Helper()
	code, m, _ := a.call(auth, method, path, body)
	if code != want {
		a.t.Fatalf("%s %s → %d, want %d (%v)", method, path, code, want, m)
	}
	return m
}

func num(m map[string]any, k string) float64 { v, _ := m[k].(float64); return v }

func list(m map[string]any) []map[string]any {
	var out []map[string]any
	l, _ := m["drawings"].([]any)
	for _, x := range l {
		out = append(out, x.(map[string]any))
	}
	return out
}

func (a *app) create(tok, kind, title, data string) int64 {
	a.t.Helper()
	d := a.expect(201, tok, "POST", "/api/v1/drawings", map[string]any{"kind": kind, "title": title, "data": data, "preview": preview})
	return int64(num(d, "id"))
}

func TestPerPersonIsolation(t *testing.T) {
	a := newApp(t, nil)
	id := a.create(a.alice, "sketch", "Alice's plan", scene)
	f := a.expect(201, a.alice, "POST", "/api/v1/folders", map[string]any{"name": "Home"})
	fid := int64(num(f, "id"))
	p := fmt.Sprintf("/api/v1/drawings/%d", id)
	// Bob — a member — sees none of it, and an admin is no exception the other way
	for _, path := range []string{p, p + "/preview.svg", p + "/file", p + "/versions"} {
		a.expect(404, a.bob, "GET", path, nil)
	}
	a.expect(404, a.bob, "PUT", p, map[string]any{"title": "mine now"})
	a.expect(404, a.bob, "DELETE", p, nil)
	a.expect(404, a.bob, "POST", p+"/duplicate", nil)
	a.expect(404, a.bob, "PUT", fmt.Sprintf("/api/v1/folders/%d", fid), map[string]any{"name": "x"})
	a.expect(400, a.bob, "POST", "/api/v1/drawings", map[string]any{"kind": "sketch", "folder_id": fid})
	if n := len(list(a.ok(a.bob, "GET", "/api/v1/drawings", nil))); n != 0 {
		t.Fatalf("bob sees %d drawings", n)
	}
	bobs := a.create(a.bob, "mermaid", "Bob's", "graph TD\n A-->B\n")
	a.expect(404, a.alice, "GET", fmt.Sprintf("/api/v1/drawings/%d", bobs), nil)
	if n := len(list(a.ok(a.alice, "GET", "/api/v1/drawings", nil))); n != 1 {
		t.Fatalf("alice (admin) sees %d drawings, want only her own", n)
	}
	if o := a.ok(a.bob, "GET", "/api/v1/dashboard", nil); num(o["counts"].(map[string]any), "all") != 1 {
		t.Fatalf("bob's overview counts alice's drawings: %v", o)
	}
	// the user id comes from the session, never from the body
	a.expect(201, a.bob, "POST", "/api/v1/drawings", map[string]any{"kind": "sketch", "user_id": a.ids["Alice"]})
	if n := len(list(a.ok(a.alice, "GET", "/api/v1/drawings", nil))); n != 1 {
		t.Fatalf("bob created a drawing for alice")
	}
}

func TestSaveRevisionsAndVersions(t *testing.T) {
	a := newApp(t, nil)
	id := a.create(a.alice, "sketch", "Plan", scene)
	p := fmt.Sprintf("/api/v1/drawings/%d", id)
	d := a.ok(a.alice, "GET", p+"?open=1", nil)
	if num(d, "rev") != 1 || d["data"] != scene {
		t.Fatalf("new drawing: %v", d)
	}
	next := strings.Replace(scene, "Kitchen sink", "Bathroom sink", 1)
	a.ok(a.alice, "PUT", p, map[string]any{"data": next, "rev": 1, "preview": preview})
	// a save made on rev 1 again (another tab) is refused, with the current rev
	stale := a.expect(409, a.alice, "PUT", p, map[string]any{"data": scene, "rev": 1})
	if num(stale, "rev") != 2 {
		t.Fatalf("conflict answer: %v", stale)
	}
	// the first change kept the original as a version
	_, _, vs := a.call(a.alice, "GET", p+"/versions", nil)
	if len(vs) != 1 {
		t.Fatalf("versions: %v", vs)
	}
	vid := int64(num(vs[0].(map[string]any), "id"))
	// within ten minutes the next change keeps no new version
	a.ok(a.alice, "PUT", p, map[string]any{"data": strings.Replace(next, "Bathroom", "Garden", 1), "rev": 2})
	if _, _, vs := a.call(a.alice, "GET", p+"/versions", nil); len(vs) != 1 {
		t.Fatalf("versions after a quick second save: %d", len(vs))
	}
	// restoring puts the old state back and keeps the current one
	r := a.ok(a.alice, "POST", fmt.Sprintf("%s/versions/%d/restore", p, vid), nil)
	if num(r, "rev") != 4 {
		t.Fatalf("rev after restore: %v", r["rev"])
	}
	if got := a.ok(a.alice, "GET", p, nil); got["data"] != scene {
		t.Fatalf("restore did not bring the old drawing back")
	}
	if _, _, vs := a.call(a.alice, "GET", p+"/versions", nil); len(vs) != 2 {
		t.Fatalf("versions after restore: %d", len(vs))
	}
	// search finds words on the sketch, not deleted ones
	if n := len(list(a.ok(a.alice, "GET", "/api/v1/drawings?q=kitchen", nil))); n != 1 {
		t.Fatalf("search by sketch text: %d", n)
	}
	if n := len(list(a.ok(a.alice, "GET", "/api/v1/drawings?q=gone", nil))); n != 0 {
		t.Fatalf("search found a deleted element")
	}
	// bad content for the kind is refused
	a.expect(400, a.alice, "PUT", p, map[string]any{"data": "not json", "rev": 4})
	a.expect(400, a.alice, "POST", "/api/v1/drawings", map[string]any{"kind": "drawio", "data": "hello"})
	a.expect(400, a.alice, "POST", "/api/v1/drawings", map[string]any{"kind": "paint"})
}

func TestPreviewIsServedSafely(t *testing.T) {
	a := newApp(t, nil)
	id := a.create(a.alice, "mermaid", "Flow", "graph TD\n A-->B\n")
	res, body := a.do(a.alice, "GET", fmt.Sprintf("/api/v1/drawings/%d/preview.svg", id), "", nil)
	if res.StatusCode != 200 || !strings.HasPrefix(string(body), "<svg") {
		t.Fatalf("preview: %d %s", res.StatusCode, body)
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
		t.Fatalf("preview served without a strict CSP: %q", csp)
	}
	// something that is not an SVG is not kept as a preview
	a.ok(a.alice, "PUT", fmt.Sprintf("/api/v1/drawings/%d", id), map[string]any{"preview": "<html><script>alert(1)</script>"})
	if res, _ := a.do(a.alice, "GET", fmt.Sprintf("/api/v1/drawings/%d/preview.svg", id), "", nil); res.StatusCode != 200 {
		t.Fatalf("a bad preview replaced the good one")
	}
	res, body = a.do(a.alice, "GET", fmt.Sprintf("/api/v1/drawings/%d/file", id), "", nil)
	if !strings.Contains(res.Header.Get("Content-Disposition"), "Flow.mmd") || string(body) != "graph TD\n A-->B\n" {
		t.Fatalf("file download: %q %q", res.Header.Get("Content-Disposition"), body)
	}
}

func TestTrashAndFolders(t *testing.T) {
	a := newApp(t, nil)
	f := a.expect(201, a.alice, "POST", "/api/v1/folders", map[string]any{"name": "Work"})
	a.expect(409, a.alice, "POST", "/api/v1/folders", map[string]any{"name": "work"})
	a.expect(400, a.alice, "POST", "/api/v1/folders", map[string]any{"name": "a/b"})
	fid := int64(num(f, "id"))
	d := a.expect(201, a.alice, "POST", "/api/v1/drawings", map[string]any{"kind": "mermaid", "folder_id": fid})
	if d["title"] != "Untitled text diagram" || d["folder_name"] != "Work" {
		t.Fatalf("new drawing: %v", d)
	}
	id := int64(num(d, "id"))
	if n := len(list(a.ok(a.alice, "GET", fmt.Sprintf("/api/v1/drawings?folder=%d", fid), nil))); n != 1 {
		t.Fatalf("folder filter: %d", n)
	}
	p := fmt.Sprintf("/api/v1/drawings/%d", id)
	a.expect(400, a.alice, "DELETE", p+"?forever=1", nil) // trash first
	a.expect(204, a.alice, "DELETE", p, nil)
	if n := len(list(a.ok(a.alice, "GET", "/api/v1/drawings", nil))); n != 0 {
		t.Fatalf("trashed drawing still listed")
	}
	if n := len(list(a.ok(a.alice, "GET", "/api/v1/drawings?view=trash", nil))); n != 1 {
		t.Fatalf("trash: %d", n)
	}
	a.expect(204, a.alice, "POST", p+"/restore", nil)
	a.expect(204, a.alice, "DELETE", fmt.Sprintf("/api/v1/folders/%d", fid), nil)
	if got := a.ok(a.alice, "GET", p, nil); got["folder_id"] != nil {
		t.Fatalf("drawing still in a deleted folder: %v", got["folder_id"])
	}
	a.expect(204, a.alice, "DELETE", p, nil)
	if r := a.ok(a.alice, "POST", "/api/v1/trash/empty", nil); num(r, "deleted") != 1 {
		t.Fatalf("empty trash: %v", r)
	}
	a.expect(404, a.alice, "GET", p, nil)
}

func TestExportImportIdempotent(t *testing.T) {
	a := newApp(t, nil)
	a.expect(201, a.alice, "POST", "/api/v1/drawings", map[string]any{"kind": "sketch", "title": "One", "data": scene, "folder": "Home", "starred": true})
	a.create(a.alice, "mermaid", "Two", "graph LR\n A-->B\n")
	a.ok(a.alice, "PUT", "/api/v1/library", map[string]any{"items": []any{map[string]any{"id": "lib1", "status": "unpublished", "elements": []any{}}}})
	_, raw := a.do(a.alice, "GET", "/api/v1/export.json", "", nil)
	// into Bob's account: everything arrives once
	for round := 0; round < 2; round++ {
		res, body := a.do(a.bob, "POST", "/api/v1/import?name=export.json", "application/json", bytes.NewReader(raw))
		if res.StatusCode != 200 {
			t.Fatalf("import: %d %s", res.StatusCode, body)
		}
		var r importResult
		_ = json.Unmarshal(body, &r)
		want := 2
		if round == 1 {
			want = 0
		}
		if r.Kind != "pocket-draw" || r.Counts["drawings"] != want {
			t.Fatalf("round %d: %+v", round, r)
		}
	}
	got := list(a.ok(a.bob, "GET", "/api/v1/drawings?sort=title", nil))
	if len(got) != 2 || got[0]["title"] != "One" || got[0]["folder_name"] != "Home" || got[0]["starred"] != true {
		t.Fatalf("bob's drawings after import: %v", got)
	}
	_, lib := a.do(a.bob, "GET", "/api/v1/library", "", nil)
	if !strings.Contains(string(lib), "lib1") {
		t.Fatalf("library not imported: %s", lib)
	}
}

func TestImportFormats(t *testing.T) {
	a := newApp(t, nil)
	imp := func(name, body string) importResult {
		t.Helper()
		res, raw := a.do(a.alice, "POST", "/api/v1/import?name="+url.QueryEscape(name), "text/plain", strings.NewReader(body))
		if res.StatusCode != 200 {
			t.Fatalf("import %s: %d %s", name, res.StatusCode, raw)
		}
		var r importResult
		_ = json.Unmarshal(raw, &r)
		return r
	}
	if r := imp("Floor plan.excalidraw", scene); r.Kind != "excalidraw" || len(r.Drawings) != 1 {
		t.Fatalf("excalidraw: %+v", r)
	}
	drawio := `<mxfile><diagram name="Page-1"><mxGraphModel><root><mxCell id="0"/><mxCell id="2" value="Front &lt;b&gt;door&lt;/b&gt;" vertex="1"/></root></mxGraphModel></diagram></mxfile>`
	if r := imp("house.drawio", drawio); r.Kind != "drawio" {
		t.Fatalf("drawio: %+v", r)
	}
	if n := len(list(a.ok(a.alice, "GET", "/api/v1/drawings?q=door", nil))); n != 1 {
		t.Fatalf("draw.io labels not searchable")
	}
	if r := imp("notes.md", "# Notes\n\n```mermaid\ngraph TD\n A-->B\n```\n\ntext\n\n```mermaid\npie\n \"a\": 1\n```\n"); r.Counts["drawings"] != 2 {
		t.Fatalf("markdown: %+v", r)
	}
	if r := imp("flow.mmd", "sequenceDiagram\n A->>B: hi\n"); r.Kind != "mermaid" {
		t.Fatalf("mmd: %+v", r)
	}
	if r := imp("shapes.excalidrawlib", `{"type":"excalidrawlib","version":2,"libraryItems":[{"id":"x1","elements":[]},{"id":"x2","elements":[]}]}`); r.Counts["library_items"] != 2 {
		t.Fatalf("library: %+v", r)
	}
	// a Mermaid Live link (pako = zlib + base64url)
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write([]byte(`{"code":"graph TD\n  Live-->Link","mermaid":"{}"}`))
	_ = zw.Close()
	link := "https://mermaid.live/edit#pako:" + base64.RawURLEncoding.EncodeToString(z.Bytes())
	if r := imp("", link); r.Kind != "mermaid-live" || len(r.Drawings) != 1 {
		t.Fatalf("mermaid live: %+v", r)
	}
	d := a.ok(a.alice, "GET", "/api/v1/drawings?q=live", nil)
	if len(list(d)) != 1 {
		t.Fatalf("mermaid live code not stored")
	}
	res, _ := a.do(a.alice, "POST", "/api/v1/import?name=x.txt", "text/plain", strings.NewReader("just words"))
	if res.StatusCode != 400 {
		t.Fatalf("unknown file accepted: %d", res.StatusCode)
	}
}

func TestSettingsArePersonal(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"default_kind": "mermaid", "files_folder": "Sketches/2026", "notify_url": "http://example.invalid/x", "title": "Our Draw", "drawio_url": "https://drawio.example"})
	a.ok(a.bob, "PUT", "/api/v1/settings", map[string]any{"title": "Bob's title", "drawio_url": "https://evil.example"})
	b := a.ok(a.bob, "GET", "/api/v1/settings", nil)
	if b["default_kind"] != "ask" || b["notify_url"] != "" || b["files_folder"] != "Drawings" {
		t.Fatalf("bob got alice's settings: %v", b)
	}
	if b["title"] != "Our Draw" || b["drawio_url"] != "https://drawio.example" {
		t.Fatalf("a member changed box settings: %v %v", b["title"], b["drawio_url"])
	}
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"default_kind": "paint"})
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"files_folder": "../etc"})
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"drawio_url": "javascript:alert(1)"})
}

// A drawings:read link reads; drawings:write adds and changes only what it added.
func TestLinkScopes(t *testing.T) {
	links := map[string]string{"pcx_read": "drawings:read", "pcx_write": "drawings:write", "pcx_other": "drawings:read"}
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/auth/introspect" {
			io.WriteString(w, `{}`)
			return
		}
		var in struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		scope, ok := links[in.Token]
		if !ok {
			io.WriteString(w, `{"active":false}`)
			return
		}
		aud := "draw"
		if in.Token == "pcx_other" {
			aud = "notes" // a token for another app
		}
		fmt.Fprintf(w, `{"active":true,"kind":"delegated","scope":%q,"audience":%q,"user":{"id":1,"sub":"alice@example.com","email":"alice@example.com","name":"Alice","role":"admin"},"app":{"id":5,"slug":"ai","name":"Pocket AI"}}`, scope, aud)
	}))
	t.Cleanup(core.Close)
	a := newApp(t, func(c *config.Config) { c.CoreURL = core.URL })
	own := a.create(a.alice, "sketch", "Alice's own", scene)
	read, write := "pcx_read", "pcx_write"
	a.expect(200, read, "GET", "/api/v1/dashboard", nil)
	a.expect(401, "pcx_other", "GET", "/api/v1/dashboard", nil)
	a.expect(200, read, "GET", "/api/v1/day/"+time.Now().Format("2006-01-02"), nil)
	a.expect(200, read, "GET", fmt.Sprintf("/api/v1/drawings/%d", own), nil)
	a.expect(403, read, "POST", "/api/v1/drawings", map[string]any{"kind": "mermaid"})
	a.expect(403, read, "PUT", "/api/v1/settings", map[string]any{"default_kind": "sketch"})
	a.expect(403, read, "GET", "/api/v1/export.json", nil)
	a.expect(403, read, "GET", fmt.Sprintf("/api/v1/drawings/%d/versions", own), nil)
	a.expect(403, write, "GET", "/api/v1/drawings", nil)
	d := a.expect(201, write, "POST", "/api/v1/drawings", map[string]any{"kind": "mermaid", "title": "Drawn by AI", "data": "graph TD\n A-->B\n"})
	if d["source"] != "ai" {
		t.Fatalf("source of a link drawing: %v", d["source"])
	}
	mine := fmt.Sprintf("/api/v1/drawings/%d", int64(num(d, "id")))
	a.expect(200, write, "PUT", mine, map[string]any{"title": "Drawn by AI, fixed"})
	a.expect(204, write, "DELETE", mine, nil)
	a.expect(403, write, "PUT", fmt.Sprintf("/api/v1/drawings/%d", own), map[string]any{"title": "taken"})
	a.expect(403, write, "DELETE", fmt.Sprintf("/api/v1/drawings/%d", own), nil)
}

func TestSearchTextExtraction(t *testing.T) {
	if got := store.SearchText("sketch", scene); got != "Kitchen sink" {
		t.Fatalf("sketch text: %q", got)
	}
	// a compressed draw.io page (base64 of raw deflate of the URL-encoded XML)
	inner := url.QueryEscape(`<mxGraphModel><root><mxCell id="2" value="Hidden label" vertex="1"/></root></mxGraphModel>`)
	var buf bytes.Buffer
	fw, _ := newRawDeflate(&buf)
	_, _ = fw.Write([]byte(inner))
	_ = fw.Close()
	doc := `<mxfile><diagram name="Plans">` + base64.StdEncoding.EncodeToString(buf.Bytes()) + `</diagram></mxfile>`
	if got := store.SearchText("drawio", doc); !strings.Contains(got, "Hidden label") || !strings.Contains(got, "Plans") {
		t.Fatalf("compressed draw.io: %q", got)
	}
}

func TestDemoSeeds(t *testing.T) {
	a := newApp(t, func(c *config.Config) {
		c.DemoData = true
		c.AuthMode = "local"
	})
	ctx := context.Background()
	if err := a.s.seedDemo(ctx, a.ids["Alice"]); err != nil {
		t.Fatal(err)
	}
	o := a.ok(a.alice, "GET", "/api/v1/dashboard", nil)
	c := o["counts"].(map[string]any)
	if num(c, "all") < 15 || num(c, "trash") != 1 {
		t.Fatalf("demo counts: %v", c)
	}
	for _, d := range list(a.ok(a.alice, "GET", "/api/v1/drawings?limit=100", nil)) {
		full := a.ok(a.alice, "GET", fmt.Sprintf("/api/v1/drawings/%d", int64(num(d, "id"))), nil)
		if err := validData(full["kind"].(string), full["data"].(string)); err != nil {
			t.Fatalf("demo drawing %v: %v", d["title"], err)
		}
	}
}

func newRawDeflate(w io.Writer) (io.WriteCloser, error) {
	return flate.NewWriter(w, flate.DefaultCompression)
}

// A drawing knows where it is used by asking Notes and Projects (as the
// person) which notes and issues link to its page; its picture answers at an
// address under that page.
func TestUsedInNotesAndProjects(t *testing.T) {
	var asked []string
	var mu sync.Mutex
	fake := func(answer string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			mu.Lock()
			asked = append(asked, r.URL.Path+"?"+r.URL.RawQuery)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, answer)
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	notes := fake(`[{"id":7,"title":"Flat ideas","category":"Home","modified":1793000000}]`)
	projects := fake(`[{"kind":"issue","id":3,"identifier":"WEB-12","title":"New layout","url":"http://projects.test/issue/WEB-12","team":"Web"}]`)
	a := newApp(t, nil)
	id := a.create(a.alice, "sketch", "Floor (plan) [v2]", scene)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{
		"notes_mode": "direct", "notes_url": notes.URL, "notes_token": "tok", "notes_public_url": "http://notes.test",
		"projects_mode": "direct", "projects_url": projects.URL, "projects_token": "tok"})
	u := a.ok(a.alice, "GET", fmt.Sprintf("/api/v1/drawings/%d/used", id), nil)
	page := fmt.Sprintf("http://draw.test/d/%d", id)
	if u["page_url"] != page || u["picture_url"] != page+"/picture.svg" {
		t.Fatalf("addresses: %v", u)
	}
	if u["markdown"] != fmt.Sprintf(`![Floor plan v2](%s/picture.svg "Floor plan v2")`, page) {
		t.Fatalf("embed markdown: %v", u["markdown"])
	}
	src := u["sources"].([]any)
	n, p := src[0].(map[string]any), src[1].(map[string]any)
	ni, pi := n["items"].([]any), p["items"].([]any)
	if len(ni) != 1 || ni[0].(map[string]any)["url"] != "http://notes.test/note/7" || len(pi) != 1 || pi[0].(map[string]any)["title"] != "WEB-12 New layout" {
		t.Fatalf("used in: %v", src)
	}
	if !strings.Contains(n["open_url"].(string), "http://notes.test/new?") || !strings.Contains(n["open_url"].(string), "image=1") {
		t.Fatalf("new note address: %v", n["open_url"])
	}
	for _, q := range asked {
		if !strings.Contains(q, "url="+url.QueryEscape(page)) {
			t.Fatalf("asked for the wrong address: %s", q)
		}
	}
	// not connected: says why, nothing breaks
	a.ok(a.bob, "PUT", "/api/v1/settings", map[string]any{"notes_mode": "off"})
	bid := a.create(a.bob, "mermaid", "B", "graph TD\n A-->B\n")
	b := a.ok(a.bob, "GET", fmt.Sprintf("/api/v1/drawings/%d/used", bid), nil)
	if bs := b["sources"].([]any)[0].(map[string]any); bs["connected"] != false || bs["detail"] == "" {
		t.Fatalf("switched off: %v", bs)
	}
	// the picture under the page, for a signed-in browser or a token; nobody else's
	res, body := a.do(a.alice, "GET", fmt.Sprintf("/d/%d/picture.svg", id), "", nil)
	if res.StatusCode != 200 || !strings.HasPrefix(string(body), "<svg") || !strings.Contains(res.Header.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatalf("picture: %d %s", res.StatusCode, body)
	}
	if res, _ := a.do(a.bob, "GET", fmt.Sprintf("/d/%d/picture.svg", id), "", nil); res.StatusCode != 404 {
		t.Fatalf("someone else's picture: %d", res.StatusCode)
	}
	if res, _ := a.do("", "GET", fmt.Sprintf("/d/%d/picture.svg", id), "", nil); res.StatusCode != 401 {
		t.Fatalf("picture without signing in: %d", res.StatusCode)
	}
}

func TestPictureSize(t *testing.T) {
	in := `<svg id="m" width="100%" xmlns="http://www.w3.org/2000/svg" style="max-width: 386.5px;" viewBox="-8 -8 386.5 1150.4"><g/></svg>`
	out := pictureSize(in)
	if !strings.HasPrefix(out, `<svg width="387" height="1151" id="m"`) || strings.Contains(out, `100%`) {
		t.Fatalf("sized: %s", out)
	}
	sized := `<svg width="200" height="100" viewBox="0 0 400 200"></svg>`
	if pictureSize(sized) != sized {
		t.Fatalf("a picture with its own size changed")
	}
}

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-draw/internal/config"
)

// moveLinks starts Draw behind a fake Core whose pcx_<who>_<export|import|read>
// tokens are Pocket Move links acting for alice@ or bob@.
func moveLinks(t *testing.T) *app {
	t.Helper()
	coreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/auth/introspect" {
			io.WriteString(w, `{}`)
			return
		}
		var in struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		parts := strings.SplitN(strings.TrimPrefix(in.Token, "pcx_"), "_", 2)
		if len(parts) != 2 {
			io.WriteString(w, `{"active":false}`)
			return
		}
		scope := map[string]string{"export": core.ScopeMoveExport, "import": core.ScopeMoveImport, "read": "drawings:read"}[parts[1]]
		email := parts[0] + "@example.com"
		fmt.Fprintf(w, `{"active":true,"kind":"delegated","scope":%q,"audience":"draw","user":{"id":1,"sub":%q,"email":%q,"name":%q,"role":"member"},"app":{"id":7,"slug":"move","name":"Pocket Move"}}`, scope, email, email, parts[0])
	}))
	t.Cleanup(coreSrv.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL = coreSrv.URL })
}

func (a *app) raw(tok, method, path, ctype string, body []byte) (int, []byte) {
	a.t.Helper()
	res, b := a.do(tok, method, path, ctype, bytes.NewReader(body))
	return res.StatusCode, b
}

func (a *app) moveImport(tok, name string, data []byte) map[string]any {
	a.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	code, out := a.raw(tok, "POST", "/api/v1/import", mw.FormDataContentType(), buf.Bytes())
	if code != 200 {
		a.t.Fatalf("import %s → %d %s", name, code, out)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	return m
}

func TestMoveLinkScopes(t *testing.T) {
	a := moveLinks(t)
	exp, imp, read := "pcx_bob_export", "pcx_bob_import", "pcx_bob_read"
	for _, c := range []struct {
		tok, method, path string
		want              int
	}{
		{exp, "GET", core.PortablePath, 200},
		{exp, "GET", "/api/v1/export.json", 200},
		{exp, "GET", "/api/v1/export.csv", 200},
		{exp, "POST", "/api/v1/import", 403},
		{exp, "GET", "/api/v1/settings", 403},
		{exp, "GET", "/api/v1/backup", 403},
		{exp, "GET", "/api/v1/drawings", 403},
		{imp, "GET", core.PortablePath, 200},
		{imp, "GET", "/api/v1/export.json", 403},
		{imp, "PUT", "/api/v1/settings", 403},
		{imp, "POST", "/api/v1/drawings", 403},
		{imp, "PUT", "/api/v1/library", 403},
		{read, "GET", core.PortablePath, 403},
		{read, "GET", "/api/v1/export.json", 403},
		{read, "POST", "/api/v1/import", 403},
	} {
		if code, body := a.raw(c.tok, c.method, c.path, "application/json", []byte("{}")); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.tok, c.method, c.path, code, c.want, body)
		}
	}
	_, body := a.raw(exp, "GET", core.PortablePath, "", nil)
	var p core.Portable
	if err := json.Unmarshal(body, &p); err != nil || len(p.Exports) != 2 || len(p.Imports) != 4 || p.Exports[0].Kind != core.KindPocket || p.Imports[0].Form != "file" {
		t.Fatalf("portable: %s", body)
	}
}

// Move brings Alice's drawings to Bob: each import adds once, also for a
// file from another app, and nothing of one person shows up for the other.
func TestMoveImportsAddOnce(t *testing.T) {
	a := moveLinks(t)
	a.create(a.alice, "mermaid", "Garden plan", "graph TD\n Shed-->Beds\n")

	code, export := a.raw("pcx_alice_export", "GET", "/api/v1/export.json", "", nil)
	if code != 200 || !strings.Contains(string(export), "Garden plan") {
		t.Fatalf("alice's export: %d %s", code, export)
	}
	if _, bob := a.raw("pcx_bob_export", "GET", "/api/v1/export.json", "", nil); strings.Contains(string(bob), "Garden plan") {
		t.Fatalf("bob's export holds alice's drawing: %s", bob)
	}

	first := a.moveImport("pcx_bob_import", "draw.json", export)
	if first["kind"] != "pocket-draw" || num(first, "imported") != 1 {
		t.Fatalf("first import: %v", first)
	}
	if second := a.moveImport("pcx_bob_import", "draw.json", export); num(second, "imported") != 0 || num(second, "skipped") != 1 {
		t.Fatalf("second import: %v", second)
	}
	drawio := []byte(`<mxfile><diagram name="Page-1"><mxGraphModel><root><mxCell id="0"/><mxCell id="2" value="Porch" vertex="1"/></root></mxGraphModel></diagram></mxfile>`)
	if d1 := a.moveImport("pcx_bob_import", "house.drawio", drawio); num(d1, "imported") != 1 {
		t.Fatalf("drawio: %v", d1)
	}
	if d2 := a.moveImport("pcx_bob_import", "house.drawio", drawio); num(d2, "imported") != 0 || num(d2, "skipped") != 1 {
		t.Fatalf("drawio again: %v", d2)
	}

	if n := len(list(a.ok(a.bob, "GET", "/api/v1/drawings", nil))); n != 2 {
		t.Fatalf("bob has %d drawings, want 2", n)
	}
	if n := len(list(a.ok(a.alice, "GET", "/api/v1/drawings", nil))); n != 1 {
		t.Fatalf("alice has %d drawings, want 1", n)
	}
}

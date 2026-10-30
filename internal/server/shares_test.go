package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestSharing(t *testing.T) {
	a := newApp(t, nil)
	id := a.create(a.alice, "sketch", "Kitchen", scene)
	p := fmt.Sprintf("/api/v1/drawings/%d", id)
	bob := a.ids["Bob"]

	// not shared yet: Bob sees nothing, and only the owner shares
	a.expect(404, a.bob, "GET", p, nil)
	a.expect(404, a.bob, "PUT", fmt.Sprintf("%s/shares/%d", p, a.ids["Alice"]), map[string]any{"role": "edit"})
	a.expect(400, a.alice, "PUT", fmt.Sprintf("%s/shares/%d", p, bob), map[string]any{"role": "admin"})

	_, _, people := a.call(a.alice, "GET", "/api/v1/people", nil)
	if len(people) != 1 || people[0].(map[string]any)["name"] != "Bob" {
		t.Fatalf("people = %v", people)
	}

	// shared to view: Bob opens it, its picture and file, but changes nothing
	a.ok(a.alice, "PUT", fmt.Sprintf("%s/shares/%d", p, bob), map[string]any{"role": "view"})
	d := a.ok(a.bob, "GET", p, nil)
	if d["access"] != "view" || d["owner_name"] != "Alice" || d["data"] == "" {
		t.Fatalf("shared drawing = %v", d)
	}
	if res, _ := a.do(a.bob, "GET", p+"/preview.svg", "", nil); res.StatusCode != 200 {
		t.Fatalf("preview → %d", res.StatusCode)
	}
	a.expect(403, a.bob, "PUT", p, map[string]any{"data": scene, "rev": num(d, "rev")})
	if l := list(a.ok(a.bob, "GET", "/api/v1/drawings?view=shared", nil)); len(l) != 1 || l[0]["access"] != "view" || l[0]["owner_name"] != "Alice" {
		t.Fatalf("shared list = %v", l)
	}
	if l := list(a.ok(a.bob, "GET", "/api/v1/drawings", nil)); len(l) != 0 {
		t.Fatalf("Bob's own list has %d", len(l))
	}
	if c := a.ok(a.bob, "GET", "/api/v1/dashboard", nil)["counts"].(map[string]any); num(c, "shared") != 1 {
		t.Fatalf("counts = %v", c)
	}

	// to edit: the content saves into Alice's drawing; the rest stays hers
	a.ok(a.alice, "PUT", fmt.Sprintf("%s/shares/%d", p, bob), map[string]any{"role": "edit"})
	changed := strings.Replace(scene, "Kitchen sink", "Pantry", 1)
	a.ok(a.bob, "PUT", p, map[string]any{"data": changed, "rev": num(d, "rev"), "preview": preview})
	if got := a.ok(a.alice, "GET", p, nil); !strings.Contains(got["data"].(string), "Pantry") || num(got, "shares") != 1 {
		t.Fatalf("Alice's drawing = %v", got)
	}
	a.expect(403, a.bob, "PUT", p, map[string]any{"title": "Mine now"})
	a.expect(404, a.bob, "DELETE", p, nil)
	a.expect(404, a.bob, "GET", p+"/shares", nil)

	// in the trash it is Alice's alone; stopping sharing ends it
	a.expect(204, a.alice, "DELETE", p, nil)
	a.expect(404, a.bob, "GET", p, nil)
	a.expect(204, a.alice, "POST", p+"/restore", nil)
	a.ok(a.bob, "GET", p, nil)
	a.expect(204, a.bob, "DELETE", fmt.Sprintf("%s/shares/%d", p, bob), nil) // Bob leaves it
	a.expect(404, a.bob, "GET", p, nil)
}

type liveConn struct {
	t *testing.T
	c *websocket.Conn
}

func (a *app) live(tok string, id int64) *liveConn {
	a.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, strings.Replace(a.url, "http", "ws", 1)+fmt.Sprintf("/live/%d", id),
		&websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + tok}}})
	if err != nil {
		a.t.Fatal(err)
	}
	a.t.Cleanup(func() { _ = c.CloseNow() })
	return &liveConn{a.t, c}
}

// next reads messages until one of type t arrives.
func (l *liveConn) next(t string) map[string]any {
	l.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, b, err := l.c.Read(ctx)
		if err != nil {
			l.t.Fatalf("waiting for %q: %v", t, err)
		}
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		if m["t"] == t {
			return m
		}
	}
}

func (l *liveConn) send(m map[string]any) {
	b, _ := json.Marshal(m)
	if err := l.c.Write(context.Background(), websocket.MessageText, b); err != nil {
		l.t.Fatal(err)
	}
}

func TestLive(t *testing.T) {
	a := newApp(t, nil)
	id := a.create(a.alice, "sketch", "Kitchen", scene)
	p := fmt.Sprintf("/api/v1/drawings/%d", id)
	bob := a.ids["Bob"]

	// only those it is shared with get in
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, res, err := websocket.Dial(ctx, strings.Replace(a.url, "http", "ws", 1)+fmt.Sprintf("/live/%d", id),
		&websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + a.bob}}}); err == nil || res.StatusCode != 404 {
		t.Fatalf("not shared: err %v", err)
	}

	al := a.live(a.alice, id)
	hello := al.next("hello")
	me := hello["you"].(map[string]any)
	if hello["saver"] != me["id"] || me["role"] != "owner" {
		t.Fatalf("hello = %v", hello)
	}

	a.ok(a.alice, "PUT", fmt.Sprintf("%s/shares/%d", p, bob), map[string]any{"role": "view"})
	bv := a.live(a.bob, id)
	if h := bv.next("hello"); len(h["peers"].([]any)) != 1 || h["saver"] != me["id"] {
		t.Fatalf("Bob's hello = %v", h)
	}
	if j := al.next("join"); j["peer"].(map[string]any)["name"] != "Bob" {
		t.Fatalf("join = %v", j)
	}
	if s := al.next("sync"); s["to"] == "" {
		t.Fatalf("sync = %v", s)
	}

	// pointers go both ways; a viewer's changes go nowhere
	bv.send(map[string]any{"t": "scene", "elements": []any{map[string]any{"id": "x"}}})
	bv.send(map[string]any{"t": "pointer", "pointer": map[string]any{"x": 1, "y": 2}})
	if m := al.next("pointer"); m["from"] == "" {
		t.Fatalf("pointer = %v", m)
	}
	al.send(map[string]any{"t": "scene", "elements": []any{map[string]any{"id": "y"}}})
	if m := bv.next("scene"); m["elements"].([]any)[0].(map[string]any)["id"] != "y" {
		t.Fatalf("scene = %v", m)
	}

	// a save is announced
	d := a.ok(a.alice, "GET", p, nil)
	a.ok(a.alice, "PUT", p, map[string]any{"data": strings.Replace(scene, "Kitchen sink", "Sink", 1), "rev": num(d, "rev")})
	if m := bv.next("saved"); num(m, "rev") != num(d, "rev")+1 {
		t.Fatalf("saved = %v", m)
	}

	// another role: Bob is let go and comes back as an editor
	a.ok(a.alice, "PUT", fmt.Sprintf("%s/shares/%d", p, bob), map[string]any{"role": "edit"})
	if m := bv.next("bye"); m["reason"] != "role" {
		t.Fatalf("bye = %v", m)
	}
	be := a.live(a.bob, id)
	be.next("hello")
	be.send(map[string]any{"t": "scene", "elements": []any{map[string]any{"id": "z"}}})
	if m := al.next("scene"); m["elements"].([]any)[0].(map[string]any)["id"] != "z" {
		t.Fatalf("editor's scene = %v", m)
	}

	// Alice goes: Bob saves from now on
	_ = al.c.Close(websocket.StatusNormalClosure, "")
	if m := be.next("saver"); m["id"] == me["id"] || m["id"] == "" {
		t.Fatalf("saver = %v", m)
	}
}

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/abjelosevic88/pocket-homelab-draw/internal/store"
)

// Live sessions: everyone who has a shared sketch open is in its room, sees
// the others' pointers and selections, and gets their changes as they make
// them. The server only relays; the browsers merge (Excalidraw's element
// versions decide). One person in the room — the saver, the owner when they
// are there, else the editor who came first — saves the merged drawing as
// usual; every save is announced so the others follow its rev.
//
// Messages are JSON objects with a type "t". From a browser:
//   pointer {pointer, button, selected}  relayed to the room
//   scene   {elements, files, full}       relayed (editors only); "to" sends to one peer
// From the server:
//   hello {you, peers, saver}  join {peer}  leave {id}  saver {id}
//   sync  {to}     the saver is asked to send everything to a newcomer
//   saved {rev}    the drawing was saved at this rev
//   reload         the drawing was put back to an earlier version: open it again
//   bye   {reason} access changed or ended; the connection closes

const (
	liveQueue    = 256
	liveWriteMax = 10 * time.Second
	livePing     = 25 * time.Second
)

// livePeer is one open window on a drawing.
type livePeer struct {
	ID     string `json:"id"`
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Role   string `json:"role"` // owner | edit | view

	out  chan []byte
	done chan struct{}
	once sync.Once
	bye  atomic.Value // string: why the server closes it
}

func (p *livePeer) close(reason string) {
	p.once.Do(func() { p.bye.Store(reason); close(p.done) })
}

// send queues a message; a peer that cannot keep up loses pointer moves, and
// is let go when it falls behind on changes (it reconnects and syncs again).
func (p *livePeer) send(b []byte, droppable bool) {
	select {
	case p.out <- b:
	default:
		if !droppable {
			p.close("slow")
		}
	}
}

type liveRoom struct {
	peers []*livePeer // in the order they came
	saver *livePeer
}

type liveHub struct {
	mu    sync.Mutex
	rooms map[int64]*liveRoom
	seq   atomic.Int64
}

func newLiveHub() *liveHub { return &liveHub{rooms: map[int64]*liveRoom{}} }

func liveMsg(v map[string]any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// elect keeps the saver while they are there; else the owner, else the
// first editor. Viewers never save.
func (r *liveRoom) elect() bool {
	for _, p := range r.peers {
		if p == r.saver {
			return false
		}
	}
	prev := r.saver
	r.saver = nil
	for _, want := range []string{"owner", store.ShareEdit} {
		for _, p := range r.peers {
			if p.Role == want {
				r.saver = p
				break
			}
		}
		if r.saver != nil {
			break
		}
	}
	return r.saver != prev
}

func (r *liveRoom) saverID() string {
	if r.saver == nil {
		return ""
	}
	return r.saver.ID
}

func (r *liveRoom) broadcast(b []byte, except *livePeer) {
	for _, p := range r.peers {
		if p != except {
			p.send(b, false)
		}
	}
}

func (h *liveHub) join(id int64, p *livePeer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.rooms[id]
	if r == nil {
		r = &liveRoom{}
		h.rooms[id] = r
	}
	others := append([]*livePeer{}, r.peers...)
	r.peers = append(r.peers, p)
	changed := r.elect()
	p.send(liveMsg(map[string]any{"t": "hello", "you": p, "peers": others, "saver": r.saverID()}), false)
	r.broadcast(liveMsg(map[string]any{"t": "join", "peer": p}), p)
	if changed {
		r.broadcast(liveMsg(map[string]any{"t": "saver", "id": r.saverID()}), p)
	}
	if r.saver != nil && r.saver != p {
		r.saver.send(liveMsg(map[string]any{"t": "sync", "to": p.ID}), false)
	}
}

func (h *liveHub) leave(id int64, p *livePeer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.rooms[id]
	if r == nil {
		return
	}
	for i, x := range r.peers {
		if x == p {
			r.peers = append(r.peers[:i], r.peers[i+1:]...)
			break
		}
	}
	if len(r.peers) == 0 {
		delete(h.rooms, id)
		return
	}
	r.broadcast(liveMsg(map[string]any{"t": "leave", "id": p.ID}), nil)
	if r.elect() {
		r.broadcast(liveMsg(map[string]any{"t": "saver", "id": r.saverID()}), nil)
	}
}

// relay passes a browser's message on to the room (or to one peer).
func (h *liveHub) relay(id int64, from *livePeer, b []byte) {
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return
	}
	var t, to string
	_ = json.Unmarshal(m["t"], &t)
	switch t {
	case "pointer":
	case "scene":
		if from.Role == store.ShareView {
			return
		}
	default:
		return
	}
	if raw, ok := m["to"]; ok {
		_ = json.Unmarshal(raw, &to)
		delete(m, "to")
	}
	m["from"], _ = json.Marshal(from.ID)
	out, err := json.Marshal(m)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.rooms[id]
	if r == nil {
		return
	}
	for _, p := range r.peers {
		if p == from || (to != "" && p.ID != to) {
			continue
		}
		p.send(out, t == "pointer")
	}
}

// tell sends a message to everyone with the drawing open.
func (h *liveHub) tell(id int64, v map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r := h.rooms[id]; r != nil {
		r.broadcast(liveMsg(v), nil)
	}
}

// drop ends the sessions of one person on a drawing (uid 0: everyone but
// the owner) — sharing stopped, their role changed, the drawing was trashed.
func (h *liveHub) drop(id, uid int64, reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.rooms[id]
	if r == nil {
		return
	}
	for _, p := range r.peers {
		if (uid != 0 && p.UserID == uid) || (uid == 0 && p.Role != "owner") {
			p.close(reason)
		}
	}
}

// present lists who has a drawing open (for the share dialog).
func (h *liveHub) present(id int64) map[int64]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[int64]bool{}
	if r := h.rooms[id]; r != nil {
		for _, p := range r.peers {
			out[p.UserID] = true
		}
	}
	return out
}

// handleLive is the WebSocket for one sketch.
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	if linkApp(ctx) != "" || u == nil {
		writeErr(w, http.StatusForbidden, "live sessions are for people, not apps")
		return
	}
	id := idParam(r, "id")
	owner, role, err := s.store.Access(ctx, u.ID, id)
	if err != nil {
		s.fail(w, err, "drawing")
		return
	}
	d, err := s.store.GetDrawing(ctx, owner, id)
	if err != nil {
		s.fail(w, err, "drawing")
		return
	}
	if d.Kind != store.KindSketch {
		writeErr(w, http.StatusBadRequest, "live sessions are for sketches")
		return
	}
	c, err := websocket.Accept(w, r, nil) // same origin only
	if err != nil {
		return
	}
	c.SetReadLimit(s.cfg.MaxBodyBytes)
	p := &livePeer{ID: strconv.FormatInt(s.live.seq.Add(1), 36), UserID: u.ID, Name: store.DisplayName(u.Name, u.Email), Role: role,
		out: make(chan []byte, liveQueue), done: make(chan struct{})}
	s.live.join(id, p)
	defer s.live.leave(id, p)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		defer cancel()
		tick := time.NewTicker(livePing)
		defer tick.Stop()
		for {
			select {
			case b := <-p.out:
				wctx, stop := context.WithTimeout(ctx, liveWriteMax)
				err := c.Write(wctx, websocket.MessageText, b)
				stop()
				if err != nil {
					return
				}
			case <-tick.C:
				pctx, stop := context.WithTimeout(ctx, liveWriteMax)
				err := c.Ping(pctx)
				stop()
				if err != nil {
					return
				}
			case <-p.done:
				reason, _ := p.bye.Load().(string)
				wctx, stop := context.WithTimeout(ctx, liveWriteMax)
				_ = c.Write(wctx, websocket.MessageText, liveMsg(map[string]any{"t": "bye", "reason": reason}))
				stop()
				_ = c.Close(websocket.StatusNormalClosure, reason)
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	for {
		typ, b, err := c.Read(ctx)
		if err != nil {
			var ce websocket.CloseError
			if !errors.As(err, &ce) && ctx.Err() == nil {
				s.log.Debug("live read", "err", err)
			}
			break
		}
		if typ == websocket.MessageText {
			s.live.relay(id, p, b)
		}
	}
	cancel()
	_ = c.CloseNow()
}

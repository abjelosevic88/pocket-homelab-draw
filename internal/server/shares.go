package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/abjelosevic88/pocket-homelab-draw/internal/store"
)

// access is how the person asking may use a drawing, and whose it is: their
// own ("owner"), or shared with them ("edit" | "view"). Apps linked through
// Pocket Core reach only the person's own drawings.
func (s *Server) access(ctx context.Context, id int64) (owner int64, role string, err error) {
	uid := actorID(ctx)
	if linkApp(ctx) != "" {
		if _, err := s.store.GetDrawing(ctx, uid, id); err != nil {
			return 0, "", err
		}
		return uid, "owner", nil
	}
	return s.store.Access(ctx, uid, id)
}

// handleListShares is who a drawing is shared with (its owner asks).
func (s *Server) handleListShares(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := idParam(r, "id")
	if _, err := s.store.GetDrawing(ctx, actorID(ctx), id); err != nil {
		s.fail(w, err, "drawing")
		return
	}
	list, err := s.store.ListShares(ctx, id)
	if err != nil {
		s.fail(w, err, "shares")
		return
	}
	here := s.live.present(id)
	out := make([]map[string]any, 0, len(list))
	for _, sh := range list {
		out = append(out, map[string]any{"user_id": sh.UserID, "name": sh.Name, "email": sh.Email, "role": sh.Role, "created_at": sh.CreatedAt, "here": here[sh.UserID]})
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePutShare shares a drawing with someone on this box, or changes
// what they may do: {"role": "edit" | "view"}.
func (s *Server) handlePutShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	if linkApp(ctx) != "" {
		writeErr(w, http.StatusForbidden, "only the owner shares a drawing")
		return
	}
	id, to := idParam(r, "id"), idParam(r, "uid")
	d, err := s.store.GetDrawing(ctx, uid, id)
	if err != nil {
		s.fail(w, err, "drawing")
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	if err := decode(r, &in); err != nil || (in.Role != store.ShareEdit && in.Role != store.ShareView) {
		writeErr(w, http.StatusBadRequest, `role is "edit" or "view"`)
		return
	}
	if to == uid {
		writeErr(w, http.StatusBadRequest, "it is already yours")
		return
	}
	who, err := s.store.GetUser(ctx, to)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "no such person here")
		return
	}
	_, before, _ := s.store.Access(ctx, to, id)
	if err := s.store.SetShare(ctx, id, to, in.Role); err != nil {
		s.fail(w, err, "share")
		return
	}
	if before != "" && before != in.Role {
		s.live.drop(id, to, "role") // they open it again with the new role
	}
	if before == "" {
		me := userFrom(ctx)
		s.publish(ctx, to, "draw.shared", fmt.Sprintf("drawing:%d", id), map[string]any{"id": id, "title": d.Title, "kind": d.Kind, "role": in.Role,
			"by": store.DisplayName(me.Name, me.Email), "url": drawingURL(s.cfg.BaseURL, id)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": who.ID, "name": who.Name, "email": who.Email, "role": in.Role})
}

// handleDeleteShare stops sharing with someone. The owner removes anyone;
// a person may also leave a drawing shared with them.
func (s *Server) handleDeleteShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id, to := idParam(r, "id"), idParam(r, "uid")
	if to != uid {
		if _, err := s.store.GetDrawing(ctx, uid, id); err != nil || linkApp(ctx) != "" {
			s.fail(w, store.ErrNotFound, "drawing")
			return
		}
	}
	if err := s.store.RemoveShare(ctx, id, to); err != nil {
		s.fail(w, err, "share")
		return
	}
	s.live.drop(id, to, "unshared")
	w.WriteHeader(http.StatusNoContent)
}

// handlePeople is everyone on this box one may share with.
func (s *Server) handlePeople(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if linkApp(ctx) != "" {
		writeErr(w, http.StatusForbidden, "not for apps")
		return
	}
	list, err := s.store.People(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "people")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

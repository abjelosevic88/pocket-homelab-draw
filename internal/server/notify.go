package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-draw/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-draw/internal/store"
)

// appName is a sibling's name for messages ("Pocket AI" for "ai").
func appName(slug string) string {
	if a, ok := appBySlug(slug); ok {
		return a.name
	}
	switch slug {
	case "ai":
		return "Pocket AI"
	case "":
		return "another app"
	}
	return "Pocket " + strings.ToUpper(slug[:1]) + slug[1:]
}

// notifyLinkAdd tells a person that another app added a drawing for them.
func (s *Server) notifyLinkAdd(uid int64, d *store.Drawing, app string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := s.store.GetSettings(ctx, uid)
	if err != nil || st.NotifyURL == "" || !st.NotifyLinkAdds {
		return
	}
	msg := notify.Message{Title: appName(app) + " added a drawing", Body: fmt.Sprintf("%s (%s)", d.Title, kindNames[d.Kind]), Priority: "info",
		Tags: []string{"art"}, Link: drawingURL(s.cfg.BaseURL, d.ID)}
	if err := notify.New(st.NotifyURL, st.NotifyFormat).Send(ctx, msg); err != nil {
		s.log.Debug("notify failed", "err", err)
	}
}

// notifyFilesError tells a person once a day that copies to Pocket Files fail.
func (s *Server) notifyFilesError(ctx context.Context, uid int64, st store.Settings, why string) {
	if st.NotifyURL == "" || !st.NotifyFiles {
		return
	}
	key := fmt.Sprintf("push:%d:%s:files", uid, store.Today())
	if _, _, done := s.store.CacheGet(ctx, key); done {
		return
	}
	_ = s.store.CachePut(ctx, key, "1")
	msg := notify.Message{Title: "Drawings are not reaching Pocket Files", Body: why + "\nThe copies will catch up once it works again.", Priority: "warning",
		Tags: []string{"warning"}, Link: s.cfg.BaseURL + "/settings/apps"}
	if err := notify.New(st.NotifyURL, st.NotifyFormat).Send(ctx, msg); err != nil {
		s.log.Debug("notify failed", "err", err)
	}
}

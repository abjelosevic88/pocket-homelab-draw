package server

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-draw/internal/config"
)

// ---- The draw.io that comes with Pocket Draw (DRAWIO_INTERNAL_URL) ----
//
// The browser loads draw.io itself (the Diagram editor is an iframe), so a
// draw.io container with no port of its own is served here, at /drawio/, on
// Pocket Draw's own address: nothing to publish, no second hostname, and the
// editor talks to the page from the same origin. draw.io is static files, so
// the path is open like the app's own assets; nothing of the person's (cookies,
// tokens) is passed on.

func (s *Server) drawioProxy() http.Handler {
	if s.cfg.DrawioInternalURL == "" {
		return nil
	}
	target, err := url.Parse(s.cfg.DrawioInternalURL)
	if err != nil || target.Host == "" {
		s.log.Warn("DRAWIO_INTERNAL_URL is not an address; draw.io is not served", "value", s.cfg.DrawioInternalURL)
		return nil
	}
	p := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.URL.Path = strings.TrimSuffix(target.Path, "/") + "/" + strings.TrimPrefix(strings.TrimPrefix(r.In.URL.Path, config.BundledDrawioPath), "/")
			r.Out.URL.RawPath = ""
			r.Out.Host = target.Host
			for _, h := range []string{"Cookie", "Authorization", "X-Api-Key"} {
				r.Out.Header.Del(h)
			}
		},
		ModifyResponse: func(res *http.Response) error {
			res.Header.Del("Set-Cookie")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			s.log.Warn("draw.io not reachable", "url", s.cfg.DrawioInternalURL, "err", err)
			http.Error(w, "draw.io is not reachable (is the drawio container running?)", http.StatusBadGateway)
		},
	}
	return p
}

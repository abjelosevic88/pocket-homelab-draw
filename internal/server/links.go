package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-draw/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Draw as the
// target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "drawings:read", Name: "See your drawings", Description: "List your sketches and diagrams, open them and show their pictures (a drawing inside a note or an issue), and see what you drew on a day — for your journal, notes and dashboards."},
	{Scope: "drawings:write", Name: "Add drawings", Description: "Add a drawing for you (a diagram written by Pocket AI, a flowchart from a project) and change or remove only the drawings it added itself."},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(dashboard|drawings|folders)$`), "drawings:read"},
	{"GET", regexp.MustCompile(`^/api/v1/drawings/\d+(/(preview\.svg|file))?$`), "drawings:read"},
	{"GET", regexp.MustCompile(`^/api/v1/day/\d{4}-\d{2}-\d{2}$`), "drawings:read"},
	{"GET", regexp.MustCompile(`^/d/\d+/picture\.svg$`), "drawings:read"},
	{"POST", regexp.MustCompile(`^/api/v1/drawings$`), "drawings:write"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/drawings/\d+$`), "drawings:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "draw" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}

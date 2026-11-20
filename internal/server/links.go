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

// portable is what Pocket Move may copy out and bring in for a person
// (docs/move.md in pocket-homelab-core); Register adds move:export/import.
// Every import goes through POST /import, which works out the file itself.
var portable = &core.Portable{
	Exports: []core.PortableExport{
		{Path: "/api/v1/export.json", File: "draw.json", Kind: core.KindPocket, Title: "Every sketch and diagram with its content and picture, your folders and your sketch library"},
		{Path: "/api/v1/export.csv", File: "drawings.csv", Kind: core.KindCSV, Title: "Your drawings as a table (titles, kinds, folders, dates)"},
	},
	Imports: []core.PortableImport{
		{Path: "/api/v1/import", Kind: core.KindPocket, Form: "file", Title: "Adds the drawings, folders and library from another box; drawings you already have are skipped"},
		{Path: "/api/v1/import", Kind: "excalidraw", Form: "file", Title: "Adds an Excalidraw drawing (.excalidraw) as a sketch"},
		{Path: "/api/v1/import", Kind: "drawio", Form: "file", Title: "Adds a draw.io diagram (.drawio)"},
		{Path: "/api/v1/import", Kind: "mermaid", Form: "file", Title: "Adds a Mermaid diagram (.mmd, or Markdown with mermaid blocks)"},
	},
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
	if portable.LinkAllows(scopes, r) {
		return true
	}
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

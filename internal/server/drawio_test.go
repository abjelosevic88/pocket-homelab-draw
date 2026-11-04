package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-draw/internal/config"
)

// The draw.io that comes with Pocket Draw is served at /drawio/, without the
// person's cookies, and is the default draw.io address.
func TestBundledDrawio(t *testing.T) {
	var gotPath, gotCookie string
	dio := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotCookie = r.URL.Path, r.Header.Get("Cookie")
		w.Header().Set("Set-Cookie", "x=1")
		_, _ = io.WriteString(w, "drawio "+r.URL.RawQuery)
	}))
	defer dio.Close()
	a := newApp(t, func(c *config.Config) {
		c.DrawioInternalURL = dio.URL
		c.DrawioURL = config.BundledDrawioPath
	})
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Get(a.url + "/drawio?embed=1")
	if err != nil || res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != "/drawio/?embed=1" {
		t.Fatalf("redirect: %v %v %q", err, res.StatusCode, res.Header.Get("Location"))
	}
	req, _ := http.NewRequest("GET", a.url+"/drawio/js/app.min.js?v=2", nil)
	req.Header.Set("Cookie", "pdw_session=secret")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || string(body) != "drawio v=2" || gotPath != "/js/app.min.js" {
		t.Fatalf("proxy: %d %q path %q", res.StatusCode, body, gotPath)
	}
	if gotCookie != "" || res.Header.Get("Set-Cookie") != "" {
		t.Fatalf("cookies passed: in %q out %q", gotCookie, res.Header.Get("Set-Cookie"))
	}
	if s := a.ok(a.alice, "GET", "/api/v1/settings", nil); s["drawio_url"] != "/drawio" || s["drawio_included"] != true {
		t.Fatalf("settings: %v %v", s["drawio_url"], s["drawio_included"])
	}
	a.expect(200, a.alice, "PUT", "/api/v1/settings", map[string]any{"drawio_url": "/drawio"})
}

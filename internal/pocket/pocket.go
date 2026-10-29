// Package pocket talks to sibling Pocket apps: Pocket Files (a copy of each
// drawing, picture and file, in a folder). Same bearer-token API everywhere.
package pocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a bearer-token JSON client for one Pocket app.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New creates a client.
func New(baseURL, token string, timeout time.Duration) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: timeout}}
}

// Configured reports whether URL and token are set.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" && c.Token != "" }

// ErrNotFound is a 404 from the sibling app.
var ErrNotFound = errors.New("not found")

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any, v any) error {
	if !c.Configured() {
		return errors.New("not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errors.New("token rejected (401)")
	}
	if res.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2000))
		_ = json.Unmarshal(b, &e)
		if e.Error != "" {
			return fmt.Errorf("HTTP %d: %s", res.StatusCode, e.Error)
		}
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if v == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Get calls a GET endpoint and decodes the JSON answer.
func (c *Client) Get(ctx context.Context, path string, q url.Values, v any) error {
	return c.do(ctx, http.MethodGet, path, q, nil, v)
}

// Health is the connection test for any Pocket app: /healthz plus a token check.
func (c *Client) Health(ctx context.Context) (string, error) {
	var out struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if !c.Configured() {
		return "", errors.New("not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/healthz", nil)
	if err != nil {
		return "", err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	var me map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v1/auth/me", nil, nil, &me); err != nil {
		return out.Version, err
	}
	return out.Version, nil
}

// ---------------- Pocket Files ----------------

// FileEntry is what Pocket Files answers for an upload.
type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Skipped bool   `json:"skipped"`
}

// Upload writes one file into the person's own files ("home"), creating the
// folders in name; an existing file is overwritten.
func (c *Client) Upload(ctx context.Context, name string, body []byte, mime string, mtime time.Time) (*FileEntry, error) {
	if !c.Configured() {
		return nil, errors.New("not configured")
	}
	q := url.Values{"space": {"home"}, "path": {""}, "name": {name}, "conflict": {"overwrite"}}
	if !mtime.IsZero() {
		q.Set("mtime", fmt.Sprint(mtime.UnixMilli()))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.BaseURL+"/api/v1/fs/upload?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", firstNonEmpty(mime, "application/octet-stream"))
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("token rejected (401)")
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2000))
		_ = json.Unmarshal(b, &e)
		if e.Error != "" {
			return nil, fmt.Errorf("HTTP %d: %s", res.StatusCode, e.Error)
		}
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	var out FileEntry
	_ = json.NewDecoder(res.Body).Decode(&out)
	return &out, nil
}

// Delete moves files in the person's own files to their trash there.
func (c *Client) Delete(ctx context.Context, paths ...string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/fs/delete", nil, map[string]any{"space": "home", "paths": paths}, nil)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

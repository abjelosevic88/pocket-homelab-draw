package store

import (
	"bytes"
	"compress/flate"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"
)

// NewUID returns a random id for a drawing (stable across export/import).
func NewUID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// maxSearch caps the words kept for search per drawing.
const maxSearch = 20000

// SearchText is the words in a drawing: the text on a sketch, a text
// diagram's source, the labels of a draw.io diagram.
func SearchText(kind, data string) string {
	var t string
	switch kind {
	case KindSketch:
		t = SketchText(data)
	case KindMermaid:
		t = data
	case KindDrawio:
		t = DrawioText(data)
	}
	t = strings.Join(strings.Fields(t), " ")
	if len(t) > maxSearch {
		t = t[:maxSearch]
	}
	return t
}

// SketchText collects the text elements of an Excalidraw scene.
func SketchText(data string) string {
	var scene struct {
		Elements []struct {
			Type      string `json:"type"`
			Text      string `json:"text"`
			Name      string `json:"name"`
			IsDeleted bool   `json:"isDeleted"`
		} `json:"elements"`
	}
	if json.Unmarshal([]byte(data), &scene) != nil {
		return ""
	}
	var parts []string
	for _, e := range scene.Elements {
		if e.IsDeleted {
			continue
		}
		if e.Text != "" {
			parts = append(parts, e.Text)
		}
		if e.Name != "" {
			parts = append(parts, e.Name)
		}
	}
	return strings.Join(parts, " ")
}

// SketchElements counts the shapes left on a sketch.
func SketchElements(data string) int {
	var scene struct {
		Elements []struct {
			IsDeleted bool `json:"isDeleted"`
		} `json:"elements"`
	}
	if json.Unmarshal([]byte(data), &scene) != nil {
		return 0
	}
	n := 0
	for _, e := range scene.Elements {
		if !e.IsDeleted {
			n++
		}
	}
	return n
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// DrawioText collects the labels of a draw.io file, compressed pages included.
func DrawioText(data string) string {
	var parts []string
	var walk func(doc string, depth int)
	walk = func(doc string, depth int) {
		dec := xml.NewDecoder(strings.NewReader(doc))
		dec.Strict = false
		inDiagram := false
		for {
			tok, err := dec.Token()
			if err != nil {
				return
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "diagram" {
					inDiagram = true
					for _, a := range t.Attr {
						if a.Name.Local == "name" && a.Value != "" && !strings.HasPrefix(a.Value, "Page-") {
							parts = append(parts, a.Value)
						}
					}
				}
				for _, a := range t.Attr {
					if (a.Name.Local == "value" || a.Name.Local == "label" || a.Name.Local == "tooltip") && a.Value != "" {
						parts = append(parts, html.UnescapeString(tagRe.ReplaceAllString(strings.ReplaceAll(a.Value, "<br>", " "), " ")))
					}
				}
			case xml.EndElement:
				if t.Name.Local == "diagram" {
					inDiagram = false
				}
			case xml.CharData:
				if inDiagram && depth < 2 {
					if inner := InflateDrawio(strings.TrimSpace(string(t))); inner != "" {
						walk(inner, depth+1)
					}
				}
			}
		}
	}
	walk(data, 0)
	return strings.Join(parts, " ")
}

// InflateDrawio opens a compressed draw.io page (base64, raw deflate, URL-encoded).
func InflateDrawio(s string) string {
	if s == "" || strings.HasPrefix(s, "<") {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(raw)), 32<<20))
	if err != nil {
		return ""
	}
	out, err := url.QueryUnescape(string(b))
	if err != nil {
		return string(b)
	}
	return out
}

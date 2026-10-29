package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math/rand"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-draw/internal/store"
)

// ---- demo data (DEMO_DATA=true: screenshots and trying the app) ----
//
// Sketches are built from Excalidraw elements, text diagrams are Mermaid,
// and the draw.io diagrams come with a picture drawn here (sketches and
// text diagrams get theirs from the browser the first time the list shows).

// sketch builds an Excalidraw scene.
type sketch struct {
	rng *rand.Rand
	els []map[string]any
	n   int
}

var palette = map[string][2]string{ // stroke, fill
	"blue":   {"#1971c2", "#a5d8ff"},
	"green":  {"#2f9e44", "#b2f2bb"},
	"yellow": {"#f08c00", "#ffec99"},
	"red":    {"#e03131", "#ffc9c9"},
	"violet": {"#6741d9", "#d0bfff"},
	"gray":   {"#495057", "#e9ecef"},
	"pink":   {"#c2255c", "#fcc2d7"},
	"none":   {"#1e1e1e", "transparent"},
}

func (k *sketch) id() string {
	k.n++
	return fmt.Sprintf("el%03d%04x", k.n, k.rng.Intn(65536))
}

func (k *sketch) base(typ string, x, y, w, h float64, color string) map[string]any {
	c := palette[color]
	return map[string]any{"id": k.id(), "type": typ, "x": x, "y": y, "width": w, "height": h, "angle": 0,
		"strokeColor": c[0], "backgroundColor": c[1], "fillStyle": "hachure", "strokeWidth": 2, "strokeStyle": "solid",
		"roughness": 1, "opacity": 100, "groupIds": []string{}, "frameId": nil, "roundness": map[string]any{"type": 3},
		"seed": k.rng.Intn(1 << 30), "version": 1, "versionNonce": k.rng.Intn(1 << 30), "isDeleted": false,
		"boundElements": []any{}, "updated": 1, "link": nil, "locked": false}
}

func textSize(s string, size float64) (float64, float64) {
	lines := strings.Split(s, "\n")
	w := 0
	for _, l := range lines {
		w = max(w, len([]rune(l)))
	}
	return float64(w) * size * 0.55, float64(len(lines)) * size * 1.25
}

func (k *sketch) text(x, y float64, s string, size float64, color string) map[string]any {
	w, h := textSize(s, size)
	t := k.base("text", x, y, w, h, color)
	t["backgroundColor"] = "transparent"
	t["roundness"] = nil
	t["text"], t["originalText"], t["fontSize"], t["fontFamily"] = s, s, size, 5
	t["textAlign"], t["verticalAlign"], t["lineHeight"], t["containerId"], t["autoResize"] = "left", "top", 1.25, nil, true
	k.els = append(k.els, t)
	return t
}

// shape adds a rectangle, ellipse or diamond with a label inside.
func (k *sketch) shape(typ string, x, y, w, h float64, label, color string) map[string]any {
	sh := k.base(typ, x, y, w, h, color)
	k.els = append(k.els, sh)
	if label != "" {
		size := 20.0
		tw, th := textSize(label, size)
		t := k.text(x+(w-tw)/2, y+(h-th)/2, label, size, "none")
		t["textAlign"], t["verticalAlign"], t["containerId"] = "center", "middle", sh["id"]
		sh["boundElements"] = []any{map[string]any{"type": "text", "id": t["id"]}}
	}
	return sh
}

func (k *sketch) arrow(x1, y1, x2, y2 float64, color string) {
	a := k.base("arrow", x1, y1, x2-x1, y2-y1, color)
	a["backgroundColor"] = "transparent"
	a["roundness"] = map[string]any{"type": 2}
	a["points"] = [][]float64{{0, 0}, {x2 - x1, y2 - y1}}
	a["startArrowhead"], a["endArrowhead"] = nil, "arrow"
	a["startBinding"], a["endBinding"], a["elbowed"] = nil, nil, false
	k.els = append(k.els, a)
}

func (k *sketch) line(x1, y1, x2, y2 float64, color string, dashed bool) {
	l := k.base("line", x1, y1, x2-x1, y2-y1, color)
	l["backgroundColor"] = "transparent"
	l["roundness"] = nil
	l["points"] = [][]float64{{0, 0}, {x2 - x1, y2 - y1}}
	if dashed {
		l["strokeStyle"] = "dashed"
		l["strokeWidth"] = 1
	}
	k.els = append(k.els, l)
}

func (k *sketch) json(grid bool) string {
	app := map[string]any{"viewBackgroundColor": "#ffffff", "gridSize": 20}
	if grid {
		app["gridModeEnabled"] = true
	}
	b, _ := json.Marshal(map[string]any{"type": "excalidraw", "version": 2, "source": "pocket-draw", "elements": k.els, "appState": app, "files": map[string]any{}})
	return string(b)
}

// first returns the scene with only the first n elements (an earlier version).
func (k *sketch) first(n int) string {
	keep := &sketch{rng: k.rng, els: k.els[:min(n, len(k.els))]}
	return keep.json(false)
}

type demoSketch struct {
	title, folder string
	starred       bool
	build         func(k *sketch)
}

var demoSketches = []demoSketch{
	{"Homelab network", "Homelab", true, func(k *sketch) {
		k.shape("ellipse", 340, 20, 180, 80, "Internet", "gray")
		k.shape("rectangle", 340, 160, 180, 70, "Router", "blue")
		k.shape("rectangle", 340, 300, 180, 70, "Switch", "blue")
		k.shape("rectangle", 80, 450, 180, 80, "Server\n(Pocket apps)", "green")
		k.shape("rectangle", 340, 450, 180, 80, "NAS\n(photos, media)", "yellow")
		k.shape("rectangle", 600, 450, 180, 80, "Wi-Fi\naccess point", "violet")
		k.shape("ellipse", 560, 610, 120, 60, "Phone", "pink")
		k.shape("ellipse", 700, 610, 120, 60, "Laptop", "pink")
		k.arrow(430, 100, 430, 160, "none")
		k.arrow(430, 230, 430, 300, "none")
		k.arrow(380, 370, 170, 450, "none")
		k.arrow(430, 370, 430, 450, "none")
		k.arrow(480, 370, 690, 450, "none")
		k.arrow(670, 530, 620, 610, "none")
		k.arrow(710, 530, 760, 610, "none")
		k.text(80, 300, "Everything private\nstays on the tailnet", 18, "red")
	}},
	{"Living room layout", "Home", true, func(k *sketch) {
		k.shape("rectangle", 40, 40, 620, 440, "", "none")
		k.shape("rectangle", 80, 80, 260, 90, "Sofa", "blue")
		k.shape("rectangle", 160, 230, 160, 100, "Coffee\ntable", "yellow")
		k.shape("rectangle", 520, 200, 80, 180, "TV unit", "gray")
		k.shape("ellipse", 120, 360, 110, 90, "Plant", "green")
		k.shape("rectangle", 380, 60, 130, 30, "Window", "violet")
		k.line(660, 300, 600, 360, "none", false)
		k.text(610, 380, "Door", 18, "none")
		k.text(60, 500, "Room is 5.2 m × 3.8 m. Rug 2 × 3 m under the table.", 18, "red")
	}},
	{"Garden beds 2026", "Home", false, func(k *sketch) {
		beds := []struct{ label, color string }{{"Tomatoes", "red"}, {"Peppers", "yellow"}, {"Herbs", "green"}, {"Beans", "green"}, {"Salad", "green"}, {"Strawberries", "pink"}}
		for i, b := range beds {
			x, y := 40+float64(i%3)*220, 60+float64(i/3)*180
			k.shape("rectangle", x, y, 190, 140, b.label, b.color)
		}
		k.text(40, 10, "Raised beds — south side", 24, "none")
		k.text(40, 420, "Water every 2 days in July. Compost in March.", 18, "blue")
	}},
	{"Weekend in Kotor — day plan", "Trips", false, func(k *sketch) {
		steps := []string{"08:00\nBreakfast\nin the old town", "10:00\nWalk the walls\nto the fortress", "13:00\nLunch by\nthe water", "15:30\nBoat to\nPerast", "19:00\nDinner\n+ sunset"}
		for i, s := range steps {
			x := 40 + float64(i)*200
			k.shape("rectangle", x, 120, 170, 120, s, []string{"blue", "green", "yellow", "violet", "pink"}[i])
			if i > 0 {
				k.arrow(x-30, 180, x, 180, "none")
			}
		}
		k.text(40, 40, "Saturday", 28, "none")
		k.text(40, 290, "Bring: hat, water, cash for the boat", 18, "red")
	}},
	{"App idea: plant reminders", "Ideas", false, func(k *sketch) {
		k.shape("ellipse", 300, 220, 220, 110, "Plant\nreminders", "green")
		ideas := []struct {
			x, y  float64
			label string
		}{{40, 60, "Watering\nschedule"}, {560, 60, "Photo log"}, {40, 400, "Push when\nit's dry"}, {560, 400, "Shared with\nthe family"}, {320, 30, "Plant\ndatabase"}}
		for _, i := range ideas {
			k.shape("rectangle", i.x, i.y, 170, 90, i.label, "yellow")
			k.line(410, 275, i.x+85, i.y+45, "gray", true)
		}
	}},
	{"Kitchen shelf measurements", "Home", false, func(k *sketch) {
		k.shape("rectangle", 60, 80, 500, 40, "", "yellow")
		k.shape("rectangle", 60, 200, 500, 40, "", "yellow")
		k.shape("rectangle", 60, 320, 500, 40, "", "yellow")
		k.line(60, 50, 560, 50, "red", false)
		k.text(270, 20, "120 cm", 18, "red")
		k.line(600, 80, 600, 360, "red", false)
		k.text(615, 210, "3 × 30 cm\napart", 18, "red")
		k.text(60, 400, "Oak, 2 cm thick. Brackets every 40 cm.", 18, "none")
	}},
	{"Morning bike route", "", false, func(k *sketch) {
		k.shape("ellipse", 40, 300, 120, 70, "Home", "blue")
		k.shape("ellipse", 300, 120, 140, 70, "Bakery", "yellow")
		k.shape("ellipse", 560, 260, 140, 70, "River path", "green")
		k.shape("ellipse", 340, 440, 140, 70, "Office", "violet")
		k.arrow(160, 320, 300, 170, "none")
		k.arrow(440, 160, 570, 270, "none")
		k.arrow(600, 330, 470, 450, "none")
		k.text(40, 40, "11 km, about 35 minutes", 22, "none")
	}},
}

type demoMermaid struct {
	title, folder string
	starred       bool
	code          string
}

var demoMermaids = []demoMermaid{
	{"Morning routine", "", true, `flowchart TD
    A[Alarm at 6:30] --> B{Slept well?}
    B -- Yes --> C[Run 5 km]
    B -- No --> D[Stretch 10 min]
    C --> E[Shower]
    D --> E
    E --> F[Breakfast + coffee]
    F --> G[Plan the day]
`},
	{"How the nightly backup runs", "Homelab", false, `sequenceDiagram
    participant T as Timer (03:00)
    participant B as Pocket Backups
    participant A as Each app
    participant N as NAS
    T->>B: start nightly job
    B->>A: ask for a consistent copy
    A-->>B: database snapshot
    B->>N: upload changed chunks
    N-->>B: done
    B-->>T: report + notify if anything failed
`},
	{"Moving house", "Home", false, `gantt
    title Moving house
    dateFormat YYYY-MM-DD
    section Before
    Sort and give away      :a1, 2026-09-01, 14d
    Book the van            :a2, 2026-09-10, 2d
    Pack books and kitchen  :a3, after a1, 10d
    section Moving day
    Load and drive          :b1, 2026-09-26, 1d
    section After
    Unpack essentials       :c1, after b1, 3d
    Change address everywhere :c2, after b1, 7d
`},
	{"Family tree", "", false, `flowchart TD
    G1[Grandpa Milan] --- G2[Grandma Vera]
    G1 --> P1[Dad]
    G1 --> P2[Aunt Ana]
    P1 --- M[Mum]
    P1 --> K1[Me]
    P1 --> K2[My sister]
    P2 --> C1[Cousin Luka]
`},
	{"Book club topics", "Ideas", false, `mindmap
  root((Book club))
    Fiction
      Classics
      New this year
    Non-fiction
      History
      Science
    Format
      Monthly
      One host per meeting
      Snacks rota
`},
	{"Where the money went in October", "", false, `pie showData
    title October spending
    "Home" : 820
    "Food" : 410
    "Transport" : 160
    "Fun" : 140
    "Savings" : 500
`},
	{"Our 2026", "", false, `timeline
    title Our 2026
    January : New job starts
    March : Trip to Kotor
    June : Garden beds built
    September : Moved house
    December : Family holidays
`},
	{"Order states", "Work", false, `stateDiagram-v2
    [*] --> Draft
    Draft --> Sent : send to client
    Sent --> Paid : payment arrives
    Sent --> Overdue : 30 days pass
    Overdue --> Paid
    Paid --> [*]
`},
	{"Recipes data model", "Work", false, `erDiagram
    RECIPE ||--o{ INGREDIENT_LINE : has
    INGREDIENT ||--o{ INGREDIENT_LINE : "used in"
    RECIPE ||--o{ STEP : has
    PLAN ||--o{ RECIPE : schedules
    RECIPE {
      string title
      int servings
      int minutes
    }
`},
}

// demoDrawio is a draw.io diagram as boxes and links (for the XML and the picture).
type demoDrawio struct {
	title, folder string
	boxes         []demoBox
	links         [][2]int
}

type demoBox struct {
	x, y, w, h  int
	label, fill string
	round       bool
}

var demoDrawios = []demoDrawio{
	{"Website sitemap", "Work", []demoBox{
		{280, 20, 160, 50, "Home", "#dae8fc", true},
		{40, 140, 140, 50, "About", "#d5e8d4", true}, {210, 140, 140, 50, "Services", "#d5e8d4", true}, {380, 140, 140, 50, "Blog", "#d5e8d4", true}, {550, 140, 140, 50, "Contact", "#d5e8d4", true},
		{210, 250, 140, 50, "Pricing", "#fff2cc", false}, {380, 250, 140, 50, "Posts", "#fff2cc", false},
	}, [][2]int{{0, 1}, {0, 2}, {0, 3}, {0, 4}, {2, 5}, {3, 6}}},
	{"Office floor plan", "Work", []demoBox{
		{20, 20, 300, 180, "Open space\n8 desks", "#dae8fc", false}, {340, 20, 160, 180, "Meeting\nroom", "#e1d5e7", false},
		{20, 220, 140, 100, "Kitchen", "#fff2cc", false}, {180, 220, 140, 100, "Storage", "#f5f5f5", false}, {340, 220, 160, 100, "Entrance", "#d5e8d4", false},
	}, nil},
}

func (d demoDrawio) xml() string {
	var b strings.Builder
	b.WriteString(`<mxfile host="pocket-draw"><diagram id="demo" name="Page-1"><mxGraphModel dx="800" dy="600" grid="1" gridSize="10"><root><mxCell id="0"/><mxCell id="1" parent="0"/>`)
	for i, x := range d.boxes {
		style := "whiteSpace=wrap;html=1;fillColor=" + x.fill + ";strokeColor=#666666;"
		if x.round {
			style = "rounded=1;" + style
		}
		fmt.Fprintf(&b, `<mxCell id="b%d" value="%s" style="%s" vertex="1" parent="1"><mxGeometry x="%d" y="%d" width="%d" height="%d" as="geometry"/></mxCell>`,
			i, html.EscapeString(strings.ReplaceAll(x.label, "\n", "<br>")), style, x.x, x.y, x.w, x.h)
	}
	for i, l := range d.links {
		fmt.Fprintf(&b, `<mxCell id="e%d" style="edgeStyle=orthogonalEdgeStyle;rounded=0;html=1;" edge="1" parent="1" source="b%d" target="b%d"><mxGeometry relative="1" as="geometry"/></mxCell>`, i, l[0], l[1])
	}
	b.WriteString(`</root></mxGraphModel></diagram></mxfile>`)
	return b.String()
}

func (d demoDrawio) svg() string {
	maxX, maxY := 0, 0
	for _, x := range d.boxes {
		maxX, maxY = max(maxX, x.x+x.w), max(maxY, x.y+x.h)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" style="background:#ffffff">`, maxX+20, maxY+20, maxX+20, maxY+20)
	for _, l := range d.links {
		a, z := d.boxes[l[0]], d.boxes[l[1]]
		x1, y1, x2, y2 := a.x+a.w/2, a.y+a.h, z.x+z.w/2, z.y
		mid := (y1 + y2) / 2
		fmt.Fprintf(&b, `<path d="M%d %d V%d H%d V%d" fill="none" stroke="#333" stroke-width="1.5"/>`, x1, y1, mid, x2, y2)
	}
	for _, x := range d.boxes {
		rx := 0
		if x.round {
			rx = 8
		}
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" rx="%d" fill="%s" stroke="#666"/>`, x.x, x.y, x.w, x.h, rx, x.fill)
		lines := strings.Split(x.label, "\n")
		for i, ln := range lines {
			y := x.y + x.h/2 + (i*2-len(lines)+1)*8 + 5
			fmt.Fprintf(&b, `<text x="%d" y="%d" font-family="Helvetica, Arial, sans-serif" font-size="13" text-anchor="middle" fill="#222">%s</text>`, x.x+x.w/2, y, html.EscapeString(ln))
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// seedDemo fills the owner's account with a few months of drawings.
func (s *Server) seedDemo(ctx context.Context, owner int64) error {
	key := fmt.Sprintf("seeded:%d:demo", owner)
	if _, _, done := s.store.CacheGet(ctx, key); done {
		return nil
	}
	rng := rand.New(rand.NewSource(42))
	now := time.Now().UTC()
	folder := func(name string) (*int64, error) {
		if name == "" {
			return nil, nil
		}
		f, err := s.store.EnsureFolder(ctx, owner, name)
		if err != nil {
			return nil, err
		}
		return &f.ID, nil
	}
	ts := func(t time.Time) string { return t.Format(time.RFC3339Nano) }
	// dates spread over four months, newest last
	type dated struct{ created, edited, opened time.Time }
	when := func(i, n int) dated {
		created := now.AddDate(0, 0, -120+i*110/max(n, 1)).Add(time.Duration(rng.Intn(10*60)) * time.Minute)
		edited := created.Add(time.Duration(rng.Intn(72*60)) * time.Minute)
		if edited.After(now) {
			edited = now.Add(-time.Duration(rng.Intn(120)) * time.Minute)
		}
		opened := edited.Add(time.Duration(rng.Intn(5*24*60)) * time.Minute)
		if opened.After(now) {
			opened = now.Add(-time.Duration(rng.Intn(60)) * time.Minute)
		}
		return dated{created, edited, opened}
	}
	total := len(demoSketches) + len(demoMermaids) + len(demoDrawios)
	order := rng.Perm(total)
	// add stores a drawing whose earlier states are versions (rev 1, 2, …).
	add := func(i int, d *store.Drawing, versions []string) error {
		d.Rev = len(versions) + 1
		if err := s.store.CreateDrawing(ctx, owner, d); err != nil {
			return err
		}
		w := when(order[i], total)
		for j, v := range versions {
			at := w.created.Add(time.Duration(j+1) * 15 * time.Minute)
			if err := s.store.AddVersion(ctx, owner, store.Version{DrawingID: d.ID, Rev: j + 1, Title: d.Title, Data: v, CreatedAt: ts(at)}); err != nil {
				return err
			}
		}
		opened := ""
		if order[i]%3 != 0 {
			opened = ts(w.opened)
		}
		return s.store.SetDates(ctx, owner, d.ID, ts(w.created), ts(w.edited), opened)
	}
	i := 0
	for _, ds := range demoSketches {
		k := &sketch{rng: rng}
		ds.build(k)
		fid, err := folder(ds.folder)
		if err != nil {
			return err
		}
		var versions []string
		if len(k.els) > 6 {
			versions = []string{k.first(len(k.els) / 3), k.first(len(k.els) * 2 / 3)}
		}
		d := &store.Drawing{Kind: store.KindSketch, Title: ds.title, Data: k.json(ds.title == "Garden beds 2026"), FolderID: fid, Starred: ds.starred, Source: "app"}
		if err := add(i, d, versions); err != nil {
			return err
		}
		i++
	}
	for _, dm := range demoMermaids {
		fid, err := folder(dm.folder)
		if err != nil {
			return err
		}
		d := &store.Drawing{Kind: store.KindMermaid, Title: dm.title, Data: dm.code, FolderID: fid, Starred: dm.starred, Source: "app"}
		if dm.title == "How the nightly backup runs" {
			d.Source = "ai"
		}
		if err := add(i, d, nil); err != nil {
			return err
		}
		i++
	}
	for _, dd := range demoDrawios {
		fid, err := folder(dd.folder)
		if err != nil {
			return err
		}
		d := &store.Drawing{Kind: store.KindDrawio, Title: dd.title, Data: dd.xml(), Preview: dd.svg(), FolderID: fid, Source: "app"}
		if err := add(i, d, nil); err != nil {
			return err
		}
		i++
	}
	// one in the trash
	k := &sketch{rng: rng}
	k.shape("rectangle", 40, 40, 200, 100, "Old idea", "gray")
	old := &store.Drawing{Kind: store.KindSketch, Title: "Old shelf idea", Data: k.json(false), Source: "app"}
	if err := s.store.CreateDrawing(ctx, owner, old); err != nil {
		return err
	}
	_ = s.store.TrashDrawing(ctx, owner, old.ID, false)
	s.log.Info("demo data added", "drawings", total+1)
	return s.store.CachePut(ctx, key, "1")
}

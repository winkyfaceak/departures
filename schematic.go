package main

import (
	"cmp"
	"fmt"
	"image/color"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// TfL line colours, as on the tube map; buses get bus red.
var lineColours = map[string]string{
	"bakerloo": "#B36305", "central": "#E32017", "circle": "#FFD300", "district": "#00782A",
	"hammersmith-city": "#F3A9BB", "jubilee": "#A0A5A9", "metropolitan": "#9B0056", "northern": "#000000",
	"piccadilly": "#003688", "victoria": "#0098D4", "waterloo-city": "#95CDBA", "elizabeth": "#6950A1",
	"dlr": "#00A4A7", "tram": "#5FB526", "liberty": "#5D6061", "lioness": "#FAA61A", "mildmay": "#0077AD",
	"suffragette": "#5BBD72", "weaver": "#823A62", "windrush": "#ED1B00",
}

var (
	paper = lipgloss.Color("#FFFFFF")
	ink   = lipgloss.Color("#0019A8") // TfL blue, for station names
	black = lipgloss.Color("#000000")
)

// Layout spacing: cells between stops on horizontal runs (room for
// alternating labels; bus stop names run longer) and rows between stops on
// vertical runs.
const (
	hGap    = 6
	busHGap = 10
	vGap    = 2
)

const (
	kBlank uint8 = iota
	kLine
	kChange // interchange circle
	kName
	kHere // your station's name
	kTrain
)

type cell struct {
	r    rune
	kind uint8
}

type xy struct{ x, y int }

// schematic is a tube-map style drawing of one line: octilinear track,
// every stop ticked and named, interchanges circled.
type schematic struct {
	w, h  int
	cells []cell
	at    map[string]xy      // stop → its cell
	paths map[[2]string][]xy // neighbouring stops → track cells from first to second
	here  string
}

// fisheye projects to km around Charing Cross, then pulls the suburbs in
// (r^0.6) so central London gets room, like the real map.
func fisheye(lat, lon float64) (float64, float64) {
	x := (lon + 0.1278) * math.Cos(51.5074*math.Pi/180) * 111.32
	y := (51.5074 - lat) * 110.57
	if r := math.Hypot(x, y); r > 0 {
		f := math.Pow(r, 0.6) / r
		x, y = x*f, y*f
	}
	return x, y
}

// simplify is Ramer–Douglas–Peucker: the indexes of pts worth keeping as bends.
func simplify(pts [][2]float64, eps float64) []int {
	keep := []int{0, len(pts) - 1}
	var rec func(a, b int)
	rec = func(a, b int) {
		best, far := -1, eps
		ax, ay, bx, by := pts[a][0], pts[a][1], pts[b][0], pts[b][1]
		for i := a + 1; i < b; i++ {
			px, py := pts[i][0], pts[i][1]
			d := math.Hypot(px-ax, py-ay)
			if l := math.Hypot(bx-ax, by-ay); l > 0 {
				d = math.Abs((bx-ax)*(ay-py)-(ax-px)*(by-ay)) / l
			}
			if d > far {
				best, far = i, d
			}
		}
		if best >= 0 {
			keep = append(keep, best)
			rec(a, best)
			rec(best, b)
		}
	}
	rec(0, len(pts)-1)
	slices.Sort(keep)
	return slices.Compact(keep)
}

// octilinear walks from a to b in tube-map directions: straight, 45°, straight.
func octilinear(a, b xy) []xy {
	dx, dy := b.x-a.x, b.y-a.y
	sx, sy := cmp.Compare(dx, 0), cmp.Compare(dy, 0)
	ax, ay := max(dx, -dx), max(dy, -dy)
	diag, straight := min(ax, ay), max(ax, ay)-min(ax, ay)
	step := xy{0, sy}
	if ax > ay {
		step = xy{sx, 0}
	}
	pts, p := []xy{a}, a
	move := func(d xy, n int) {
		for range n {
			p = xy{p.x + d.x, p.y + d.y}
			pts = append(pts, p)
		}
	}
	move(step, straight/2)
	move(xy{sx, sy}, diag)
	move(step, straight-straight/2)
	return pts
}

// buildSchematic lays out a line. Buses only get the direction your stop is
// on, and no interchange circles (every stop serves several routes).
func buildSchematic(r routeSeq, here string, bus bool) schematic {
	type stopInfo struct {
		name   string
		X, Y   float64
		change bool
	}
	stops := map[string]stopInfo{}
	for _, seq := range r.StopPointSequences {
		for _, s := range seq.StopPoint {
			x, y := fisheye(s.Lat, s.Lon)
			stops[s.ID] = stopInfo{short(s.Name), x, y, !bus && len(s.Lines) > 1}
		}
	}
	adj := map[string][]string{}
	for _, rt := range r.OrderedLineRoutes {
		if bus && !slices.Contains(rt.NaptanIDs, here) {
			continue
		}
		for i := 1; i < len(rt.NaptanIDs); i++ {
			a, b := rt.NaptanIDs[i-1], rt.NaptanIDs[i]
			_, okA := stops[a]
			_, okB := stops[b]
			if a != b && okA && okB && !slices.Contains(adj[a], b) {
				adj[a], adj[b] = append(adj[a], b), append(adj[b], a)
			}
		}
	}
	ids := make([]string, 0, len(adj))
	for id := range adj {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return schematic{}
	}

	// Chains of track between junctions and termini.
	junction := map[string]bool{}
	for _, id := range ids {
		junction[id] = len(adj[id]) != 2
	}
	if !slices.ContainsFunc(ids, func(id string) bool { return junction[id] }) {
		junction[ids[0]] = true // a pure loop
	}
	walked := map[[2]string]bool{}
	var chains [][]string
	for _, k := range ids {
		if !junction[k] {
			continue
		}
		for _, n := range adj[k] {
			if walked[[2]string{k, n}] {
				continue
			}
			chain, prev, cur := []string{k}, k, n
			for {
				walked[[2]string{prev, cur}], walked[[2]string{cur, prev}] = true, true
				chain = append(chain, cur)
				if junction[cur] {
					break
				}
				next := adj[cur][0]
				if next == prev {
					next = adj[cur][1]
				}
				prev, cur = cur, next
			}
			chains = append(chains, chain)
		}
	}

	// Split chains at their real bends; each segment is drawn as one octilinear path.
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, id := range ids {
		s := stops[id]
		minX, maxX = math.Min(minX, s.X), math.Max(maxX, s.X)
		minY, maxY = math.Min(minY, s.Y), math.Max(maxY, s.Y)
	}
	var segments [][]string
	for _, c := range chains {
		pts := make([][2]float64, len(c))
		for i, id := range c {
			pts[i] = [2]float64{stops[id].X, stops[id].Y}
		}
		keep := simplify(pts, 0.04*math.Max(maxX-minX, maxY-minY))
		for i := 1; i < len(keep); i++ {
			segments = append(segments, c[keep[i-1]:keep[i]+1])
		}
	}

	// Scale so every segment has room for its stops along the axis it mostly
	// runs. Cells are twice as tall as wide, so x gets twice the scale.
	scale, gap := 0.0, float64(hGap)
	if bus {
		gap = busHGap
	}
	for _, sg := range segments {
		a, b := stops[sg[0]], stops[sg[len(sg)-1]]
		dx, dy := math.Abs(b.X-a.X), math.Abs(b.Y-a.Y)
		need := float64(len(sg) - 1)
		if 2*dx >= dy && dx > 0 {
			scale = math.Max(scale, need*gap/(2*dx))
		} else if dy > 0 {
			scale = math.Max(scale, need*vGap/dy)
		}
	}
	// ponytail: capped so a cramped line can't make a huge canvas; labels that don't fit are skipped
	scale = math.Min(scale, 450/math.Max(maxX-minX, 1e-9))
	sx, sy := 2*scale, scale
	const mx, my = 26, 2 // margins for labels
	s := schematic{
		w:     2*mx + int(math.Round((maxX-minX)*sx)) + 1,
		h:     2*my + int(math.Round((maxY-minY)*sy)) + 1,
		at:    map[string]xy{},
		paths: map[[2]string][]xy{},
		here:  here,
	}
	s.cells = make([]cell, s.w*s.h)
	for _, id := range ids {
		st := stops[id]
		s.at[id] = xy{mx + int(math.Round((st.X-minX)*sx)), my + int(math.Round((st.Y-minY)*sy))}
	}

	// Track, with stops evenly spaced along each segment.
	dir := map[string]xy{}   // track direction at each stop
	alt := map[string]bool{} // alternate label side on horizontal runs
	for _, sg := range segments {
		path := octilinear(s.at[sg[0]], s.at[sg[len(sg)-1]])
		last := len(path) - 1
		idx := make([]int, len(sg))
		for i := range sg {
			idx[i] = int(math.Round(float64(i*last) / float64(len(sg)-1)))
			if i > 0 && i < len(sg)-1 {
				s.at[sg[i]] = path[idx[i]]
				alt[sg[i]] = i%2 == 1
			}
		}
		for i := 1; i < len(sg); i++ {
			sub := path[idx[i-1] : idx[i]+1]
			s.paths[[2]string{sg[i-1], sg[i]}] = sub
			rev := slices.Clone(sub)
			slices.Reverse(rev)
			s.paths[[2]string{sg[i], sg[i-1]}] = rev
		}
		for i, p := range path {
			s.set(p, cell{trackRune(stepAt(path, i)), kLine})
		}
		for i, id := range sg {
			if _, ok := dir[id]; !ok {
				dir[id] = stepAt(path, idx[i])
			}
		}
	}

	// Names: yours first, then interchanges, then the rest; skip any that won't fit.
	order := slices.Clone(ids)
	sort.SliceStable(order, func(i, j int) bool {
		rank := func(id string) int {
			switch {
			case id == here:
				return 0
			case stops[id].change:
				return 1
			}
			return 2
		}
		return rank(order[i]) < rank(order[j])
	})
	for _, id := range order {
		p, d, name := s.at[id], dir[id], []rune(stops[id].name)
		n := len(name)
		type spot struct {
			x, y int
			tick rune
		}
		side := []spot{{p.x + 2, p.y, '┣'}, {p.x - 1 - n, p.y, '┫'}}
		above := []spot{{p.x - n/2, p.y - 1, '┻'}, {p.x + 1, p.y - 1, '┻'}, {p.x - n, p.y - 1, '┻'}}
		below := []spot{{p.x - n/2, p.y + 1, '┳'}, {p.x + 1, p.y + 1, '┳'}, {p.x - n, p.y + 1, '┳'}}
		if alt[id] {
			above, below = below, above
		}
		var spots []spot
		switch {
		case d.y == 0: // horizontal run: above or below, alternating
			spots = slices.Concat(above[:1], below[:1], above[1:], below[1:])
		case d.x == 0:
			spots = slices.Concat(side, above[1:], below[1:])
		default:
			spots = slices.Concat(side, above, below)
		}
		tick, kind := '•', uint8(kName)
		if id == here {
			kind = kHere
		}
		for i, sp := range spots {
			if !s.free(sp.x, sp.y, n) && !(id == here && i == len(spots)-1) {
				continue
			}
			if (d.y == 0) != (sp.y != p.y) { // tick only points at its label on a straight run
				sp.tick = '•'
			}
			for j, r := range name {
				s.set(xy{sp.x + j, sp.y}, cell{r, kind})
			}
			tick = sp.tick
			break
		}
		switch {
		case id == here:
			s.set(p, cell{'◉', kChange})
		case stops[id].change:
			s.set(p, cell{'○', kChange})
		default:
			s.set(p, cell{tick, kLine})
		}
	}
	return s
}

// stepAt is the track direction at path[i].
func stepAt(path []xy, i int) xy {
	if len(path) < 2 {
		return xy{}
	}
	if i >= len(path)-1 {
		i = len(path) - 2
	}
	return xy{path[i+1].x - path[i].x, path[i+1].y - path[i].y}
}

func trackRune(d xy) rune {
	switch {
	case d.y == 0:
		return '━'
	case d.x == 0:
		return '┃'
	case d.x == d.y:
		return '╲'
	}
	return '╱'
}

func (s *schematic) set(p xy, c cell) {
	if p.x >= 0 && p.y >= 0 && p.x < s.w && p.y < s.h {
		s.cells[p.y*s.w+p.x] = c
	}
}

// free reports whether n cells from (x, y) are on the canvas and empty,
// with two cells clear of any other name either side.
func (s *schematic) free(x, y, n int) bool {
	if y < 0 || y >= s.h || x < 2 || x+n+2 > s.w {
		return false
	}
	for i := -2; i < n+2; i++ {
		k := s.cells[y*s.w+x+i].kind
		if k >= kName || i >= 0 && i < n && k != kBlank {
			return false
		}
	}
	return true
}

var arrows = map[xy]rune{
	{1, 0}: '▶', {-1, 0}: '◀', {0, -1}: '▲', {0, 1}: '▼',
	{1, -1}: '◥', {1, 1}: '◢', {-1, 1}: '◣', {-1, -1}: '◤',
}

// trains places each vehicle on the track between its last and next stop,
// pointing the way it's going.
func (m model) trains() map[xy]rune {
	byVehicle := map[string][]arrival{}
	for _, a := range m.traffic {
		byVehicle[a.VehicleID] = append(byVehicle[a.VehicleID], a)
	}
	age := time.Since(m.trafficAt).Seconds()
	out := map[xy]rune{}
	for _, preds := range byVehicle {
		sort.Slice(preds, func(i, j int) bool { return preds[i].TimeToStation < preds[j].TimeToStation })
		// Skip stops it has passed since the fetch.
		k := 0
		for k+1 < len(preds) && preds[k].TimeToStation-age < 0 {
			k++
		}
		next, prev := preds[k], prevStop(m.route, preds[k].NaptanID, preds[k].DestinationNaptanID)
		if k > 0 {
			prev = preds[k-1].NaptanID
		}
		// Its own predictions give the run time between stops.
		run := 120.0 // ponytail: guess when the vehicle has one prediction left
		gap := 0.0
		if k+1 < len(preds) {
			gap = preds[k+1].TimeToStation - next.TimeToStation
		} else if k > 0 {
			gap = next.TimeToStation - preds[k-1].TimeToStation
		}
		if gap >= 30 && gap <= 600 {
			run = gap
		}
		path, ok := m.schema.paths[[2]string{prev, next.NaptanID}]
		if !ok {
			if p, ok := m.schema.at[next.NaptanID]; ok {
				out[p] = '■'
			}
			continue
		}
		f := math.Max(0, math.Min(1, 1-(next.TimeToStation-age)/run))
		i := int(math.Round(f * float64(len(path)-1)))
		out[path[i]] = arrows[stepAt(path, i)]
	}
	return out
}

// contrast picks black or white text for a background colour.
func contrast(hex string) color.Color {
	var r, g, b int
	fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	if 0.299*float64(r)+0.587*float64(g)+0.114*float64(b) > 150 {
		return black
	}
	return paper
}

// mapOrigin is the canvas cell at the top left of the screen: centred on
// your station, panned, and kept on the canvas (small maps sit in the middle).
func (m model) mapOrigin() (int, int) {
	s, vw, vh := m.schema, m.w, max(1, m.h-2)
	clamp := func(o, size, view int) int {
		if size <= view {
			return (size - view) / 2
		}
		return max(0, min(o, size-view))
	}
	return clamp(s.at[s.here].x-vw/2+m.panX, s.w, vw), clamp(s.at[s.here].y-vh/2+m.panY, s.h, vh)
}

func (m model) schematicView() string {
	hex := cmp.Or(lineColours[m.line.ID], "#DC241F")
	colour := lipgloss.Color(hex)
	base := lipgloss.NewStyle().Background(paper)
	styles := [...]lipgloss.Style{
		kBlank:  base,
		kLine:   base.Foreground(colour).Bold(true),
		kChange: base.Foreground(black).Bold(true),
		kName:   base.Foreground(ink),
		kHere:   base.Foreground(paper).Background(ink).Bold(true),
		kTrain:  base.Foreground(contrast(hex)).Background(colour).Bold(true),
	}
	trains := m.trains()
	vw, vh := m.w, max(1, m.h-2)

	title := fmt.Sprintf(" %s · %d moving ", m.line.Name, len(trains))
	if m.mode == thameslink { // TfL has Thameslink departures, not train positions
		title = " Thameslink "
	}
	bar := lipgloss.NewStyle().Foreground(contrast(hex)).Background(colour).Bold(true).Width(vw)
	footer := " ←↑↓→ pan · m board · q quit"
	if m.err != nil {
		footer += " · " + m.err.Error()
	}
	out := []string{bar.Render(fit(title, vw))}

	s := m.schema
	if s.w == 0 {
		msg := "Loading…"
		if !m.loading {
			msg = "No route to draw"
		}
		out = append(out, base.Width(vw).Render(" "+msg))
		for len(out) < vh+1 {
			out = append(out, base.Width(vw).Render(""))
		}
	} else {
		ox, oy := m.mapOrigin()
		for y := oy; y < oy+vh; y++ {
			var b, run strings.Builder
			kind := uint8(255)
			for x := ox; x < ox+vw; x++ {
				c := cell{' ', kBlank}
				if x >= 0 && y >= 0 && x < s.w && y < s.h && s.cells[y*s.w+x].kind != kBlank {
					c = s.cells[y*s.w+x]
				}
				if r, ok := trains[xy{x, y}]; ok {
					c = cell{r, kTrain}
				}
				if c.kind != kind && run.Len() > 0 {
					b.WriteString(styles[kind].Render(run.String()))
					run.Reset()
				}
				kind = c.kind
				run.WriteRune(c.r)
			}
			b.WriteString(styles[kind].Render(run.String()))
			out = append(out, b.String())
		}
	}
	return strings.Join(append(out, base.Foreground(ink).Width(vw).Render(fit(footer, vw))), "\n")
}

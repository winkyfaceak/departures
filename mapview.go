package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

type routeSeq struct {
	StopPointSequences []struct {
		StopPoint []struct {
			ID       string
			Lat, Lon float64
		}
	}
	OrderedLineRoutes []struct{ NaptanIDs []string }
}

type pt struct{ lat, lon float64 }

// prevStop is the stop before next on a route heading to dest ("" when
// TfL gives no destination, as for buses, whose routes are one-way anyway).
func prevStop(r routeSeq, next, dest string) string {
	for _, route := range r.OrderedLineRoutes {
		ids := route.NaptanIDs
		for i := 1; i < len(ids); i++ {
			if ids[i] != next {
				continue
			}
			if dest == "" {
				return ids[i-1]
			}
			for _, d := range ids[i:] {
				if d == dest {
					return ids[i-1]
				}
			}
		}
	}
	return ""
}

// tflMap returns the line's routes and where each vehicle is now.
func (m model) tflMap() (routes [][]pt, vehicles []pt) {
	at := map[string]pt{}
	for _, seq := range m.route.StopPointSequences {
		for _, s := range seq.StopPoint {
			at[s.ID] = pt{s.Lat, s.Lon}
		}
	}
	for _, r := range m.route.OrderedLineRoutes {
		var line []pt
		for _, id := range r.NaptanIDs {
			if p, ok := at[id]; ok {
				line = append(line, p)
			}
		}
		routes = append(routes, line)
	}

	// A vehicle's soonest prediction is the stop it's heading for.
	next := map[string]arrival{}
	for _, a := range m.traffic {
		if b, ok := next[a.VehicleID]; !ok || a.TimeToStation < b.TimeToStation {
			next[a.VehicleID] = a
		}
	}
	age := time.Since(m.trafficAt).Seconds()
	for _, a := range next {
		to, ok := at[a.NaptanID]
		if !ok {
			continue
		}
		from, ok := at[prevStop(m.route, a.NaptanID, a.DestinationNaptanID)]
		if !ok {
			from = to
		}
		// ponytail: assumes ~2 min between stops; use timetabled run times if it looks jumpy
		f := math.Max(0, math.Min(1, (a.TimeToStation-age)/120))
		vehicles = append(vehicles, pt{to.lat + (from.lat-to.lat)*f, to.lon + (from.lon-to.lon)*f})
	}
	return routes, vehicles
}

// railMap returns each departure's route through this station and where
// those trains last reported.
func (m model) railMap() (routes [][]pt, trains []pt) {
	here := pt{m.stop.Lat, m.stop.Lon}
	for _, s := range m.services {
		var line []pt
		last, seen := pt{}, false
		for _, c := range calls(s.PreviousCallingPoints) {
			if st, ok := railByCRS[c.Crs]; ok {
				line = append(line, pt{st.Lat, st.Lon})
				if c.At != "" && c.At != "No report" && c.At != "Cancelled" {
					last, seen = line[len(line)-1], true
				}
			}
		}
		line = append(line, here)
		for _, c := range calls(s.SubsequentCallingPoints) {
			if st, ok := railByCRS[c.Crs]; ok {
				line = append(line, pt{st.Lat, st.Lon})
			}
		}
		routes = append(routes, line)
		if seen {
			trains = append(trains, last)
		}
	}
	return routes, trains
}

func (m model) mapView() string {
	iw, ih := m.w-4, m.h-3
	var routes [][]pt
	var vehicles []pt
	title := short(m.stop.CommonName) + " · next departures"
	if m.mode == nationalRail {
		routes, vehicles = m.railMap()
	} else {
		routes, vehicles = m.tflMap()
		title = fmt.Sprintf("%s · %d vehicles", m.line.Name, len(vehicles))
	}
	out := []string{fit(title, iw), ""}
	switch {
	case m.err == errNoKey:
		out = append(out, errNoKey.Error())
	case len(routes) == 0 && (m.loading || m.mode != nationalRail && m.routeLine != m.line.ID):
		out = append(out, "Loading…")
	case len(routes) == 0 && m.err != nil:
		out = append(out, "Failed: "+m.err.Error())
	case len(routes) == 0:
		out = append(out, "Nothing to map")
	default:
		out = append(out, drawMap(iw, ih-2, routes, vehicles, pt{m.stop.Lat, m.stop.Lon}, short(m.stop.CommonName))...)
	}
	for len(out) < ih {
		out = append(out, "")
	}
	return m.frame(out, " ● vehicle  ◆ "+short(m.stop.CommonName)+" · m board · q quit")
}

// braille[x][y] is the dot bit for column x, row y of a braille cell.
var braille = [2][4]rune{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}}

// drawMap plots routes as braille dot lines on a w×h cell canvas, with
// vehicles as ● and here as ◆ plus its label.
func drawMap(w, h int, routes [][]pt, vehicles []pt, here pt, label string) []string {
	if w < 1 || h < 1 {
		return nil
	}
	// Equirectangular projection, squashed by cos(lat) so London isn't stretched.
	k := math.Cos(here.lat * math.Pi / 180)
	minX, minY, maxX, maxY := here.lon*k, -here.lat, here.lon*k, -here.lat
	for _, r := range routes {
		for _, p := range r {
			minX, maxX = math.Min(minX, p.lon*k), math.Max(maxX, p.lon*k)
			minY, maxY = math.Min(minY, -p.lat), math.Max(maxY, -p.lat)
		}
	}
	W, H := float64(w*2-1), float64(h*4-1) // braille cells are 2×4 roughly square dots
	scale := math.Min(W/math.Max(maxX-minX, 1e-9), H/math.Max(maxY-minY, 1e-9))
	ox, oy := (W-(maxX-minX)*scale)/2, (H-(maxY-minY)*scale)/2
	proj := func(p pt) (float64, float64) {
		return ox + (p.lon*k-minX)*scale, oy + (-p.lat-minY)*scale
	}

	cells := make([]rune, w*h)
	marks := make([]rune, w*h)
	set := func(x, y float64) {
		xi, yi := int(math.Round(x)), int(math.Round(y))
		if xi >= 0 && yi >= 0 && xi < w*2 && yi < h*4 {
			cells[yi/4*w+xi/2] |= braille[xi%2][yi%4]
		}
	}
	mark := func(p pt, r rune) (int, int) {
		x, y := proj(p)
		cx, cy := int(math.Round(x))/2, int(math.Round(y))/4
		if cx >= 0 && cy >= 0 && cx < w && cy < h {
			marks[cy*w+cx] = r
		}
		return cx, cy
	}

	for _, r := range routes {
		for i := 1; i < len(r); i++ {
			x0, y0 := proj(r[i-1])
			x1, y1 := proj(r[i])
			steps := math.Max(1, math.Max(math.Abs(x1-x0), math.Abs(y1-y0)))
			for s := 0.0; s <= steps; s++ {
				set(x0+(x1-x0)*s/steps, y0+(y1-y0)*s/steps)
			}
		}
	}
	for _, v := range vehicles {
		mark(v, '●')
	}
	cx, cy := mark(here, '◆')
	text := []rune(" " + label)
	start := cx + 1
	if start+len(text) > w {
		start = cx - len(text) // put the label on the left near the right edge
	}
	for i, r := range text {
		if x := start + i; x >= 0 && x < w && x != cx {
			marks[cy*w+x] = r
		}
	}

	// Render runs of route dots dim and marks lit.
	out := make([]string, h)
	for y := range h {
		var b, run strings.Builder
		runLit := false
		flush := func() {
			if runLit {
				b.WriteString(lit.Render(run.String()))
			} else {
				b.WriteString(dim.Render(run.String()))
			}
			run.Reset()
		}
		for x := range w {
			r, isLit := ' ', false
			if c := marks[y*w+x]; c != 0 {
				r, isLit = c, true
			} else if c := cells[y*w+x]; c != 0 {
				r = 0x2800 + c
			}
			if isLit != runLit {
				flush()
				runLit = isLit
			}
			run.WriteRune(r)
		}
		flush()
		out[y] = b.String()
	}
	return out
}

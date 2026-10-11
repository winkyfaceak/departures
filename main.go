// Command departures is a dot-matrix departure board for London trains and
// buses (TfL API, no key needed) and National Rail stations (free
// Rail Data Marketplace key).
package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"image/color"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	api          = "https://api.tfl.gov.uk"
	refresh      = 30 * time.Second // keyless TfL allows 50 requests/min
	nationalRail = "national-rail"
	disrupted    = "disruptions"
)

var modes = []string{disrupted, "tube", "elizabeth-line", "overground", "dlr", "tram", "bus", nationalRail}

var (
	amber  = lipgloss.Color("#FFB000")
	unlit  = lipgloss.NewStyle().Foreground(lipgloss.Color("#3D2A00"))
	lit    = lipgloss.NewStyle().Foreground(amber).Bold(true)
	dim    = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A6000"))
	picked = lit.Reverse(true)
	box    = lit.Border(lipgloss.RoundedBorder()).BorderForeground(amber).Padding(0, 1)
)

// 3x5 LED font for the clock: glyphs for "0123456789:" side by side, '#' lit.
const fontSrc = `### ..# ### ### #.# ### ### ### ### ### .
#.# ..# ..# ..# #.# #.. #.. ..# #.# #.# #
#.# ..# ### ### ### ### ### ..# ### ### .
#.# ..# #.. ..# ..# ..# #.# ..# #.# ..# #
### ..# ### ### ..# ### ### ..# ### ### .`

var font = func() map[rune][]string {
	f := map[rune][]string{}
	for _, row := range strings.Split(fontSrc, "\n") {
		for i, g := range strings.Fields(row) {
			c := rune("0123456789:"[i])
			f[c] = append(f[c], g)
		}
	}
	return f
}()

// dots renders text as 5 rows of LED pixels, '#' lit and '.' unlit.
func dots(text string) []string {
	rows := make([]string, 5)
	for r := range rows {
		var glyphs []string
		for _, c := range text {
			glyphs = append(glyphs, font[c][r])
		}
		rows[r] = strings.Join(glyphs, ".")
	}
	return rows
}

var suffix = regexp.MustCompile(`( (Underground|DLR|Rail|Tram))? (Station|Stop)$|-Underground$| \((Central|Bakerloo|Circle|District|H&C|Dist&Picc)( Line)?\)`)

// short turns "Brixton Underground Station" into "Brixton", and
// "Paddington (H&C Line)-Underground" into "Paddington".
func short(name string) string {
	return strings.Join(strings.Fields(suffix.ReplaceAllString(name, "")), " ")
}

type line struct{ ID, Name string }

type stop struct {
	NaptanID, CommonName, Indicator string
	Lat, Lon                        float64
	AdditionalProperties            []struct{ Key, Value string }
}

func (s stop) label() string {
	l := s.CommonName
	if s.Indicator != "" {
		l += " (" + s.Indicator + ")"
	}
	for _, p := range s.AdditionalProperties {
		if p.Key == "Towards" && p.Value != "" {
			l += " → " + p.Value
		}
	}
	return l
}

type arrival struct {
	VehicleID, NaptanID, DestinationNaptanID       string
	DestinationName, PlatformName, CurrentLocation string
	TimeToStation                                  float64
}

type departure struct {
	secs float64
	arrival
}

// departures lists arrivals soonest first, counted down by age since the
// fetch. Trains gone >30s are dropped.
func departures(arrivals []arrival, age time.Duration) []departure {
	var out []departure
	for _, a := range arrivals {
		if d := (departure{a.TimeToStation - age.Seconds(), a}); d.secs > -30 {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].secs < out[j].secs })
	return out
}

// row is one line on the board, whichever network it came from.
type row struct{ left, right, platform, ticker string }

var client = &http.Client{Timeout: 10 * time.Second}

func get[T any](url string) (T, error) {
	var v T
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return v, err
	}
	req.Header.Set("User-Agent", "departures-board")
	if strings.HasPrefix(url, railAPI) {
		req.Header.Set("x-apikey", railKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return v, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return v, fmt.Errorf("%s", resp.Status)
	}
	return v, json.NewDecoder(resp.Body).Decode(&v)
}

// loaded carries a fetch result; gen drops replies for screens already left.
type loaded struct {
	gen int
	v   any
	err error
}

type (
	tickMsg    struct{}
	refreshMsg int
)

func fetch[T any](gen int, url string) tea.Cmd {
	return func() tea.Msg {
		v, err := get[T](url)
		return loaded{gen, v, err}
	}
}

func tick() tea.Cmd {
	return tea.Tick(125*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

type screen int

const (
	pickMode screen = iota
	pickLine
	pickStop
	board
	mapScreen
	disruptions
)

type model struct {
	w, h      int
	screen    screen
	query     string
	sel       int
	loading   bool
	err       error
	gen       int
	mode      string
	lines     []line
	stops     []stop
	line      line
	stop      stop
	platform  int
	arrivals  []arrival // TfL board
	fetchedAt time.Time
	services  []railService // National Rail board and map
	route     routeSeq      // TfL map
	routeLine string        // line route belongs to
	schema    schematic     // route drawn for this stop
	panX      int
	panY      int
	traffic   []arrival // TfL map: every prediction on the line
	trafficAt time.Time
	status    []lineStatus // TfL line status: this line on the board, all lines on disruptions
	notes     []string     // National Rail station messages
	scroll    int          // disruptions screen
}

func (m model) Init() tea.Cmd { return tick() }

func (m model) labels() []string {
	var l []string
	switch m.screen {
	case pickMode:
		l = modes
	case pickLine:
		for _, x := range m.lines {
			l = append(l, x.Name)
		}
	case pickStop:
		for _, x := range m.stops {
			l = append(l, x.label())
		}
	}
	return l
}

// shown returns indexes of labels matching the query, prefix matches first.
func (m model) shown() []int {
	q := strings.ToLower(m.query)
	var pre, rest []int
	for i, l := range m.labels() {
		l = strings.ToLower(l)
		if strings.HasPrefix(l, q) {
			pre = append(pre, i)
		} else if strings.Contains(l, q) {
			rest = append(rest, i)
		}
	}
	return append(pre, rest...)
}

func (m model) rows() []row {
	var rows []row
	if m.mode == nationalRail {
		for _, s := range m.services {
			rows = append(rows, s.row())
		}
		return rows
	}
	for _, d := range departures(m.arrivals, time.Since(m.fetchedAt)) {
		eta := "Due"
		if d.secs >= 60 {
			eta = fmt.Sprintf("%d min", int(d.secs/60))
		}
		rows = append(rows, row{short(d.DestinationName), eta, d.PlatformName, d.CurrentLocation})
	}
	return rows
}

func platforms(rows []row) []string {
	seen := map[string]bool{}
	p := []string{""}
	for _, r := range rows {
		if !seen[r.platform] {
			seen[r.platform] = true
			p = append(p, r.platform)
		}
	}
	sort.Strings(p[1:])
	return p
}

// live fetches whatever the current board, map or disruptions screen shows.
func (m model) live() tea.Cmd {
	switch {
	case m.screen == disruptions:
		return fetchStatus(m.gen)
	case m.mode == nationalRail:
		return fetchRail(m.gen, m.stop.NaptanID)
	case m.screen == mapScreen:
		return fetch[[]arrival](m.gen, api+"/Line/"+m.line.ID+"/Arrivals")
	default:
		return fetchBoard(m.gen, m.line.ID, m.stop.NaptanID)
	}
}

// show switches to the board or map and starts its fetches.
func (m model) show(s screen) (tea.Model, tea.Cmd) {
	m.screen, m.err, m.scroll, m.panX, m.panY = s, nil, 0, 0, 0
	m.gen++
	if s == mapScreen && m.routeLine == m.line.ID && m.schema.here != m.stop.NaptanID {
		m.schema = buildSchematic(m.route, m.stop.NaptanID, m.mode == "bus")
	}
	if m.mode == nationalRail && railKey == "" {
		m.err = errNoKey
		return m, nil
	}
	m.loading = true
	cmd := m.live()
	if s == mapScreen && m.mode != nationalRail && m.routeLine != m.line.ID {
		cmd = tea.Batch(cmd, fetch[routeSeq](m.gen, api+"/Line/"+m.line.ID+"/Route/Sequence/all"))
	}
	return m, cmd
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		return m, tick()
	case refreshMsg:
		if int(msg) == m.gen && m.screen >= board {
			return m, m.live()
		}
	case loaded:
		if msg.gen != m.gen {
			return m, nil
		}
		m.loading, m.err = false, msg.err
		gen := m.gen
		later := tea.Tick(refresh, func(time.Time) tea.Msg { return refreshMsg(gen) })
		switch v := msg.v.(type) {
		case []line:
			if msg.err == nil {
				m.lines, m.screen, m.query, m.sel = v, pickLine, "", 0
			}
		case []stop:
			if msg.err == nil {
				sort.Slice(v, func(i, j int) bool { return v[i].label() < v[j].label() })
				m.stops, m.screen, m.query, m.sel = v, pickStop, "", 0
			}
		case routeSeq:
			if msg.err == nil {
				m.route, m.routeLine = v, m.line.ID
				m.schema = buildSchematic(v, m.stop.NaptanID, m.mode == "bus")
			}
		case tflBoard:
			if msg.err == nil {
				m.arrivals, m.status, m.fetchedAt = v.arrivals, v.status, time.Now()
			}
			return m, later
		case []arrival:
			if msg.err == nil {
				m.traffic, m.trafficAt = v, time.Now()
			}
			return m, later
		case []lineStatus:
			if msg.err == nil {
				m.status = v
			}
			return m, later
		case railBoard:
			if msg.err == nil {
				m.services, m.notes = v.TrainServices, v.notes()
			}
			return m, later
		}
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		return m, tea.Quit
	}
	if m.screen == mapScreen {
		switch k {
		case "q":
			return m, tea.Quit
		case "m", "b", "esc":
			return m.show(board)
		case "left":
			m.panX -= m.w / 4
		case "right":
			m.panX += m.w / 4
		case "up":
			m.panY -= m.h / 4
		case "down":
			m.panY += m.h / 4
		}
		// Forget panning past the edge so the way back is immediate.
		ox, oy := m.mapOrigin()
		m.panX, m.panY = ox-(m.schema.at[m.schema.here].x-m.w/2), oy-(m.schema.at[m.schema.here].y-max(1, m.h-2)/2)
		return m, nil
	}
	if m.screen == disruptions {
		switch k {
		case "q":
			return m, tea.Quit
		case "b", "esc":
			m.screen, m.err = pickMode, nil
			m.gen++
		case "up":
			m.scroll = max(0, m.scroll-1)
		case "down":
			m.scroll++
			m.scroll = m.disruptionsTop()
		}
		return m, nil
	}
	if m.screen == board {
		switch k {
		case "q":
			return m, tea.Quit
		case "b", "esc":
			m.screen, m.err = pickStop, nil
			m.gen++
		case "p":
			m.platform++
		case "m":
			return m.show(mapScreen)
		}
		return m, nil
	}
	if m.loading {
		if k == "esc" {
			m.loading = false
			m.gen++
		}
		return m, nil
	}

	shown := m.shown()
	switch k {
	case "esc":
		switch {
		case m.screen == pickMode:
			return m, tea.Quit
		case m.screen == pickStop && m.mode == nationalRail:
			m.screen = pickMode
		default:
			m.screen--
		}
		m.query, m.sel, m.err = "", 0, nil
	case "up":
		m.sel = max(0, m.sel-1)
	case "down":
		m.sel = min(len(shown)-1, m.sel+1)
	case "backspace":
		if r := []rune(m.query); len(r) > 0 {
			m.query, m.sel = string(r[:len(r)-1]), 0
		}
	case "enter":
		if len(shown) == 0 {
			return m, nil
		}
		i := shown[m.sel]
		m.gen++
		m.err = nil
		switch m.screen {
		case pickMode:
			m.mode = modes[i]
			if m.mode == disrupted {
				return m.show(disruptions)
			}
			if m.mode == nationalRail {
				m.line = line{nationalRail, "National Rail"}
				m.stops, m.screen, m.query, m.sel = railStations, pickStop, "", 0
				return m, nil
			}
			m.loading = true
			return m, fetch[[]line](m.gen, api+"/Line/Mode/"+m.mode)
		case pickLine:
			m.line, m.loading = m.lines[i], true
			return m, fetch[[]stop](m.gen, api+"/Line/"+m.line.ID+"/StopPoints")
		case pickStop:
			m.stop, m.arrivals, m.services, m.notes, m.platform = m.stops[i], nil, nil, nil, 0
			return m.show(board)
		}
	default:
		if msg.Text != "" {
			m.query, m.sel = m.query+msg.Text, 0
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	var s string
	switch {
	case m.w == 0:
	case m.screen == board:
		s = m.boardView()
	case m.screen == mapScreen:
		s = m.mapView()
	case m.screen == disruptions:
		s = m.disruptionsView()
	default:
		s = m.pickView()
	}
	v := tea.NewView(s)
	v.AltScreen = true
	v.BackgroundColor = color.Black
	if m.screen == mapScreen && m.mode != nationalRail {
		v.BackgroundColor = color.White // the tube map is on paper
	}
	v.WindowTitle = "Departures"
	return v
}

func (m model) pickView() string {
	title := [...]string{"Mode", "Line", "Stop"}[m.screen]
	out := []string{lit.Render(" " + title + ": " + m.query + "_")}
	labels, shown := m.labels(), m.shown()
	visible := max(1, m.h-3)
	top := max(0, m.sel-visible+1)
	for i := top; i < len(shown) && i < top+visible; i++ {
		style := lit
		if i == m.sel {
			style = picked
		}
		out = append(out, " "+style.Render(fit(labels[shown[i]], m.w-2)))
	}
	for len(out) < m.h-1 {
		out = append(out, "")
	}
	help := "type to filter · ↑↓ move · enter select · esc back"
	if m.loading {
		help = "Loading…"
	} else if m.err != nil {
		help = "Failed: " + m.err.Error()
	}
	return strings.Join(append(out, dim.Render(" "+help)), "\n")
}

// frame wraps board or map content in the bordered box with a footer.
func (m model) frame(content []string, footer string) string {
	if m.err != nil && m.err != errNoKey {
		footer += " · " + m.err.Error()
	}
	return box.Width(m.w).Render(strings.Join(content, "\n")) + "\n" + dim.Render(fit(footer, m.w))
}

func (m model) boardView() string {
	iw, ih := m.w-4, m.h-3 // inside border+padding; one line kept for the footer
	all := m.rows()
	plats := platforms(all)
	platform := plats[m.platform%len(plats)]
	var rows []row
	for _, r := range all {
		if platform == "" || r.platform == platform {
			rows = append(rows, r)
		}
	}

	clock := dots(time.Now().Format("15:04:05"))
	big := len(clock[0])*2 <= iw && ih >= 12
	clockH := 1
	if big {
		clockH = 5
	}
	rowsH := max(0, ih-4-clockH) // title, blank, ticker, blank

	out := []string{fit(short(m.stop.CommonName)+" · "+m.line.Name+" · "+cmp.Or(platform, "All platforms"), iw), fit(m.notice(), iw)}
	for i, r := range rows {
		if i == rowsH {
			break
		}
		left := r.left
		if m.mode != nationalRail {
			left = fmt.Sprintf("%d  %s", i+1, left)
		}
		left = fit(left, iw-lipgloss.Width(r.right)-1)
		out = append(out, left+strings.Repeat(" ", max(1, iw-lipgloss.Width(left)-lipgloss.Width(r.right)))+r.right)
	}
	switch {
	case m.err == errNoKey:
		out = append(out, errNoKey.Error()+".", "", "Get a free key at raildata.org.uk: subscribe to", "“Live Departure Board”, copy the consumer key,", "set NATIONAL_RAIL_KEY to it and restart.")
	case len(rows) == 0 && m.loading:
		out = append(out, "Loading…")
	case len(rows) == 0 && m.err != nil:
		out = append(out, "Failed: "+m.err.Error())
	case len(rows) == 0:
		out = append(out, "No departures")
	}
	for len(out) < 2+rowsH {
		out = append(out, "")
	}
	out = out[:2+rowsH]

	msg := m.troubleReason()
	if len(rows) > 0 {
		msg = strings.TrimSpace(msg + "   " + rows[0].ticker)
	}
	ticker := ""
	if msg != "" {
		pad := strings.Repeat(" ", iw)
		r := []rune(pad + msg + pad)
		off := int(time.Now().UnixMilli()/125) % (len(r) - iw)
		ticker = string(r[off : off+iw])
	}
	out = append(out, ticker, "")

	if big {
		for _, row := range clock {
			var b strings.Builder
			for i, px := range row {
				if i > 0 {
					b.WriteString(" ")
				}
				if px == '#' {
					b.WriteString(lit.Render("●"))
				} else {
					b.WriteString(unlit.Render("·"))
				}
			}
			out = append(out, lipgloss.PlaceHorizontal(iw, lipgloss.Center, b.String()))
		}
	} else {
		out = append(out, lipgloss.PlaceHorizontal(iw, lipgloss.Center, time.Now().Format("15:04:05")))
	}
	return m.frame(out, " p platform · m map · b back · q quit")
}

// fit truncates s to n runes.
func fit(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:max(0, n)])
	}
	return s
}

func main() {
	if _, err := tea.NewProgram(model{}).Run(); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}

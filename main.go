// Command departures is a dot-matrix departure board for London trains and
// buses, using the TfL API (no key needed).
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
	api     = "https://api.tfl.gov.uk"
	refresh = 30 * time.Second // keyless TfL allows 50 requests/min
)

var modes = []string{"tube", "elizabeth-line", "overground", "dlr", "tram", "bus"}

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

var suffix = regexp.MustCompile(`( (Underground|DLR|Rail|Tram))? (Station|Stop)$`)

// short turns "Brixton Underground Station" into "Brixton".
func short(name string) string { return suffix.ReplaceAllString(name, "") }

type line struct{ ID, Name string }

type stop struct {
	NaptanID, CommonName, Indicator string
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
	DestinationName, PlatformName, CurrentLocation string
	TimeToStation                                  float64
}

type departure struct {
	secs float64
	arrival
}

// departures lists arrivals soonest first, counted down by age since the
// fetch, on one platform ("" for all). Trains gone >30s are dropped.
func departures(arrivals []arrival, age time.Duration, platform string) []departure {
	var out []departure
	for _, a := range arrivals {
		d := departure{a.TimeToStation - age.Seconds(), a}
		if (platform == "" || a.PlatformName == platform) && d.secs > -30 {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].secs < out[j].secs })
	return out
}

var client = &http.Client{Timeout: 10 * time.Second}

func get[T any](path string) (T, error) {
	var v T
	req, err := http.NewRequest("GET", api+path, nil)
	if err != nil {
		return v, err
	}
	req.Header.Set("User-Agent", "departures-board")
	resp, err := client.Do(req)
	if err != nil {
		return v, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return v, fmt.Errorf("TfL: %s", resp.Status)
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

func fetch[T any](gen int, path string) tea.Cmd {
	return func() tea.Msg {
		v, err := get[T](path)
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
	arrivals  []arrival
	fetchedAt time.Time
	platform  int
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

func (m model) platforms() []string {
	seen := map[string]bool{}
	p := []string{""}
	for _, a := range m.arrivals {
		if !seen[a.PlatformName] {
			seen[a.PlatformName] = true
			p = append(p, a.PlatformName)
		}
	}
	sort.Strings(p[1:])
	return p
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		return m, tick()
	case refreshMsg:
		if int(msg) == m.gen && m.screen == board {
			return m, fetch[[]arrival](m.gen, "/Line/"+m.line.ID+"/Arrivals/"+m.stop.NaptanID)
		}
	case loaded:
		if msg.gen != m.gen {
			return m, nil
		}
		m.loading, m.err = false, msg.err
		if m.screen == board {
			if msg.err == nil {
				m.arrivals, m.fetchedAt = msg.v.([]arrival), time.Now()
			}
			gen := m.gen
			return m, tea.Tick(refresh, func(time.Time) tea.Msg { return refreshMsg(gen) })
		}
		if msg.err != nil {
			return m, nil
		}
		m.query, m.sel = "", 0
		switch v := msg.v.(type) {
		case []line:
			m.lines, m.screen = v, pickLine
		case []stop:
			sort.Slice(v, func(i, j int) bool { return v[i].label() < v[j].label() })
			m.stops, m.screen = v, pickStop
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
	if m.screen == board {
		switch k {
		case "q":
			return m, tea.Quit
		case "b", "esc":
			m.screen, m.err = pickStop, nil
			m.gen++
		case "p":
			m.platform++
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
		if m.screen == pickMode {
			return m, tea.Quit
		}
		m.screen, m.query, m.sel, m.err = m.screen-1, "", 0, nil
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
		m.loading, m.err = true, nil
		switch m.screen {
		case pickMode:
			m.mode = modes[i]
			return m, fetch[[]line](m.gen, "/Line/Mode/"+m.mode)
		case pickLine:
			m.line = m.lines[i]
			return m, fetch[[]stop](m.gen, "/Line/"+m.line.ID+"/StopPoints")
		case pickStop:
			m.stop, m.screen, m.arrivals, m.platform = m.stops[i], board, nil, 0
			return m, fetch[[]arrival](m.gen, "/Line/"+m.line.ID+"/Arrivals/"+m.stop.NaptanID)
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
	if m.w > 0 {
		if m.screen == board {
			s = m.boardView()
		} else {
			s = m.pickView()
		}
	}
	v := tea.NewView(s)
	v.AltScreen = true
	v.BackgroundColor = color.Black
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

func (m model) boardView() string {
	iw, ih := m.w-4, m.h-3 // inside border+padding; one line kept for the footer
	platforms := m.platforms()
	platform := platforms[m.platform%len(platforms)]
	deps := departures(m.arrivals, time.Since(m.fetchedAt), platform)

	clock := dots(time.Now().Format("15:04:05"))
	big := len(clock[0])*2 <= iw && ih >= 12
	clockH := 1
	if big {
		clockH = 5
	}
	rowsH := max(0, ih-4-clockH) // title, blank, ticker, blank

	out := []string{fit(short(m.stop.CommonName)+" · "+m.line.Name+" · "+cmp.Or(platform, "All platforms"), iw), ""}
	for i, d := range deps {
		if i == rowsH {
			break
		}
		eta := "Due"
		if d.secs >= 60 {
			eta = fmt.Sprintf("%d min", int(d.secs/60))
		}
		left := fit(fmt.Sprintf("%d  %s", i+1, short(d.DestinationName)), iw-10)
		out = append(out, left+strings.Repeat(" ", max(1, iw-lipgloss.Width(left)-len(eta)))+eta)
	}
	if len(deps) == 0 && m.loading {
		out = append(out, "Loading…")
	} else if len(deps) == 0 {
		out = append(out, "No departures")
	}
	for len(out) < 2+rowsH {
		out = append(out, "")
	}

	ticker := ""
	if len(deps) > 0 && deps[0].CurrentLocation != "" {
		pad := strings.Repeat(" ", iw)
		r := []rune(pad + deps[0].CurrentLocation + pad)
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

	footer := " p platform · b back · q quit"
	if m.err != nil {
		footer += " · no signal, retrying"
	}
	return box.Width(m.w).Render(strings.Join(out, "\n")) + "\n" + dim.Render(footer)
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

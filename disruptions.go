package main

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// railModes are the TfL modes the disruptions screen always covers; a bus
// route is added when that's what you're looking at.
const railModes = "tube,elizabeth-line,overground,dlr,tram"

type lineStatus struct {
	ID, Name     string
	LineStatuses []struct {
		StatusSeverity            int
		StatusSeverityDescription string
		Reason                    string
	}
}

// fine is Good Service (10) or No Issues (18) on every status.
func (l lineStatus) fine() bool {
	for _, s := range l.LineStatuses {
		if s.StatusSeverity != 10 && s.StatusSeverity != 18 {
			return false
		}
	}
	return true
}

func (l lineStatus) summary() string {
	var d []string
	for _, s := range l.LineStatuses {
		if !slices.Contains(d, s.StatusSeverityDescription) {
			d = append(d, s.StatusSeverityDescription)
		}
	}
	return strings.Join(d, " · ")
}

// tflBoard is a stop's arrivals plus its line's status.
type tflBoard struct {
	arrivals []arrival
	status   []lineStatus
}

func fetchBoard(gen int, line, stop string) tea.Cmd {
	return func() tea.Msg {
		a, err := get[[]arrival](api + "/Line/" + line + "/Arrivals/" + stop)
		if err != nil {
			return loaded{gen, tflBoard{}, err}
		}
		s, _ := get[[]lineStatus](api + "/Line/" + line + "/Status") // board still works without status
		return loaded{gen, tflBoard{a, s}, nil}
	}
}

func fetchStatus(gen int, mode, line string) tea.Cmd {
	return func() tea.Msg {
		all, err := get[[]lineStatus](api + "/Line/Mode/" + railModes + "/Status")
		if err == nil && !slices.Contains(strings.Split(railModes, ","), mode) {
			var own []lineStatus
			own, err = get[[]lineStatus](api + "/Line/" + line + "/Status")
			all = append(own, all...)
		}
		return loaded{gen, all, err}
	}
}

// trouble is this line's status when it isn't good service.
func (m model) trouble() (lineStatus, bool) {
	for _, l := range m.status {
		if l.ID == m.line.ID && !l.fine() {
			return l, true
		}
	}
	return lineStatus{}, false
}

// notice is the board's warning line: this line's status or station messages.
func (m model) notice() string {
	if l, ok := m.trouble(); ok {
		return "▲ " + l.summary() + " · d for details"
	}
	if n := len(m.notes); n > 0 {
		return fmt.Sprintf("▲ Station messages (%d) · d to read", n)
	}
	return ""
}

// troubleReason is the text the board scrolls when the line has problems.
func (m model) troubleReason() string {
	l, _ := m.trouble()
	var r []string
	for _, s := range l.LineStatuses {
		if s.Reason != "" && !slices.Contains(r, s.Reason) {
			r = append(r, strings.TrimSpace(s.Reason))
		}
	}
	return strings.Join(r, "   ")
}

// disruptionLines is the disruptions screen's title and scrollable body.
func (m model) disruptionLines(iw int) (string, []string) {
	wrap := lipgloss.NewStyle().Width(iw - 2)
	para := func(out []string, text string) []string {
		for _, l := range strings.Split(wrap.Render(text), "\n") {
			out = append(out, dim.Render("  "+l))
		}
		return out
	}
	both := func(left, right string) string {
		left = fit(left, iw-lipgloss.Width(right)-1)
		return left + strings.Repeat(" ", max(1, iw-lipgloss.Width(left)-lipgloss.Width(right))) + right
	}

	var title string
	var body []string
	if m.mode == nationalRail {
		title = short(m.stop.CommonName) + " · disruptions"
		for _, n := range m.notes {
			body = append(para(body, n), "")
		}
		for _, s := range m.services {
			if reason := cmp.Or(s.CancelReason, s.DelayReason); reason != "" {
				r := s.row()
				body = append(para(append(body, both(r.left, s.Etd)), reason), "")
			}
		}
	} else {
		title = "Service status"
		// Your line first, then problems, then good service.
		lines := slices.Clone(m.status)
		rank := func(l lineStatus) int {
			switch {
			case l.ID == m.line.ID:
				return 0
			case !l.fine():
				return 1
			}
			return 2
		}
		sort.SliceStable(lines, func(i, j int) bool { return rank(lines[i]) < rank(lines[j]) })
		for _, l := range lines {
			body = append(body, both(l.Name, l.summary()))
			var seen []string
			for _, s := range l.LineStatuses {
				if s.Reason != "" && !slices.Contains(seen, s.Reason) {
					seen = append(seen, s.Reason)
					body = para(body, s.Reason)
				}
			}
		}
	}
	switch {
	case len(body) == 0 && m.err == errNoKey:
		body = []string{errNoKey.Error()}
	case len(body) == 0 && m.loading:
		body = []string{"Loading…"}
	case len(body) == 0 && m.err != nil:
		body = []string{"Failed: " + m.err.Error()}
	case len(body) == 0:
		body = []string{"No disruption reported."}
	}
	return title, body
}

// disruptionsTop clamps the scroll position to the body.
func (m model) disruptionsTop() int {
	_, body := m.disruptionLines(m.w - 4)
	return min(m.scroll, max(0, len(body)-max(1, m.h-5)))
}

func (m model) disruptionsView() string {
	iw, ih := m.w-4, m.h-3
	title, body := m.disruptionLines(iw)
	visible := max(1, ih-2)
	top := m.disruptionsTop()
	out := append([]string{fit(title, iw), ""}, body[top:min(len(body), top+visible)]...)
	for len(out) < ih {
		out = append(out, "")
	}
	return m.frame(out, " ↑↓ scroll · d board · q quit")
}

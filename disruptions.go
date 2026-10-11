package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// railModes are the TfL modes the disruptions screen covers.
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

func fetchStatus(gen int) tea.Cmd {
	return fetch[[]lineStatus](gen, api+"/Line/Mode/"+railModes+"/Status")
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
		return "▲ " + l.summary()
	}
	if n := len(m.notes); n > 0 {
		return fmt.Sprintf("▲ Station messages (%d)", n)
	}
	return ""
}

// troubleReason is the text the board scrolls for line problems or station messages.
func (m model) troubleReason() string {
	r := slices.Clone(m.notes)
	l, _ := m.trouble()
	for _, s := range l.LineStatuses {
		if s.Reason != "" && !slices.Contains(r, strings.TrimSpace(s.Reason)) {
			r = append(r, strings.TrimSpace(s.Reason))
		}
	}
	return strings.Join(r, "   ")
}

// disruptionLines is the disruptions screen's scrollable body: problems
// first with their reasons, then good service.
func (m model) disruptionLines(iw int) []string {
	wrap := lipgloss.NewStyle().Width(iw - 2)
	lines := slices.Clone(m.status)
	sort.SliceStable(lines, func(i, j int) bool { return !lines[i].fine() && lines[j].fine() })
	var body []string
	for _, l := range lines {
		right := l.summary()
		left := fit(l.Name, iw-lipgloss.Width(right)-1)
		body = append(body, left+strings.Repeat(" ", max(1, iw-lipgloss.Width(left)-lipgloss.Width(right)))+right)
		var seen []string
		for _, s := range l.LineStatuses {
			if s.Reason != "" && !slices.Contains(seen, s.Reason) {
				seen = append(seen, s.Reason)
				for _, w := range strings.Split(wrap.Render(s.Reason), "\n") {
					body = append(body, dim.Render("  "+w))
				}
			}
		}
	}
	switch {
	case len(body) == 0 && m.loading:
		body = []string{"Loading…"}
	case len(body) == 0 && m.err != nil:
		body = []string{"Failed: " + m.err.Error()}
	}
	return body
}

// disruptionsTop clamps the scroll position to the body.
func (m model) disruptionsTop() int {
	return min(m.scroll, max(0, len(m.disruptionLines(m.w-4))-max(1, m.h-5)))
}

func (m model) disruptionsView() string {
	iw, ih := m.w-4, m.h-3
	body, top := m.disruptionLines(iw), m.disruptionsTop()
	out := append([]string{"Service status · tube, Elizabeth line, Overground, DLR, tram", ""}, body[top:min(len(body), top+ih-2)]...)
	out[0] = fit(out[0], iw)
	for len(out) < ih {
		out = append(out, "")
	}
	return m.frame(out, " ↑↓ scroll · esc back · q quit")
}

package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func TestBoardLogic(t *testing.T) {
	for in, want := range map[string]string{
		"Brixton Underground Station": "Brixton",
		"Canary Wharf DLR Station":    "Canary Wharf",
		"Bank":                        "Bank",
	} {
		if got := short(in); got != want {
			t.Errorf("short(%q) = %q, want %q", in, got, want)
		}
	}

	arr := []arrival{{TimeToStation: 300}, {TimeToStation: 60}, {TimeToStation: 20}}
	if d := departures(arr, 0); len(d) != 3 || d[0].secs != 20 || d[2].secs != 300 {
		t.Errorf("not sorted: %+v", d)
	}
	if d := departures(arr, 60*time.Second); len(d) != 2 || d[0].secs != 0 { // 20s train departed >30s ago
		t.Errorf("after 60s: %+v", d)
	}

	for _, row := range dots("12:34") {
		if len(row) != 17 { // 4 digits*3 + colon + 4 gaps
			t.Errorf("dots row width %d, want 17", len(row))
		}
	}
}

func TestRail(t *testing.T) {
	if s := railByCRS["KGX"]; s.CommonName == "" || s.Lat < 51 {
		t.Fatalf("KGX missing from embedded stations: %+v", s)
	}
	// Shape of a Rail Data Marketplace GetDepBoardWithDetails response.
	var b railBoard
	err := json.Unmarshal([]byte(`{"trainServices":[{"std":"14:15","etd":"On time","platform":"1",
		"destination":[{"locationName":"Bristol Temple Meads","crs":"BRI"}],
		"previousCallingPoints":[{"callingPoint":[{"locationName":"Reading","crs":"RDG","st":"13:50","at":"On time"}]}],
		"subsequentCallingPoints":[{"callingPoint":[{"locationName":"Swindon","crs":"SWI","st":"15:05","et":"On time"},
			{"locationName":"Bristol Temple Meads","crs":"BRI","st":"15:45","et":"On time"}]}]}]}`), &b)
	if err != nil || len(b.TrainServices) != 1 {
		t.Fatalf("decode: %v %+v", err, b)
	}
	r := b.TrainServices[0].row()
	want := row{"14:15  Bristol Temple Meads", "Plat 1  On time", "1", "Calling at: Swindon and Bristol Temple Meads."}
	if r != want {
		t.Errorf("row = %+v, want %+v", r, want)
	}
	m := model{mode: nationalRail, stop: railByCRS["PAD"], services: b.TrainServices}
	if routes, trains := m.railMap(); len(routes) != 1 || len(routes[0]) != 4 || len(trains) != 1 {
		t.Errorf("railMap: %d routes, %v, %d trains", len(routes), routes, len(trains))
	}
}

func TestMap(t *testing.T) {
	var r routeSeq
	r.OrderedLineRoutes = []struct{ NaptanIDs []string }{{[]string{"A", "B", "C"}}, {[]string{"C", "B", "A"}}}
	for _, c := range []struct{ next, dest, want string }{
		{"B", "C", "A"}, // heading C: came from A
		{"B", "A", "C"}, // heading A: came from C
		{"B", "", "A"},  // no destination: first route
		{"A", "C", ""},  // at the start
	} {
		if got := prevStop(r, c.next, c.dest); got != c.want {
			t.Errorf("prevStop(%s→%s) = %q, want %q", c.next, c.dest, got, c.want)
		}
	}

	rows := drawMap(30, 8, [][]pt{{{51.5, -0.2}, {51.6, 0}}}, []pt{{51.55, -0.1}}, pt{51.5, -0.2}, "Here")
	if len(rows) != 8 {
		t.Fatalf("drawMap rows %d, want 8", len(rows))
	}
	for i, row := range rows {
		if w := lipgloss.Width(row); w != 30 {
			t.Errorf("drawMap row %d width %d, want 30", i, w)
		}
	}
}

func TestDisruptions(t *testing.T) {
	var all []lineStatus
	err := json.Unmarshal([]byte(`[
		{"id":"victoria","name":"Victoria","lineStatuses":[{"statusSeverity":10,"statusSeverityDescription":"Good Service"}]},
		{"id":"central","name":"Central","lineStatuses":[{"statusSeverity":10,"statusSeverityDescription":"Good Service"}]},
		{"id":"piccadilly","name":"Piccadilly","lineStatuses":[
			{"statusSeverity":6,"statusSeverityDescription":"Severe Delays","reason":"Signal failure. "},
			{"statusSeverity":5,"statusSeverityDescription":"Part Closure","reason":"Engineering works."}]}]`), &all)
	if err != nil {
		t.Fatal(err)
	}
	if !all[0].fine() || all[2].fine() || all[2].summary() != "Severe Delays · Part Closure" {
		t.Errorf("fine/summary wrong: %+v", all)
	}

	m := model{w: 60, h: 20, status: all, line: line{ID: "central"}}
	if m.notice() != "" {
		t.Errorf("good service shouldn't warn: %q", m.notice())
	}
	body := m.disruptionLines(56)
	if !strings.HasPrefix(body[0], "Piccadilly") || !strings.HasPrefix(body[3], "Victoria") {
		t.Errorf("want problems (with reasons) first, then good service: %q", body)
	}
	m.line.ID = "piccadilly"
	if m.notice() != "▲ Severe Delays · Part Closure" || m.troubleReason() != "Signal failure.   Engineering works." {
		t.Errorf("notice %q reason %q", m.notice(), m.troubleReason())
	}

	if got := plain(`<p>Disruption at <a href="x">Reading</a> &amp; Slough.</p>`); got != "Disruption at Reading & Slough." {
		t.Errorf("plain = %q", got)
	}
	m = model{mode: nationalRail, notes: []string{"Lifts out of order."}}
	if m.notice() != "▲ Station messages (1)" || m.troubleReason() != "Lifts out of order." {
		t.Errorf("station messages: notice %q ticker %q", m.notice(), m.troubleReason())
	}
	s := railService{Std: "09:00", Etd: "Delayed", DelayReason: "This train has been delayed by a signalling fault"}
	if r := s.row(); r.ticker != "This train has been delayed by a signalling fault.  " {
		t.Errorf("delay reason ticker = %q", r.ticker)
	}
}

func TestSchematic(t *testing.T) {
	// A, B, C run west to east; D branches north from B.
	var r routeSeq
	if err := json.Unmarshal([]byte(`{
		"stopPointSequences":[{"stopPoint":[
			{"id":"A","name":"Alpha Underground Station","lat":51.50,"lon":-0.20,"lines":[{"id":"x"},{"id":"y"}]},
			{"id":"B","name":"Bravo Underground Station","lat":51.50,"lon":-0.15},
			{"id":"C","name":"Charlie Underground Station","lat":51.50,"lon":-0.10},
			{"id":"D","name":"Delta Underground Station","lat":51.56,"lon":-0.15}]}],
		"orderedLineRoutes":[{"naptanIds":["A","B","C"]},{"naptanIds":["D","B","A"]}]}`), &r); err != nil {
		t.Fatal(err)
	}
	s := buildSchematic(r, "B", false)
	text := ""
	for y := range s.h {
		for x := range s.w {
			text += string(s.cells[y*s.w+x].r)
		}
		text += "\n"
	}
	for _, name := range []string{"Alpha", "Bravo", "Charlie", "Delta", "◉", "○"} {
		if !strings.Contains(text, name) {
			t.Errorf("schematic missing %q:\n%s", name, text)
		}
	}
	p := s.paths[[2]string{"A", "B"}]
	if len(p) < 2 || p[0] != s.at["A"] || p[len(p)-1] != s.at["B"] || s.paths[[2]string{"B", "A"}][0] != s.at["B"] {
		t.Errorf("A→B path %v doesn't join A %v to B %v", p, s.at["A"], s.at["B"])
	}

	// A train due at B in 60s, then C 120s later: halfway from A to B, heading east.
	m := model{route: r, schema: s, trafficAt: time.Now(), traffic: []arrival{
		{VehicleID: "1", NaptanID: "B", DestinationNaptanID: "C", TimeToStation: 60},
		{VehicleID: "1", NaptanID: "C", DestinationNaptanID: "C", TimeToStation: 180},
	}}
	trains := m.trains()
	if len(trains) != 1 || trains[p[len(p)/2]] != '▶' {
		t.Errorf("want ▶ at %v, got %v", p[len(p)/2], trains)
	}
}

func TestThameslinkAndBuses(t *testing.T) {
	var ds []timedDeparture
	if err := json.Unmarshal([]byte(`[
		{"platformName":"Platform 4","destinationName":"Bedford Rail Station","departureStatus":"OnTime",
		 "scheduledTimeOfDeparture":"2026-10-11T10:28:00Z","estimatedTimeOfDeparture":"2026-10-11T10:28:00Z"},
		{"platformName":"Platform 3","destinationName":"Sutton (London) Rail Station","departureStatus":"Cancelled",
		 "scheduledTimeOfDeparture":"2026-10-11T10:16:00Z","cause":"Shortage of train crew"},
		{"platformName":"Platform 3","destinationName":"Three Bridges Rail Station","departureStatus":"Delayed",
		 "scheduledTimeOfDeparture":"2026-10-11T10:20:00Z","estimatedTimeOfDeparture":"2026-10-11T10:25:00Z"},
		{"platformName":"Platform 4","destinationName":"Gone Rail Station","departureStatus":"OnTime",
		 "scheduledTimeOfDeparture":"2026-10-11T10:00:00Z","estimatedTimeOfDeparture":"2026-10-11T10:00:00Z"}]`), &ds); err != nil {
		t.Fatal(err)
	}
	now, _ := time.Parse(time.RFC3339, "2026-10-11T10:15:30Z")
	rows := timedRows(ds, now)
	local := func(s string) string { tm, _ := time.Parse(time.RFC3339, s); return tm.Local().Format("15:04") }
	want := []row{
		{local("2026-10-11T10:16:00Z") + "  Sutton (London)", "Plat 3  Cancelled", "Platform 3", "Shortage of train crew"},
		{local("2026-10-11T10:20:00Z") + "  Three Bridges", "Plat 3  Exp " + local("2026-10-11T10:25:00Z"), "Platform 3", ""},
		{local("2026-10-11T10:28:00Z") + "  Bedford", "Plat 4  On time", "Platform 4", ""},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}

	m := model{screen: pickLine, mode: "bus", gen: 1}
	next, _ := m.Update(loaded{1, []stop{{NaptanID: "490G00004202", CommonName: "Abney Park", StopType: "NaptanOnstreetBusCoachStopPair"},
		{NaptanID: "490004202E", CommonName: "Abney Park", Indicator: "->E", StopType: "NaptanPublicBusCoachTram"}}, nil})
	if s := next.(model).stops; len(s) != 1 || s[0].NaptanID != "490004202E" {
		t.Errorf("bus stop pairs should be dropped: %+v", s)
	}
}

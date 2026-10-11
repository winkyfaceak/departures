package main

import (
	"encoding/json"
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

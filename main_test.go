package main

import (
	"testing"
	"time"
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

	arr := []arrival{{PlatformName: "A", TimeToStation: 300}, {PlatformName: "B", TimeToStation: 60}, {PlatformName: "A", TimeToStation: 20}}
	if d := departures(arr, 0, ""); len(d) != 3 || d[0].secs != 20 || d[2].secs != 300 {
		t.Errorf("all platforms not sorted: %+v", d)
	}
	// 20s train departed >30s ago, platform B filtered out
	if d := departures(arr, 60*time.Second, "A"); len(d) != 1 || d[0].secs != 240 {
		t.Errorf("platform A after 60s: %+v", d)
	}

	for _, row := range dots("12:34") {
		if len(row) != 17 { // 4 digits*3 + colon + 4 gaps
			t.Errorf("dots row width %d, want 17", len(row))
		}
	}
}

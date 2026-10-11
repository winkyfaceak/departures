package main

import (
	_ "embed"
	"encoding/csv"
	"errors"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// National Rail live departures from the Rail Data Marketplace (free key).
const railAPI = "https://api1.raildata.org.uk/1010-live-departure-board-dep1_2/LDBWS/api/20220120/GetDepBoardWithDetails/"

var (
	railKey  = os.Getenv("NATIONAL_RAIL_KEY")
	errNoKey = errors.New("National Rail needs NATIONAL_RAIL_KEY set")
)

// stations.csv is from github.com/davwheat/uk-railway-stations (ODbL-1.0).
//
//go:embed stations.csv
var stationsCSV string

// railStations lists every station by name; the indicator is its CRS code.
var railStations, railByCRS = func() ([]stop, map[string]stop) {
	recs, err := csv.NewReader(strings.NewReader(stationsCSV)).ReadAll()
	if err != nil {
		panic(err)
	}
	var list []stop
	by := map[string]stop{}
	for _, r := range recs[1:] { // stationName,lat,long,crsCode,...
		lat, _ := strconv.ParseFloat(r[1], 64)
		lon, _ := strconv.ParseFloat(r[2], 64)
		s := stop{NaptanID: r[3], CommonName: r[0], Indicator: r[3], Lat: lat, Lon: lon}
		list = append(list, s)
		by[r[3]] = s
	}
	return list, by
}()

type railBoard struct{ TrainServices []railService }

type railService struct {
	Std, Etd, Platform                             string
	Destination                                    []struct{ LocationName string }
	PreviousCallingPoints, SubsequentCallingPoints []struct{ CallingPoint []callingPoint }
}

type callingPoint struct{ LocationName, Crs, At string }

func fetchRail(gen int, crs string) tea.Cmd {
	return fetch[railBoard](gen, railAPI+crs+"?numRows=10")
}

// calls returns the main calling points (splits and joins list extra ones after).
func calls(cp []struct{ CallingPoint []callingPoint }) []callingPoint {
	if len(cp) == 0 {
		return nil
	}
	return cp[0].CallingPoint
}

func (s railService) row() row {
	var dest []string
	for _, d := range s.Destination {
		dest = append(dest, d.LocationName)
	}
	right := s.Etd
	if s.Platform != "" {
		right = "Plat " + s.Platform + "  " + s.Etd
	}
	var stops []string
	for _, c := range calls(s.SubsequentCallingPoints) {
		stops = append(stops, c.LocationName)
	}
	ticker := ""
	if n := len(stops); n == 1 {
		ticker = "Calling at: " + stops[0] + " only."
	} else if n > 1 {
		ticker = "Calling at: " + strings.Join(stops[:n-1], ", ") + " and " + stops[n-1] + "."
	}
	return row{s.Std + "  " + strings.Join(dest, " & "), right, s.Platform, ticker}
}

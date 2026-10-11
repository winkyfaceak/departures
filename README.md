# departures

A dot-matrix departure board in your terminal for London trains and buses, using live [TfL](https://api.tfl.gov.uk) data (no API key needed), and for every National Rail station in Great Britain.

```
╭──────────────────────────────────────────────────────────────────────────────╮
│ Oxford Circus · Victoria · Northbound - Platform 6                           │
│                                                                              │
│ 1  Walthamstow Central                                                 4 min │
│ 2  Walthamstow Central                                                 7 min │
│                                                                              │
│      Between Pimlico and Victoria                                            │
│                                                                              │
│            ● ● ● · · · ● · · · ● ● ● · ● ● ● · · · ● ● ● · ● ● ●             │
│            ● · ● · · · ● · ● · ● · ● · · · ● · ● · ● · · · ● · ●             │
│            ● · ● · · · ● · · · ● · ● · ● ● ● · · · ● ● ● · ● ● ●             │
│            ● · ● · · · ● · ● · ● · ● · · · ● · ● · · · ● · ● · ●             │
│            ● ● ● · · · ● · · · ● ● ● · ● ● ● · · · ● ● ● · ● ● ●             │
╰──────────────────────────────────────────────────────────────────────────────╯
 p platform · b back · q quit
```

## Install

```sh
go install github.com/winkyfaceak/departures@latest
```

## Use

Run `departures`, then pick a mode (tube, Elizabeth line, Overground, DLR, tram, bus, national-rail), a line, and a station or stop. The first menu entry, **disruptions**, shows the status of every tube, Elizabeth line, Overground, DLR and tram line, problems first with TfL's reasons (`↑↓` scrolls, Esc goes back). National Rail skips the line and goes straight to stations; type a name or a code like `KGX`. Type to filter, Enter to select, Esc to go back.

On the board: `p` cycles platforms, `m` opens the map, `b` goes back, `q` quits. Arrivals refresh every 30 seconds. If your line or bus route has delays or closures, the board warns you under the title and scrolls the reason; on National Rail it scrolls station messages and why a train is delayed or cancelled.

The map (`m`) draws your line like the tube map: in its own colour on white, straight runs and 45° bends, every stop ticked and named, interchanges circled, your station highlighted. Every train or bus is an arrow on the track showing which way it's heading, placed between its last and next stop from its live predictions and moving as the countdown runs. Arrow keys pan; `m` goes back to the board. For National Rail it draws the routes of the next departures through the station in braille dots, with each train at the last stop it reported.

### National Rail key

National Rail times need a free key from the [Rail Data Marketplace](https://raildata.org.uk): sign up, subscribe to **Live Departure Board**, and copy the consumer key from its Specification tab. Then:

```sh
export NATIONAL_RAIL_KEY=your-key     # bash/zsh
set -Ux NATIONAL_RAIL_KEY your-key    # fish
```

## Data

Live times from [TfL](https://api.tfl.gov.uk) and the Rail Data Marketplace. Station list from [davwheat/uk-railway-stations](https://github.com/davwheat/uk-railway-stations), under the [ODbL](https://opendatacommons.org/licenses/odbl/1-0/).

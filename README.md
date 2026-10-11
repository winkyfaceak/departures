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

Run `departures`, then pick a mode (tube, Elizabeth line, Overground, DLR, tram, bus, national-rail), a line, and a station or stop. National Rail skips the line and goes straight to stations; type a name or a code like `KGX`. Type to filter, Enter to select, Esc to go back.

On the board: `p` cycles platforms, `m` opens the map, `b` goes back, `q` quits. Arrivals refresh every 30 seconds.

The map draws the line in braille dots with every train or bus on it (●) and your stop (◆). For National Rail it draws the routes of the next departures through the station, with each train at the last stop it reported. `m` goes back to the board.

### National Rail key

National Rail times need a free key from the [Rail Data Marketplace](https://raildata.org.uk): sign up, subscribe to **Live Departure Board**, and copy the consumer key from its Specification tab. Then:

```sh
export NATIONAL_RAIL_KEY=your-key     # bash/zsh
set -Ux NATIONAL_RAIL_KEY your-key    # fish
```

## Data

Live times from [TfL](https://api.tfl.gov.uk) and the Rail Data Marketplace. Station list from [davwheat/uk-railway-stations](https://github.com/davwheat/uk-railway-stations), under the [ODbL](https://opendatacommons.org/licenses/odbl/1-0/).

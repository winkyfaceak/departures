# departures

A dot-matrix departure board in your terminal for London trains and buses, using live [TfL](https://api.tfl.gov.uk) data. No API key needed.

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

Run `departures`, then pick a mode (tube, Elizabeth line, Overground, DLR, tram, bus), a line, and a station or stop. Type to filter, Enter to select, Esc to go back.

On the board: `p` cycles platforms, `b` goes back, `q` quits. Arrivals refresh every 30 seconds.

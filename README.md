# f1mcp

An [MCP](https://modelcontextprotocol.io) server for Formula 1 data, written in Go and packaged as a Docker image.

It combines two public F1 APIs behind one consistent set of tools, so the model asks for things like "the 2024 Monaco race" and never has to know which API answered or how their IDs and formats differ.

> **Status: work in progress.** The season tools work; session detail (2023 onward) and track and visual tools are next.

## Tools

| Tool | What it answers |
|---|---|
| `get_schedule` | A season's calendar: rounds, circuits, session start times (UTC), sprint weekends |
| `get_session_results` | A session's classification: race and sprint results, qualifying Q1–Q3, practice best laps |
| `get_standings` | Drivers' or teams' championship after any round |
| `list_drivers` | Drivers in a season or event, with team, number, and from 2023 team colour and a headshot link |

Events are chosen by `year` and `round`: a number, `last`, `next`, or part of the event, circuit, city or country name (`"Monaco"`, `"Spa"`). Times and gaps are in seconds.

## What it covers

| Period | Data |
|---|---|
| 2023 onward | Full detail: results, standings, laps and sectors, pit stops, tyre stints, weather, race control, gaps, positions, car telemetry, track maps with car positions |
| Before 2023 | Simple history: schedule, results, standings, drivers and teams |

### Design principles

- **One interface, two backends.** Tools take `year`, `round` and a session name (`race`, `qualifying`, `sprint`, `sprint_qualifying`, `fp1`–`fp3`). The server maps these to each API's own IDs and picks the backend that has the data.
- **Standardised output.** Durations are seconds with decimals (`100.177`), timestamps are UTC RFC 3339 with milliseconds, and teams have one canonical name across both APIs.
- **Trimmed results.** Tools return the fields that are useful, not every field the APIs expose.

### Track maps

**[Browse the track map gallery](https://mariuspot.github.io/f1mcp/)**: every circuit and corner, rebuilt by GitHub Actions whenever the track data changes.

Circuit outlines, corners, start/finish and sector lines are generated ahead of time by `cmd/trackgen` and embedded in the binary. Maps are drawn in an F1 style: asphalt with sector-coloured edges, kerbs, a chequered start/finish line and numbered corners, with an elevation profile of the lap underneath, plus a close-up image of each corner showing where it sits on that profile. They are available two ways:

- the `get_track_map` tool, which returns the map image plus corner data, and can plot car positions for a given lap or moment
- MCP resources (`track://<circuit>/<year>`) for browsing or attaching maps by hand

## Getting started

Requires Go 1.26+ and Docker.

```sh
make build          # build bin/f1mcp
make test           # run tests (offline, using saved API responses)
make docker-build   # build the f1mcp:dev image
```

### Using it from an MCP client

The server speaks MCP over stdio. To add it to Claude Code:

```sh
claude mcp add f1mcp -- docker run --rm -i f1mcp:dev
```

For other clients, configure the command `docker run --rm -i f1mcp:dev`. To serve over streamable HTTP instead of stdio:

```sh
docker run --rm -p 8080:8080 f1mcp:dev -transport http
```

### Track data

Track layouts are stored in `internal/tracks/data/circuits`, one file per circuit per season. To update them:

```sh
make tracks-fetch    # download layouts that aren't stored yet, then fix up all stored ones
make tracks-render   # draw maps, corner images and the HTML gallery into assets/tracks
```

`fetch` only downloads what's missing. Use `go run ./cmd/trackgen fetch -only monaco,spa` to re-fetch specific circuits, or `-force` for everything. Start/finish and sector lines are measured once per circuit from OpenF1 timing and car positions, and stored in `cmd/trackgen/data/lines.json`. Elevation comes from one lap of OpenF1 car positions per circuit, stored in `cmd/trackgen/data/elevation/`. Rendered images are not committed.

Incidents (red flags, safety cars, yellow flags) can be stored and drawn on the maps too:

```sh
go run ./cmd/trackgen incident -session 11377 -event sc -lap 36   # store what caused a safety car
```

It works out the flagged marshal sectors and the cars that stopped or slowed from OpenF1 data, and saves the result in `cmd/trackgen/data/incidents/`; `make tracks-render` then adds it to the gallery.

Laps can be animated as GIFs, one driver or several raced against each other by lap time:

```sh
go run ./cmd/trackgen lap -session 11373 -count 1          # the fastest lap of a session
go run ./cmd/trackgen lap -session 11373 -drivers RUS,VER  # two drivers' best laps
```

Driver photos and flags for the videos come from Wikimedia Commons (freely licensed photos, credited on each gallery page) and flagcdn.com (public domain):

```sh
go run ./cmd/trackgen headshots -year 2026 -drivers RUS,VER
```

Each stored lap (in `cmd/trackgen/data/laps/`) is rendered as two GIFs over the whole track (the cars with speed, gear, throttle, brake and gap; and the same with braking zones, braking speeds and gear changes left on the track) and, if `ffmpeg` is installed, a 4:5 portrait MP4 for phones with a camera that follows the car, zoomed in like the corner images, between the timing panel and an elevation profile.

Corner and straight names live in `internal/tracks/data/names.json`, keyed by circuit. A corner can be a single turn (`"9": "Copse"`), a complex sharing one name (`"10-13": "Maggotts and Becketts"`), or have an alternative name (`"17": {"name": "Mansell Corner", "alt": "Peraltada"}`). Straights are named by the turns at each end. Names are taken from each circuit's Wikipedia article.

### Tests and golden files

Client tests replay real API responses saved in `testdata/`. To refresh them from the live API:

```sh
go test ./internal/openf1 ./internal/jolpica -update -count=1
```

## Project layout

```
cmd/f1mcp/          server entrypoint
internal/server/    builds the MCP server
internal/tools/     MCP tool definitions
internal/f1/        answers in one shape from both APIs: events, sessions, results, caching
internal/openf1/    OpenF1 API client
internal/jolpica/   Jolpica API client
internal/golden/    golden-file helper for client tests
internal/tracks/    circuit layouts (embedded) and track map rendering
cmd/trackgen/       generator for the circuit layouts and images
http/               REST Client files for exploring the APIs
```

## Data sources and credits

This project would not be possible without these community projects:

- [Jolpica F1](https://github.com/jolpica/jolpica-f1): historical results, standings and schedules (the successor to the Ergast API)
- [OpenF1](https://openf1.org): detailed session data from 2023 onward
- [MultiViewer](https://multiviewer.app): circuit outlines and corner positions used to generate track maps
- [Wikimedia Commons](https://commons.wikimedia.org): freely licensed driver photos, credited individually in the gallery
- [flagcdn.com](https://flagcdn.com): national flags

Please respect their rate limits and terms if you run this server.

## Disclaimer

This is an unofficial, non-commercial hobby project. It is not associated in any way with the Formula 1 companies. F1, FORMULA ONE, FORMULA 1, FIA FORMULA ONE WORLD CHAMPIONSHIP, GRAND PRIX and related marks are trade marks of Formula One Licensing B.V. Formula 1 data belongs to Formula One Management.

## License

[MIT](LICENSE)

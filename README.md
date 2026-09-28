# f1mcp

An [MCP](https://modelcontextprotocol.io) server for Formula 1 data, written in Go and packaged as a Docker image.

It combines two public F1 APIs behind one consistent set of tools, so the model asks for things like "the 2024 Monaco race" and never has to know which API answered or how their IDs and formats differ.

> **Status: work in progress.** The server runs and registers its tools, but most tools are still stubs. The OpenF1 and Jolpica clients and track map generation are done; shared types and tools are next.

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

Circuit outlines, corners, start/finish and sector lines are generated ahead of time by `cmd/trackgen` and embedded in the binary. Maps are drawn in an F1 style: asphalt with sector-coloured edges, kerbs, a chequered start/finish line and numbered corners, plus a close-up image of each corner. They are available two ways:

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

For other clients, configure the command `docker run --rm -i f1mcp:dev`.

### Track data

Track layouts are stored in `internal/tracks/data/circuits`, one file per circuit per season. To update them:

```sh
make tracks-fetch    # download layouts that aren't stored yet, then fix up all stored ones
make tracks-render   # draw maps and corner images into assets/tracks for review
```

`fetch` only downloads what's missing. Use `go run ./cmd/trackgen fetch -only monaco,spa` to re-fetch specific circuits, or `-force` for everything. Start/finish and sector lines are measured once per circuit from OpenF1 timing and car positions, and stored in `cmd/trackgen/lines.json`. Rendered images are not committed.

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

Please respect their rate limits and terms if you run this server.

## Disclaimer

This is an unofficial, non-commercial hobby project. It is not associated in any way with the Formula 1 companies. F1, FORMULA ONE, FORMULA 1, FIA FORMULA ONE WORLD CHAMPIONSHIP, GRAND PRIX and related marks are trade marks of Formula One Licensing B.V. Formula 1 data belongs to Formula One Management.

## License

[MIT](LICENSE)

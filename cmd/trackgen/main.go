// Command trackgen generates the circuit layouts embedded in internal/tracks
// and preview images of them.
//
//	go run ./cmd/trackgen fetch   # download missing layouts, then fix up all stored ones
//	go run ./cmd/trackgen incident -session 9523  # store the incident behind a red flag
//	go run ./cmd/trackgen lap -session 11373 -count 2  # store the two fastest laps of a session
//	go run ./cmd/trackgen render  # write map and corner images into assets/tracks
//
// Layouts come from MultiViewer's circuit API, keyed by OpenF1's circuit_key,
// and are stored in internal/tracks/data/circuits. Only missing ones are
// downloaded; use -only or -force to re-fetch. Circuit IDs come from Jolpica,
// matched to OpenF1 race sessions by start time. Start/finish and sector
// lines are measured once per circuit from OpenF1 timing and car positions,
// and stored in cmd/trackgen/data/lines.json. One lap of car positions per circuit,
// stored in cmd/trackgen/data/elevation, gives each track its elevation profile.
//
// Run render after fetch as a separate command so it picks up the new
// embedded data.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage()
	}
	ctx := context.Background()
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "fetch":
		fs := flag.NewFlagSet("fetch", flag.ExitOnError)
		from := fs.Int("from", 2023, "first season")
		to := fs.Int("to", time.Now().Year(), "last season")
		dir := fs.String("dir", "internal/tracks/data/circuits", "track files directory")
		linesFile := fs.String("lines", "cmd/trackgen/data/lines.json", "stored timing lines")
		elevationDir := fs.String("elevation", "cmd/trackgen/data/elevation", "stored elevation laps")
		force := fs.Bool("force", false, "re-fetch every layout and timing line")
		only := fs.String("only", "", "comma-separated circuit IDs to re-fetch")
		fs.Parse(args)
		o := fetchOptions{from: *from, to: *to, dir: *dir, linesFile: *linesFile, elevationDir: *elevationDir, force: *force, only: map[string]bool{}}
		for id := range strings.SplitSeq(*only, ",") {
			if id != "" {
				o.only[id] = true
			}
		}
		if err := fetch(ctx, o); err != nil {
			log.Fatal(err)
		}
	case "incident":
		fs := flag.NewFlagSet("incident", flag.ExitOnError)
		session := fs.Int("session", 0, "OpenF1 session key")
		event := fs.String("event", "red", "what to find the cause of: red, sc, vsc or yellow")
		lap := fs.Int("lap", 0, "use the first event at or after this lap")
		drivers := fs.String("drivers", "", "comma-separated car numbers involved, marked where they were")
		dir := fs.String("dir", "cmd/trackgen/data/incidents", "stored incidents")
		fs.Parse(args)
		if *session == 0 {
			usage()
		}
		if err := findIncident(ctx, *session, *event, *lap, carNumbers(*drivers), *dir); err != nil {
			log.Fatal(err)
		}
	case "lap":
		fs := flag.NewFlagSet("lap", flag.ExitOnError)
		session := fs.Int("session", 0, "OpenF1 session key")
		drivers := fs.String("drivers", "", "comma-separated car numbers or codes, e.g. RUS,VER (default: the fastest -count drivers)")
		count := fs.Int("count", 1, "how many of the fastest drivers' best laps to store")
		dir := fs.String("dir", "cmd/trackgen/data/laps", "stored laps")
		fs.Parse(args)
		if *session == 0 {
			usage()
		}
		if err := findLaps(ctx, *session, splitList(*drivers), *count, *dir); err != nil {
			log.Fatal(err)
		}
	case "headshots":
		fs := flag.NewFlagSet("headshots", flag.ExitOnError)
		year := fs.Int("year", time.Now().Year(), "season the drivers raced in")
		drivers := fs.String("drivers", "", "comma-separated driver codes, e.g. RUS,VER")
		dir := fs.String("dir", "cmd/trackgen/data/headshots", "stored headshots")
		fs.Parse(args)
		if *drivers == "" {
			usage()
		}
		if err := findHeadshots(ctx, *year, splitList(*drivers), *dir); err != nil {
			log.Fatal(err)
		}
	case "render":
		fs := flag.NewFlagSet("render", flag.ExitOnError)
		out := fs.String("out", "assets/tracks", "output directory")
		incidents := fs.String("incidents", "cmd/trackgen/data/incidents", "stored incidents")
		laps := fs.String("laps", "cmd/trackgen/data/laps", "stored laps")
		headshots := fs.String("headshots", "cmd/trackgen/data/headshots", "stored headshots")
		fs.Parse(args)
		if err := render(*out, *incidents, *laps, *headshots); err != nil {
			log.Fatal(err)
		}
	default:
		usage()
	}
}

func splitList(s string) []string {
	var out []string
	for f := range strings.SplitSeq(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func carNumbers(s string) []int {
	var out []int
	for f := range strings.SplitSeq(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(f)); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: trackgen fetch [-from 2023] [-to YEAR] [-only IDS] [-force]\n       trackgen incident -session KEY [-event red|sc|vsc|yellow] [-lap N] [-drivers N,N]\n       trackgen lap -session KEY [-count N | -drivers N,N]\n       trackgen headshots -drivers CODES [-year YEAR]\n       trackgen render [-out DIR]")
	os.Exit(2)
}

// Command trackgen generates the circuit layouts embedded in internal/tracks
// and preview images of them.
//
//	go run ./cmd/trackgen fetch   # download missing layouts, then fix up all stored ones
//	go run ./cmd/trackgen render  # write map and corner images into assets/tracks
//
// Layouts come from MultiViewer's circuit API, keyed by OpenF1's circuit_key,
// and are stored in internal/tracks/data/circuits. Only missing ones are
// downloaded; use -only or -force to re-fetch. Circuit IDs come from Jolpica,
// matched to OpenF1 race sessions by start time. Start/finish and sector
// lines are measured once per circuit from OpenF1 timing and car positions,
// and stored in cmd/trackgen/lines.json.
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
		linesFile := fs.String("lines", "cmd/trackgen/lines.json", "stored timing lines")
		force := fs.Bool("force", false, "re-fetch every layout and timing line")
		only := fs.String("only", "", "comma-separated circuit IDs to re-fetch")
		fs.Parse(args)
		o := fetchOptions{from: *from, to: *to, dir: *dir, linesFile: *linesFile, force: *force, only: map[string]bool{}}
		for id := range strings.SplitSeq(*only, ",") {
			if id != "" {
				o.only[id] = true
			}
		}
		if err := fetch(ctx, o); err != nil {
			log.Fatal(err)
		}
	case "render":
		fs := flag.NewFlagSet("render", flag.ExitOnError)
		out := fs.String("out", "assets/tracks", "output directory")
		fs.Parse(args)
		if err := render(*out); err != nil {
			log.Fatal(err)
		}
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: trackgen fetch [-from 2023] [-to YEAR] [-only IDS] [-force]\n       trackgen render [-out DIR]")
	os.Exit(2)
}

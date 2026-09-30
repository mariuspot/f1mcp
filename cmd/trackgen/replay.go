package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mariuspot/f1mcp/internal/live"
	"github.com/mariuspot/f1mcp/internal/replay"
)

// replayAt plays a collected session as fast as possible and prints the
// timing tower at each of the given times of day (UTC, e.g. "16:40"), to
// check the live state against what happened.
func replayAt(ctx context.Context, dir string, at []string, cars bool) error {
	src, err := replay.Open(dir, replay.Options{SkipCars: !cars})
	if err != nil {
		return err
	}
	defer src.Close()
	day := src.Manifest.Start.UTC().Truncate(24 * time.Hour)
	var marks []time.Time
	for _, a := range at {
		t, err := time.Parse("15:04", strings.TrimSpace(a))
		if err != nil {
			return fmt.Errorf("-at %q: want HH:MM (UTC)", a)
		}
		marks = append(marks, day.Add(time.Duration(t.Hour())*time.Hour+time.Duration(t.Minute())*time.Minute))
	}
	state := live.NewState()
	records, start := 0, time.Now()
	err = replay.Play(ctx, src, time.Time{}, time.Time{}, 0, func(r live.Record) {
		for len(marks) > 0 && r.Time.After(marks[0]) {
			printTower(state.Snapshot(), marks[0])
			marks = marks[1:]
		}
		state.Apply(r)
		records++
	})
	if err != nil {
		return err
	}
	for _, m := range marks {
		printTower(state.Snapshot(), m)
	}
	fmt.Printf("%d records in %s\n", records, time.Since(start).Round(time.Millisecond))
	return nil
}

func printTower(s live.Snapshot, at time.Time) {
	fmt.Printf("\n== %s  %s  lap %d  flag %s", at.Format("15:04"), s.Session, s.Lap, s.Flag)
	if s.Weather != nil {
		fmt.Printf("  air %.0f°C track %.0f°C rain %v", s.Weather.AirC, s.Weather.TrackC, s.Weather.Rain)
	}
	if s.BestLap != nil {
		fmt.Printf("  fastest %s %.3f (lap %d)", s.BestLap.Driver, s.BestLap.Seconds, s.BestLap.Lap)
	}
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(w, "pos\tcar\tgap\tint\tlaps\tlast\tbest\ttyre\tstops\t")
	for _, c := range s.Cars {
		gap, intv := c.GapText, ""
		if c.GapToLeader != nil {
			gap = fmt.Sprintf("%.3f", *c.GapToLeader)
		}
		if c.Interval != nil {
			intv = fmt.Sprintf("%.3f", *c.Interval)
		}
		last, best := "", ""
		if c.LastLap != nil {
			last = fmt.Sprintf("%.3f", c.LastLap.Seconds)
		}
		if c.BestLap != nil {
			best = fmt.Sprintf("%.3f", c.BestLap.Seconds)
		}
		tyre := ""
		if c.Compound != "" {
			tyre = fmt.Sprintf("%s %d", c.Compound[:1], c.TyreAge)
		}
		code := c.Code
		if c.Out != "" {
			code += " (" + c.Out + ")"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%d\t%s\t%s\t%s\t%d\t\n", c.Position, code, gap, intv, c.Lap, last, best, tyre, len(c.Stops))
	}
	w.Flush()
	for i, m := range s.Messages {
		if i == 3 {
			break
		}
		fmt.Printf("  rc %s %s\n", m.Time.Format("15:04:05"), m.Message)
	}
}

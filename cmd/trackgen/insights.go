package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mariuspot/f1mcp/internal/insight"
	"github.com/mariuspot/f1mcp/internal/live"
	"github.com/mariuspot/f1mcp/internal/replay"
)

// insights replays laps from..to of a collected session as fast as
// possible, commenting as the live page would (by session time), and prints
// each batch of events and the insight on it. With dry, it prints the
// prompts instead of calling Claude.
func insights(ctx context.Context, dir string, from, to int, dry bool, cacheDir string) error {
	src, err := replay.Open(dir, replay.Options{SkipCars: true})
	if err != nil {
		return err
	}
	defer src.Close()
	start, err := replay.LapStart(dir, from)
	if err != nil {
		return err
	}
	end, err := replay.LapStart(dir, to)
	if err != nil {
		end = time.Time{}
	}
	claude := insight.NewClaude(os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("F1MCP_INSIGHT_MODEL"), nil)
	if claude == nil && !dry {
		return fmt.Errorf("set ANTHROPIC_API_KEY, or use -dry")
	}
	if claude == nil {
		claude = insight.NewClaude("dry-run", "", nil)
	}
	state := live.NewState()
	run := insight.NewCommentator(claude, cacheDir).Manual(state, "eval")
	var said []string
	flush := func(force bool) error {
		batch := run.DueBySessionTime()
		if batch == nil {
			return nil
		}
		fmt.Printf("\n── lap %d %s ──\n", state.Snapshot().Lap, batch[len(batch)-1].Time.Format("15:04:05"))
		for _, e := range batch {
			fmt.Printf("   · %s\n", e.Text)
		}
		if dry {
			fmt.Println(insight.Prompt(state.Snapshot(), batch, state.Recent(20), said))
			return nil
		}
		text, err := run.Comment(ctx, batch)
		if err != nil {
			return err
		}
		if text == "" {
			text = "(SKIP)"
		} else {
			said = append(said, text)
		}
		fmt.Printf(" » %s\n", text)
		return nil
	}
	err = replay.Play(ctx, src, time.Time{}, end, 0, func(r live.Record) {
		events := state.Step(r)
		if r.Time.Before(start) {
			return
		}
		run.Add(events)
		// The run decides when a batch is due, as on the live page.
		if err := flush(false); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	})
	if err != nil {
		return err
	}
	return flush(true)
}

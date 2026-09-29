package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/mariuspot/f1mcp/internal/f1"
	"github.com/mariuspot/f1mcp/internal/jolpica"
	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tracks"
)

// LapFile is one or more stored laps from a session, animated together by
// render.
type LapFile = f1.LapReplay

// backfillLaps adds what older stored laps lack: each lap's start time, the
// weather when it was driven and its tyres.
func backfillLaps(ctx context.Context, dir string) error {
	of1 := openf1.NewClient("", nil)
	svc := f1.New(jolpica.NewClient("", nil), of1)
	files, err := loadLapFiles(dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		key := strconv.Itoa(f.SessionKey)
		for i, l := range f.Laps {
			if !l.Started.IsZero() {
				continue
			}
			laps, err := of1.Laps(ctx, openf1.LapsFilter{SessionKey: key, DriverNumber: l.Number, LapNumber: l.Lap})
			if err != nil {
				return err
			}
			if len(laps) > 0 {
				f.Laps[i].Started = laps[0].DateStart.UTC()
			}
		}
		if f.Weather == nil && len(f.Laps) > 0 && !f.Laps[0].Started.IsZero() {
			if f.Weather, err = svc.LapWeatherAt(ctx, f.SessionKey, f.Laps[0].Started); err != nil {
				return err
			}
		}
		if slices.ContainsFunc(f.Laps, func(l tracks.LapTrace) bool { return l.Compound == "" }) {
			stints, err := of1.Stints(ctx, openf1.SessionDriverFilter{SessionKey: key})
			if err != nil {
				return err
			}
			for i := range f.Laps {
				f1.SetTyre(&f.Laps[i], stints)
			}
		}
		if err := writeJSON(filepath.Join(dir, f.Name+".json"), f); err != nil {
			return err
		}
		w := "no weather"
		if f.Weather != nil {
			w = f.Weather.String()
		}
		var tyres []string
		for _, l := range f.Laps {
			tyres = append(tyres, fmt.Sprintf("%s %s (%d laps old)", l.Driver, l.Compound, l.TyreAge))
		}
		log.Printf("%s: %s · %s", f.Name, w, strings.Join(tyres, ", "))
	}
	return nil
}

// findLaps stores each driver's best lap of a session: the drivers given (by
// car number or code, e.g. "63" or "RUS"), or else the fastest count
// drivers. It makes 5 requests plus 2 per lap.
func findLaps(ctx context.Context, sessionKey int, driverArgs []string, count int, dir string) error {
	svc := f1.New(jolpica.NewClient("", nil), openf1.NewClient("", nil))
	var picks []f1.LapPick
	for _, d := range driverArgs {
		picks = append(picks, f1.LapPick{Driver: d})
	}
	f, err := svc.ReplayLaps(ctx, sessionKey, 0, picks, count)
	if err != nil {
		return err
	}
	for _, l := range f.Laps {
		log.Printf("%s lap %d: %s, %d positions, %d telemetry samples", l.Driver, l.Lap,
			formatSeconds(l.Duration), len(l.Positions), len(l.Telemetry))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, f.Name+".json"), f)
}

func loadLapFiles(dir string) ([]LapFile, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var out []LapFile
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var f LapFile
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		out = append(out, f)
	}
	return out, nil
}

func formatSeconds(s float64) string {
	m := int(s) / 60
	return fmt.Sprintf("%d:%06.3f", m, s-float64(m*60))
}

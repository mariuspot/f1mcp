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
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tracks"
)

// LapFile is one or more stored laps from a session, animated together by
// render.
type LapFile struct {
	Name       string            `json:"name"`
	CircuitID  string            `json:"circuit_id"`
	Year       int               `json:"year"`
	SessionKey int               `json:"session_key"`
	Session    string            `json:"session"` // e.g. "Azerbaijan Grand Prix · Qualifying"
	Laps       []tracks.LapTrace `json:"laps"`
}

// findLaps stores each driver's best lap of a session: the drivers given (by
// car number or code, e.g. "63" or "RUS"), or else the fastest count
// drivers. It makes 4 requests plus 2 per lap.
func findLaps(ctx context.Context, sessionKey int, driverArgs []string, count int, dir string) error {
	of1 := openf1.NewClient("", nil)
	key := strconv.Itoa(sessionKey)
	sessions, err := of1.Sessions(ctx, openf1.SessionsFilter{SessionKey: key})
	if err != nil || len(sessions) == 0 {
		return fmt.Errorf("session %d: %v", sessionKey, err)
	}
	s := sessions[0]
	t, err := trackByKey(s.CircuitKey, s.Year)
	if err != nil {
		return err
	}

	laps, err := of1.Laps(ctx, openf1.LapsFilter{SessionKey: key})
	if err != nil {
		return err
	}
	best := map[int]openf1.Lap{}
	for _, l := range laps {
		if l.LapDuration == nil || l.DateStart.IsZero() || l.IsPitOutLap {
			continue
		}
		if b, ok := best[l.DriverNumber]; !ok || *l.LapDuration < *b.LapDuration {
			best[l.DriverNumber] = l
		}
	}
	info, err := of1.Drivers(ctx, openf1.SessionDriverFilter{SessionKey: key})
	if err != nil {
		return err
	}
	byNumber := map[int]openf1.Driver{}
	for _, d := range info {
		byNumber[d.DriverNumber] = d
	}
	var chosen []int
	for _, a := range driverArgs {
		n, err := strconv.Atoi(a)
		if err != nil {
			i := slices.IndexFunc(info, func(d openf1.Driver) bool { return strings.EqualFold(d.NameAcronym, a) })
			if i < 0 {
				return fmt.Errorf("no driver %q in session %d", a, sessionKey)
			}
			n = info[i].DriverNumber
		}
		chosen = append(chosen, n)
	}
	if len(chosen) == 0 {
		for n := range best {
			chosen = append(chosen, n)
		}
		slices.SortFunc(chosen, func(a, b int) int {
			return int((*best[a].LapDuration - *best[b].LapDuration) * 1000)
		})
		chosen = chosen[:min(count, len(chosen))]
	}

	f := LapFile{CircuitID: t.CircuitID, Year: s.Year, SessionKey: sessionKey, Session: s.CountryName + " · " + s.SessionName}
	var names []string
	for _, n := range chosen {
		l, ok := best[n]
		if !ok {
			return fmt.Errorf("driver %d has no timed lap in session %d", n, sessionKey)
		}
		trace, err := recordTrace(ctx, of1, key, l, byNumber[n])
		if err != nil {
			return err
		}
		f.Laps = append(f.Laps, trace)
		names = append(names, strings.ToLower(trace.Driver))
		log.Printf("%s lap %d: %s, %d positions, %d telemetry samples", trace.Driver, trace.Lap,
			formatSeconds(trace.Duration), len(trace.Positions), len(trace.Telemetry))
	}
	f.Name = fmt.Sprintf("%s-%d-%s-%s", t.CircuitID, s.Year, slug(s.SessionName), strings.Join(names, "-vs-"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, f.Name+".json"), f)
}

// recordTrace fetches a lap's positions and telemetry.
func recordTrace(ctx context.Context, of1 *openf1.Client, key string, l openf1.Lap, d openf1.Driver) (tracks.LapTrace, error) {
	dur := *l.LapDuration
	w := openf1.WindowFilter{
		SessionKey: key, DriverNumber: l.DriverNumber,
		After: l.DateStart.Add(-time.Second), Before: l.DateStart.Add(seconds(dur) + time.Second),
	}
	locs, err := of1.Locations(ctx, w)
	if err != nil {
		return tracks.LapTrace{}, err
	}
	cars, err := of1.CarData(ctx, w)
	if err != nil {
		return tracks.LapTrace{}, err
	}
	tr := tracks.LapTrace{Driver: d.NameAcronym, Number: l.DriverNumber, Color: "#" + d.TeamColour, Lap: l.LapNumber, Duration: dur}
	for _, p := range locs {
		tr.Positions = append(tr.Positions, [3]float64{round(p.Date.Sub(l.DateStart).Seconds(), 3), float64(p.X), float64(p.Y)})
	}
	for _, c := range cars {
		tr.Telemetry = append(tr.Telemetry, [5]float64{round(c.Date.Sub(l.DateStart).Seconds(), 3), float64(c.Speed), float64(c.Throttle), float64(c.Brake), float64(c.NGear)})
	}
	return tr, nil
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

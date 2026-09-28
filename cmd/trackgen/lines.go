package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tracks"
)

// Lines are the timing lines of a circuit, found from where a car was when
// OpenF1 says its lap and sectors started.
type Lines struct {
	Start   tracks.Point `json:"start"`
	Sector2 tracks.Point `json:"sector2"`
	Sector3 tracks.Point `json:"sector3"`
	// Where the lines were measured, for reference.
	Year       int `json:"year"`
	SessionKey int `json:"session_key"`
}

func loadLines(file string) (map[string]Lines, error) {
	lines := map[string]Lines{}
	b, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return lines, nil
	}
	if err != nil {
		return nil, err
	}
	return lines, json.Unmarshal(b, &lines)
}

// findLines measures a circuit's timing lines from a session, trying a few
// drivers' lap 10 until one gives lines that valid accepts.
func findLines(ctx context.Context, c *openf1.Client, sessionKey int, valid func(Lines) bool) (Lines, error) {
	key := strconv.Itoa(sessionKey)
	// A lap mid-race avoids the grid start and pit-lane finishes.
	laps, err := c.Laps(ctx, openf1.LapsFilter{SessionKey: key, LapNumber: 10})
	if err != nil {
		return Lines{}, err
	}
	tried := 0
	for _, lap := range laps {
		if lap.DateStart.IsZero() || lap.IsPitOutLap || lap.DurationSector1 == nil || lap.DurationSector2 == nil {
			continue
		}
		if tried++; tried > 3 {
			break
		}
		s2 := lap.DateStart.Add(seconds(*lap.DurationSector1))
		s3 := s2.Add(seconds(*lap.DurationSector2))
		var pts [3]tracks.Point
		ok := true
		for i, at := range []time.Time{lap.DateStart, s2, s3} {
			if pts[i], ok, err = positionAt(ctx, c, key, lap.DriverNumber, at); err != nil {
				return Lines{}, err
			} else if !ok {
				break
			}
		}
		if !ok {
			continue
		}
		l := Lines{Start: pts[0], Sector2: pts[1], Sector3: pts[2], SessionKey: sessionKey}
		if valid(l) {
			return l, nil
		}
	}
	return Lines{}, errors.New("no lap gave plausible timing lines")
}

func seconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

// positionAt interpolates a driver's position at a moment from the location
// samples either side of it.
func positionAt(ctx context.Context, c *openf1.Client, sessionKey string, driver int, at time.Time) (tracks.Point, bool, error) {
	locs, err := c.Locations(ctx, openf1.WindowFilter{
		SessionKey:   sessionKey,
		DriverNumber: driver,
		After:        at.Add(-time.Second),
		Before:       at.Add(time.Second),
	})
	if err != nil {
		return tracks.Point{}, false, err
	}
	for i := 1; i < len(locs); i++ {
		a, b := locs[i-1], locs[i]
		if a.Date.After(at) || b.Date.Before(at) {
			continue
		}
		span := b.Date.Sub(a.Date).Seconds()
		f := 0.0
		if span > 0 {
			f = at.Sub(a.Date).Seconds() / span
		}
		return tracks.Point{
			math.Round(float64(a.X) + f*float64(b.X-a.X)),
			math.Round(float64(a.Y) + f*float64(b.Y-a.Y)),
		}, true, nil
	}
	return tracks.Point{}, false, nil
}

// plausibleLines reports whether the lines fit a track: the start line lies
// between the last corner and turn 1, and the sector lines follow it in lap
// order. OpenF1 timing is sometimes seconds out (Hungary 2026), which puts
// the lines in the wrong place.
func plausibleLines(t *tracks.Track, l Lines) bool {
	n := len(t.Outline)
	if n == 0 || len(t.Corners) == 0 {
		return false
	}
	last := nearest(t.Outline, t.Corners[len(t.Corners)-1].Position)
	first := nearest(t.Outline, t.Corners[0].Position)
	s := nearest(t.Outline, l.Start)
	if (s-last+n)%n > (first-last+n)%n {
		return false
	}
	d2 := (nearest(t.Outline, l.Sector2) - s + n) % n
	d3 := (nearest(t.Outline, l.Sector3) - s + n) % n
	return 0 < d2 && d2 < d3
}

func nearest(pts []tracks.Point, p tracks.Point) int {
	best, bestD := 0, math.Inf(1)
	for i, q := range pts {
		if d := math.Hypot(q[0]-p[0], q[1]-p[1]); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

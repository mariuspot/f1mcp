package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tracks"
)

// ElevationLap is one lap of car positions, including height, recorded by
// OpenF1. Coordinates are in decimetres, like track outlines.
type ElevationLap struct {
	Year       int      `json:"year"`
	SessionKey int      `json:"session_key"`
	Driver     int      `json:"driver"`
	Lap        int      `json:"lap"`
	Samples    [][3]int `json:"samples"` // [x, y, z]
}

func elevationFile(dir, circuitID string) string {
	return filepath.Join(dir, circuitID+".json")
}

func loadElevation(dir, circuitID string) (*ElevationLap, error) {
	b, err := os.ReadFile(elevationFile(dir, circuitID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var e ElevationLap
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, fmt.Errorf("%s: %w", circuitID, err)
	}
	return &e, nil
}

// findMissingElevation records one lap of positions for each circuit that has
// timing lines but no stored elevation lap, from the session its lines were
// measured in.
func findMissingElevation(ctx context.Context, of1 *openf1.Client, o fetchOptions, lines map[string]Lines) error {
	if err := os.MkdirAll(o.elevationDir, 0o755); err != nil {
		return err
	}
	for id, l := range lines {
		if _, err := os.Stat(elevationFile(o.elevationDir, id)); err == nil && !o.refetch(id) {
			continue
		}
		e, err := recordLap(ctx, of1, l.SessionKey)
		if err != nil {
			return fmt.Errorf("%s elevation: %w", id, err)
		}
		e.Year = l.Year
		if err := writeJSON(elevationFile(o.elevationDir, id), e); err != nil {
			return err
		}
		zs := make([]int, len(e.Samples))
		for i, s := range e.Samples {
			zs[i] = s[2]
		}
		lo, hi := zs[0], zs[0]
		for _, z := range zs {
			lo, hi = min(lo, z), max(hi, z)
		}
		log.Printf("%-15s elevation: %d samples, %.1f m change", id, len(e.Samples), float64(hi-lo)/10)
	}
	return nil
}

// recordLap returns the positions of the first clean lap 10 in a session.
func recordLap(ctx context.Context, c *openf1.Client, sessionKey int) (*ElevationLap, error) {
	key := strconv.Itoa(sessionKey)
	laps, err := c.Laps(ctx, openf1.LapsFilter{SessionKey: key, LapNumber: 10})
	if err != nil {
		return nil, err
	}
	for _, lap := range laps {
		if lap.DateStart.IsZero() || lap.IsPitOutLap || lap.LapDuration == nil {
			continue
		}
		locs, err := c.Locations(ctx, openf1.WindowFilter{
			SessionKey:   key,
			DriverNumber: lap.DriverNumber,
			After:        lap.DateStart,
			Before:       lap.DateStart.Add(seconds(*lap.LapDuration)),
		})
		if err != nil {
			return nil, err
		}
		if len(locs) < 100 {
			continue
		}
		e := &ElevationLap{SessionKey: sessionKey, Driver: lap.DriverNumber, Lap: lap.LapNumber}
		for _, l := range locs {
			e.Samples = append(e.Samples, [3]int{l.X, l.Y, l.Z})
		}
		return e, nil
	}
	return nil, errors.New("no lap with enough location samples")
}

// applyElevation gives every outline point a height in metres above the
// lowest point of the lap, from the recorded lap: each sample is matched to
// its nearest outline point, heights are interpolated along the lap between
// matched points, then smoothed.
func applyElevation(t *tracks.Track, e *ElevationLap) {
	n := len(t.Outline)
	if e == nil || n == 0 {
		t.Elevation = nil
		return
	}
	sum := make([]float64, n)
	count := make([]int, n)
	for _, s := range e.Samples {
		i := nearest(t.Outline, tracks.Point{float64(s[0]), float64(s[1])})
		sum[i] += float64(s[2])
		count[i]++
	}
	var known []int
	for i := range n {
		if count[i] > 0 {
			known = append(known, i)
		}
	}
	if len(known) < 2 {
		t.Elevation = nil
		return
	}

	// Interpolate between consecutive known points, wrapping round the lap.
	z := make([]float64, n)
	for k, a := range known {
		b := known[(k+1)%len(known)]
		za, zb := sum[a]/float64(count[a]), sum[b]/float64(count[b])
		span := (b - a + n) % n
		if span == 0 {
			span = n
		}
		for j := 0; j < span; j++ {
			z[(a+j)%n] = za + (zb-za)*float64(j)/float64(span)
		}
	}

	// Smooth with a moving average of a few points either side.
	const w = 3
	smooth := make([]float64, n)
	for i := range n {
		total := 0.0
		for j := -w; j <= w; j++ {
			total += z[(i+j+n)%n]
		}
		smooth[i] = total / (2*w + 1)
	}

	lo := math.Inf(1)
	for _, v := range smooth {
		lo = min(lo, v)
	}
	t.Elevation = make([]float64, n)
	for i, v := range smooth {
		t.Elevation[i] = math.Round(v-lo) / 10 // decimetres to metres, 0.1 m
	}
}

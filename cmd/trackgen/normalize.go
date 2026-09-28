package main

import (
	"log"
	"slices"

	"github.com/mariuspot/f1mcp/internal/tracks"
)

// normalize fixes up a track as fetched from MultiViewer. It needs no network
// and is safe to run repeatedly on stored tracks:
//   - duplicate corner numbers keep only their first occurrence along the lap
//   - the outline runs in driving direction
//   - the outline starts at the start/finish line and sector starts are set,
//     when the circuit's timing lines are known
func normalize(t *tracks.Track, lines *Lines) {
	if orientOutline(t) {
		log.Printf("%d %s: outline reversed to match driving direction", t.Year, t.CircuitID)
	}
	if lines == nil {
		t.SectorStarts = nil
	} else if !plausibleLines(t, *lines) {
		log.Printf("%d %s: stored timing lines don't fit this layout, leaving them out", t.Year, t.CircuitID)
		t.SectorStarts = nil
	} else {
		i := nearest(t.Outline, lines.Start)
		t.Outline = append(t.Outline[i:], t.Outline[:i]...)
		t.SectorStarts = []int{nearest(t.Outline, lines.Sector2), nearest(t.Outline, lines.Sector3)}
	}
	dedupeCorners(t)
}

// orientOutline reverses the outline if the corners, in number order, run
// backwards along it. It reports whether it reversed the outline.
func orientOutline(t *tracks.Track) bool {
	n := len(t.Outline)
	if n == 0 || len(t.Corners) < 3 {
		return false
	}
	forward, backward := 0, 0
	prev := nearest(t.Outline, t.Corners[0].Position)
	for _, c := range t.Corners[1:] {
		i := nearest(t.Outline, c.Position)
		if step := (i - prev + n) % n; step != 0 && step < n/2 {
			forward++
		} else if step != 0 {
			backward++
		}
		prev = i
	}
	if backward > forward {
		slices.Reverse(t.Outline)
		return true
	}
	return false
}

// dedupeCorners drops corners whose number already appeared earlier along
// the lap (MultiViewer lists turns 1 and 12 twice at the Hungaroring), and
// sorts corners by number.
func dedupeCorners(t *tracks.Track) {
	slices.SortStableFunc(t.Corners, func(a, b tracks.Corner) int {
		return nearest(t.Outline, a.Position) - nearest(t.Outline, b.Position)
	})
	seen := map[int]bool{}
	t.Corners = slices.DeleteFunc(t.Corners, func(c tracks.Corner) bool {
		if seen[c.Number] {
			log.Printf("%d %s: dropping duplicate turn %d", t.Year, t.CircuitID, c.Number)
			return true
		}
		seen[c.Number] = true
		return false
	})
	slices.SortStableFunc(t.Corners, func(a, b tracks.Corner) int { return a.Number - b.Number })
}

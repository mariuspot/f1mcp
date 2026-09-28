package tracks

import "fmt"

// atCornerDistance is how close, in metres along the lap, a point must be to
// a corner to count as at it.
const atCornerDistance = 60.0

// Location describes where a point is on a track.
type Location struct {
	// Distance is how far along the lap the point is, in metres from the
	// start/finish line.
	Distance float64
	// At is the corner the point is at, if any.
	At *Corner
	// Before and After are the corners either side of the point along the
	// lap, wrapping round.
	Before, After *Corner
	// Straight is the name of the straight the point is on, if any.
	Straight string
	// straightEnds are the corners at either end of Straight.
	straightEnds [2]int
	// MarshalSector is the marshal sector the point is in, or 0 if unknown.
	MarshalSector int
}

// Locate describes where a point is on the track.
func (t *Track) Locate(p Point) Location {
	n := len(t.Outline)
	var loc Location
	if n == 0 {
		return loc
	}
	dist, lap := lapDistances(t.Outline)
	i := nearestIndex(t.Outline, p)
	loc.Distance = dist[i]

	// Corners either side, and whether the point is at one.
	best := lap
	for k := range t.Corners {
		c := &t.Corners[k]
		d := dist[nearestIndex(t.Outline, c.Position)]
		gap := d - loc.Distance
		if gap < -lap/2 {
			gap += lap
		} else if gap > lap/2 {
			gap -= lap
		}
		if abs := max(gap, -gap); abs < atCornerDistance && abs < best {
			loc.At, best = c, abs
		}
	}
	if len(t.Corners) > 0 {
		// The first corner at or after the point, wrapping round the lap.
		next := 0
		for k, c := range t.Corners {
			if dist[nearestIndex(t.Outline, c.Position)] >= loc.Distance {
				next = k
				break
			}
			next = (k + 1) % len(t.Corners)
		}
		loc.After = &t.Corners[next]
		loc.Before = &t.Corners[(next-1+len(t.Corners))%len(t.Corners)]
	}

	for _, s := range t.Straights {
		from, okFrom := cornerIndex(t, s.From)
		to, okTo := cornerIndex(t, s.To)
		if !okFrom || !okTo {
			continue
		}
		if (to >= from && i >= from && i <= to) || (to < from && (i >= from || i <= to)) {
			loc.Straight, loc.straightEnds = s.Name, [2]int{s.From, s.To}
		}
	}

	for k, m := range t.MarshalSectors {
		start := nearestIndex(t.Outline, m.Position)
		end := nearestIndex(t.Outline, t.MarshalSectors[(k+1)%len(t.MarshalSectors)].Position)
		if (end >= start && i >= start && i < end) || (end < start && (i >= start || i < end)) {
			loc.MarshalSector = m.Number
		}
	}
	return loc
}

// String describes the location in words, e.g. "Turn 8, Portier" or
// "Avenue d'Ostende, between turns 2 and 3".
func (l Location) String() string {
	if l.At != nil {
		// A corner at the end of a straight isn't on it.
		if l.Straight != "" && l.At.Number != l.straightEnds[0] && l.At.Number != l.straightEnds[1] {
			return cornerLabel(*l.At) + ", " + l.Straight
		}
		return cornerLabel(*l.At)
	}
	between := ""
	if l.Before != nil && l.After != nil {
		between = fmt.Sprintf("between turns %d and %d", l.Before.Number, l.After.Number)
	}
	switch {
	case l.Straight != "" && between != "":
		return l.Straight + ", " + between
	case l.Straight != "":
		return l.Straight
	default:
		return between
	}
}

// Approaching names the corner a point is at or heading into, e.g.
// "Turn 1, La Source"; useful for where a car brakes.
func (l Location) Approaching() string {
	switch {
	case l.At != nil:
		return cornerLabel(*l.At)
	case l.After != nil:
		return cornerLabel(*l.After)
	}
	return ""
}

func cornerLabel(c Corner) string {
	if c.Name != "" {
		return fmt.Sprintf("Turn %d, %s", c.Number, c.Name)
	}
	return fmt.Sprintf("Turn %d", c.Number)
}

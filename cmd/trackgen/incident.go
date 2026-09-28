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
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tracks"
)

// Incident is a stored incident, drawn on its track map and nearest corner
// by render.
type Incident struct {
	Name       string         `json:"name"`
	CircuitID  string         `json:"circuit_id"`
	Year       int            `json:"year"`
	SessionKey int            `json:"session_key"`
	Session    string         `json:"session"` // e.g. "Monaco Grand Prix · Race"
	Corner     int            `json:"corner"`  // turn shown in the close-up
	Overlay    tracks.Overlay `json:"overlay"`
}

const (
	yellowColor       = "#FFD100"
	doubleYellowColor = "#FF8A00"
)

// incidentMessageRE matches race control messages naming the turn and cars
// of an incident, e.g. "TURN 8 INCIDENT INVOLVING CARS 10 (GAS) AND 31 (OCO)".
var (
	incidentMessageRE = regexp.MustCompile(`TURN (\d+) INCIDENT INVOLVING CARS? ([0-9A-Z (),]+?)(?: NOTED| UNDER| REVIEWED| WILL|$)`)
	carRE             = regexp.MustCompile(`(\d+) \(([A-Z]{3})\)`)
)

// Events an incident can be found from.
var incidentEvents = map[string]struct {
	match  func(openf1.RaceControl) bool
	banner string
	color  string
	slug   string
}{
	"red": {func(m openf1.RaceControl) bool { return m.Flag != nil && *m.Flag == "RED" }, "RED FLAG", kerbRedHex, "red-flag"},
	"sc": {func(m openf1.RaceControl) bool {
		return m.Category == "SafetyCar" && strings.Contains(m.Message, "SAFETY CAR DEPLOYED") && !strings.Contains(m.Message, "VIRTUAL")
	}, "SAFETY CAR", "#E0A800", "safety-car"},
	"vsc": {func(m openf1.RaceControl) bool {
		return m.Category == "SafetyCar" && strings.Contains(m.Message, "VIRTUAL SAFETY CAR DEPLOYED")
	}, "VIRTUAL SAFETY CAR", "#E0A800", "virtual-safety-car"},
	"yellow": {func(m openf1.RaceControl) bool {
		return m.Flag != nil && strings.Contains(*m.Flag, "YELLOW") && m.Sector != nil
	}, "YELLOW FLAG", "#E0A800", "yellow-flag"},
}

const kerbRedHex = "#E10600"

// findIncident works out what caused a red flag, safety car, virtual safety
// car or yellow flag (the first at or after lap) and stores it in dir: the
// marshal sectors under yellow flags around it, the cars that stopped once
// the first yellow was shown, the named drivers' positions at that moment,
// and incidents race control named at a turn on the same lap.
func findIncident(ctx context.Context, sessionKey int, event string, fromLap int, involved []int, dir string) error {
	ev, ok := incidentEvents[event]
	if !ok {
		return fmt.Errorf("unknown event %q (want red, sc, vsc or yellow)", event)
	}
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
	meetings, err := of1.Meetings(ctx, openf1.MeetingsFilter{Year: s.Year})
	if err != nil {
		return err
	}
	meeting := s.CountryName
	for _, m := range meetings {
		if m.MeetingKey == s.MeetingKey {
			meeting = m.MeetingName
		}
	}

	msgs, err := of1.RaceControl(ctx, openf1.SessionFilter{SessionKey: key})
	if err != nil {
		return err
	}
	var red *openf1.RaceControl
	for i, m := range msgs {
		if ev.match(m) && m.LapNumber != nil && *m.LapNumber >= fromLap {
			red = &msgs[i]
			break
		}
	}
	if red == nil {
		return fmt.Errorf("session %d has no %s from lap %d", sessionKey, ev.banner, fromLap)
	}
	lap := 0
	if red.LapNumber != nil {
		lap = *red.LapNumber
	}

	drivers, err := of1.Drivers(ctx, openf1.SessionDriverFilter{SessionKey: key})
	if err != nil {
		return err
	}
	byNumber := map[int]openf1.Driver{}
	for _, d := range drivers {
		byNumber[d.DriverNumber] = d
	}

	o := tracks.Overlay{
		Banner:      fmt.Sprintf("%s · %s · Lap %d · %s UTC", ev.banner, s.SessionName, lap, red.Date.UTC().Format("15:04")),
		BannerColor: ev.color,
	}

	// The first yellow flag in the 90 s before the event is when the
	// incident happened. Flags from then until the event are shown.
	anchor, flagsFrom, flagsTo := red.Date.Add(-25*time.Second), red.Date.Add(-90*time.Second), red.Date
	if event == "yellow" {
		anchor, flagsFrom, flagsTo = red.Date, red.Date.Add(-time.Second), red.Date.Add(10*time.Second)
	} else {
		// The first yellow still showing when the event came; earlier ones
		// already cleared belong to other incidents.
		for i, m := range msgs {
			if m.Flag == nil || !strings.Contains(*m.Flag, "YELLOW") || m.Sector == nil || m.Date.Before(flagsFrom) || m.Date.After(red.Date) {
				continue
			}
			if !clearedBefore(msgs[i+1:], *m.Sector, red.Date) {
				anchor, flagsFrom = m.Date, m.Date.Add(-time.Second)
				break
			}
		}
	}

	// Sector flags around the event. The worst ones, and their neighbours,
	// are where to look for cars.
	flags := map[int]string{}
	for _, m := range msgs {
		if m.Date.Before(flagsFrom) || m.Date.After(flagsTo) || m.Sector == nil || m.Flag == nil {
			continue
		}
		switch *m.Flag {
		case "DOUBLE YELLOW":
			flags[*m.Sector] = "DOUBLE YELLOW"
		case "YELLOW":
			if flags[*m.Sector] == "" {
				flags[*m.Sector] = "YELLOW"
			}
		}
	}
	sectors := make([]int, 0, len(flags))
	worst := "YELLOW"
	for n, f := range flags {
		sectors = append(sectors, n)
		if f == "DOUBLE YELLOW" {
			worst = f
		}
	}
	slices.Sort(sectors)
	focus := map[int]bool{}
	for n, f := range flags {
		if f == worst {
			focus[n], focus[prevSector(t, n)], focus[nextSector(t, n)] = true, true, true
		}
	}

	// Cars racing before the incident (a crash comes a few seconds before
	// race control shows the flag) that then stopped, or slowed to under 30%
	// of their speed, in the flagged part of the track; and the named
	// drivers wherever they were when it happened.
	var stopped, slowed, named []string
	var crash []tracks.Point
	stoppedCars := map[int]bool{}
	for _, d := range drivers {
		locs, err := of1.Locations(ctx, openf1.WindowFilter{
			SessionKey: key, DriverNumber: d.DriverNumber,
			After: anchor.Add(-25 * time.Second), Before: anchor.Add(20 * time.Second),
		})
		if err != nil {
			return err
		}
		if len(locs) < 2 {
			continue
		}
		if slices.Contains(involved, d.DriverNumber) {
			at := positionNear(locs, anchor)
			crash = append(crash, at)
			named = append(named, d.NameAcronym)
			o.Markers = append(o.Markers, tracks.Marker{Position: at, Label: d.NameAcronym, Color: d.TeamColour})
			continue
		}
		speedBefore := dist(positionNear(locs, anchor.Add(-25*time.Second)), positionNear(locs, anchor.Add(-12*time.Second))) / 13
		end := locs[len(locs)-1]
		endPt := tracks.Point{float64(end.X), float64(end.Y)}
		speedAfter := dist(positionNear(locs, anchor.Add(3*time.Second)), endPt) / 17
		if speedBefore < 20 || offTrack(t, endPt) { // 72 km/h
			continue
		}
		inFocus := len(focus) == 0 || focus[t.Locate(endPt).MarshalSector]
		switch {
		case speedAfter < 2 && inFocus: // about 7 km/h
			stopped = append(stopped, d.NameAcronym)
		case speedAfter < speedBefore*0.3 && inFocus && len(focus) > 0:
			slowed = append(slowed, d.NameAcronym)
		default:
			continue
		}
		crash = append(crash, endPt)
		stoppedCars[d.DriverNumber] = true
		o.Markers = append(o.Markers, tracks.Marker{Position: endPt, Label: d.NameAcronym, Color: d.TeamColour})
	}

	// Cars that crashed well before the event have already stopped, often
	// in a gravel trap, so fall back to the cars that retired on this lap or
	// the one before, marked where they were when the event came.
	var retired []string
	if len(crash) == 0 {
		results, err := of1.SessionResults(ctx, openf1.SessionFilter{SessionKey: key})
		if err != nil {
			return err
		}
		for _, r := range results {
			if !r.DNF || r.NumberOfLaps < lap-2 || r.NumberOfLaps > lap {
				continue
			}
			d := byNumber[r.DriverNumber]
			locs, err := of1.Locations(ctx, openf1.WindowFilter{
				SessionKey: key, DriverNumber: r.DriverNumber,
				After: red.Date.Add(-10 * time.Second), Before: red.Date,
			})
			if err != nil {
				return err
			}
			if len(locs) == 0 {
				continue
			}
			end := locs[len(locs)-1]
			pt := tracks.Point{float64(end.X), float64(end.Y)}
			if offTrack(t, pt) {
				continue
			}
			crash = append(crash, pt)
			retired = append(retired, d.NameAcronym)
			stoppedCars[r.DriverNumber] = true
			o.Markers = append(o.Markers, tracks.Marker{Position: pt, Label: d.NameAcronym, Color: d.TeamColour})
		}
	}

	// Where the incident was: the cars, else the flags. Flags near the cars
	// are the focus; others are drawn faintly for context.
	relevant := map[int]bool{}
	var where tracks.Location
	if len(crash) > 0 {
		where = t.Locate(centroid(crash))
		for _, pt := range crash {
			if n := t.Locate(pt).MarshalSector; n > 0 {
				relevant[n], relevant[prevSector(t, n)], relevant[nextSector(t, n)] = true, true, true
			}
		}
	}
	for _, n := range sectors {
		from, to, ok := t.MarshalSectorSpan(n)
		if !ok {
			continue
		}
		h := tracks.Highlight{From: from, To: to, Color: yellowColor, Style: tracks.Band, Label: fmt.Sprintf("Yellow · sector %d", n)}
		if flags[n] == "DOUBLE YELLOW" {
			h.Color, h.Style, h.Label = doubleYellowColor, tracks.DoubleBand, fmt.Sprintf("Double yellow · sector %d", n)
		}
		h.Faint = len(relevant) > 0 && !relevant[n]
		o.Highlights = append(o.Highlights, h)
	}
	if len(crash) == 0 && len(sectors) > 0 {
		from, to, _ := t.MarshalSectorSpan(sectors[0])
		where = t.Locate(midpoint(t, from, to))
	}

	// Incidents race control named at a turn on the same lap.
	var noted []string
	for _, m := range msgs {
		if m.LapNumber == nil || *m.LapNumber != lap || m.Date.Before(red.Date.Add(-time.Minute)) {
			continue
		}
		sub := incidentMessageRE.FindStringSubmatch(m.Message)
		if sub == nil {
			continue
		}
		turn, _ := strconv.Atoi(sub[1])
		pos, ok := cornerPosition(t, turn)
		if !ok {
			continue
		}
		var names []string
		for _, car := range carRE.FindAllStringSubmatch(sub[2], -1) {
			n, _ := strconv.Atoi(car[1])
			if stoppedCars[n] || slices.Contains(names, car[2]) {
				continue
			}
			names = append(names, car[2])
			o.Markers = append(o.Markers, tracks.Marker{Position: pos, Label: car[2], Color: byNumber[n].TeamColour})
		}
		if len(names) > 0 {
			note := fmt.Sprintf("turn %d incident, %s", turn, strings.Join(names, " and "))
			if !slices.Contains(noted, note) {
				noted = append(noted, note)
			}
		}
	}

	var caption []string
	if len(named) > 0 {
		caption = append(caption, strings.Join(named, ", "))
	}
	if len(stopped) > 0 {
		caption = append(caption, "Stopped: "+strings.Join(stopped, ", "))
	}
	if len(slowed) > 0 {
		caption = append(caption, "Slowed: "+strings.Join(slowed, ", "))
	}
	if len(retired) > 0 {
		caption = append(caption, "Retired: "+strings.Join(retired, ", "))
	}
	if s := where.String(); s != "" {
		caption = append(caption, s)
	}
	if len(noted) > 0 {
		caption = append(caption, "Also noted: "+strings.Join(noted, "; "))
	}
	o.Caption = strings.Join(caption, " · ")

	corner := 0
	switch {
	case where.At != nil:
		corner = where.At.Number
	case where.Before != nil:
		corner = nearestCornerNumber(t, crash, *where.Before, *where.After)
	}

	inc := Incident{
		Name:       fmt.Sprintf("%s-%d-%s-lap-%d-%s", t.CircuitID, s.Year, slug(s.SessionName), lap, ev.slug),
		CircuitID:  t.CircuitID,
		Year:       s.Year,
		SessionKey: sessionKey,
		Session:    meeting + " · " + s.SessionName,
		Corner:     corner,
		Overlay:    o,
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, inc.Name+".json"), inc); err != nil {
		return err
	}
	log.Printf("%s: %s", inc.Name, o.Caption)
	return nil
}

func loadIncidents(dir string) ([]Incident, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var out []Incident
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var inc Incident
		if err := json.Unmarshal(b, &inc); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out = append(out, inc)
	}
	return out, nil
}

// trackByKey finds the stored track with an OpenF1 circuit key.
func trackByKey(circuitKey, year int) (*tracks.Track, error) {
	ids, err := tracks.Circuits()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		t, err := tracks.Load(id, year)
		if err != nil {
			return nil, err
		}
		if t.CircuitKey == circuitKey {
			return t, nil
		}
	}
	return nil, errors.New("no stored track for this circuit")
}

// offTrack reports whether a point is more than 150 m from the racing line,
// e.g. a car in the paddock. Cars in gravel traps are still on track.
func offTrack(t *tracks.Track, p tracks.Point) bool {
	best := math.Inf(1)
	for _, q := range t.Outline {
		best = min(best, math.Hypot(q[0]-p[0], q[1]-p[1]))
	}
	return best/10 > 150
}

// clearedBefore reports whether a marshal sector's flag is cleared in msgs
// before t.
func clearedBefore(msgs []openf1.RaceControl, sector int, t time.Time) bool {
	for _, m := range msgs {
		if m.Date.After(t) {
			return false
		}
		if m.Sector != nil && *m.Sector == sector && m.Flag != nil && *m.Flag == "CLEAR" {
			return true
		}
	}
	return false
}

// positionNear returns the location sample closest in time to t.
func positionNear(locs []openf1.Location, t time.Time) tracks.Point {
	best := locs[0]
	for _, l := range locs {
		if l.Date.Sub(t).Abs() < best.Date.Sub(t).Abs() {
			best = l
		}
	}
	return tracks.Point{float64(best.X), float64(best.Y)}
}

// dist returns the distance between two points in metres.
func dist(a, b tracks.Point) float64 {
	return math.Hypot(a[0]-b[0], a[1]-b[1]) / 10
}

func centroid(pts []tracks.Point) tracks.Point {
	var c tracks.Point
	for _, p := range pts {
		c[0] += p[0] / float64(len(pts))
		c[1] += p[1] / float64(len(pts))
	}
	return c
}

// midpoint returns the outline point halfway between two points along the lap.
func midpoint(t *tracks.Track, from, to tracks.Point) tracks.Point {
	n := len(t.Outline)
	a, b := nearest(t.Outline, from), nearest(t.Outline, to)
	if b < a {
		b += n
	}
	return t.Outline[((a+b)/2)%n]
}

func prevSector(t *tracks.Track, n int) int {
	i := slices.IndexFunc(t.MarshalSectors, func(m tracks.MarshalSector) bool { return m.Number == n })
	if i < 0 {
		return n
	}
	return t.MarshalSectors[(i-1+len(t.MarshalSectors))%len(t.MarshalSectors)].Number
}

func nextSector(t *tracks.Track, n int) int {
	i := slices.IndexFunc(t.MarshalSectors, func(m tracks.MarshalSector) bool { return m.Number == n })
	if i < 0 {
		return n
	}
	return t.MarshalSectors[(i+1)%len(t.MarshalSectors)].Number
}

func cornerPosition(t *tracks.Track, turn int) (tracks.Point, bool) {
	for _, c := range t.Corners {
		if c.Number == turn {
			return c.Position, true
		}
	}
	return tracks.Point{}, false
}

// nearestCornerNumber picks whichever of two corners is closer to the points.
func nearestCornerNumber(t *tracks.Track, pts []tracks.Point, a, b tracks.Corner) int {
	if len(pts) == 0 {
		return a.Number
	}
	c := centroid(pts)
	da := math.Hypot(a.Position[0]-c[0], a.Position[1]-c[1])
	db := math.Hypot(b.Position[0]-c[0], b.Position[1]-c[1])
	if db < da {
		return b.Number
	}
	return a.Number
}

func slug(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), " ", "-")
}

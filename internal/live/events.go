package live

import (
	"fmt"
	"strings"
	"time"
)

// Event is something worth telling: a flag, a pit stop, an overtake, a car
// closing in. Text is a plain description; the insight agent explains it.
type Event struct {
	ID       int       `json:"id"`
	Time     time.Time `json:"time"`
	Lap      int       `json:"lap,omitempty"`
	Kind     string    `json:"kind"`     // flag, penalty, lead, pit, fastest_lap, overtake, closing, pace, weather, radio
	Priority int       `json:"priority"` // 1 minor, 2 notable, 3 major
	Drivers  []string  `json:"drivers,omitempty"`
	Text     string    `json:"text"`
}

// Event kinds.
const (
	KindFlag       = "flag"
	KindPenalty    = "penalty"
	KindLead       = "lead"
	KindPit        = "pit"
	KindFastestLap = "fastest_lap"
	KindOvertake   = "overtake"
	KindClosing    = "closing"
	KindPace       = "pace"
	KindWeather    = "weather"
	KindRadio      = "radio"
)

// How far apart, in laps, the same pair of cars can be reported as closing
// in or having a pace difference.
const (
	closingEvery = 5
	paceEvery    = 8
)

// lapRecord is a completed lap kept for pace comparisons.
type lapRecord struct {
	lap      int
	seconds  float64
	racing   bool     // a green-flag lap, not in or out of the pits
	interval *float64 // to the car ahead when the lap ended
	compound string
	age      int
}

func (s *State) emit(kind string, priority int, drivers []string, format string, args ...any) {
	s.nextID++
	e := Event{ID: s.nextID, Time: s.now, Lap: s.leaderLap, Kind: kind, Priority: priority, Drivers: drivers, Text: fmt.Sprintf(format, args...)}
	s.events = append(s.events, e)
	s.pending = append(s.pending, e)
}

// EventsSince returns the events after the one with the given ID (0 for
// all), oldest first.
func (s *State) EventsSince(id int) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i, e := range s.events {
		if e.ID > id {
			return append([]Event(nil), s.events[i:]...)
		}
	}
	return nil
}

func (s *State) flagEvent(prev Flag, lap int) {
	switch s.flag {
	case SC:
		s.emit(KindFlag, 3, nil, "Safety car deployed on lap %d", lap)
	case VSC:
		s.emit(KindFlag, 3, nil, "Virtual safety car on lap %d", lap)
	case Red:
		s.emit(KindFlag, 3, nil, "Red flag on lap %d: the session is stopped", lap)
	case Ended:
		s.emit(KindFlag, 3, nil, "Chequered flag")
	case Green:
		if prev == SC || prev == VSC || prev == Red {
			s.emit(KindFlag, 3, nil, "Green flag: racing resumes on lap %d", lap)
		}
	}
}

// penaltyText tidies a stewards' message, e.g. "FIA STEWARDS: 10 SECOND
// TIME PENALTY FOR CAR 81 (PIA) - CAUSING A COLLISION" into "10 second time
// penalty for PIA: causing a collision".
func penaltyText(msg, code string) string {
	msg = strings.TrimPrefix(msg, "FIA STEWARDS: ")
	what, why, _ := strings.Cut(msg, " - ")
	what = strings.ToLower(what)
	if i := strings.Index(what, " for car "); i >= 0 && code != "" {
		what = what[:i] + " for " + code
	}
	if why != "" {
		return what + ": " + strings.ToLower(why)
	}
	return what
}

func (s *State) carAhead(c *Car) *Car {
	if c.Position <= 1 {
		return nil
	}
	for _, o := range s.cars {
		if o.Position == c.Position-1 && o.Out == "" {
			return o
		}
	}
	return nil
}

// lapDone records a completed lap and looks for a car closing in on, or
// clearly faster than, the car ahead.
func (s *State) lapDone(c *Car, lt *LapTime) {
	pitted := false
	for _, st := range c.Stops {
		if st.Lap == lt.Lap {
			pitted = true
		}
	}
	rec := lapRecord{lap: lt.Lap, seconds: lt.Seconds, racing: !lt.PitOut && !pitted && (s.flag == Green || s.flag == Yellow), compound: c.Compound, age: c.TyreAge}
	if c.Interval != nil {
		v := *c.Interval
		rec.interval = &v
	}
	c.laps = append(c.laps, rec)
	if len(c.laps) > 10 {
		c.laps = c.laps[1:]
	}
	ahead := s.carAhead(c)
	if ahead == nil || !rec.racing {
		return
	}

	// Closing: the gap to the car ahead down on each of the last 3 laps.
	if n := len(c.laps); n >= 4 {
		last := c.laps[n-4:]
		closing := true
		for i, l := range last {
			if !l.racing || l.interval == nil || (i > 0 && *l.interval >= *last[i-1].interval) {
				closing = false
				break
			}
		}
		if closing {
			first, now := *last[0].interval, *last[3].interval
			key := "closing:" + c.Code + ">" + ahead.Code
			if first-now >= 0.6 && now < 2.5 && lt.Lap-s.reported[key] >= closingEvery {
				s.reported[key] = lt.Lap
				priority := 1
				if ahead.Position <= 10 {
					priority = 2
				}
				s.emit(KindClosing, priority, []string{c.Code, ahead.Code}, "%s is closing on %s for P%d: %.1f s behind, gaining %.2f s a lap",
					c.Code, ahead.Code, ahead.Position, now, (first-now)/3)
			}
		}
	}

	// Pace: over the last 3 racing laps each, clearly faster than the car
	// ahead on different tyres.
	mine, theirs := recentPace(c.laps, 3), recentPace(ahead.laps, 3)
	if mine == 0 || theirs == 0 || rec.interval == nil || *rec.interval > 8 {
		return
	}
	diff := theirs - mine
	differentTyres := c.Compound != ahead.Compound || abs(c.TyreAge-ahead.TyreAge) >= 5
	key := "pace:" + c.Code + ">" + ahead.Code
	if diff >= 0.5 && differentTyres && lt.Lap-s.reported[key] >= paceEvery {
		s.reported[key] = lt.Lap
		s.emit(KindPace, 2, []string{c.Code, ahead.Code}, "%s is %.1f s a lap faster than %s over the last 3 laps: %s against %s",
			c.Code, diff, ahead.Code, tyreText(c.Compound, c.TyreAge), tyreText(ahead.Compound, ahead.TyreAge))
	}
}

// recentPace is the average of the last n racing laps, or 0 if there
// aren't n of them in a row.
func recentPace(laps []lapRecord, n int) float64 {
	if len(laps) < n {
		return 0
	}
	sum := 0.0
	for _, l := range laps[len(laps)-n:] {
		if !l.racing {
			return 0
		}
		sum += l.seconds
	}
	return sum / float64(n)
}

func tyreText(compound string, age int) string {
	c := strings.ToLower(compound)
	if c == "intermediate" {
		c = "intermediates"
	} else if c != "" {
		c += "s"
	}
	if age == 0 {
		return "new " + c
	}
	return fmt.Sprintf("%d-lap %s", age, c)
}

func lapTimeText(s float64) string {
	m := int(s) / 60
	return fmt.Sprintf("%d:%06.3f", m, s-float64(m*60))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// pitEvent reports a car's new tyres, with its last stop. New tyres can
// arrive before the stop itself, so the lap just finished, the in-lap, is
// marked as not a racing lap here too.
func (s *State) pitEvent(c *Car, compound string, age int) {
	for i := range c.laps {
		if c.laps[i].lap >= c.Lap {
			c.laps[i].racing = false
		}
	}
	was := tyreText(c.Compound, c.TyreAge)
	now := tyreText(compound, age)
	if s.flag == Red {
		s.emit(KindPit, 2, []string{c.Code}, "%s changes tyres under the red flag: %s for %s", c.Code, was, now)
		return
	}
	text := fmt.Sprintf("%s pits from P%d: %s for %s", c.Code, c.Position, was, now)
	if n := len(c.Stops); n > 0 && c.Stops[n-1].StopSeconds != nil {
		text += fmt.Sprintf(", %.1f s stationary", *c.Stops[n-1].StopSeconds)
	}
	s.emit(KindPit, 2, []string{c.Code}, "%s", text)
}

// rainWatch reports rain starting or stopping once it has been so for
// rainReadings readings in a row (about one a minute), and not before the
// race is under way, so a passing shower on the grid isn't news.
type rainWatch struct {
	reported *bool
	pending  bool
	count    int
}

const rainReadings = 3

func (s *State) rainChange(rain bool, trackC float64) {
	w := &s.rain
	if w.reported == nil {
		w.reported = &rain
		return
	}
	if rain == *w.reported {
		w.count = 0
		return
	}
	if rain != w.pending {
		w.pending, w.count = rain, 0
	}
	w.count++
	if w.count < rainReadings || !s.started {
		return
	}
	*w.reported, w.count = rain, 0
	if rain {
		s.emit(KindWeather, 2, nil, "Rain is falling again (track %.0f °C)", trackC)
	} else {
		s.emit(KindWeather, 2, nil, "The rain has stopped (track %.0f °C)", trackC)
	}
}

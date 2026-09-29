package f1

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
)

// sessionData is an OpenF1 session and its drivers.
type sessionData struct {
	Event   Event
	Session string
	key     string
	year    int
	drivers map[int]DriverRef
	teams   map[int]string
	// part, if set, is the part of qualifying (1 to 3) that laps are
	// limited to: those started from partFrom until partTo (zero for the
	// end of the session).
	part             int
	partFrom, partTo time.Time
}

// Parts of qualifying, which the lap tools take as sessions.
var qualifyingParts = map[string]struct {
	session string
	part    int
}{
	"q1": {Qualifying, 1}, "q2": {Qualifying, 2}, "q3": {Qualifying, 3},
	"sq1": {SprintQualifying, 1}, "sq2": {SprintQualifying, 2}, "sq3": {SprintQualifying, 3},
}

// SplitSession splits a session given to a lap tool into the session and
// the part of qualifying, e.g. "q2" into "qualifying" and 2. Other sessions
// have part 0.
func SplitSession(session string) (string, int) {
	if p, ok := qualifyingParts[strings.ToLower(session)]; ok {
		return p.session, p.part
	}
	return session, 0
}

// ValidLapSession checks a session given to a lap tool: a session, or a
// part of qualifying.
func ValidLapSession(session string) error {
	if _, part := SplitSession(session); part > 0 {
		return nil
	}
	if err := ValidSession(session); err != nil {
		return fmt.Errorf("%w, or a part of qualifying: q1, q2, q3, sq1, sq2 or sq3", err)
	}
	return nil
}

// lapSession is session for the lap tools, which also take a part of
// qualifying, e.g. "q2", and then only see laps started in that part.
func (s *Service) lapSession(ctx context.Context, e Event, session string) (*sessionData, error) {
	if err := ValidLapSession(session); err != nil {
		return nil, err
	}
	base, part := SplitSession(session)
	if part == 0 {
		return s.session(ctx, e, session)
	}
	sd, err := s.session(ctx, e, base)
	if err != nil {
		return nil, err
	}
	if sd.partFrom, sd.partTo, err = s.partWindow(ctx, sd.key, sd.year, part); err != nil {
		return nil, fmt.Errorf("%d %s: %w", e.Year, e.Name, err)
	}
	sd.part, sd.Session = part, strings.ToLower(session)
	return sd, nil
}

// partWindow returns when a part of qualifying started, and when the next
// part started (zero for the last), from race control's messages.
func (s *Service) partWindow(ctx context.Context, key string, year, part int) (from, to time.Time, err error) {
	msgs, err := memo(s, "openf1/race_control/"+key, year, func() ([]openf1.RaceControl, error) {
		return s.of1.RaceControl(ctx, openf1.SessionFilter{SessionKey: key})
	})
	if err != nil {
		return from, to, err
	}
	starts := map[int]time.Time{}
	for _, m := range msgs {
		if m.QualifyingPhase != nil && m.Message == "SESSION STARTED" {
			if _, ok := starts[*m.QualifyingPhase]; !ok {
				starts[*m.QualifyingPhase] = m.Date
			}
		}
	}
	from, ok := starts[part]
	if !ok {
		return from, to, fmt.Errorf("no record of when part %d of qualifying started", part)
	}
	return from, starts[part+1], nil
}

// inPart reports whether a lap started in the session's part of
// qualifying, or the session has no part.
func (sd *sessionData) inPart(l openf1.Lap) bool {
	if sd.part == 0 {
		return true
	}
	return !l.DateStart.Before(sd.partFrom) && (sd.partTo.IsZero() || l.DateStart.Before(sd.partTo))
}

// session finds an event's session in OpenF1, with its drivers.
func (s *Service) session(ctx context.Context, e Event, session string) (*sessionData, error) {
	if err := ValidSession(session); err != nil {
		return nil, err
	}
	os, err := s.openf1Session(ctx, e, session)
	if err != nil {
		return nil, err
	}
	key := strconv.Itoa(os.SessionKey)
	ds, err := memo(s, "openf1/drivers/"+key, e.Year, func() ([]openf1.Driver, error) {
		return s.of1.Drivers(ctx, openf1.SessionDriverFilter{SessionKey: key})
	})
	if err != nil {
		return nil, err
	}
	sd := &sessionData{Event: e, Session: session, key: key, year: e.Year, drivers: map[int]DriverRef{}, teams: map[int]string{}}
	for _, d := range ds {
		sd.drivers[d.DriverNumber] = DriverRef{Code: d.NameAcronym, Number: d.DriverNumber, Name: d.FirstName + " " + d.LastName}
		sd.teams[d.DriverNumber] = d.TeamName
	}
	return sd, nil
}

// driver returns the number of the driver named by arg: a car number, a
// code such as "VER", or part of their name.
func (sd *sessionData) driver(arg string) (int, error) {
	arg = strings.TrimSpace(arg)
	if n, err := strconv.Atoi(arg); err == nil {
		if _, ok := sd.drivers[n]; ok {
			return n, nil
		}
		return 0, fmt.Errorf("no car %d in %d %s %s", n, sd.Event.Year, sd.Event.Name, sd.Session)
	}
	var matches []int
	for n, d := range sd.drivers {
		if strings.EqualFold(d.Code, arg) {
			return n, nil
		}
		if strings.Contains(strings.ToLower(d.Name), strings.ToLower(arg)) {
			matches = append(matches, n)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return 0, fmt.Errorf("%q matches several drivers; use their code", arg)
	}
	return 0, fmt.Errorf("no driver %q in %d %s %s", arg, sd.Event.Year, sd.Event.Name, sd.Session)
}

// DriverNumbers returns the car numbers of drivers in an event's session,
// each given by car number, code or name.
func (s *Service) DriverNumbers(ctx context.Context, e Event, session string, drivers []string) ([]int, error) {
	if len(drivers) == 0 {
		return nil, nil
	}
	sd, err := s.session(ctx, e, session)
	if err != nil {
		return nil, err
	}
	var out []int
	for _, d := range drivers {
		n, err := sd.driver(d)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func (sd *sessionData) ref(number int) DriverRef {
	if d, ok := sd.drivers[number]; ok {
		return d
	}
	return DriverRef{Number: number}
}

// laps returns every lap of the session, or of its part of qualifying.
func (s *Service) laps(ctx context.Context, sd *sessionData) ([]openf1.Lap, error) {
	all, err := memo(s, "openf1/laps/"+sd.key, sd.year, func() ([]openf1.Lap, error) {
		return s.of1.Laps(ctx, openf1.LapsFilter{SessionKey: sd.key})
	})
	if err != nil || sd.part == 0 {
		return all, err
	}
	var out []openf1.Lap
	for _, l := range all {
		if sd.inPart(l) {
			out = append(out, l)
		}
	}
	return out, nil
}

// Lap choices, besides a lap number.
const (
	PickBest       = "best"        // the fastest
	PickFirst      = "first"       // the first timed lap that isn't an out-lap
	PickLast       = "last"        // the last timed lap that isn't an out-lap
	PickLastFlying = "last_flying" // the last push lap: not an out-lap, and within 107% of the best
	flyingLapLimit = 1.07
)

// pickLap chooses one of a driver's laps: by number (e.g. "17"), or best,
// first, last or last_flying ("" is best). Only timed laps are chosen.
func pickLap(laps []openf1.Lap, choice string) (openf1.Lap, error) {
	var timed []openf1.Lap
	for _, l := range laps {
		if l.LapDuration != nil && !l.DateStart.IsZero() {
			timed = append(timed, l)
		}
	}
	slices.SortFunc(timed, func(a, b openf1.Lap) int { return a.LapNumber - b.LapNumber })
	if n, err := strconv.Atoi(choice); err == nil {
		for _, l := range timed {
			if l.LapNumber == n {
				return l, nil
			}
		}
		return openf1.Lap{}, fmt.Errorf("no timed lap %d", n)
	}
	var full []openf1.Lap // not out-laps
	best := -1
	for _, l := range timed {
		if l.IsPitOutLap {
			continue
		}
		full = append(full, l)
		if best < 0 || *l.LapDuration < *full[best].LapDuration {
			best = len(full) - 1
		}
	}
	if len(full) == 0 {
		return openf1.Lap{}, fmt.Errorf("no timed laps")
	}
	switch strings.ToLower(choice) {
	case "", PickBest:
		return full[best], nil
	case PickFirst:
		return full[0], nil
	case PickLast:
		return full[len(full)-1], nil
	case PickLastFlying:
		limit := *full[best].LapDuration * flyingLapLimit
		for i := len(full) - 1; i >= 0; i-- {
			if *full[i].LapDuration <= limit {
				return full[i], nil
			}
		}
	}
	return openf1.Lap{}, fmt.Errorf("unknown lap %q (want a lap number, best, first, last or last_flying)", choice)
}

// Page is where a paged list continues. Pass NextCursor back as cursor to
// get the next page.
type Page struct {
	Total      int    `json:"total"`
	NextCursor string `json:"next_cursor,omitempty"`
}

const (
	defaultLimit = 200
	maxLimit     = 1000
)

// Paginate returns one page of items from cursor (an offset), and where
// the next page starts.
func Paginate[T any](items []T, cursor string, limit int) ([]T, Page) {
	if limit <= 0 {
		limit = defaultLimit
	}
	limit = min(limit, maxLimit)
	from, _ := strconv.Atoi(cursor)
	from = min(max(from, 0), len(items))
	to := min(from+limit, len(items))
	p := Page{Total: len(items)}
	if to < len(items) {
		p.NextCursor = strconv.Itoa(to)
	}
	return items[from:to], p
}

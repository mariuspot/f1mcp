package f1

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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

func (sd *sessionData) ref(number int) DriverRef {
	if d, ok := sd.drivers[number]; ok {
		return d
	}
	return DriverRef{Number: number}
}

// laps returns every lap of the session.
func (s *Service) laps(ctx context.Context, sd *sessionData) ([]openf1.Lap, error) {
	return memo(s, "openf1/laps/"+sd.key, sd.year, func() ([]openf1.Lap, error) {
		return s.of1.Laps(ctx, openf1.LapsFilter{SessionKey: sd.key})
	})
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

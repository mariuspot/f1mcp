package f1

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mariuspot/f1mcp/internal/jolpica"
	"github.com/mariuspot/f1mcp/internal/openf1"
)

// FirstDetailedYear is the first season OpenF1 has session detail for.
const FirstDetailedYear = 2023

// ErrNoDetail is returned for session detail before FirstDetailedYear.
var ErrNoDetail = fmt.Errorf("detailed session data is only available from %d", FirstDetailedYear)

// Service answers questions from Jolpica and OpenF1, caching what it
// fetches: past seasons never change, so they're kept; the current season
// is refetched after a while.
type Service struct {
	jol *jolpica.Client
	of1 *openf1.Client
	now func() time.Time

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	value   any
	expires time.Time // zero: never
}

// currentTTL is how long answers about the current season are kept.
const currentTTL = 10 * time.Minute

func New(jol *jolpica.Client, of1 *openf1.Client) *Service {
	return &Service{jol: jol, of1: of1, now: time.Now, cache: map[string]cached{}}
}

// memo returns the cached answer for key, or computes and caches it. year
// decides how long it's kept.
func memo[T any](s *Service, key string, year int, fetch func() (T, error)) (T, error) {
	s.mu.Lock()
	c, ok := s.cache[key]
	s.mu.Unlock()
	if ok && (c.expires.IsZero() || s.now().Before(c.expires)) {
		return c.value.(T), nil
	}
	v, err := fetch()
	if err != nil {
		return v, err
	}
	var expires time.Time
	if year >= s.now().Year() {
		expires = s.now().Add(currentTTL)
	}
	s.mu.Lock()
	s.cache[key] = cached{value: v, expires: expires}
	s.mu.Unlock()
	return v, nil
}

// Year returns y, or the current year if y is 0.
func (s *Service) Year(y int) int {
	return s.year(y)
}

// year returns y, or the current year if y is 0.
func (s *Service) year(y int) int {
	if y == 0 {
		return s.now().Year()
	}
	return y
}

// ResolveEvent finds a race weekend by round: a number, "last" (the latest
// weekend whose race has started), "next" (the first whose race hasn't),
// or part of its name, circuit, city or country, e.g. "Monaco" or "Spa".
// An empty round means "last".
func (s *Service) ResolveEvent(ctx context.Context, year int, round string) (Event, error) {
	year = s.year(year)
	events, err := s.Schedule(ctx, year)
	if err != nil {
		return Event{}, err
	}
	if len(events) == 0 {
		return Event{}, fmt.Errorf("no races found for %d", year)
	}
	round = strings.TrimSpace(strings.ToLower(round))
	now := s.now()
	switch round {
	case "", "last", "latest":
		var last *Event
		for i, e := range events {
			if start, ok := e.Start(Race); ok && !start.After(now) {
				last = &events[i]
			}
		}
		if last == nil {
			return Event{}, fmt.Errorf("no %d race has happened yet", year)
		}
		return *last, nil
	case "next":
		for _, e := range events {
			if start, ok := e.Start(Race); ok && start.After(now) {
				return e, nil
			}
		}
		return Event{}, fmt.Errorf("no %d races left", year)
	}
	if n, err := strconv.Atoi(round); err == nil {
		for _, e := range events {
			if e.Round == n {
				return e, nil
			}
		}
		return Event{}, fmt.Errorf("%d has no round %d (it has %d rounds)", year, n, len(events))
	}
	var matches []Event
	for _, e := range events {
		for _, field := range []string{e.Name, e.Circuit.Name, e.Circuit.Locality, e.Circuit.Country, e.Circuit.ID} {
			if strings.Contains(strings.ToLower(field), round) {
				matches = append(matches, e)
				break
			}
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return Event{}, fmt.Errorf("no %d race matches %q", year, round)
	default:
		var names []string
		for _, m := range matches {
			names = append(names, fmt.Sprintf("round %d %s", m.Round, m.Name))
		}
		return Event{}, fmt.Errorf("%q matches several %d races: %s", round, year, strings.Join(names, "; "))
	}
}

// ValidSession checks a session name.
func ValidSession(session string) error {
	for _, n := range SessionNames {
		if n == session {
			return nil
		}
	}
	return fmt.Errorf("unknown session %q (want one of %s)", session, strings.Join(SessionNames, ", "))
}

// openf1Names are OpenF1's session names for each session; the first is
// the current one.
var openf1Names = map[string][]string{
	Race:             {"Race"},
	Qualifying:       {"Qualifying"},
	Sprint:           {"Sprint"},
	SprintQualifying: {"Sprint Qualifying", "Sprint Shootout"},
	Practice1:        {"Practice 1"},
	Practice2:        {"Practice 2"},
	Practice3:        {"Practice 3"},
}

// openf1Session finds OpenF1's session for an event's session.
func (s *Service) openf1Session(ctx context.Context, e Event, session string) (openf1.Session, error) {
	if e.Year < FirstDetailedYear {
		return openf1.Session{}, ErrNoDetail
	}
	sessions, err := memo(s, fmt.Sprintf("openf1/sessions/%d", e.Year), e.Year, func() ([]openf1.Session, error) {
		return s.of1.Sessions(ctx, openf1.SessionsFilter{Year: e.Year})
	})
	if err != nil {
		return openf1.Session{}, err
	}
	// The meeting is the one whose race starts when the event's does.
	race, ok := e.Start(Race)
	if !ok {
		return openf1.Session{}, fmt.Errorf("%d %s has no race start time", e.Year, e.Name)
	}
	meeting := 0
	for _, os := range sessions {
		if os.SessionName == "Race" && os.DateStart.Sub(race).Abs() <= 36*time.Hour {
			meeting = os.MeetingKey
		}
	}
	if meeting == 0 {
		return openf1.Session{}, fmt.Errorf("OpenF1 has no data for %d %s yet", e.Year, e.Name)
	}
	for _, name := range openf1Names[session] {
		for _, os := range sessions {
			if os.MeetingKey == meeting && os.SessionName == name {
				if os.IsCancelled {
					return openf1.Session{}, fmt.Errorf("%d %s %s was cancelled", e.Year, e.Name, session)
				}
				return os, nil
			}
		}
	}
	return openf1.Session{}, fmt.Errorf("%d %s has no %s session", e.Year, e.Name, session)
}

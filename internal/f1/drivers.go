package f1

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mariuspot/f1mcp/internal/jolpica"
	"github.com/mariuspot/f1mcp/internal/openf1"
)

// Drivers returns the drivers in a season, or in one event if e is set.
// From 2023 they come with their race number, team colour and headshot
// link from OpenF1 (for the event's race, or the season's latest session).
func (s *Service) Drivers(ctx context.Context, year int, e *Event) ([]Driver, error) {
	year = s.year(year)
	round, roundKey := "", "season"
	if e != nil {
		year, round, roundKey = e.Year, strconv.Itoa(e.Round), strconv.Itoa(e.Round)
	}
	return memo(s, fmt.Sprintf("drivers/%d/%s", year, roundKey), year, func() ([]Driver, error) {
		base, err := s.jol.Drivers(ctx, strconv.Itoa(year), round)
		if err != nil {
			return nil, err
		}
		// Teams from the standings at that point, keyed by driver code.
		teams := map[string]Team{}
		standingsRound := 0
		if e != nil {
			standingsRound = e.Round
		}
		if list, err := s.driverStandingsList(ctx, year, standingsRound); err == nil {
			for _, d := range list {
				if n := len(d.Constructors); n > 0 {
					c := d.Constructors[n-1]
					teams[d.Driver.DriverID] = Team{ID: c.ConstructorID, Name: c.Name}
				}
			}
		}
		live := map[string]openf1.Driver{}
		if year >= FirstDetailedYear {
			if ds, err := s.openf1Drivers(ctx, year, e); err == nil {
				for _, d := range ds {
					live[d.NameAcronym] = d
				}
			}
		}
		out := make([]Driver, 0, len(base))
		for _, b := range base {
			d := Driver{
				DriverRef:    driverRef(b, 0),
				Nationality:  b.Nationality,
				WikipediaURL: b.URL,
			}
			// Jolpica's team names are used everywhere; OpenF1 adds colour.
			if t, ok := teams[b.DriverID]; ok {
				d.Team = &t
			}
			if l, ok := live[b.Code]; ok && b.Code != "" {
				d.Number = l.DriverNumber
				if d.Team == nil {
					d.Team = &Team{Name: l.TeamName}
				}
				d.Team.Color = "#" + strings.TrimPrefix(l.TeamColour, "#")
				d.HeadshotURL = l.HeadshotURL
			}
			out = append(out, d)
		}
		return out, nil
	})
}

// driverStandingsList returns Jolpica's driver standings after a round, or
// the latest if round is 0.
func (s *Service) driverStandingsList(ctx context.Context, year, round int) ([]jolpica.DriverStanding, error) {
	r := ""
	if round > 0 {
		r = strconv.Itoa(round)
	}
	return memo(s, fmt.Sprintf("jolpica/driverstandings/%d/%s", year, r), year, func() ([]jolpica.DriverStanding, error) {
		list, err := s.jol.DriverStandings(ctx, strconv.Itoa(year), r)
		if err != nil {
			return nil, err
		}
		return list.DriverStandings, nil
	})
}

// openf1Drivers returns OpenF1's drivers for an event's race, or for the
// latest session of the year.
func (s *Service) openf1Drivers(ctx context.Context, year int, e *Event) ([]openf1.Driver, error) {
	key := "latest"
	if e != nil {
		os, err := s.openf1Session(ctx, *e, Race)
		if err != nil {
			os, err = s.openf1Session(ctx, *e, Qualifying)
		}
		if err != nil {
			return nil, err
		}
		key = strconv.Itoa(os.SessionKey)
	} else if year < s.now().Year() {
		// The season's last race.
		events, err := s.Schedule(ctx, year)
		if err != nil || len(events) == 0 {
			return nil, err
		}
		os, err := s.openf1Session(ctx, events[len(events)-1], Race)
		if err != nil {
			return nil, err
		}
		key = strconv.Itoa(os.SessionKey)
	}
	return s.of1.Drivers(ctx, openf1.SessionDriverFilter{SessionKey: key})
}

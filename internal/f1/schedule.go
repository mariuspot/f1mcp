package f1

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/mariuspot/f1mcp/internal/jolpica"
	"github.com/mariuspot/f1mcp/internal/openf1"
)

// Schedule returns a season's race weekends in round order.
func (s *Service) Schedule(ctx context.Context, year int) ([]Event, error) {
	year = s.year(year)
	return memo(s, fmt.Sprintf("schedule/%d", year), year, func() ([]Event, error) {
		races, err := s.jol.Schedule(ctx, strconv.Itoa(year))
		if err != nil {
			return nil, err
		}
		var meetings []openf1.Meeting
		if year >= FirstDetailedYear {
			// Only for flags; the schedule is still useful without them.
			meetings, _ = s.of1.Meetings(ctx, openf1.MeetingsFilter{Year: year})
		}
		events := make([]Event, 0, len(races))
		for _, r := range races {
			events = append(events, toEvent(r, meetings))
		}
		return events, nil
	})
}

func toEvent(r jolpica.Race, meetings []openf1.Meeting) Event {
	e := Event{
		Year:  atoi(r.Season),
		Round: atoi(r.Round),
		Name:  r.RaceName,
		Circuit: Circuit{
			ID:       r.Circuit.CircuitID,
			Name:     r.Circuit.CircuitName,
			Locality: r.Circuit.Location.Locality,
			Country:  r.Circuit.Location.Country,
		},
		Sprint: r.Sprint != nil,
	}
	add := func(session string, st *jolpica.SessionTime) {
		if st != nil {
			e.Sessions = append(e.Sessions, SessionStart{Session: session, Start: sessionTime(st.Date, st.Time)})
		}
	}
	add(Practice1, r.FirstPractice)
	add(Practice2, r.SecondPractice)
	add(Practice3, r.ThirdPractice)
	add(SprintQualifying, r.SprintQualifying)
	add(SprintQualifying, r.SprintShootout)
	add(Sprint, r.Sprint)
	add(Qualifying, r.Qualifying)
	add(Race, &jolpica.SessionTime{Date: r.Date, Time: r.Time})

	if race, ok := e.Start(Race); ok {
		for _, m := range meetings {
			if !race.Before(m.DateStart) && race.Before(m.DateEnd.Add(36*time.Hour)) && !m.IsCancelled {
				e.CountryFlagURL = m.CountryFlag
			}
		}
	}
	return e
}

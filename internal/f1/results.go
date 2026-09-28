package f1

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/mariuspot/f1mcp/internal/jolpica"
	"github.com/mariuspot/f1mcp/internal/openf1"
)

// SessionResults returns a session's classification: race, sprint and
// qualifying from Jolpica for any year, practice and sprint qualifying from
// OpenF1 from 2023.
func (s *Service) SessionResults(ctx context.Context, e Event, session string) ([]SessionResult, error) {
	if err := ValidSession(session); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("results/%d/%d/%s", e.Year, e.Round, session)
	return memo(s, key, e.Year, func() ([]SessionResult, error) {
		year, round := strconv.Itoa(e.Year), strconv.Itoa(e.Round)
		var (
			race *jolpica.Race
			err  error
		)
		switch session {
		case Race:
			race, err = s.jol.RaceResults(ctx, year, round)
		case Sprint:
			if !e.Sprint {
				return nil, fmt.Errorf("%d %s has no sprint", e.Year, e.Name)
			}
			race, err = s.jol.SprintResults(ctx, year, round)
		case Qualifying:
			race, err = s.jol.QualifyingResults(ctx, year, round)
		default:
			return s.openf1Results(ctx, e, session)
		}
		if errors.Is(err, jolpica.ErrNotFound) {
			return nil, fmt.Errorf("no %s results for %d %s yet", session, e.Year, e.Name)
		}
		if err != nil {
			return nil, err
		}
		if session == Qualifying {
			return qualifyingResults(race.QualifyingResults), nil
		}
		results := race.Results
		if session == Sprint {
			results = race.SprintResults
		}
		return raceResults(results), nil
	})
}

func raceResults(rs []jolpica.Result) []SessionResult {
	out := make([]SessionResult, 0, len(rs))
	for _, r := range rs {
		sr := SessionResult{
			Classified: r.PositionText,
			Driver:     driverRef(r.Driver, atoi(r.Number)),
			Team:       r.Constructor.Name,
			Grid:       atoi(r.Grid),
			Laps:       atoi(r.Laps),
			Status:     r.Status,
			Points:     atof(r.Points),
		}
		if _, err := strconv.Atoi(r.PositionText); err == nil {
			sr.Position = atoi(r.Position)
		}
		if r.Time != nil {
			if sr.Position == 1 {
				ms := atof(r.Time.Millis) / 1000
				sr.TimeSeconds = &ms
			} else {
				sr.GapSeconds = seconds(r.Time.Time)
			}
		}
		if r.FastestLap != nil {
			sr.FastestLap = seconds(r.FastestLap.Time.Time)
			sr.FastestLapNo = atoi(r.FastestLap.Lap)
		}
		out = append(out, sr)
	}
	return out
}

func qualifyingResults(qs []jolpica.QualifyingResult) []SessionResult {
	out := make([]SessionResult, 0, len(qs))
	for _, q := range qs {
		out = append(out, SessionResult{
			Position:   atoi(q.Position),
			Classified: q.Position,
			Driver:     driverRef(q.Driver, atoi(q.Number)),
			Team:       q.Constructor.Name,
			Q1:         seconds(q.Q1),
			Q2:         seconds(q.Q2),
			Q3:         seconds(q.Q3),
		})
	}
	return out
}

func driverRef(d jolpica.Driver, number int) DriverRef {
	if number == 0 {
		number = atoi(d.PermanentNumber)
	}
	return DriverRef{Code: d.Code, Number: number, Name: d.GivenName + " " + d.FamilyName}
}

// openf1Results reads practice and sprint qualifying results from OpenF1.
func (s *Service) openf1Results(ctx context.Context, e Event, session string) ([]SessionResult, error) {
	os, err := s.openf1Session(ctx, e, session)
	if err != nil {
		return nil, err
	}
	key := strconv.Itoa(os.SessionKey)
	results, err := s.of1.SessionResults(ctx, openf1.SessionFilter{SessionKey: key})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no %s results for %d %s yet", session, e.Year, e.Name)
	}
	drivers, err := s.of1.Drivers(ctx, openf1.SessionDriverFilter{SessionKey: key})
	if err != nil {
		return nil, err
	}
	byNumber := map[int]openf1.Driver{}
	for _, d := range drivers {
		byNumber[d.DriverNumber] = d
	}
	out := make([]SessionResult, 0, len(results))
	for _, r := range results {
		d := byNumber[r.DriverNumber]
		sr := SessionResult{
			Driver: DriverRef{Code: d.NameAcronym, Number: r.DriverNumber, Name: d.FirstName + " " + d.LastName},
			Team:   d.TeamName,
			Laps:   r.NumberOfLaps,
		}
		if r.Position != nil {
			sr.Position = *r.Position
			sr.Classified = strconv.Itoa(*r.Position)
		}
		switch {
		case r.DNS:
			sr.Status = "Did not start"
		case r.DNF:
			sr.Status = "Did not finish"
		case r.DSQ:
			sr.Status = "Disqualified"
		}
		if qs := rawList(r.Duration); len(qs) == 3 {
			sr.Q1, sr.Q2, sr.Q3 = qs[0], qs[1], qs[2]
		} else {
			sr.BestLap = rawSeconds(r.Duration)
			sr.GapSeconds = rawSeconds(r.GapToLeader)
		}
		out = append(out, sr)
	}
	return out, nil
}

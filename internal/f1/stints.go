package f1

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/mariuspot/f1mcp/internal/jolpica"
	"github.com/mariuspot/f1mcp/internal/openf1"
)

// Stint is a run on one set of tyres.
type Stint struct {
	Driver         DriverRef `json:"driver"`
	Stint          int       `json:"stint"`
	Compound       string    `json:"compound"` // e.g. "SOFT", "MEDIUM", "HARD", "INTERMEDIATE", "WET"
	LapStart       int       `json:"lap_start"`
	LapEnd         int       `json:"lap_end"`
	Laps           int       `json:"laps"`
	TyreAgeAtStart int       `json:"tyre_age_at_start"` // laps already on the tyres
}

// PitStop is a stop in the pit lane during a race.
type PitStop struct {
	Driver            DriverRef `json:"driver"`
	Stop              int       `json:"stop"`
	Lap               int       `json:"lap"`
	PitLaneSeconds    *float64  `json:"pit_lane_seconds,omitempty"`   // entry to exit
	StationarySeconds *float64  `json:"stationary_seconds,omitempty"` // when OpenF1 has it
	CompoundBefore    string    `json:"compound_before,omitempty"`
	CompoundAfter     string    `json:"compound_after,omitempty"`
}

// Stints returns each driver's tyre stints in a session, optionally for one
// driver.
func (s *Service) Stints(ctx context.Context, e Event, session, driver string) ([]Stint, error) {
	sd, err := s.session(ctx, e, session)
	if err != nil {
		return nil, err
	}
	number := 0
	if driver != "" {
		if number, err = sd.driver(driver); err != nil {
			return nil, err
		}
	}
	raw, err := s.stints(ctx, sd)
	if err != nil {
		return nil, err
	}
	var out []Stint
	for _, st := range raw {
		if number != 0 && st.DriverNumber != number {
			continue
		}
		out = append(out, Stint{
			Driver: sd.ref(st.DriverNumber), Stint: st.StintNumber, Compound: st.Compound,
			LapStart: st.LapStart, LapEnd: st.LapEnd, Laps: st.LapEnd - st.LapStart + 1, TyreAgeAtStart: st.TyreAgeAtStart,
		})
	}
	slices.SortFunc(out, func(a, b Stint) int {
		if a.Driver.Number != b.Driver.Number {
			return a.Driver.Number - b.Driver.Number
		}
		return a.Stint - b.Stint
	})
	return out, nil
}

func (s *Service) stints(ctx context.Context, sd *sessionData) ([]openf1.Stint, error) {
	return memo(s, "openf1/stints/"+sd.key, sd.year, func() ([]openf1.Stint, error) {
		return s.of1.Stints(ctx, openf1.SessionDriverFilter{SessionKey: sd.key})
	})
}

// PitStops returns a race's pit stops, optionally for one driver: pit lane
// time from Jolpica, stationary time and tyres from OpenF1.
func (s *Service) PitStops(ctx context.Context, e Event, driver string) ([]PitStop, error) {
	if e.Year < FirstDetailedYear {
		return nil, ErrNoDetail
	}
	sd, err := s.session(ctx, e, Race)
	if err != nil {
		return nil, err
	}
	number := 0
	if driver != "" {
		if number, err = sd.driver(driver); err != nil {
			return nil, err
		}
	}
	year, round := strconv.Itoa(e.Year), strconv.Itoa(e.Round)
	race, err := memo(s, fmt.Sprintf("jolpica/pitstops/%s/%s", year, round), e.Year, func() (*jolpica.Race, error) {
		return s.jol.PitStops(ctx, year, round)
	})
	if errors.Is(err, jolpica.ErrNotFound) {
		return nil, fmt.Errorf("no pit stops recorded for %d %s yet", e.Year, e.Name)
	}
	if err != nil {
		return nil, err
	}
	// Jolpica names drivers by ID; the race results give their numbers.
	results, err := memo(s, fmt.Sprintf("jolpica/results/%s/%s", year, round), e.Year, func() (*jolpica.Race, error) {
		return s.jol.RaceResults(ctx, year, round)
	})
	if err != nil {
		return nil, err
	}
	numbers := map[string]int{}
	for _, r := range results.Results {
		numbers[r.Driver.DriverID] = atoi(r.Number)
	}
	stints, _ := s.stints(ctx, sd)
	pits, _ := memo(s, "openf1/pit/"+sd.key, sd.year, func() ([]openf1.Pit, error) {
		return s.of1.Pits(ctx, openf1.SessionDriverFilter{SessionKey: sd.key})
	})

	var out []PitStop
	for _, p := range race.PitStops {
		n := numbers[p.DriverID]
		if number != 0 && n != number {
			continue
		}
		ps := PitStop{Driver: sd.ref(n), Stop: atoi(p.Stop), Lap: atoi(p.Lap), PitLaneSeconds: seconds(p.Duration)}
		for _, op := range pits {
			if op.DriverNumber == n && op.LapNumber == ps.Lap {
				ps.StationarySeconds = op.StopDuration
			}
		}
		for _, st := range stints {
			if st.DriverNumber != n {
				continue
			}
			if st.LapStart <= ps.Lap && ps.Lap <= st.LapEnd {
				ps.CompoundBefore = st.Compound
			}
			if st.LapStart == ps.Lap+1 {
				ps.CompoundAfter = st.Compound
			}
		}
		out = append(out, ps)
	}
	return out, nil
}

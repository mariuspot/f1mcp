package f1

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tracks"
)

// PreviewData is what's known before a race or sprint starts, for a
// preview: the circuit, this year's grid, the championship, and last
// year's race at the same circuit.
type PreviewData struct {
	Event   Event  `json:"event"`
	Session string `json:"session"` // race or sprint

	Circuit *CircuitNotes `json:"circuit,omitempty"`

	// Qualifying is the session that set the grid (qualifying, or sprint
	// qualifying for a sprint).
	Qualifying []SessionResult `json:"qualifying,omitempty"`
	// Grid is the starting grid, including penalties, when OpenF1 has it.
	Grid []GridPlace `json:"grid,omitempty"`

	Standings  []Standing `json:"standings,omitempty"` // drivers, going into the round
	Teams      []Standing `json:"teams,omitempty"`
	AfterRound int        `json:"after_round,omitempty"`

	LastYear *PastRace `json:"last_year,omitempty"`
}

// GridPlace is a car's starting place, and where it qualified if that
// differs.
type GridPlace struct {
	Position  int       `json:"position"`
	Driver    DriverRef `json:"driver"`
	Team      string    `json:"team,omitempty"`
	Qualified int       `json:"qualified,omitempty"` // when further up than Position
}

// CircuitNotes is the circuit as our track data has it.
type CircuitNotes struct {
	Name      string   `json:"name"`
	LengthKM  float64  `json:"length_km"`
	Turns     int      `json:"turns"`
	Corners   []string `json:"named_corners,omitempty"` // e.g. "Turn 1, Senna S"
	Straights []string `json:"straights,omitempty"`
	ClimbM    float64  `json:"elevation_change_m,omitempty"`
}

// PastRace is a previous race at the circuit.
type PastRace struct {
	Event      Event           `json:"event"`
	Laps       int             `json:"laps"` // the winner's
	Result     []SessionResult `json:"result"`
	Strategies []string        `json:"strategies,omitempty"` // top finishers', e.g. "VER (P1, from P1): started on SOFT, stop lap 27 to MEDIUM"
	Stops      map[int]int     `json:"stops,omitempty"`      // finishers by number of stops
	PitLaneS   float64         `json:"pit_lane_seconds,omitempty"`
	SafetyCars int             `json:"safety_cars"`
	VSCs       int             `json:"vscs"`
	RedFlags   int             `json:"red_flags"`
}

// EventAt returns the event with a session starting at t.
func (s *Service) EventAt(ctx context.Context, year int, t time.Time) (Event, error) {
	events, err := s.Schedule(ctx, year)
	if err != nil {
		return Event{}, err
	}
	for _, e := range events {
		for _, ss := range e.Sessions {
			if !ss.Start.IsZero() && ss.Start.Sub(t).Abs() < 3*time.Hour {
				return e, nil
			}
		}
	}
	return Event{}, fmt.Errorf("no %d event with a session at %s", year, t.Format(time.RFC3339))
}

// Preview gathers what's known before a race or sprint. Parts that can't
// be found are left out.
func (s *Service) Preview(ctx context.Context, e Event, session string) (PreviewData, error) {
	p := PreviewData{Event: e, Session: session}
	quali := Qualifying
	if session == Sprint {
		quali = SprintQualifying
	}

	if t, err := tracks.Load(e.Circuit.ID, e.Year); err == nil {
		n := &CircuitNotes{Name: t.Name, LengthKM: t.LengthKM(), Turns: len(t.Corners), ClimbM: t.ElevationChange()}
		for _, c := range t.Corners {
			if c.Name != "" {
				n.Corners = append(n.Corners, fmt.Sprintf("Turn %d, %s", c.Number, c.Name))
			}
		}
		for _, st := range t.Straights {
			n.Straights = append(n.Straights, st.Name)
		}
		p.Circuit = n
	}

	if q, err := s.SessionResults(ctx, e, quali); err == nil {
		p.Qualifying = q
	}
	p.Grid = s.grid(ctx, e, quali, p.Qualifying)

	if e.Round > 1 {
		if st, round, err := s.Standings(ctx, e.Year, e.Round-1, "drivers"); err == nil {
			p.Standings, p.AfterRound = st[:min(10, len(st))], round
		}
		if st, _, err := s.Standings(ctx, e.Year, e.Round-1, "teams"); err == nil {
			p.Teams = st[:min(5, len(st))]
		}
	}

	if last, err := s.pastRace(ctx, e.Year-1, e.Circuit.ID, session); err == nil {
		p.LastYear = last
	}
	return p, nil
}

// grid is the starting grid from OpenF1, marking who starts away from where
// they qualified.
func (s *Service) grid(ctx context.Context, e Event, quali string, results []SessionResult) []GridPlace {
	if e.Year < FirstDetailedYear {
		return nil
	}
	os, err := s.openf1Session(ctx, e, quali)
	if err != nil {
		return nil
	}
	key := strconv.Itoa(os.SessionKey)
	slots, err := memo(s, "openf1/starting_grid/"+key, e.Year, func() ([]openf1.GridSlot, error) {
		return s.of1.StartingGrid(ctx, openf1.SessionFilter{SessionKey: key})
	})
	if err != nil || len(slots) == 0 {
		return nil
	}
	qualified := map[int]int{}
	teams := map[int]string{}
	for _, r := range results {
		qualified[r.Driver.Number] = r.Position
		teams[r.Driver.Number] = r.Team
	}
	sd, err := s.session(ctx, e, quali)
	if err != nil {
		return nil
	}
	var out []GridPlace
	for _, g := range slots {
		gp := GridPlace{Position: g.Position, Driver: sd.ref(g.DriverNumber), Team: teams[g.DriverNumber]}
		// Only drivers starting further back than they qualified; those
		// behind them move up a place or two as a result.
		if q := qualified[g.DriverNumber]; q != 0 && g.Position > q {
			gp.Qualified = q
		}
		out = append(out, gp)
	}
	slices.SortFunc(out, func(a, b GridPlace) int { return a.Position - b.Position })
	return out
}

// pastRace summarises the race (or sprint) at a circuit in a year.
func (s *Service) pastRace(ctx context.Context, year int, circuit, session string) (*PastRace, error) {
	events, err := s.Schedule(ctx, year)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(events, func(e Event) bool { return e.Circuit.ID == circuit })
	if i < 0 {
		return nil, fmt.Errorf("no %d race at %s", year, circuit)
	}
	e := events[i]
	results, err := s.SessionResults(ctx, e, session)
	if err != nil || len(results) == 0 {
		return nil, fmt.Errorf("no %d %s result at %s: %v", year, session, circuit, err)
	}
	p := &PastRace{Event: e, Laps: results[0].Laps, Result: results[:min(10, len(results))]}
	if year < FirstDetailedYear {
		return p, nil
	}

	// Laps with the race stopped under a red flag: tyres changed then
	// aren't pit stops.
	redLaps := map[int]bool{}
	if msgs, err := s.RaceControl(ctx, e, session, "", 0, 0, true); err == nil {
		for _, m := range msgs {
			switch {
			case strings.Contains(m.Message, "VIRTUAL SAFETY CAR DEPLOYED"):
				p.VSCs++
			case strings.Contains(m.Message, "SAFETY CAR DEPLOYED"):
				p.SafetyCars++
			case m.Flag == "RED":
				p.RedFlags++
				// Race control and the tyre data can number the laps
				// either side of the stoppage.
				if m.Lap != nil {
					redLaps[*m.Lap-1], redLaps[*m.Lap], redLaps[*m.Lap+1] = true, true, true
				}
			}
		}
	}
	// Strategies: the tyre stints, with only real pit stops counted as
	// stops; tyres changed while the race was stopped aren't.
	var pits []PitStop
	if session == Race {
		pits, _ = s.PitStops(ctx, e, "")
	}
	realStop := func(n, lap int) bool {
		if redLaps[lap] {
			return false
		}
		for _, ps := range pits {
			if ps.Driver.Number == n && (ps.Lap == lap || ps.Lap == lap+1) && (ps.PitLaneSeconds == nil || *ps.PitLaneSeconds < 60) {
				return true
			}
		}
		return false
	}
	if stints, err := s.Stints(ctx, e, session, ""); err == nil {
		byCar := map[int][]Stint{}
		for _, st := range stints {
			byCar[st.Driver.Number] = append(byCar[st.Driver.Number], st)
		}
		p.Stops = map[int]int{}
		for _, r := range results {
			if r.Position == 0 {
				continue
			}
			sts := byCar[r.Driver.Number]
			if len(sts) == 0 {
				continue
			}
			var parts []string
			stops := 0
			for i, st := range sts {
				if i == 0 {
					parts = append(parts, "started on "+st.Compound)
					continue
				}
				lap := st.LapStart - 1
				if realStop(r.Driver.Number, lap) || len(pits) == 0 {
					stops++
					parts = append(parts, fmt.Sprintf("stop lap %d to %s", lap, st.Compound))
				} else {
					parts = append(parts, fmt.Sprintf("%s fitted during a stoppage on lap %d", st.Compound, lap))
				}
			}
			p.Stops[stops]++
			if r.Position <= 6 {
				p.Strategies = append(p.Strategies, fmt.Sprintf("%s (P%d, from P%d): %s", r.Driver.Code, r.Position, r.Grid, strings.Join(parts, ", ")))
			}
		}
	}
	if len(pits) > 0 {
		var lanes []float64
		for _, ps := range pits {
			if ps.PitLaneSeconds != nil && *ps.PitLaneSeconds < 60 {
				lanes = append(lanes, *ps.PitLaneSeconds)
			}
		}
		if len(lanes) > 0 {
			slices.Sort(lanes)
			p.PitLaneS = lanes[len(lanes)/2]
		}
	}
	return p, nil
}

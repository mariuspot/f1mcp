package f1

import (
	"context"
	"math"
	"slices"
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
)

// LapTime is one lap by one driver.
type LapTime struct {
	Driver    DriverRef  `json:"driver"`
	Lap       int        `json:"lap"`
	Seconds   *float64   `json:"seconds,omitempty"`
	Sectors   []*float64 `json:"sectors,omitempty"` // sector 1 to 3, in seconds
	PitOutLap bool       `json:"pit_out_lap,omitempty"`
	Started   time.Time  `json:"started,omitempty"`
}

// BestLap is a driver's fastest lap of a session.
type BestLap struct {
	Position int `json:"position"`
	LapTime
	GapSeconds float64 `json:"gap_seconds"` // to the fastest
	Laps       int     `json:"laps"`        // laps the driver completed
}

func toLapTime(sd *sessionData, l openf1.Lap) LapTime {
	lt := LapTime{Driver: sd.ref(l.DriverNumber), Lap: l.LapNumber, Seconds: l.LapDuration, PitOutLap: l.IsPitOutLap, Started: l.DateStart}
	if l.DurationSector1 != nil || l.DurationSector2 != nil || l.DurationSector3 != nil {
		lt.Sectors = []*float64{l.DurationSector1, l.DurationSector2, l.DurationSector3}
	}
	return lt
}

// BestLaps returns each driver's fastest lap in a session, fastest first.
func (s *Service) BestLaps(ctx context.Context, e Event, session string) ([]BestLap, error) {
	sd, err := s.session(ctx, e, session)
	if err != nil {
		return nil, err
	}
	laps, err := s.laps(ctx, sd)
	if err != nil {
		return nil, err
	}
	best := map[int]openf1.Lap{}
	count := map[int]int{}
	for _, l := range laps {
		if l.LapDuration == nil {
			continue
		}
		count[l.DriverNumber]++
		if b, ok := best[l.DriverNumber]; !ok || *l.LapDuration < *b.LapDuration {
			best[l.DriverNumber] = l
		}
	}
	out := make([]BestLap, 0, len(best))
	for n, l := range best {
		out = append(out, BestLap{LapTime: toLapTime(sd, l), Laps: count[n]})
	}
	slices.SortFunc(out, func(a, b BestLap) int {
		return compareFloat(*a.Seconds, *b.Seconds)
	})
	for i := range out {
		out[i].Position = i + 1
		out[i].GapSeconds = round3(*out[i].Seconds - *out[0].Seconds)
	}
	return out, nil
}

// Laps returns laps from a session: every lap of one driver, one lap for
// every driver, or both filters together. driver may be "" and lap 0.
func (s *Service) Laps(ctx context.Context, e Event, session, driver string, lap int) ([]LapTime, error) {
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
	laps, err := s.laps(ctx, sd)
	if err != nil {
		return nil, err
	}
	var out []LapTime
	for _, l := range laps {
		if (number == 0 || l.DriverNumber == number) && (lap == 0 || l.LapNumber == lap) {
			out = append(out, toLapTime(sd, l))
		}
	}
	slices.SortFunc(out, func(a, b LapTime) int {
		if a.Lap != b.Lap {
			return a.Lap - b.Lap
		}
		return compareFloat(ptrOr(a.Seconds, math.Inf(1)), ptrOr(b.Seconds, math.Inf(1)))
	})
	return out, nil
}

func compareFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func ptrOr(p *float64, v float64) float64 {
	if p == nil {
		return v
	}
	return *p
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

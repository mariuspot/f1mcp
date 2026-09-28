package f1

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// OrderEntry is a driver's place at the end of a lap.
type OrderEntry struct {
	Position        int       `json:"position"`
	Driver          DriverRef `json:"driver"`
	Team            string    `json:"team,omitempty"`
	LapsCompleted   int       `json:"laps_completed"`
	GapSeconds      *float64  `json:"gap_seconds,omitempty"`      // to the leader, same lap
	IntervalSeconds *float64  `json:"interval_seconds,omitempty"` // to the car ahead, same lap
	LapsDown        int       `json:"laps_down,omitempty"`
}

// RaceOrder is the running order at the end of a lap.
type RaceOrder struct {
	Lap      int              `json:"lap"`
	Of       int              `json:"of"` // laps the leader completed
	Order    []OrderEntry     `json:"order"`
	LapChart map[string][]int `json:"lap_chart,omitempty"` // driver code: position at the end of each lap (0: not running)
}

// lapEnds returns, for each driver, when they finished each lap, from when
// their next lap started or from their lap's start plus its time.
func (s *Service) lapEnds(ctx context.Context, sd *sessionData) (map[int]map[int]time.Time, int, error) {
	laps, err := s.laps(ctx, sd)
	if err != nil {
		return nil, 0, err
	}
	starts := map[int]map[int]time.Time{}
	ends := map[int]map[int]time.Time{}
	maxLap := 0
	for _, l := range laps {
		if starts[l.DriverNumber] == nil {
			starts[l.DriverNumber], ends[l.DriverNumber] = map[int]time.Time{}, map[int]time.Time{}
		}
		if !l.DateStart.IsZero() {
			starts[l.DriverNumber][l.LapNumber] = l.DateStart
			if l.LapDuration != nil {
				ends[l.DriverNumber][l.LapNumber] = l.DateStart.Add(time.Duration(*l.LapDuration * float64(time.Second)))
			}
		}
	}
	for n, byLap := range starts {
		for lap, t := range byLap {
			if lap > 1 {
				if _, ok := ends[n][lap-1]; !ok {
					ends[n][lap-1] = t
				}
			}
		}
	}
	for _, byLap := range ends {
		for lap := range byLap {
			maxLap = max(maxLap, lap)
		}
	}
	return ends, maxLap, nil
}

// RaceOrder returns the running order at the end of a lap (the last lap if
// lap is 0), with gaps worked out from when each car crossed the line. With
// chart, it also returns every driver's position at the end of every lap.
func (s *Service) RaceOrder(ctx context.Context, e Event, session string, lap int, chart bool) (RaceOrder, error) {
	sd, err := s.session(ctx, e, session)
	if err != nil {
		return RaceOrder{}, err
	}
	ends, maxLap, err := s.lapEnds(ctx, sd)
	if err != nil {
		return RaceOrder{}, err
	}
	if maxLap == 0 {
		return RaceOrder{}, fmt.Errorf("no laps recorded for %d %s %s", e.Year, e.Name, session)
	}
	if lap <= 0 || lap > maxLap {
		lap = maxLap
	}
	ro := RaceOrder{Lap: lap, Of: maxLap, Order: orderAt(sd, ends, lap)}
	if chart {
		ro.LapChart = map[string][]int{}
		for l := 1; l <= maxLap; l++ {
			for _, o := range orderAt(sd, ends, l) {
				key := o.Driver.Code
				if key == "" {
					key = fmt.Sprint(o.Driver.Number)
				}
				if ro.LapChart[key] == nil {
					ro.LapChart[key] = make([]int, maxLap)
				}
				if o.LapsCompleted >= l {
					ro.LapChart[key][l-1] = o.Position
				}
			}
		}
	}
	return ro, nil
}

func orderAt(sd *sessionData, ends map[int]map[int]time.Time, lap int) []OrderEntry {
	type car struct {
		number    int
		completed int
		at        time.Time // when they finished their last lap up to lap
	}
	var cars []car
	for n, byLap := range ends {
		c := car{number: n}
		for l := lap; l >= 1; l-- {
			if t, ok := byLap[l]; ok {
				c.completed, c.at = l, t
				break
			}
		}
		cars = append(cars, c)
	}
	slices.SortFunc(cars, func(a, b car) int {
		if a.completed != b.completed {
			return b.completed - a.completed
		}
		return a.at.Compare(b.at)
	})
	out := make([]OrderEntry, 0, len(cars))
	for i, c := range cars {
		o := OrderEntry{Position: i + 1, Driver: sd.ref(c.number), Team: sd.teams[c.number], LapsCompleted: c.completed}
		lead := cars[0]
		if c.completed == lead.completed && i > 0 {
			g := round3(c.at.Sub(lead.at).Seconds())
			iv := round3(c.at.Sub(cars[i-1].at).Seconds())
			o.GapSeconds, o.IntervalSeconds = &g, &iv
		} else if c.completed < lead.completed {
			o.LapsDown = lead.completed - c.completed
		}
		out = append(out, o)
	}
	return out
}

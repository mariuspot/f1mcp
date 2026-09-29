package f1

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tracks"
)

// TelemetrySample is one car data reading, about 4 a second.
type TelemetrySample struct {
	Seconds  float64 `json:"t"` // from the start of the lap
	SpeedKPH int     `json:"speed_kph"`
	Throttle int     `json:"throttle_pct"`
	Brake    bool    `json:"brake"`
	Gear     int     `json:"gear"`
	RPM      int     `json:"rpm"`
}

// BrakingZone is a stretch of a lap on the brakes.
type BrakingZone struct {
	StartSeconds float64 `json:"start_t"`
	Seconds      float64 `json:"seconds"`
	FromKPH      int     `json:"from_kph"`
	ToKPH        int     `json:"to_kph"`           // slowest while braking
	Corner       string  `json:"corner,omitempty"` // the corner braked for, e.g. "Turn 1, La Source"
}

// LapTelemetry summarises a driver's lap.
type LapTelemetry struct {
	Driver          DriverRef         `json:"driver"`
	Lap             int               `json:"lap"`
	LapSeconds      *float64          `json:"lap_seconds,omitempty"`
	TopSpeedKPH     int               `json:"top_speed_kph"`
	TopSpeedWhere   string            `json:"top_speed_where,omitempty"`
	MinSpeedKPH     int               `json:"min_speed_kph"`
	FullThrottlePct float64           `json:"full_throttle_pct"`
	BrakingPct      float64           `json:"braking_pct"`
	GearChanges     int               `json:"gear_changes"`
	Braking         []BrakingZone     `json:"braking"`
	Samples         []TelemetrySample `json:"-"`
}

// CarTelemetry returns a driver's lap with its car data. Braking zones and
// the top speed are placed on the track, e.g. "Turn 1, La Source".
func (s *Service) CarTelemetry(ctx context.Context, e Event, session, driver string, lap int) (LapTelemetry, error) {
	sd, err := s.lapSession(ctx, e, session)
	if err != nil {
		return LapTelemetry{}, err
	}
	number, err := sd.driver(driver)
	if err != nil {
		return LapTelemetry{}, err
	}
	laps, err := s.laps(ctx, sd)
	if err != nil {
		return LapTelemetry{}, err
	}
	var l *openf1.Lap
	for i := range laps {
		if laps[i].DriverNumber == number && laps[i].LapNumber == lap {
			l = &laps[i]
		}
	}
	if l == nil || l.DateStart.IsZero() || l.LapDuration == nil {
		return LapTelemetry{}, fmt.Errorf("%s has no timed lap %d in %d %s %s", sd.ref(number).Code, lap, e.Year, e.Name, session)
	}
	w := openf1.WindowFilter{
		SessionKey: sd.key, DriverNumber: number,
		After: l.DateStart, Before: l.DateStart.Add(time.Duration(*l.LapDuration * float64(time.Second))),
	}
	key := fmt.Sprintf("%s/%d/%d", sd.key, number, lap)
	cars, err := memo(s, "openf1/car_data/"+key, sd.year, func() ([]openf1.CarData, error) {
		return s.of1.CarData(ctx, w)
	})
	if err != nil {
		return LapTelemetry{}, err
	}
	if len(cars) == 0 {
		return LapTelemetry{}, fmt.Errorf("no car data for %s lap %d", sd.ref(number).Code, lap)
	}
	// Positions are only for naming where things happen; carry on without.
	locs, _ := memo(s, "openf1/location/"+key, sd.year, func() ([]openf1.Location, error) {
		return s.of1.Locations(ctx, w)
	})
	track, _ := tracks.Load(e.Circuit.ID, e.Year)
	locate := func(at time.Time) *tracks.Location {
		if track == nil || len(locs) == 0 {
			return nil
		}
		best := locs[0]
		for _, p := range locs {
			if p.Date.Sub(at).Abs() < best.Date.Sub(at).Abs() {
				best = p
			}
		}
		loc := track.Locate(tracks.Point{float64(best.X), float64(best.Y)})
		return &loc
	}
	where := func(at time.Time) string {
		if loc := locate(at); loc != nil {
			return loc.String()
		}
		return ""
	}
	braked := func(at time.Time) string {
		if loc := locate(at); loc != nil {
			return loc.Approaching()
		}
		return ""
	}

	lt := LapTelemetry{Driver: sd.ref(number), Lap: lap, LapSeconds: l.LapDuration, MinSpeedKPH: math.MaxInt}
	full, braking := 0, 0
	var zone *BrakingZone
	var topAt time.Time
	for i, c := range cars {
		t := round3(c.Date.Sub(l.DateStart).Seconds())
		lt.Samples = append(lt.Samples, TelemetrySample{Seconds: t, SpeedKPH: c.Speed, Throttle: c.Throttle, Brake: c.Brake > 0, Gear: c.NGear, RPM: c.RPM})
		if c.Speed > lt.TopSpeedKPH {
			lt.TopSpeedKPH, topAt = c.Speed, c.Date
		}
		lt.MinSpeedKPH = min(lt.MinSpeedKPH, c.Speed)
		if c.Throttle >= 99 {
			full++
		}
		if i > 0 && c.NGear != cars[i-1].NGear {
			lt.GearChanges++
		}
		switch {
		case c.Brake > 0 && zone == nil:
			braking++
			zone = &BrakingZone{StartSeconds: t, FromKPH: c.Speed, ToKPH: c.Speed, Corner: braked(c.Date)}
		case c.Brake > 0:
			braking++
			zone.ToKPH = min(zone.ToKPH, c.Speed)
		case zone != nil:
			zone.Seconds = round3(t - zone.StartSeconds)
			lt.Braking = append(lt.Braking, *zone)
			zone = nil
		}
	}
	if zone != nil {
		zone.Seconds = round3(lt.Samples[len(lt.Samples)-1].Seconds - zone.StartSeconds)
		lt.Braking = append(lt.Braking, *zone)
	}
	lt.FullThrottlePct = math.Round(float64(full)/float64(len(cars))*1000) / 10
	lt.BrakingPct = math.Round(float64(braking)/float64(len(cars))*1000) / 10
	lt.TopSpeedWhere = where(topAt)
	return lt, nil
}

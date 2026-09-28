package f1

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
)

// Range summarises a measurement over a session.
type Range struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
	Avg float64 `json:"avg"`
}

// WeatherSummary summarises a session's weather.
type WeatherSummary struct {
	Samples     int       `json:"samples"`
	From        time.Time `json:"from"`
	To          time.Time `json:"to"`
	AirTempC    Range     `json:"air_temp_c"`
	TrackTempC  Range     `json:"track_temp_c"`
	HumidityPct Range     `json:"humidity_pct"`
	WindSpeedMS Range     `json:"wind_speed_m_s"`
	Rain        bool      `json:"rain"`
	RainFrom    time.Time `json:"rain_from,omitempty"`
	RainTo      time.Time `json:"rain_to,omitempty"`
}

// WeatherSample is one weather reading, about once a minute.
type WeatherSample struct {
	Time        time.Time `json:"time"`
	AirTempC    float64   `json:"air_temp_c"`
	TrackTempC  float64   `json:"track_temp_c"`
	HumidityPct float64   `json:"humidity_pct"`
	PressureMB  float64   `json:"pressure_mbar"`
	WindSpeedMS float64   `json:"wind_speed_m_s"`
	WindDirDeg  int       `json:"wind_direction_deg"`
	Rain        bool      `json:"rain"`
}

// Weather returns a session's weather readings.
func (s *Service) Weather(ctx context.Context, e Event, session string) ([]WeatherSample, error) {
	sd, err := s.session(ctx, e, session)
	if err != nil {
		return nil, err
	}
	raw, err := memo(s, "openf1/weather/"+sd.key, sd.year, func() ([]openf1.Weather, error) {
		return s.of1.Weather(ctx, openf1.SessionFilter{SessionKey: sd.key})
	})
	if err != nil {
		return nil, err
	}
	out := make([]WeatherSample, 0, len(raw))
	for _, w := range raw {
		out = append(out, WeatherSample{
			Time: w.Date.UTC(), AirTempC: w.AirTemperature, TrackTempC: w.TrackTemperature, HumidityPct: w.Humidity,
			PressureMB: w.Pressure, WindSpeedMS: w.WindSpeed, WindDirDeg: w.WindDirection, Rain: w.Rainfall > 0,
		})
	}
	return out, nil
}

// SummariseWeather summarises weather readings.
func SummariseWeather(ws []WeatherSample) (WeatherSummary, error) {
	if len(ws) == 0 {
		return WeatherSummary{}, fmt.Errorf("no weather readings")
	}
	sum := WeatherSummary{Samples: len(ws), From: ws[0].Time, To: ws[len(ws)-1].Time}
	rng := func(get func(WeatherSample) float64) Range {
		r := Range{Min: math.Inf(1), Max: math.Inf(-1)}
		for _, w := range ws {
			v := get(w)
			r.Min, r.Max, r.Avg = min(r.Min, v), max(r.Max, v), r.Avg+v/float64(len(ws))
		}
		r.Avg = math.Round(r.Avg*10) / 10
		return r
	}
	sum.AirTempC = rng(func(w WeatherSample) float64 { return w.AirTempC })
	sum.TrackTempC = rng(func(w WeatherSample) float64 { return w.TrackTempC })
	sum.HumidityPct = rng(func(w WeatherSample) float64 { return w.HumidityPct })
	sum.WindSpeedMS = rng(func(w WeatherSample) float64 { return w.WindSpeedMS })
	for _, w := range ws {
		if w.Rain {
			if !sum.Rain {
				sum.Rain, sum.RainFrom = true, w.Time
			}
			sum.RainTo = w.Time
		}
	}
	return sum, nil
}

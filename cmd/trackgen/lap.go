package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tracks"
)

// LapFile is one or more stored laps from a session, animated together by
// render.
type LapFile struct {
	Name       string            `json:"name"`
	CircuitID  string            `json:"circuit_id"`
	Year       int               `json:"year"`
	SessionKey int               `json:"session_key"`
	Session    string            `json:"session"` // e.g. "Azerbaijan Grand Prix · Qualifying"
	Laps       []tracks.LapTrace `json:"laps"`
	// Weather is the reading nearest the start of the first lap.
	Weather *LapWeather `json:"weather,omitempty"`
}

// LapWeather is the weather when a lap was driven.
type LapWeather struct {
	AirC        float64 `json:"air_c"`
	TrackC      float64 `json:"track_c"`
	HumidityPct float64 `json:"humidity_pct"`
	WindMS      float64 `json:"wind_m_s"`
	Rain        bool    `json:"rain"`
}

// String describes the weather in one line, e.g. "Air 30 °C · Track 36 °C
// · Humidity 70% · Wind 1.2 m/s · Dry".
func (w LapWeather) String() string {
	cond := "Dry"
	if w.Rain {
		cond = "Rain"
	}
	return fmt.Sprintf("Air %.0f °C · Track %.0f °C · Humidity %.0f%% · Wind %.1f m/s · %s", w.AirC, w.TrackC, w.HumidityPct, w.WindMS, cond)
}

// lapWeather fetches the weather reading nearest a moment in a session.
func lapWeather(ctx context.Context, of1 *openf1.Client, key string, at time.Time) (*LapWeather, error) {
	ws, err := of1.Weather(ctx, openf1.SessionFilter{SessionKey: key})
	if err != nil || len(ws) == 0 {
		return nil, err
	}
	best := ws[0]
	for _, w := range ws {
		if w.Date.Sub(at).Abs() < best.Date.Sub(at).Abs() {
			best = w
		}
	}
	return &LapWeather{AirC: best.AirTemperature, TrackC: best.TrackTemperature, HumidityPct: best.Humidity, WindMS: best.WindSpeed, Rain: best.Rainfall > 0}, nil
}

// backfillLaps adds what older stored laps lack: each lap's start time and
// the weather when it was driven.
func backfillLaps(ctx context.Context, dir string) error {
	of1 := openf1.NewClient("", nil)
	files, err := loadLapFiles(dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		key := strconv.Itoa(f.SessionKey)
		for i, l := range f.Laps {
			if !l.Started.IsZero() {
				continue
			}
			laps, err := of1.Laps(ctx, openf1.LapsFilter{SessionKey: key, DriverNumber: l.Number, LapNumber: l.Lap})
			if err != nil {
				return err
			}
			if len(laps) > 0 {
				f.Laps[i].Started = laps[0].DateStart.UTC()
			}
		}
		if f.Weather == nil && len(f.Laps) > 0 && !f.Laps[0].Started.IsZero() {
			if f.Weather, err = lapWeather(ctx, of1, key, f.Laps[0].Started); err != nil {
				return err
			}
		}
		if err := writeJSON(filepath.Join(dir, f.Name+".json"), f); err != nil {
			return err
		}
		w := "no weather"
		if f.Weather != nil {
			w = f.Weather.String()
		}
		log.Printf("%s: %s", f.Name, w)
	}
	return nil
}

// findLaps stores each driver's best lap of a session: the drivers given (by
// car number or code, e.g. "63" or "RUS"), or else the fastest count
// drivers. It makes 4 requests plus 2 per lap.
func findLaps(ctx context.Context, sessionKey int, driverArgs []string, count int, dir string) error {
	of1 := openf1.NewClient("", nil)
	key := strconv.Itoa(sessionKey)
	sessions, err := of1.Sessions(ctx, openf1.SessionsFilter{SessionKey: key})
	if err != nil || len(sessions) == 0 {
		return fmt.Errorf("session %d: %v", sessionKey, err)
	}
	s := sessions[0]
	t, err := trackByKey(s.CircuitKey, s.Year)
	if err != nil {
		return err
	}

	laps, err := of1.Laps(ctx, openf1.LapsFilter{SessionKey: key})
	if err != nil {
		return err
	}
	best := map[int]openf1.Lap{}
	for _, l := range laps {
		if l.LapDuration == nil || l.DateStart.IsZero() || l.IsPitOutLap {
			continue
		}
		if b, ok := best[l.DriverNumber]; !ok || *l.LapDuration < *b.LapDuration {
			best[l.DriverNumber] = l
		}
	}
	info, err := of1.Drivers(ctx, openf1.SessionDriverFilter{SessionKey: key})
	if err != nil {
		return err
	}
	byNumber := map[int]openf1.Driver{}
	for _, d := range info {
		byNumber[d.DriverNumber] = d
	}
	var chosen []int
	for _, a := range driverArgs {
		n, err := strconv.Atoi(a)
		if err != nil {
			i := slices.IndexFunc(info, func(d openf1.Driver) bool { return strings.EqualFold(d.NameAcronym, a) })
			if i < 0 {
				return fmt.Errorf("no driver %q in session %d", a, sessionKey)
			}
			n = info[i].DriverNumber
		}
		chosen = append(chosen, n)
	}
	if len(chosen) == 0 {
		for n := range best {
			chosen = append(chosen, n)
		}
		slices.SortFunc(chosen, func(a, b int) int {
			return int((*best[a].LapDuration - *best[b].LapDuration) * 1000)
		})
		chosen = chosen[:min(count, len(chosen))]
	}

	f := LapFile{CircuitID: t.CircuitID, Year: s.Year, SessionKey: sessionKey, Session: s.CountryName + " · " + s.SessionName}
	var names []string
	for _, n := range chosen {
		l, ok := best[n]
		if !ok {
			return fmt.Errorf("driver %d has no timed lap in session %d", n, sessionKey)
		}
		trace, err := recordTrace(ctx, of1, key, l, byNumber[n])
		if err != nil {
			return err
		}
		f.Laps = append(f.Laps, trace)
		names = append(names, strings.ToLower(trace.Driver))
		log.Printf("%s lap %d: %s, %d positions, %d telemetry samples", trace.Driver, trace.Lap,
			formatSeconds(trace.Duration), len(trace.Positions), len(trace.Telemetry))
	}
	f.Name = fmt.Sprintf("%s-%d-%s-%s", t.CircuitID, s.Year, slug(s.SessionName), strings.Join(names, "-vs-"))
	if w, err := lapWeather(ctx, of1, key, f.Laps[0].Started); err == nil {
		f.Weather = w
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, f.Name+".json"), f)
}

// recordTrace fetches a lap's positions and telemetry.
func recordTrace(ctx context.Context, of1 *openf1.Client, key string, l openf1.Lap, d openf1.Driver) (tracks.LapTrace, error) {
	dur := *l.LapDuration
	w := openf1.WindowFilter{
		SessionKey: key, DriverNumber: l.DriverNumber,
		After: l.DateStart.Add(-time.Second), Before: l.DateStart.Add(seconds(dur) + time.Second),
	}
	locs, err := of1.Locations(ctx, w)
	if err != nil {
		return tracks.LapTrace{}, err
	}
	cars, err := of1.CarData(ctx, w)
	if err != nil {
		return tracks.LapTrace{}, err
	}
	tr := tracks.LapTrace{Driver: d.NameAcronym, Number: l.DriverNumber, Color: "#" + d.TeamColour, Lap: l.LapNumber, Started: l.DateStart.UTC(), Duration: dur}
	for _, p := range locs {
		tr.Positions = append(tr.Positions, [3]float64{round(p.Date.Sub(l.DateStart).Seconds(), 3), float64(p.X), float64(p.Y)})
	}
	for _, c := range cars {
		tr.Telemetry = append(tr.Telemetry, [5]float64{round(c.Date.Sub(l.DateStart).Seconds(), 3), float64(c.Speed), float64(c.Throttle), float64(c.Brake), float64(c.NGear)})
	}
	return tr, nil
}

func loadLapFiles(dir string) ([]LapFile, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var out []LapFile
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var f LapFile
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		out = append(out, f)
	}
	return out, nil
}

func formatSeconds(s float64) string {
	m := int(s) / 60
	return fmt.Sprintf("%d:%06.3f", m, s-float64(m*60))
}

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

// LapReplay is one or more laps from a session, recorded to be animated
// together over the track map.
type LapReplay struct {
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

// SessionKey returns the OpenF1 key of an event's session.
func (s *Service) SessionKey(ctx context.Context, e Event, session string) (int, error) {
	if err := ValidSession(session); err != nil {
		return 0, err
	}
	os, err := s.openf1Session(ctx, e, session)
	return os.SessionKey, err
}

// LapWeatherAt returns the weather reading nearest a moment in a session.
func (s *Service) LapWeatherAt(ctx context.Context, sessionKey int, at time.Time) (*LapWeather, error) {
	ws, err := s.of1.Weather(ctx, openf1.SessionFilter{SessionKey: strconv.Itoa(sessionKey)})
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

// ReplayLaps records laps from a session to animate: for each driver given
// (by car number, code or name) their lap number lap, or their best lap if
// lap is 0. With no drivers, the best laps of the fastest count drivers. It
// makes 6 requests plus 2 per lap.
func (s *Service) ReplayLaps(ctx context.Context, sessionKey int, drivers []string, lap, count int) (LapReplay, error) {
	key := strconv.Itoa(sessionKey)
	sessions, err := s.of1.Sessions(ctx, openf1.SessionsFilter{SessionKey: key})
	if err != nil || len(sessions) == 0 {
		return LapReplay{}, fmt.Errorf("session %d: %v", sessionKey, err)
	}
	sess := sessions[0]
	t, err := TrackByKey(sess.CircuitKey, sess.Year)
	if err != nil {
		return LapReplay{}, err
	}
	info, err := s.of1.Drivers(ctx, openf1.SessionDriverFilter{SessionKey: key})
	if err != nil {
		return LapReplay{}, err
	}
	sd := &sessionData{Event: Event{Year: sess.Year, Name: sess.CountryName}, Session: sess.SessionName, drivers: map[int]DriverRef{}}
	byNumber := map[int]openf1.Driver{}
	for _, d := range info {
		byNumber[d.DriverNumber] = d
		sd.drivers[d.DriverNumber] = DriverRef{Code: d.NameAcronym, Number: d.DriverNumber, Name: d.FirstName + " " + d.LastName}
	}
	laps, err := s.of1.Laps(ctx, openf1.LapsFilter{SessionKey: key})
	if err != nil {
		return LapReplay{}, err
	}
	// The lap to show for each driver: the numbered one, or their best.
	chosenLap := map[int]openf1.Lap{}
	for _, l := range laps {
		if l.LapDuration == nil || l.DateStart.IsZero() {
			continue
		}
		if lap > 0 {
			if l.LapNumber == lap {
				chosenLap[l.DriverNumber] = l
			}
			continue
		}
		if l.IsPitOutLap {
			continue
		}
		if b, ok := chosenLap[l.DriverNumber]; !ok || *l.LapDuration < *b.LapDuration {
			chosenLap[l.DriverNumber] = l
		}
	}
	var chosen []int
	for _, a := range drivers {
		n, err := sd.driver(a)
		if err != nil {
			return LapReplay{}, err
		}
		chosen = append(chosen, n)
	}
	if len(chosen) == 0 {
		for n := range chosenLap {
			chosen = append(chosen, n)
		}
		slices.SortFunc(chosen, func(a, b int) int {
			return compareFloat(*chosenLap[a].LapDuration, *chosenLap[b].LapDuration)
		})
		chosen = chosen[:min(max(count, 1), len(chosen))]
	}

	r := LapReplay{CircuitID: t.CircuitID, Year: sess.Year, SessionKey: sessionKey, Session: sess.CountryName + " · " + sess.SessionName}
	stints, err := s.of1.Stints(ctx, openf1.SessionDriverFilter{SessionKey: key})
	if err != nil {
		return LapReplay{}, err
	}
	var names []string
	for _, n := range chosen {
		l, ok := chosenLap[n]
		if !ok {
			if lap > 0 {
				return LapReplay{}, fmt.Errorf("%s has no timed lap %d in %d %s %s", sd.ref(n).Code, lap, sess.Year, sess.CountryName, sess.SessionName)
			}
			return LapReplay{}, fmt.Errorf("%s has no timed lap in %d %s %s", sd.ref(n).Code, sess.Year, sess.CountryName, sess.SessionName)
		}
		trace, err := s.recordTrace(ctx, key, l, byNumber[n])
		if err != nil {
			return LapReplay{}, err
		}
		SetTyre(&trace, stints)
		r.Laps = append(r.Laps, trace)
		names = append(names, strings.ToLower(trace.Driver))
	}
	if len(r.Laps) == 0 {
		return LapReplay{}, fmt.Errorf("no timed laps in %d %s %s", sess.Year, sess.CountryName, sess.SessionName)
	}
	r.Name = fmt.Sprintf("%s-%d-%s-%s", t.CircuitID, sess.Year, Slug(sess.SessionName), strings.Join(names, "-vs-"))
	if lap > 0 {
		r.Name += fmt.Sprintf("-lap-%d", lap)
	}
	if w, err := s.LapWeatherAt(ctx, sessionKey, r.Laps[0].Started); err == nil {
		r.Weather = w
	}
	return r, nil
}

// SetTyre sets the compound and age of a lap's tyres from the session's
// stints.
func SetTyre(l *tracks.LapTrace, stints []openf1.Stint) {
	for _, st := range stints {
		if st.DriverNumber == l.Number && st.LapStart <= l.Lap && l.Lap <= st.LapEnd {
			l.Compound, l.TyreAge = st.Compound, st.TyreAgeAtStart+l.Lap-st.LapStart
		}
	}
}

// recordTrace fetches a lap's positions and telemetry.
func (s *Service) recordTrace(ctx context.Context, key string, l openf1.Lap, d openf1.Driver) (tracks.LapTrace, error) {
	dur := *l.LapDuration
	w := openf1.WindowFilter{
		SessionKey: key, DriverNumber: l.DriverNumber,
		After: l.DateStart.Add(-time.Second), Before: l.DateStart.Add(time.Duration((dur + 1) * float64(time.Second))),
	}
	locs, err := s.of1.Locations(ctx, w)
	if err != nil {
		return tracks.LapTrace{}, err
	}
	cars, err := s.of1.CarData(ctx, w)
	if err != nil {
		return tracks.LapTrace{}, err
	}
	tr := tracks.LapTrace{Driver: d.NameAcronym, Number: l.DriverNumber, Color: "#" + d.TeamColour, Lap: l.LapNumber, Started: l.DateStart.UTC(), Duration: dur}
	for _, p := range locs {
		tr.Positions = append(tr.Positions, [3]float64{round3(p.Date.Sub(l.DateStart).Seconds()), float64(p.X), float64(p.Y)})
	}
	for _, c := range cars {
		tr.Telemetry = append(tr.Telemetry, [5]float64{round3(c.Date.Sub(l.DateStart).Seconds()), float64(c.Speed), float64(c.Throttle), float64(c.Brake), float64(c.NGear)})
	}
	if len(tr.Positions) < 2 {
		return tracks.LapTrace{}, fmt.Errorf("no positions recorded for %s lap %d", d.NameAcronym, l.LapNumber)
	}
	return tr, nil
}

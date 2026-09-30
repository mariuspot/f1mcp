// Package replay collects everything OpenF1 has about a session, and plays
// it back as if it were live.
package replay

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
)

// Endpoints fetched once for the whole session.
var sessionEndpoints = []string{
	"sessions", "drivers", "laps", "pit", "stints", "race_control", "weather",
	"team_radio", "intervals", "position", "overtakes", "session_result", "starting_grid",
}

// Endpoints with several records a second for each car, fetched per car in
// windows so no one response is too large.
var carEndpoints = []string{"car_data", "location"}

const carWindow = time.Hour

// Manifest describes a collected session.
type Manifest struct {
	SessionKey  int            `json:"session_key"`
	MeetingKey  int            `json:"meeting_key"`
	Year        int            `json:"year"`
	Country     string         `json:"country"`
	Session     string         `json:"session"`
	Start       time.Time      `json:"start"`
	End         time.Time      `json:"end"` // the end of the last lap
	Drivers     []int          `json:"drivers"`
	Records     map[string]int `json:"records"` // per endpoint
	CollectedAt time.Time      `json:"collected_at"`
}

// Collect saves every OpenF1 endpoint for a session into dir: one gzipped
// JSON array per endpoint, e.g. laps.json.gz, and per car for car_data and
// location, e.g. car_data/1.json.gz. Files already there are kept, so an
// interrupted collection can be run again.
func Collect(ctx context.Context, c *openf1.Client, sessionKey int, dir string, logf func(string, ...any)) (Manifest, error) {
	m := Manifest{SessionKey: sessionKey, Records: map[string]int{}}
	fetch := func(name string, f openf1.RawFilter, file string) ([]json.RawMessage, error) {
		path := filepath.Join(dir, file)
		if recs, err := readGz(path); err == nil {
			return recs, nil
		}
		recs, err := c.Raw(ctx, "/"+name, f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if err := writeGz(path, recs); err != nil {
			return nil, err
		}
		logf("%s: %d records", file, len(recs))
		return recs, nil
	}

	for _, name := range sessionEndpoints {
		recs, err := fetch(name, openf1.RawFilter{SessionKey: sessionKey}, name+".json.gz")
		if err != nil {
			return m, err
		}
		m.Records[name] = len(recs)
		switch name {
		case "sessions":
			if len(recs) == 0 {
				return m, fmt.Errorf("no session %d", sessionKey)
			}
			var s openf1.Session
			if err := json.Unmarshal(recs[0], &s); err != nil {
				return m, err
			}
			m.MeetingKey, m.Year, m.Country, m.Session, m.Start = s.MeetingKey, s.Year, s.CountryName, s.SessionName, s.DateStart
		case "drivers":
			for _, r := range recs {
				var d openf1.Driver
				if json.Unmarshal(r, &d) == nil {
					m.Drivers = append(m.Drivers, d.DriverNumber)
				}
			}
			slices.Sort(m.Drivers)
		case "laps":
			for _, r := range recs {
				var l openf1.Lap
				if json.Unmarshal(r, &l) == nil && !l.DateStart.IsZero() && l.LapDuration != nil {
					end := l.DateStart.Add(time.Duration(*l.LapDuration * float64(time.Second)))
					if end.After(m.End) {
						m.End = end
					}
				}
			}
		}
	}
	if recs, err := fetch("meetings", openf1.RawFilter{MeetingKey: m.MeetingKey}, "meetings.json.gz"); err == nil {
		m.Records["meetings"] = len(recs)
	} else {
		return m, err
	}

	// Car data and location from a little before the start to a little
	// after the last lap.
	from, to := m.Start.Add(-10*time.Minute), m.End.Add(10*time.Minute)
	for _, name := range carEndpoints {
		for _, d := range m.Drivers {
			var all []json.RawMessage
			file := filepath.Join(name, strconv.Itoa(d)+".json.gz")
			if recs, err := readGz(filepath.Join(dir, file)); err == nil {
				m.Records[name] += len(recs)
				continue
			}
			for w := from; w.Before(to); w = w.Add(carWindow) {
				end := w.Add(carWindow)
				if end.After(to) {
					end = to
				}
				recs, err := c.Raw(ctx, "/"+name, openf1.RawFilter{SessionKey: sessionKey, DriverNumber: d, After: w, Before: end})
				if err != nil {
					return m, fmt.Errorf("%s car %d: %w", name, d, err)
				}
				all = append(all, recs...)
			}
			if err := writeGz(filepath.Join(dir, file), all); err != nil {
				return m, err
			}
			logf("%s: %d records", file, len(all))
			m.Records[name] += len(all)
		}
	}

	m.CollectedAt = time.Now().UTC()
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return m, err
	}
	return m, os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644)
}

func writeGz(path string, recs []json.RawMessage) error {
	if recs == nil {
		recs = []json.RawMessage{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(f)
	if err := json.NewEncoder(zw).Encode(recs); err != nil {
		f.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readGz reads a collected endpoint.
func readGz(path string) ([]json.RawMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	var recs []json.RawMessage
	err = json.NewDecoder(zr).Decode(&recs)
	return recs, err
}

package f1

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
)

// RaceControlMessage is a message from race control: flags, safety cars,
// penalties, investigations and more.
type RaceControlMessage struct {
	Time     time.Time  `json:"time"`
	Lap      *int       `json:"lap,omitempty"`
	Category string     `json:"category"` // e.g. "Flag", "SafetyCar", "Drs", "Other"
	Flag     string     `json:"flag,omitempty"`
	Scope    string     `json:"scope,omitempty"`  // "Track", "Sector" or "Driver"
	Sector   *int       `json:"sector,omitempty"` // marshal sector
	Driver   *DriverRef `json:"driver,omitempty"`
	Message  string     `json:"message"`
}

// keyMessage reports whether a message is one of a session's key events:
// red and chequered flags, safety cars, penalties and investigations.
func keyMessage(m openf1.RaceControl) bool {
	if m.Category == "SafetyCar" {
		return true
	}
	if m.Flag != nil && (*m.Flag == "RED" || *m.Flag == "CHEQUERED") {
		return true
	}
	for _, w := range []string{"PENALTY", "INVESTIGATION", "DISQUALIFIED", "REPRIMAND", "BLACK AND WHITE"} {
		if strings.Contains(m.Message, w) {
			return true
		}
	}
	return false
}

// RaceControl returns a session's race control messages: only the key
// events unless all is set, optionally one category, and optionally
// between two laps (0 for no limit).
func (s *Service) RaceControl(ctx context.Context, e Event, session, category string, fromLap, toLap int, all bool) ([]RaceControlMessage, error) {
	sd, err := s.session(ctx, e, session)
	if err != nil {
		return nil, err
	}
	msgs, err := memo(s, "openf1/race_control/"+sd.key, sd.year, func() ([]openf1.RaceControl, error) {
		return s.of1.RaceControl(ctx, openf1.SessionFilter{SessionKey: sd.key})
	})
	if err != nil {
		return nil, err
	}
	if toLap <= 0 {
		toLap = math.MaxInt
	}
	var out []RaceControlMessage
	for _, m := range msgs {
		if !all && !keyMessage(m) {
			continue
		}
		if category != "" && !strings.EqualFold(m.Category, category) {
			continue
		}
		if m.LapNumber != nil && (*m.LapNumber < fromLap || *m.LapNumber > toLap) {
			continue
		}
		rc := RaceControlMessage{Time: m.Date.UTC(), Lap: m.LapNumber, Category: m.Category, Sector: m.Sector, Message: m.Message}
		if m.Flag != nil {
			rc.Flag = *m.Flag
		}
		if m.Scope != nil {
			rc.Scope = *m.Scope
		}
		if m.DriverNumber != nil {
			d := sd.ref(*m.DriverNumber)
			rc.Driver = &d
		}
		out = append(out, rc)
	}
	return out, nil
}

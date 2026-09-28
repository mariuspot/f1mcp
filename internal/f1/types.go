// Package f1 answers Formula 1 questions in one consistent shape, whichever
// API the data comes from: Jolpica for schedules, results and standings
// back to 1950, and OpenF1 for session detail from 2023. Events are named
// by year and round, sessions by name, durations are seconds and times are
// UTC.
package f1

import "time"

// Sessions of a race weekend, as tools name them.
const (
	Race             = "race"
	Qualifying       = "qualifying"
	Sprint           = "sprint"
	SprintQualifying = "sprint_qualifying"
	Practice1        = "fp1"
	Practice2        = "fp2"
	Practice3        = "fp3"
)

// SessionNames lists every session name, in weekend order.
var SessionNames = []string{Practice1, Practice2, Practice3, SprintQualifying, Sprint, Qualifying, Race}

// Event is a race weekend.
type Event struct {
	Year     int            `json:"year"`
	Round    int            `json:"round"`
	Name     string         `json:"name"` // e.g. "Monaco Grand Prix"
	Circuit  Circuit        `json:"circuit"`
	Sprint   bool           `json:"sprint"` // whether the weekend has a sprint
	Sessions []SessionStart `json:"sessions"`
	// CountryFlagURL links to the country's flag (from 2023).
	CountryFlagURL string `json:"country_flag_url,omitempty"`
}

// Start returns when the event's session starts, and whether it has one.
func (e Event) Start(session string) (time.Time, bool) {
	for _, s := range e.Sessions {
		if s.Session == session {
			return s.Start, true
		}
	}
	return time.Time{}, false
}

type Circuit struct {
	ID       string `json:"id"` // e.g. "monaco", as used by track tools
	Name     string `json:"name"`
	Locality string `json:"locality"`
	Country  string `json:"country"`
}

type SessionStart struct {
	Session string    `json:"session"` // e.g. "qualifying"
	Start   time.Time `json:"start"`   // UTC; zero if not known yet
}

// DriverRef names a driver in results and standings.
type DriverRef struct {
	Code   string `json:"code,omitempty"` // e.g. "VER"; drivers before about 2014 may not have one
	Number int    `json:"number,omitempty"`
	Name   string `json:"name"`
}

type Team struct {
	ID    string `json:"id"` // e.g. "red_bull"
	Name  string `json:"name"`
	Color string `json:"color,omitempty"` // hex, from 2023
}

// Driver is a driver entered in a season or event.
type Driver struct {
	DriverRef
	Team         *Team  `json:"team,omitempty"`
	Nationality  string `json:"nationality,omitempty"`
	HeadshotURL  string `json:"headshot_url,omitempty"` // Formula 1's photo, from 2023; link only
	WikipediaURL string `json:"wikipedia_url,omitempty"`
}

// SessionResult is one driver's result in a session. Which fields are set
// depends on the session: race and sprint results have points, grid and
// status; qualifying has Q1 to Q3; practice has the best lap and gap.
type SessionResult struct {
	Position   int       `json:"position,omitempty"`   // 0 if not classified
	Classified string    `json:"classified,omitempty"` // e.g. "1", "R" (retired), "W", "D"
	Driver     DriverRef `json:"driver"`
	Team       string    `json:"team,omitempty"`

	Grid         int      `json:"grid,omitempty"`
	Laps         int      `json:"laps,omitempty"`
	Status       string   `json:"status,omitempty"`
	Points       float64  `json:"points,omitempty"`
	TimeSeconds  *float64 `json:"time_seconds,omitempty"` // race time, for the winner
	GapSeconds   *float64 `json:"gap_seconds,omitempty"`  // behind the winner or fastest
	FastestLap   *float64 `json:"fastest_lap_seconds,omitempty"`
	FastestLapNo int      `json:"fastest_lap_number,omitempty"`

	Q1 *float64 `json:"q1_seconds,omitempty"`
	Q2 *float64 `json:"q2_seconds,omitempty"`
	Q3 *float64 `json:"q3_seconds,omitempty"`

	BestLap *float64 `json:"best_lap_seconds,omitempty"` // practice
}

// Standing is a position in the drivers' or teams' championship.
type Standing struct {
	Position int        `json:"position"`
	Driver   *DriverRef `json:"driver,omitempty"` // drivers' championship
	Team     string     `json:"team"`
	Points   float64    `json:"points"`
	Wins     int        `json:"wins"`
}

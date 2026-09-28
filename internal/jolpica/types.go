package jolpica

// Response types for the Jolpica (Ergast-compatible) API. They mirror the API:
// numbers arrive as strings and dates and times are separate fields.
// Conversion to standard types happens in the layer above.

type mrData struct {
	Limit            string           `json:"limit"`
	Offset           string           `json:"offset"`
	Total            string           `json:"total"`
	RaceTable        raceTable        `json:"RaceTable"`
	StandingsTable   standingsTable   `json:"StandingsTable"`
	DriverTable      driverTable      `json:"DriverTable"`
	ConstructorTable constructorTable `json:"ConstructorTable"`
}

type raceTable struct {
	Races []Race `json:"Races"`
}

type standingsTable struct {
	StandingsLists []StandingsList `json:"StandingsLists"`
}

type driverTable struct {
	Drivers []Driver `json:"Drivers"`
}

type constructorTable struct {
	Constructors []Constructor `json:"Constructors"`
}

// Race is a race weekend. Which result lists are filled depends on the
// endpoint called.
type Race struct {
	Season         string       `json:"season"`
	Round          string       `json:"round"`
	URL            string       `json:"url"`
	RaceName       string       `json:"raceName"`
	Circuit        Circuit      `json:"Circuit"`
	Date           string       `json:"date"`
	Time           string       `json:"time"`
	FirstPractice  *SessionTime `json:"FirstPractice"`
	SecondPractice *SessionTime `json:"SecondPractice"`
	ThirdPractice  *SessionTime `json:"ThirdPractice"`
	Qualifying     *SessionTime `json:"Qualifying"`
	Sprint         *SessionTime `json:"Sprint"`
	// SprintQualifying was called SprintShootout in 2023.
	SprintQualifying *SessionTime `json:"SprintQualifying"`
	SprintShootout   *SessionTime `json:"SprintShootout"`

	Results           []Result           `json:"Results"`
	QualifyingResults []QualifyingResult `json:"QualifyingResults"`
	SprintResults     []Result           `json:"SprintResults"`
	PitStops          []PitStop          `json:"PitStops"`
}

type SessionTime struct {
	Date string `json:"date"`
	Time string `json:"time"`
}

type Circuit struct {
	CircuitID   string   `json:"circuitId"`
	URL         string   `json:"url"`
	CircuitName string   `json:"circuitName"`
	Location    Location `json:"Location"`
}

type Location struct {
	Lat      string `json:"lat"`
	Long     string `json:"long"`
	Locality string `json:"locality"`
	Country  string `json:"country"`
}

type Driver struct {
	DriverID        string `json:"driverId"`
	PermanentNumber string `json:"permanentNumber"`
	Code            string `json:"code"`
	URL             string `json:"url"`
	GivenName       string `json:"givenName"`
	FamilyName      string `json:"familyName"`
	DateOfBirth     string `json:"dateOfBirth"`
	Nationality     string `json:"nationality"`
}

type Constructor struct {
	ConstructorID string `json:"constructorId"`
	URL           string `json:"url"`
	Name          string `json:"name"`
	Nationality   string `json:"nationality"`
}

// Result is a race or sprint result.
type Result struct {
	Number       string      `json:"number"`
	Position     string      `json:"position"`
	PositionText string      `json:"positionText"`
	Points       string      `json:"points"`
	Driver       Driver      `json:"Driver"`
	Constructor  Constructor `json:"Constructor"`
	Grid         string      `json:"grid"`
	Laps         string      `json:"laps"`
	Status       string      `json:"status"`
	Time         *RaceTime   `json:"Time"`
	FastestLap   *FastestLap `json:"FastestLap"`
}

type RaceTime struct {
	Millis string `json:"millis"`
	Time   string `json:"time"`
}

type FastestLap struct {
	Rank string `json:"rank"`
	Lap  string `json:"lap"`
	Time struct {
		Time string `json:"time"`
	} `json:"Time"`
}

type QualifyingResult struct {
	Number      string      `json:"number"`
	Position    string      `json:"position"`
	Driver      Driver      `json:"Driver"`
	Constructor Constructor `json:"Constructor"`
	Q1          string      `json:"Q1"`
	Q2          string      `json:"Q2"`
	Q3          string      `json:"Q3"`
}

type PitStop struct {
	DriverID string `json:"driverId"`
	Lap      string `json:"lap"`
	Stop     string `json:"stop"`
	// Time is the local time of day at the circuit, e.g. "21:13:21".
	Time     string `json:"time"`
	Duration string `json:"duration"`
}

type StandingsList struct {
	Season               string                `json:"season"`
	Round                string                `json:"round"`
	DriverStandings      []DriverStanding      `json:"DriverStandings"`
	ConstructorStandings []ConstructorStanding `json:"ConstructorStandings"`
}

type DriverStanding struct {
	Position     string        `json:"position"`
	PositionText string        `json:"positionText"`
	Points       string        `json:"points"`
	Wins         string        `json:"wins"`
	Driver       Driver        `json:"Driver"`
	Constructors []Constructor `json:"Constructors"`
}

type ConstructorStanding struct {
	Position     string      `json:"position"`
	PositionText string      `json:"positionText"`
	Points       string      `json:"points"`
	Wins         string      `json:"wins"`
	Constructor  Constructor `json:"Constructor"`
}

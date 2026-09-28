package openf1

import (
	"encoding/json"
	"time"
)

// Response types for OpenF1 endpoints. Pointer fields are ones the API
// returns as null in some rows (e.g. the first lap has no lap_duration).

type Meeting struct {
	MeetingKey          int       `json:"meeting_key"`
	MeetingName         string    `json:"meeting_name"`
	MeetingOfficialName string    `json:"meeting_official_name"`
	Location            string    `json:"location"`
	CountryKey          int       `json:"country_key"`
	CountryCode         string    `json:"country_code"`
	CountryName         string    `json:"country_name"`
	CountryFlag         string    `json:"country_flag"`
	CircuitKey          int       `json:"circuit_key"`
	CircuitShortName    string    `json:"circuit_short_name"`
	CircuitType         string    `json:"circuit_type"`
	CircuitInfoURL      string    `json:"circuit_info_url"`
	CircuitImage        string    `json:"circuit_image"`
	GMTOffset           string    `json:"gmt_offset"`
	DateStart           time.Time `json:"date_start"`
	DateEnd             time.Time `json:"date_end"`
	Year                int       `json:"year"`
	IsCancelled         bool      `json:"is_cancelled"`
}

type Session struct {
	SessionKey       int       `json:"session_key"`
	SessionType      string    `json:"session_type"`
	SessionName      string    `json:"session_name"`
	DateStart        time.Time `json:"date_start"`
	DateEnd          time.Time `json:"date_end"`
	MeetingKey       int       `json:"meeting_key"`
	CircuitKey       int       `json:"circuit_key"`
	CircuitShortName string    `json:"circuit_short_name"`
	CountryKey       int       `json:"country_key"`
	CountryCode      string    `json:"country_code"`
	CountryName      string    `json:"country_name"`
	Location         string    `json:"location"`
	GMTOffset        string    `json:"gmt_offset"`
	Year             int       `json:"year"`
	IsCancelled      bool      `json:"is_cancelled"`
}

type Driver struct {
	MeetingKey    int    `json:"meeting_key"`
	SessionKey    int    `json:"session_key"`
	DriverNumber  int    `json:"driver_number"`
	BroadcastName string `json:"broadcast_name"`
	FullName      string `json:"full_name"`
	NameAcronym   string `json:"name_acronym"`
	TeamName      string `json:"team_name"`
	TeamColour    string `json:"team_colour"`
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	HeadshotURL   string `json:"headshot_url"`
	CountryCode   string `json:"country_code"`
}

type Lap struct {
	MeetingKey      int       `json:"meeting_key"`
	SessionKey      int       `json:"session_key"`
	DriverNumber    int       `json:"driver_number"`
	LapNumber       int       `json:"lap_number"`
	DateStart       time.Time `json:"date_start"`
	LapDuration     *float64  `json:"lap_duration"`
	DurationSector1 *float64  `json:"duration_sector_1"`
	DurationSector2 *float64  `json:"duration_sector_2"`
	DurationSector3 *float64  `json:"duration_sector_3"`
	I1Speed         *int      `json:"i1_speed"`
	I2Speed         *int      `json:"i2_speed"`
	STSpeed         *int      `json:"st_speed"`
	IsPitOutLap     bool      `json:"is_pit_out_lap"`
	SegmentsSector1 []*int    `json:"segments_sector_1"`
	SegmentsSector2 []*int    `json:"segments_sector_2"`
	SegmentsSector3 []*int    `json:"segments_sector_3"`
}

type Stint struct {
	MeetingKey     int    `json:"meeting_key"`
	SessionKey     int    `json:"session_key"`
	StintNumber    int    `json:"stint_number"`
	DriverNumber   int    `json:"driver_number"`
	LapStart       int    `json:"lap_start"`
	LapEnd         int    `json:"lap_end"`
	Compound       string `json:"compound"`
	TyreAgeAtStart int    `json:"tyre_age_at_start"`
}

type Pit struct {
	MeetingKey   int       `json:"meeting_key"`
	SessionKey   int       `json:"session_key"`
	DriverNumber int       `json:"driver_number"`
	Date         time.Time `json:"date"`
	LapNumber    int       `json:"lap_number"`
	PitDuration  *float64  `json:"pit_duration"`
	LaneDuration *float64  `json:"lane_duration"`
	StopDuration *float64  `json:"stop_duration"`
}

type RaceControl struct {
	MeetingKey      int       `json:"meeting_key"`
	SessionKey      int       `json:"session_key"`
	Date            time.Time `json:"date"`
	DriverNumber    *int      `json:"driver_number"`
	LapNumber       *int      `json:"lap_number"`
	Category        string    `json:"category"`
	Flag            *string   `json:"flag"`
	Scope           *string   `json:"scope"`
	Sector          *int      `json:"sector"`
	QualifyingPhase *int      `json:"qualifying_phase"`
	Message         string    `json:"message"`
}

type Weather struct {
	MeetingKey       int       `json:"meeting_key"`
	SessionKey       int       `json:"session_key"`
	Date             time.Time `json:"date"`
	AirTemperature   float64   `json:"air_temperature"`
	TrackTemperature float64   `json:"track_temperature"`
	Humidity         float64   `json:"humidity"`
	Pressure         float64   `json:"pressure"`
	Rainfall         int       `json:"rainfall"`
	WindDirection    int       `json:"wind_direction"`
	WindSpeed        float64   `json:"wind_speed"`
}

type Interval struct {
	MeetingKey   int       `json:"meeting_key"`
	SessionKey   int       `json:"session_key"`
	DriverNumber int       `json:"driver_number"`
	Date         time.Time `json:"date"`
	// Interval and GapToLeader are usually seconds, but can be a string such
	// as "+1 LAP" for lapped cars, or null for the leader.
	// TODO: decide how to model these.
	Interval    json.RawMessage `json:"interval"`
	GapToLeader json.RawMessage `json:"gap_to_leader"`
}

type Position struct {
	MeetingKey   int       `json:"meeting_key"`
	SessionKey   int       `json:"session_key"`
	DriverNumber int       `json:"driver_number"`
	Date         time.Time `json:"date"`
	Position     int       `json:"position"`
}

type CarData struct {
	MeetingKey   int       `json:"meeting_key"`
	SessionKey   int       `json:"session_key"`
	DriverNumber int       `json:"driver_number"`
	Date         time.Time `json:"date"`
	Speed        int       `json:"speed"`
	RPM          int       `json:"rpm"`
	NGear        int       `json:"n_gear"`
	Throttle     int       `json:"throttle"`
	Brake        int       `json:"brake"`
	DRS          int       `json:"drs"`
}

// Location is a car's position in the track's coordinate frame.
type Location struct {
	MeetingKey   int       `json:"meeting_key"`
	SessionKey   int       `json:"session_key"`
	DriverNumber int       `json:"driver_number"`
	Date         time.Time `json:"date"`
	X            int       `json:"x"`
	Y            int       `json:"y"`
	Z            int       `json:"z"`
}

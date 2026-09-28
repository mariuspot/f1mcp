package tools

import "github.com/modelcontextprotocol/go-sdk/mcp"

// Tools backed by the OpenF1 API: detailed session data from 2023 onward.

type MeetingsArgs struct {
	Year        int    `json:"year,omitempty" jsonschema:"year, e.g. 2024; defaults to the current year"`
	CountryName string `json:"country_name,omitempty" jsonschema:"filter by country, e.g. Monaco"`
}

type SessionsArgs struct {
	Year        int    `json:"year,omitempty" jsonschema:"year, e.g. 2024"`
	MeetingKey  string `json:"meeting_key,omitempty" jsonschema:"meeting key from list_meetings, or 'latest'"`
	SessionName string `json:"session_name,omitempty" jsonschema:"filter by session name, e.g. Race, Qualifying, Sprint, Practice 1"`
}

type SessionArgs struct {
	SessionKey string `json:"session_key" jsonschema:"session key from list_sessions, or 'latest'"`
}

type SessionDriverArgs struct {
	SessionKey   string `json:"session_key" jsonschema:"session key from list_sessions, or 'latest'"`
	DriverNumber int    `json:"driver_number,omitempty" jsonschema:"filter by car number, e.g. 1"`
}

type LapsArgs struct {
	SessionKey   string `json:"session_key" jsonschema:"session key from list_sessions, or 'latest'"`
	DriverNumber int    `json:"driver_number,omitempty" jsonschema:"filter by car number, e.g. 1"`
	LapNumber    int    `json:"lap_number,omitempty" jsonschema:"filter by lap number"`
}

type TelemetryArgs struct {
	SessionKey   string `json:"session_key" jsonschema:"session key from list_sessions, or 'latest'"`
	DriverNumber int    `json:"driver_number" jsonschema:"car number, e.g. 1"`
	LapNumber    int    `json:"lap_number" jsonschema:"lap number"`
}

func registerOpenF1(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_meetings",
		Description: "List Formula 1 meetings (race weekends) for a year. Data available from 2023 onward.",
	}, notImplemented[MeetingsArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_sessions",
		Description: "List Formula 1 sessions (practice, qualifying, sprint, race) for a year or meeting. Returns session keys used by the other session tools.",
	}, notImplemented[SessionsArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_session_drivers",
		Description: "Get the drivers taking part in a session, with car numbers and teams.",
	}, notImplemented[SessionArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_laps",
		Description: "Get lap times and sector times for a session, optionally for one driver or lap.",
	}, notImplemented[LapsArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_stints",
		Description: "Get tyre stints (compound and tyre age) for a session, optionally for one driver.",
	}, notImplemented[SessionDriverArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_pit_stops",
		Description: "Get pit stops and pit lane durations for a session.",
	}, notImplemented[SessionArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_race_control",
		Description: "Get race control messages for a session: flags, penalties, safety car and incidents.",
	}, notImplemented[SessionArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_weather",
		Description: "Get weather conditions during a session: air and track temperature, rainfall, wind.",
	}, notImplemented[SessionArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_intervals",
		Description: "Get gaps to the leader and to the car ahead during a race, optionally for one driver.",
	}, notImplemented[SessionDriverArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_positions",
		Description: "Get position changes during a session, optionally for one driver.",
	}, notImplemented[SessionDriverArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_car_telemetry",
		Description: "Get car telemetry (speed, throttle, brake, gear, RPM, DRS) for one driver on one lap.",
	}, notImplemented[TelemetryArgs])
}

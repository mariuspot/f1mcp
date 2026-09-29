package tools

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/f1"
)

// Session detail tools (from 2023). Each gives a summary by default and the
// full data with detail, paged with limit and cursor where it can be long.

type PageArgs struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"how many items per page (default 200, max 1000)"`
	Cursor string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page"`
}

type LapsArgs struct {
	LapSessionArgs
	Driver string `json:"driver,omitempty" jsonschema:"driver code (VER), car number or name; lists every lap of that driver"`
	Lap    int    `json:"lap,omitempty" jsonschema:"lap number; lists that lap for every driver"`
	PageArgs
}

type LapsResult struct {
	Event    f1.Event     `json:"event"`
	Session  string       `json:"session"`
	BestLaps []f1.BestLap `json:"best_laps,omitempty"`
	Laps     []f1.LapTime `json:"laps,omitempty"`
	Page     *f1.Page     `json:"page,omitempty"`
}

type RaceOrderArgs struct {
	SessionArgs
	Lap      int  `json:"lap,omitempty" jsonschema:"order at the end of this lap; defaults to the last lap"`
	LapChart bool `json:"lap_chart,omitempty" jsonschema:"also return every driver's position at the end of every lap"`
}

type RaceOrderResult struct {
	Event   f1.Event `json:"event"`
	Session string   `json:"session"`
	f1.RaceOrder
}

type DriverFilterArgs struct {
	SessionArgs
	Driver string `json:"driver,omitempty" jsonschema:"only this driver: code (VER), car number or name"`
}

type PitStopsArgs struct {
	EventArgs
	Driver string `json:"driver,omitempty" jsonschema:"only this driver: code (VER), car number or name"`
}

type PitStopsResult struct {
	Event    f1.Event     `json:"event"`
	PitStops []f1.PitStop `json:"pit_stops"`
}

type StintsResult struct {
	Event   f1.Event   `json:"event"`
	Session string     `json:"session"`
	Stints  []f1.Stint `json:"stints"`
}

type RaceControlArgs struct {
	SessionArgs
	All      bool   `json:"all,omitempty" jsonschema:"every message, not only key events (red flags, safety cars, penalties, investigations)"`
	Category string `json:"category,omitempty" jsonschema:"only this category: Flag, SafetyCar, Drs, CarEvent or Other"`
	FromLap  int    `json:"from_lap,omitempty"`
	ToLap    int    `json:"to_lap,omitempty"`
	PageArgs
}

type RaceControlResult struct {
	Event    f1.Event                `json:"event"`
	Session  string                  `json:"session"`
	Messages []f1.RaceControlMessage `json:"messages"`
	Page     f1.Page                 `json:"page"`
}

type WeatherArgs struct {
	SessionArgs
	Detail bool `json:"detail,omitempty" jsonschema:"return the readings (about one a minute), not just the summary"`
	PageArgs
}

type WeatherResult struct {
	Event    f1.Event           `json:"event"`
	Session  string             `json:"session"`
	Summary  f1.WeatherSummary  `json:"summary"`
	Readings []f1.WeatherSample `json:"readings,omitempty"`
	Page     *f1.Page           `json:"page,omitempty"`
}

type TelemetryArgs struct {
	LapSessionArgs
	Driver string `json:"driver" jsonschema:"driver code (VER), car number or name"`
	Lap    int    `json:"lap" jsonschema:"lap number"`
	Detail bool   `json:"detail,omitempty" jsonschema:"also return the car data readings (about 4 a second)"`
	PageArgs
}

type TelemetryResult struct {
	Event   f1.Event `json:"event"`
	Session string   `json:"session"`
	f1.LapTelemetry
	Readings []f1.TelemetrySample `json:"readings,omitempty"`
	Page     *f1.Page             `json:"page,omitempty"`
}

// sessionEvent resolves the event and defaults the session to the race.
func sessionEvent(ctx context.Context, svc *f1.Service, a *SessionArgs) (f1.Event, error) {
	if a.Session == "" {
		a.Session = f1.Race
	}
	return svc.ResolveEvent(ctx, a.Year, a.Round)
}

// lapSessionEvent is sessionEvent for the lap tools.
func lapSessionEvent(ctx context.Context, svc *f1.Service, a *LapSessionArgs) (f1.Event, error) {
	if a.Session == "" {
		a.Session = f1.Race
	}
	a.Session = strings.ToLower(a.Session)
	return svc.ResolveEvent(ctx, a.Year, a.Round)
}

func registerSession(s *mcp.Server, svc *f1.Service) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_laps",
		Description: "Get lap times and sector times (seconds) for a session or a part of qualifying (q1, q2, q3), from 2023. By default, each " +
			"driver's best lap, fastest first, with the gap to the fastest. With driver: every lap of that driver. With lap: that lap for every driver.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a LapsArgs) (*mcp.CallToolResult, LapsResult, error) {
		e, err := lapSessionEvent(ctx, svc, &a.LapSessionArgs)
		if err != nil {
			return nil, LapsResult{}, err
		}
		out := LapsResult{Event: e, Session: a.Session}
		if a.Driver == "" && a.Lap == 0 {
			out.BestLaps, err = svc.BestLaps(ctx, e, a.Session)
			return nil, out, err
		}
		laps, err := svc.Laps(ctx, e, a.Session, a.Driver, a.Lap)
		if err != nil {
			return nil, LapsResult{}, err
		}
		p := f1.Page{}
		out.Laps, p = f1.Paginate(laps, a.Cursor, a.Limit)
		out.Page = &p
		return nil, out, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_race_order",
		Description: "Get the running order at the end of a lap of a race or sprint (default: the last lap), from 2023: position, laps completed, " +
			"gap to the leader and interval to the car ahead in seconds (from lap start times, so within a few hundredths of the official gaps), " +
			"or laps down. With lap_chart, every driver's position on every lap.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a RaceOrderArgs) (*mcp.CallToolResult, RaceOrderResult, error) {
		e, err := sessionEvent(ctx, svc, &a.SessionArgs)
		if err != nil {
			return nil, RaceOrderResult{}, err
		}
		ro, err := svc.RaceOrder(ctx, e, a.Session, a.Lap, a.LapChart)
		if err != nil {
			return nil, RaceOrderResult{}, err
		}
		return nil, RaceOrderResult{Event: e, Session: a.Session, RaceOrder: ro}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_pit_stops",
		Description: "Get a race's pit stops, from 2023: driver, stop number, lap, pit lane time and (when known) stationary time in seconds, " +
			"and the tyre compounds before and after.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a PitStopsArgs) (*mcp.CallToolResult, PitStopsResult, error) {
		e, err := svc.ResolveEvent(ctx, a.Year, a.Round)
		if err != nil {
			return nil, PitStopsResult{}, err
		}
		ps, err := svc.PitStops(ctx, e, a.Driver)
		if err != nil {
			return nil, PitStopsResult{}, err
		}
		return nil, PitStopsResult{Event: e, PitStops: ps}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_tyre_stints",
		Description: "Get each driver's tyre stints in a session, from 2023: compound, first and last lap, laps run, and tyre age at the start.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a DriverFilterArgs) (*mcp.CallToolResult, StintsResult, error) {
		e, err := sessionEvent(ctx, svc, &a.SessionArgs)
		if err != nil {
			return nil, StintsResult{}, err
		}
		st, err := svc.Stints(ctx, e, a.Session, a.Driver)
		if err != nil {
			return nil, StintsResult{}, err
		}
		return nil, StintsResult{Event: e, Session: a.Session, Stints: st}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_race_control",
		Description: "Get race control messages for a session, from 2023: by default the key events (red flags, safety cars, penalties, " +
			"investigations, the chequered flag); with all, every message including yellow flags by marshal sector. Filter by category and laps.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a RaceControlArgs) (*mcp.CallToolResult, RaceControlResult, error) {
		e, err := sessionEvent(ctx, svc, &a.SessionArgs)
		if err != nil {
			return nil, RaceControlResult{}, err
		}
		msgs, err := svc.RaceControl(ctx, e, a.Session, a.Category, a.FromLap, a.ToLap, a.All)
		if err != nil {
			return nil, RaceControlResult{}, err
		}
		out := RaceControlResult{Event: e, Session: a.Session}
		out.Messages, out.Page = f1.Paginate(msgs, a.Cursor, a.Limit)
		return nil, out, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_weather",
		Description: "Get the weather during a session, from 2023: air and track temperature (°C), humidity, wind (m/s) as min, max and average, " +
			"and whether and when it rained. With detail, the readings (about one a minute).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a WeatherArgs) (*mcp.CallToolResult, WeatherResult, error) {
		e, err := sessionEvent(ctx, svc, &a.SessionArgs)
		if err != nil {
			return nil, WeatherResult{}, err
		}
		ws, err := svc.Weather(ctx, e, a.Session)
		if err != nil {
			return nil, WeatherResult{}, err
		}
		sum, err := f1.SummariseWeather(ws)
		if err != nil {
			return nil, WeatherResult{}, err
		}
		out := WeatherResult{Event: e, Session: a.Session, Summary: sum}
		if a.Detail {
			p := f1.Page{}
			out.Readings, p = f1.Paginate(ws, a.Cursor, a.Limit)
			out.Page = &p
		}
		return nil, out, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_car_telemetry",
		Description: "Get car telemetry for one driver's lap, from 2023: top and minimum speed, full throttle and braking share, gear changes, " +
			"and each braking zone (speed before and at its slowest, duration, and the corner braked for, e.g. 'Turn 1, La Source'). " +
			"With detail, the readings (about 4 a second: speed, throttle, brake, gear, RPM).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a TelemetryArgs) (*mcp.CallToolResult, TelemetryResult, error) {
		e, err := lapSessionEvent(ctx, svc, &a.LapSessionArgs)
		if err != nil {
			return nil, TelemetryResult{}, err
		}
		lt, err := svc.CarTelemetry(ctx, e, a.Session, a.Driver, a.Lap)
		if err != nil {
			return nil, TelemetryResult{}, err
		}
		out := TelemetryResult{Event: e, Session: a.Session, LapTelemetry: lt}
		if a.Detail {
			p := f1.Page{}
			out.Readings, p = f1.Paginate(lt.Samples, a.Cursor, a.Limit)
			out.Page = &p
		}
		return nil, out, nil
	})
}

package tools

import (
	"context"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/f1"
)

type ScheduleArgs struct {
	Year int `json:"year,omitempty" jsonschema:"season, e.g. 2024; defaults to the current season"`
}

type ScheduleResult struct {
	Year   int        `json:"year"`
	Events []f1.Event `json:"events"`
}

type SessionResultsResult struct {
	Event   f1.Event           `json:"event"`
	Session string             `json:"session"`
	Results []f1.SessionResult `json:"results"`
}

type StandingsArgs struct {
	Year         int    `json:"year,omitempty" jsonschema:"season, e.g. 2024; defaults to the current season"`
	Round        string `json:"round,omitempty" jsonschema:"standings after this round (number or name); defaults to the latest"`
	Championship string `json:"championship,omitempty" jsonschema:"drivers (default) or teams"`
}

type StandingsResult struct {
	Year         int           `json:"year"`
	AfterRound   int           `json:"after_round"`
	Championship string        `json:"championship"`
	Standings    []f1.Standing `json:"standings"`
}

type DriversArgs struct {
	Year  int    `json:"year,omitempty" jsonschema:"season, e.g. 2024; defaults to the current season"`
	Round string `json:"round,omitempty" jsonschema:"only drivers entered in this round (number, 'last', 'next' or name); defaults to the whole season"`
}

type DriversResult struct {
	Year    int         `json:"year"`
	Event   *f1.Event   `json:"event,omitempty"`
	Drivers []f1.Driver `json:"drivers"`
}

func registerSeason(r *Registry, svc *f1.Service) {
	add(r, &mcp.Tool{
		Name: "get_schedule",
		Description: "Get a Formula 1 season's race calendar: each round's event name, circuit (with the circuit ID used by track tools), " +
			"session start times in UTC, and whether it has a sprint. Any season from 1950.",
	}, func(ctx context.Context, a ScheduleArgs) (ScheduleResult, []Image, error) {
		events, err := svc.Schedule(ctx, a.Year)
		if err != nil {
			return ScheduleResult{}, nil, err
		}
		year := a.Year
		if len(events) > 0 {
			year = events[0].Year
		}
		return ScheduleResult{Year: year, Events: events}, nil, nil
	})

	add(r, &mcp.Tool{
		Name: "get_session_results",
		Description: "Get the classification of a session: race and sprint (positions, grid, laps, status, points, time or gap in seconds, fastest lap), " +
			"qualifying (Q1-Q3 times in seconds), or practice and sprint qualifying (best lap and gap; from 2023). Races, sprints and qualifying from 1950.",
	}, func(ctx context.Context, a SessionArgs) (SessionResultsResult, []Image, error) {
		if a.Session == "" {
			a.Session = f1.Race
		}
		e, err := svc.ResolveEvent(ctx, a.Year, a.Round)
		if err != nil {
			return SessionResultsResult{}, nil, err
		}
		rs, err := svc.SessionResults(ctx, e, a.Session)
		if err != nil {
			return SessionResultsResult{}, nil, err
		}
		return SessionResultsResult{Event: e, Session: a.Session, Results: rs}, nil, nil
	})

	add(r, &mcp.Tool{
		Name:        "get_standings",
		Description: "Get the drivers' or teams' championship standings (position, points, wins), after a given round or the latest. Any season from 1950.",
	}, func(ctx context.Context, a StandingsArgs) (StandingsResult, []Image, error) {
		round := 0
		if a.Round != "" {
			if n, err := strconv.Atoi(a.Round); err == nil {
				round = n
			} else {
				e, err := svc.ResolveEvent(ctx, a.Year, a.Round)
				if err != nil {
					return StandingsResult{}, nil, err
				}
				round = e.Round
			}
		}
		if a.Championship == "" {
			a.Championship = f1.DriversChampionship
		}
		st, after, err := svc.Standings(ctx, a.Year, round, a.Championship)
		if err != nil {
			return StandingsResult{}, nil, err
		}
		return StandingsResult{Year: svc.Year(a.Year), AfterRound: after, Championship: a.Championship, Standings: st}, nil, nil
	})

	add(r, &mcp.Tool{
		Name: "list_drivers",
		Description: "List the drivers in a season or one event: code, name, race number, team, nationality and Wikipedia link, " +
			"plus team colour and a headshot photo link from 2023.",
	}, func(ctx context.Context, a DriversArgs) (DriversResult, []Image, error) {
		var e *f1.Event
		if a.Round != "" {
			ev, err := svc.ResolveEvent(ctx, a.Year, a.Round)
			if err != nil {
				return DriversResult{}, nil, err
			}
			e = &ev
		}
		ds, err := svc.Drivers(ctx, a.Year, e)
		if err != nil {
			return DriversResult{}, nil, err
		}
		return DriversResult{Year: svc.Year(a.Year), Event: e, Drivers: ds}, nil, nil
	})
}

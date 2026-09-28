package tools

import "github.com/modelcontextprotocol/go-sdk/mcp"

// Tools backed by the Jolpica F1 API (Ergast-compatible): historical data from 1950 onward.

type SeasonArgs struct {
	Season string `json:"season,omitempty" jsonschema:"season year, e.g. 2024, or 'current'; defaults to current"`
}

type RoundArgs struct {
	Season string `json:"season,omitempty" jsonschema:"season year, e.g. 2024, or 'current'; defaults to current"`
	Round  string `json:"round,omitempty" jsonschema:"round number, or 'last' for the most recent; defaults to last"`
}

type StandingsArgs struct {
	Season string `json:"season,omitempty" jsonschema:"season year, e.g. 2024, or 'current'; defaults to current"`
	Round  string `json:"round,omitempty" jsonschema:"round number to get standings after; defaults to the latest round"`
}

func registerJolpica(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_schedule",
		Description: "Get the Formula 1 race calendar for a season.",
	}, notImplemented[SeasonArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_race_results",
		Description: "Get the classified results of a Formula 1 race.",
	}, notImplemented[RoundArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_qualifying_results",
		Description: "Get the qualifying results (Q1, Q2, Q3 times) for a Formula 1 race weekend.",
	}, notImplemented[RoundArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_sprint_results",
		Description: "Get the sprint race results for a Formula 1 race weekend that has a sprint.",
	}, notImplemented[RoundArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_driver_standings",
		Description: "Get the Formula 1 drivers' championship standings for a season, optionally after a specific round.",
	}, notImplemented[StandingsArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_constructor_standings",
		Description: "Get the Formula 1 constructors' championship standings for a season, optionally after a specific round.",
	}, notImplemented[StandingsArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_drivers",
		Description: "List the drivers who competed in a Formula 1 season.",
	}, notImplemented[SeasonArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_constructors",
		Description: "List the constructors (teams) that competed in a Formula 1 season.",
	}, notImplemented[SeasonArgs])

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_circuits",
		Description: "List the circuits used in a Formula 1 season.",
	}, notImplemented[SeasonArgs])
}

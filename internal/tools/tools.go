// Package tools defines the MCP tools exposed by f1mcp. Each tool is a thin
// wrapper over the f1 service, which hides where the data comes from.
package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/f1"
)

// Register adds all tools to s.
func Register(s *mcp.Server, svc *f1.Service) {
	registerSeason(s, svc)
	registerSession(s, svc)
	registerTrack(s, svc)
	registerReplay(s, svc)
}

// Arguments shared by several tools.

type EventArgs struct {
	Year  int    `json:"year,omitempty" jsonschema:"season, e.g. 2024; defaults to the current season"`
	Round string `json:"round,omitempty" jsonschema:"round number, 'last' (default), 'next', or part of the event, circuit, city or country name, e.g. 'Monaco' or 'Spa'"`
}

type SessionArgs struct {
	EventArgs
	Session string `json:"session,omitempty" jsonschema:"race (default), qualifying, sprint, sprint_qualifying, fp1, fp2 or fp3"`
}

// Package server builds the f1mcp MCP server.
package server

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/f1"
	"github.com/mariuspot/f1mcp/internal/jolpica"
	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tools"
)

// New returns an MCP server with all tools registered, backed by the live
// Jolpica and OpenF1 APIs.
func New(version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "f1mcp", Version: version}, nil)
	svc := f1.New(jolpica.NewClient("", nil), openf1.NewClient("", nil))
	tools.Register(s, svc)
	return s
}

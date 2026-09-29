// Package server builds the f1mcp MCP server and its tools.
package server

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/f1"
	"github.com/mariuspot/f1mcp/internal/jolpica"
	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tools"
)

// New returns an MCP server with all tools registered, backed by the live
// Jolpica and OpenF1 APIs, and the same tools for the web API.
func New(version string) (*mcp.Server, *tools.Registry) {
	s := mcp.NewServer(&mcp.Implementation{Name: "f1mcp", Version: version}, nil)
	svc := f1.New(jolpica.NewClient("", nil), openf1.NewClient("", nil))
	return s, tools.Register(s, svc)
}

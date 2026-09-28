// Package server builds the f1mcp MCP server.
package server

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/tools"
)

// New returns an MCP server with all tools registered.
func New(version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "f1mcp", Version: version}, nil)
	tools.Register(s)
	return s
}

// Package tools defines the MCP tools exposed by f1mcp.
package tools

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register adds all tools to s.
func Register(s *mcp.Server) {
	registerJolpica(s)
	registerOpenF1(s)
}

var errNotImplemented = errors.New("not implemented")

// notImplemented is a placeholder handler for scaffolded tools.
func notImplemented[In any](context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error) {
	return nil, nil, errNotImplemented
}

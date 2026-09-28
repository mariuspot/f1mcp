// Command f1mcp runs the f1mcp MCP server.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/server"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s := server.New(version)
	if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

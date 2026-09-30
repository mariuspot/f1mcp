// Package server builds the f1mcp MCP server and its tools.
package server

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/f1"
	"github.com/mariuspot/f1mcp/internal/jolpica"
	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tools"
	"github.com/mariuspot/f1mcp/internal/transcribe"
)

// New returns an MCP server with all tools registered, backed by the live
// Jolpica and OpenF1 APIs, and the same tools for the web API. With
// OPENAI_API_KEY set, team radio is transcribed, and the transcripts are
// kept in F1MCP_CACHE_DIR if that's set.
func New(version string) (*mcp.Server, *tools.Registry) {
	s := mcp.NewServer(&mcp.Implementation{Name: "f1mcp", Version: version}, nil)
	svc := Service()
	if t := transcribe.New(os.Getenv("OPENAI_API_KEY"), transcriptDir(), nil); t != nil {
		svc.WithTranscriber(t)
	}
	return s, tools.Register(s, svc)
}

var (
	svcOnce sync.Once
	svc     *f1.Service
)

// Service is the f1 service shared by the tools and the live page, so they
// share one cache and one OpenF1 rate limit.
func Service() *f1.Service {
	svcOnce.Do(func() {
		svc = f1.New(jolpica.NewClient("", nil), openf1.NewClient("", nil))
	})
	return svc
}

func transcriptDir() string {
	if dir := os.Getenv("F1MCP_CACHE_DIR"); dir != "" {
		return filepath.Join(dir, "transcripts")
	}
	return ""
}

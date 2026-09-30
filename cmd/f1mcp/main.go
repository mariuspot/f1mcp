// Command f1mcp runs the f1mcp MCP server, over stdio (the default, e.g. for
// `docker run -i`) or HTTP. Over HTTP it also serves the tools as a JSON web
// API for the chat front end, under /api/ and /img/; MCP is at every other
// path.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/api"
	"github.com/mariuspot/f1mcp/internal/insight"
	"github.com/mariuspot/f1mcp/internal/replay"
	"github.com/mariuspot/f1mcp/internal/server"
)

var version = "dev"

func main() {
	transport := flag.String("transport", envOr("F1MCP_TRANSPORT", "stdio"), "stdio or http")
	addr := flag.String("addr", envOr("F1MCP_ADDR", ":8080"), "listen address for -transport http")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s, reg := server.New(version)
	switch *transport {
	case "stdio":
		if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	case "http":
		mux := http.NewServeMux()
		web := api.Handler(reg, 256<<20)
		mux.Handle("/api/", web)
		mux.Handle("/img/", web)
		// The live page, playing sessions collected into F1MCP_REPLAYS_DIR.
		player := replay.NewPlayer(os.Getenv("F1MCP_REPLAYS_DIR"))
		// With ANTHROPIC_API_KEY set, Claude comments on what's happening.
		claude := insight.NewClaude(os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("F1MCP_INSIGHT_MODEL"), nil)
		if c := insight.NewCommentator(claude, insightDir()); c != nil {
			player.WithCommentator(c)
		}
		liveAPI := api.LiveHandler(player)
		mux.Handle("/api/live", liveAPI)
		mux.Handle("/api/live/", liveAPI)
		mux.Handle("/", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, nil))
		srv := &http.Server{Addr: *addr, Handler: mux}
		go func() {
			<-ctx.Done()
			srv.Shutdown(context.Background())
		}()
		log.Printf("f1mcp listening on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown transport %q (want stdio or http)", *transport)
	}
}

// insightDir is where insights are kept, so replays played again cost
// nothing: F1MCP_CACHE_DIR/insights, if that's set.
func insightDir() string {
	if dir := os.Getenv("F1MCP_CACHE_DIR"); dir != "" {
		return filepath.Join(dir, "insights")
	}
	return ""
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

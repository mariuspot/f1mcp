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
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/api"
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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

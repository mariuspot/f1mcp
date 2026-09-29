// Package tools defines the MCP tools exposed by f1mcp. Each tool is a thin
// wrapper over the f1 service, which hides where the data comes from.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/f1"
)

// Register adds all tools to s, and returns them for the web API too.
func Register(s *mcp.Server, svc *f1.Service) *Registry {
	r := &Registry{s: s, byName: map[string]*Tool{}}
	registerSeason(r, svc)
	registerSession(r, svc)
	registerTrack(r, svc)
	registerReplay(r, svc)
	registerRadio(r, svc)
	return r
}

// Image is a picture a tool returns alongside its data, e.g. a track map.
type Image struct {
	Data     []byte
	MIMEType string
	Name     string // what it shows, e.g. "map", "corner", "faster"
	// Link, if set, is an MCP resource with the same picture.
	Link *mcp.ResourceLink
}

// Tool is a tool as the web API sees it.
type Tool struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	InputSchema *jsonschema.Schema `json:"input_schema"`
	// Call runs the tool with JSON arguments.
	Call func(ctx context.Context, args json.RawMessage) (any, []Image, error) `json:"-"`
}

// Registry holds the tools, each added both to the MCP server and to the
// web API.
type Registry struct {
	s      *mcp.Server
	tools  []*Tool
	byName map[string]*Tool
}

// Tools returns the tools in the order they were added.
func (r *Registry) Tools() []*Tool { return r.tools }

// Tool returns the tool with a name, or nil.
func (r *Registry) Tool(name string) *Tool { return r.byName[name] }

// add adds a tool whose handler returns its data and any images: over MCP
// the images are image content, and over the web API they are links.
func add[In, Out any](r *Registry, t *mcp.Tool, h func(ctx context.Context, a In) (Out, []Image, error)) {
	mcp.AddTool(r.s, t, func(ctx context.Context, _ *mcp.CallToolRequest, a In) (*mcp.CallToolResult, Out, error) {
		out, imgs, err := h(ctx, a)
		if err != nil || len(imgs) == 0 {
			return nil, out, err
		}
		res := &mcp.CallToolResult{}
		for _, img := range imgs {
			res.Content = append(res.Content, &mcp.ImageContent{Data: img.Data, MIMEType: img.MIMEType})
			if img.Link != nil {
				res.Content = append(res.Content, img.Link)
			}
		}
		return res, out, nil
	})
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		panic(fmt.Sprintf("tool %s: %v", t.Name, err))
	}
	tool := &Tool{Name: t.Name, Description: t.Description, InputSchema: schema,
		Call: func(ctx context.Context, args json.RawMessage) (any, []Image, error) {
			var a In
			if len(bytes.TrimSpace(args)) > 0 {
				dec := json.NewDecoder(bytes.NewReader(args))
				dec.DisallowUnknownFields()
				if err := dec.Decode(&a); err != nil {
					return nil, nil, fmt.Errorf("%w: %v", ErrBadArguments, err)
				}
			}
			return h(ctx, a)
		}}
	r.tools = append(r.tools, tool)
	r.byName[t.Name] = tool
}

// ErrBadArguments is returned for arguments that don't fit a tool.
var ErrBadArguments = fmt.Errorf("bad arguments")

// Arguments shared by several tools.

type EventArgs struct {
	Year  int    `json:"year,omitempty" jsonschema:"season, e.g. 2024; defaults to the current season"`
	Round string `json:"round,omitempty" jsonschema:"round number, 'last' (default), 'next', or part of the event, circuit, city or country name, e.g. 'Monaco' or 'Spa'"`
}

type SessionArgs struct {
	EventArgs
	Session string `json:"session,omitempty" jsonschema:"race (default), qualifying, sprint, sprint_qualifying, fp1, fp2 or fp3"`
}

// LapSessionArgs is SessionArgs for the lap tools, which also take a part
// of qualifying.
type LapSessionArgs struct {
	EventArgs
	Session string `json:"session,omitempty" jsonschema:"race (default), qualifying, sprint, sprint_qualifying, fp1, fp2, fp3, or a part of qualifying: q1, q2, q3, or sq1, sq2, sq3 for sprint qualifying"`
}

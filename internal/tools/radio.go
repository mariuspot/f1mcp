package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/f1"
)

type RadioArgs struct {
	LapSessionArgs
	Driver  string `json:"driver,omitempty" jsonschema:"only this driver: code (VER), car number or name"`
	FromLap int    `json:"from_lap,omitempty" jsonschema:"only clips from this lap on"`
	ToLap   int    `json:"to_lap,omitempty" jsonschema:"only clips up to this lap"`
	PageArgs
}

type RadioResult struct {
	Event   f1.Event       `json:"event"`
	Session string         `json:"session"`
	Clips   []f1.RadioClip `json:"clips"`
	Page    f1.Page        `json:"page"`
}

// radioPage is the default number of clips per page: each new clip is
// transcribed, so pages are kept short.
const radioPage = 30

func registerRadio(r *Registry, svc *f1.Service) {
	add(r, &mcp.Tool{
		Name: "get_team_radio",
		Description: "Get the team radio clips broadcast in a session or a part of qualifying, from 2023: when, the driver and their lap, " +
			"a link to Formula 1's recording (MP3), and what was said (a speech-to-text transcript, when available). Only the clips " +
			"Formula 1 chose to broadcast exist, usually 10-60 a session. Filter by driver and laps.",
	}, func(ctx context.Context, a RadioArgs) (RadioResult, []Image, error) {
		e, err := lapSessionEvent(ctx, svc, &a.LapSessionArgs)
		if err != nil {
			return RadioResult{}, nil, err
		}
		clips, err := svc.TeamRadio(ctx, e, a.Session, a.Driver, a.FromLap, a.ToLap)
		if err != nil {
			return RadioResult{}, nil, err
		}
		if a.Limit == 0 {
			a.Limit = radioPage
		}
		out := RadioResult{Event: e, Session: a.Session}
		out.Clips, out.Page = f1.Paginate(clips, a.Cursor, a.Limit)
		svc.TranscribeClips(ctx, out.Clips)
		return out, nil, nil
	})
}

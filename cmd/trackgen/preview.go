package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mariuspot/f1mcp/internal/f1"
	"github.com/mariuspot/f1mcp/internal/insight"
	"github.com/mariuspot/f1mcp/internal/jolpica"
	"github.com/mariuspot/f1mcp/internal/openf1"
)

// preview prints the pre-race data for a race or sprint and, unless dry,
// the preview Claude writes from it.
func preview(ctx context.Context, year int, round, session string, dry bool) error {
	svc := f1.New(jolpica.NewClient("", nil), openf1.NewClient("", nil))
	e, err := svc.ResolveEvent(ctx, year, round)
	if err != nil {
		return err
	}
	if dry {
		d, err := svc.Preview(ctx, e, session)
		if err != nil {
			return err
		}
		fmt.Println(insight.PreviewPrompt(d, nil))
		return nil
	}
	model := os.Getenv("F1MCP_PREVIEW_MODEL")
	if model == "" {
		model = insight.DefaultPreviewModel
	}
	p := insight.NewPreviewer(insight.NewClaude(os.Getenv("ANTHROPIC_API_KEY"), model, nil), svc, "")
	if p == nil {
		return fmt.Errorf("set ANTHROPIC_API_KEY, or use -dry")
	}
	msgs, err := p.Messages(ctx, e, session, nil)
	if err != nil {
		return err
	}
	out, _ := json.MarshalIndent(msgs, "", "  ")
	fmt.Println(string(out))
	return nil
}

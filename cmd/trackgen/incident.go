package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/mariuspot/f1mcp/internal/f1"
	"github.com/mariuspot/f1mcp/internal/jolpica"
	"github.com/mariuspot/f1mcp/internal/openf1"
)

// Incident is a stored incident, drawn on its track map and nearest corner
// by render.
type Incident = f1.Incident

// findIncident works out what caused an event in an OpenF1 session (see
// f1.Service.FindIncident) and stores it in dir.
func findIncident(ctx context.Context, sessionKey int, event string, fromLap int, involved []int, dir string) error {
	svc := f1.New(jolpica.NewClient("", nil), openf1.NewClient("", nil))
	inc, err := svc.FindIncident(ctx, sessionKey, event, fromLap, involved)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, inc.Name+".json"), inc); err != nil {
		return err
	}
	log.Printf("%s: %s", inc.Name, inc.Overlay.Caption)
	return nil
}

func loadIncidents(dir string) ([]Incident, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var out []Incident
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var inc Incident
		if err := json.Unmarshal(b, &inc); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out = append(out, inc)
	}
	return out, nil
}

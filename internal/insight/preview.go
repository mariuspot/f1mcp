package insight

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mariuspot/f1mcp/internal/f1"
	"github.com/mariuspot/f1mcp/internal/live"
)

// PreviewVersion is part of a cached preview's key.
const PreviewVersion = "1"

// DefaultPreviewModel writes previews: one call per race, so the most
// capable model.
const DefaultPreviewModel = "claude-opus-5-5"

// PreviewMessage is one message of a pre-race preview.
type PreviewMessage struct {
	Topic string `json:"topic"` // e.g. "grid", "strategy"
	Text  string `json:"text"`
}

// Previewer writes pre-race previews from what's known before the start.
type Previewer struct {
	claude *Claude
	svc    *f1.Service
	dir    string
}

// NewPreviewer returns a previewer, or nil if claude is nil.
func NewPreviewer(claude *Claude, svc *f1.Service, dir string) *Previewer {
	if claude == nil || svc == nil {
		return nil
	}
	return &Previewer{claude: claude, svc: svc, dir: dir}
}

// PreviewSystem is the preview writer's instructions.
const PreviewSystem = `You are a Formula 1 commentator warming fans up in the half hour before lights out, writing short messages for the live page.

You get what's known before the start: the circuit, the qualifying result and starting grid, the championship going in, last year's race at this circuit (grid, result, strategies, pit lane time, safety cars), and the latest weather.

Write 5 or 6 messages, in this order, each on its own topic:
1. welcome: the race, the circuit and the conditions right now.
2. grid: the front rows and anything notable in qualifying.
3. last_year: what happened here last year, and how this year's grid compares.
4. strategy: the likely strategy, from last year's stops and today's weather, and the pit lane time.
5. championship: what's at stake in the standings.
6. watch: drivers to watch (starting out of position, a grid penalty, a big move in qualifying) and where on the circuit to watch.

Rules:
- Each message one to three sentences, at most 60 words. Lively, like a commentator, but no hype words piled up and no emoji.
- Name drivers by their three-letter codes in capitals (VER, NOR).
- Every fact must come from the data. Weather, strategy and outcomes you infer must be marked "likely" or "could". Don't invent overtaking spots, DRS zones, tyre allocations or race distance beyond what's given; you may name corners and straights from the circuit data.
- A driver whose grid place differs from where they qualified has a penalty or change; say they start from further back without guessing why unless the data says.
- Skip a topic if there's no data for it.

Answer with only a JSON array of objects with "topic" and "text", for example:
[{"topic":"welcome","text":"..."},{"topic":"grid","text":"..."}]`

// Messages writes, or reads from the cache, the preview of an event's race
// or sprint, with the weather as it is now.
func (p *Previewer) Messages(ctx context.Context, e f1.Event, session string, w *live.Weather) ([]PreviewMessage, error) {
	file := ""
	if p.dir != "" {
		file = filepath.Join(p.dir, "previews", fmt.Sprintf("%d-%02d-%s-%s-v%s.json", e.Year, e.Round, session, sanitize(p.claude.Model()), PreviewVersion))
		if b, err := os.ReadFile(file); err == nil {
			var msgs []PreviewMessage
			if json.Unmarshal(b, &msgs) == nil {
				return msgs, nil
			}
		}
	}
	data, err := p.svc.Preview(ctx, e, session)
	if err != nil {
		return nil, err
	}
	reply, err := p.claude.Complete(ctx, PreviewSystem, PreviewPrompt(data, w), 8000, "medium")
	if err != nil {
		return nil, err
	}
	msgs, err := parsePreview(reply)
	if err != nil {
		return nil, err
	}
	if file != "" {
		if b, err := json.MarshalIndent(msgs, "", "  "); err == nil && os.MkdirAll(filepath.Dir(file), 0o755) == nil {
			_ = os.WriteFile(file, b, 0o644)
		}
	}
	return msgs, nil
}

// PreviewPrompt describes what's known before the start, and the weather.
func PreviewPrompt(d f1.PreviewData, w *live.Weather) string {
	var b strings.Builder
	e := d.Event
	fmt.Fprintf(&b, "%d %s (round %d), the %s, at %s, %s.\n", e.Year, e.Name, e.Round, d.Session, e.Circuit.Name, e.Circuit.Locality)
	if w != nil {
		rain := "dry"
		if w.Rain {
			rain = "raining"
		}
		fmt.Fprintf(&b, "Weather now: %s, air %.0f °C, track %.0f °C, humidity %.0f%%, wind %.1f m/s.\n", rain, w.AirC, w.TrackC, w.Humidity, w.WindMS)
	}
	if c := d.Circuit; c != nil {
		fmt.Fprintf(&b, "\nCircuit: %.3f km, %d turns, %.0f m of elevation change.", c.LengthKM, c.Turns, c.ClimbM)
		if len(c.Corners) > 0 {
			fmt.Fprintf(&b, " Named corners: %s.", strings.Join(c.Corners, "; "))
		}
		if len(c.Straights) > 0 {
			fmt.Fprintf(&b, " Straights: %s.", strings.Join(c.Straights, ", "))
		}
		b.WriteString("\n")
	}
	if len(d.Grid) > 0 {
		b.WriteString("\nStarting grid:\n")
		for _, g := range d.Grid {
			line := fmt.Sprintf("P%d %s (%s)", g.Position, g.Driver.Code, g.Team)
			if g.Qualified > 0 {
				line += fmt.Sprintf(", qualified P%d", g.Qualified)
			}
			b.WriteString(line + "\n")
		}
	}
	if len(d.Qualifying) > 0 {
		b.WriteString("\nQualifying:\n")
		for _, q := range d.Qualifying[:min(10, len(d.Qualifying))] {
			best, part := q.Q3, "Q3"
			if best == nil {
				best, part = q.Q2, "Q2"
			}
			if best == nil {
				best, part = q.Q1, "Q1"
			}
			if best == nil {
				best, part = q.BestLap, ""
			}
			t := ""
			if best != nil {
				t = " " + lapText(*best) + " " + part
			}
			fmt.Fprintf(&b, "P%d %s%s\n", q.Position, q.Driver.Code, t)
		}
	}
	if len(d.Standings) > 0 {
		fmt.Fprintf(&b, "\nDrivers' championship after round %d:", d.AfterRound)
		for _, s := range d.Standings {
			fmt.Fprintf(&b, " %d. %s %g", s.Position, s.Driver.Code, s.Points)
			if s.Position < len(d.Standings) {
				b.WriteString(",")
			}
		}
		b.WriteString("\nTeams:")
		for _, s := range d.Teams {
			fmt.Fprintf(&b, " %d. %s %g", s.Position, s.Team, s.Points)
		}
		b.WriteString("\n")
	}
	if l := d.LastYear; l != nil {
		fmt.Fprintf(&b, "\nLast year here (%d %s): %d laps.", l.Event.Year, l.Event.Name, l.Laps)
		fmt.Fprintf(&b, " Safety cars: %d, VSCs: %d, red flags: %d.", l.SafetyCars, l.VSCs, l.RedFlags)
		if l.PitLaneS > 0 {
			fmt.Fprintf(&b, " Pit lane time about %.0f s.", l.PitLaneS)
		}
		if len(l.Stops) > 0 {
			var parts []string
			for n := range 6 {
				if c := l.Stops[n]; c > 0 {
					parts = append(parts, fmt.Sprintf("%d stop(s): %d finishers", n, c))
				}
			}
			fmt.Fprintf(&b, " Stops: %s.", strings.Join(parts, ", "))
		}
		b.WriteString("\nTop 10:")
		for _, r := range l.Result {
			fmt.Fprintf(&b, " P%d %s (from P%d),", r.Position, r.Driver.Code, r.Grid)
		}
		b.WriteString("\n")
		if len(l.Strategies) > 0 {
			b.WriteString("Strategies of the top 6:\n")
			for _, s := range l.Strategies {
				b.WriteString("- " + s + "\n")
			}
		}
	}
	b.WriteString("\nYour messages, as a JSON array:")
	return b.String()
}

// parsePreview reads the JSON array of messages from Claude's reply.
func parsePreview(reply string) ([]PreviewMessage, error) {
	start, end := strings.Index(reply, "["), strings.LastIndex(reply, "]")
	if start < 0 || end < start {
		return nil, fmt.Errorf("preview: no JSON array in reply")
	}
	var msgs []PreviewMessage
	if err := json.Unmarshal([]byte(reply[start:end+1]), &msgs); err != nil {
		return nil, fmt.Errorf("preview: %w", err)
	}
	var out []PreviewMessage
	for _, m := range msgs {
		if strings.TrimSpace(m.Text) != "" {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("preview: no messages")
	}
	return out, nil
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, strings.ToLower(s))
}

// Before the start, the preview's messages are spread over this much
// session time, one per PreviewEvery, and at least previewGap of real time
// apart.
const (
	PreviewLead  = 30 * time.Minute
	PreviewEvery = 5 * time.Minute
	previewGap   = 8 * time.Second
)

// Run posts the preview of a race or sprint starting at start to state,
// one message at a time from PreviewLead before the start, until ctx is
// done.
func (p *Previewer) Run(ctx context.Context, state *live.State, year int, session string, start time.Time) {
	e, err := p.svc.EventAt(ctx, year, start)
	if err != nil {
		log.Printf("preview: %v", err)
		return
	}
	msgs, err := p.Messages(ctx, e, session, state.Snapshot().Weather)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("preview: %v", err)
		}
		return
	}
	var last time.Time
	for i, m := range msgs {
		slot := start.Add(-PreviewLead + time.Duration(i)*PreviewEvery)
		for state.Now().Before(slot) || time.Since(last) < previewGap {
			select {
			case <-ctx.Done():
				return
			case <-time.After(250 * time.Millisecond):
			}
		}
		state.AddInsight(live.Insight{Kind: "preview", Topic: m.Topic, Text: m.Text})
		last = time.Now()
	}
}

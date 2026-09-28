package tools

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/gif"
	"math"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/f1"
	"github.com/mariuspot/f1mcp/internal/tracks"
)

// Tools that draw what happened in a session on the track map (from 2023).

type IncidentArgs struct {
	SessionArgs
	Event   string   `json:"event,omitempty" jsonschema:"what to explain: red (red flag, default), sc (safety car), vsc (virtual safety car) or yellow (yellow flag)"`
	FromLap int      `json:"from_lap,omitempty" jsonschema:"the first such event on or after this lap (default: the first of the session)"`
	Drivers []string `json:"drivers,omitempty" jsonschema:"drivers known to be involved (code, car number or name), marked where they were when it happened"`
}

type IncidentResult struct {
	Event   f1.Event `json:"event"`
	Session string   `json:"session"`
	// Banner is what happened, e.g. "RED FLAG · LAP 35", and Caption who
	// and where, e.g. "ALB, NOR stopped · Turn 15".
	Banner  string   `json:"banner"`
	Caption string   `json:"caption,omitempty"`
	Corner  int      `json:"corner,omitempty"` // the turn nearest the incident
	Cars    []string `json:"cars,omitempty"`   // the cars involved, where they stopped or slowed
	Flags   []string `json:"flags,omitempty"`  // the yellow-flag sectors around it
}

type LapAnimationArgs struct {
	SessionArgs
	Drivers []string `json:"drivers,omitempty" jsonschema:"up to 4 drivers (code, car number or name), compared lap against lap; default: the fastest driver"`
	Lap     int      `json:"lap,omitempty" jsonschema:"this lap number for each driver (default: each driver's best lap)"`
	Format  string   `json:"format,omitempty" jsonschema:"png (default): a still of the end of the lap; gif: an animation of the whole lap at 3x speed (larger, a few MB)"`
	Braking bool     `json:"braking,omitempty" jsonschema:"also mark where each car braked (speed before and at the slowest) and changed gear"`
}

type LapSummary struct {
	Driver     string   `json:"driver"` // code, e.g. "VER"
	Number     int      `json:"number"`
	Lap        int      `json:"lap"`
	Seconds    float64  `json:"seconds"`
	GapSeconds *float64 `json:"gap_seconds,omitempty"` // to the first driver
	Compound   string   `json:"compound,omitempty"`
	TyreAge    int      `json:"tyre_age_laps"` // laps the tyre had done before this one
	TopSpeed   float64  `json:"top_speed_kph"`
}

type LapAnimationResult struct {
	Event   f1.Event       `json:"event"`
	Session string         `json:"session"`
	Laps    []LapSummary   `json:"laps"`
	Weather *f1.LapWeather `json:"weather,omitempty"`
}

func registerReplay(s *mcp.Server, svc *f1.Service) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_incident",
		Description: "Explain a red flag, safety car, virtual safety car or yellow flag in a session, from 2023: which cars stopped or slowed and where " +
			"(nearest turn), and the yellow-flag sectors around them, drawn on the track map (PNG) with a close-up of the nearest turn.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a IncidentArgs) (*mcp.CallToolResult, IncidentResult, error) {
		e, err := sessionEvent(ctx, svc, &a.SessionArgs)
		if err != nil {
			return nil, IncidentResult{}, err
		}
		if a.Event == "" {
			a.Event = "red"
		}
		key, err := svc.SessionKey(ctx, e, a.Session)
		if err != nil {
			return nil, IncidentResult{}, err
		}
		involved, err := svc.DriverNumbers(ctx, e, a.Session, a.Drivers)
		if err != nil {
			return nil, IncidentResult{}, err
		}
		inc, err := svc.FindIncident(ctx, key, strings.ToLower(a.Event), a.FromLap, involved)
		if err != nil {
			return nil, IncidentResult{}, err
		}
		t, err := tracks.Load(inc.CircuitID, inc.Year)
		if err != nil {
			return nil, IncidentResult{}, err
		}
		o := tracks.Options{Years: strconv.Itoa(inc.Year), Overlay: &inc.Overlay}
		var content []mcp.Content
		img, err := tracks.Render(t, o)
		if err != nil {
			return nil, IncidentResult{}, err
		}
		if content, err = appendPNG(content, img); err != nil {
			return nil, IncidentResult{}, err
		}
		if inc.Corner > 0 {
			if img, err = tracks.RenderCorner(t, inc.Corner, o); err != nil {
				return nil, IncidentResult{}, err
			}
			if content, err = appendPNG(content, img); err != nil {
				return nil, IncidentResult{}, err
			}
		}
		out := IncidentResult{Event: e, Session: a.Session, Banner: inc.Overlay.Banner, Caption: inc.Overlay.Caption, Corner: inc.Corner}
		for _, m := range inc.Overlay.Markers {
			out.Cars = append(out.Cars, m.Label)
		}
		for _, h := range inc.Overlay.Highlights {
			if !h.Faint && h.Label != "" {
				out.Flags = append(out.Flags, h.Label)
			}
		}
		return &mcp.CallToolResult{Content: content}, out, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_lap_animation",
		Description: "Replay laps on the track map, from 2023: one driver's lap, or several drivers' laps compared as if they started together, " +
			"with speed, gear, throttle, brake, tyre and the gap at each point. A PNG still of the end of the lap by default, or an animated GIF. " +
			"Also returns each lap's time, gap, tyre compound and age, top speed, and the weather.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a LapAnimationArgs) (*mcp.CallToolResult, LapAnimationResult, error) {
		if len(a.Drivers) > 4 {
			return nil, LapAnimationResult{}, fmt.Errorf("at most 4 drivers, not %d", len(a.Drivers))
		}
		format := strings.ToLower(a.Format)
		if format != "" && format != "png" && format != "gif" {
			return nil, LapAnimationResult{}, fmt.Errorf("format %q: want png or gif", a.Format)
		}
		e, err := sessionEvent(ctx, svc, &a.SessionArgs)
		if err != nil {
			return nil, LapAnimationResult{}, err
		}
		key, err := svc.SessionKey(ctx, e, a.Session)
		if err != nil {
			return nil, LapAnimationResult{}, err
		}
		r, err := svc.ReplayLaps(ctx, key, a.Drivers, a.Lap, 1)
		if err != nil {
			return nil, LapAnimationResult{}, err
		}
		t, err := tracks.Load(r.CircuitID, r.Year)
		if err != nil {
			return nil, LapAnimationResult{}, err
		}
		out := LapAnimationResult{Event: e, Session: a.Session, Weather: r.Weather}
		var who []string
		for i, l := range r.Laps {
			ls := LapSummary{Driver: l.Driver, Number: l.Number, Lap: l.Lap, Seconds: l.Duration, Compound: l.Compound, TyreAge: l.TyreAge}
			if i > 0 {
				gap := math.Round((l.Duration-r.Laps[0].Duration)*1000) / 1000
				ls.GapSeconds = &gap
			}
			for _, tel := range l.Telemetry {
				ls.TopSpeed = max(ls.TopSpeed, tel[1])
			}
			out.Laps = append(out.Laps, ls)
			who = append(who, fmt.Sprintf("%s %s", l.Driver, lapTime(l.Duration)))
		}
		o := tracks.Options{Years: strconv.Itoa(r.Year), LapEvents: a.Braking,
			Overlay: &tracks.Overlay{Banner: r.Session + " · " + strings.Join(who, " vs "), BannerColor: "#2C2C3A"}}
		if r.Weather != nil {
			o.Overlay.Caption = r.Weather.String()
		}
		var content []mcp.Content
		if format == "gif" {
			g, err := tracks.RenderLapGIF(t, r.Laps, o)
			if err != nil {
				return nil, LapAnimationResult{}, err
			}
			var b bytes.Buffer
			if err := gif.EncodeAll(&b, g); err != nil {
				return nil, LapAnimationResult{}, err
			}
			content = append(content, &mcp.ImageContent{Data: b.Bytes(), MIMEType: "image/gif"})
		} else {
			img, err := tracks.RenderLapStill(t, r.Laps, o)
			if err != nil {
				return nil, LapAnimationResult{}, err
			}
			if content, err = appendPNG(content, img); err != nil {
				return nil, LapAnimationResult{}, err
			}
		}
		return &mcp.CallToolResult{Content: content}, out, nil
	})
}

func appendPNG(content []mcp.Content, img image.Image) ([]mcp.Content, error) {
	b, err := pngBytes(img)
	if err != nil {
		return content, err
	}
	return append(content, &mcp.ImageContent{Data: b, MIMEType: "image/png"}), nil
}

// lapTime formats seconds as a lap time, e.g. "1:42.526".
func lapTime(s float64) string {
	m := int(s) / 60
	return fmt.Sprintf("%d:%06.3f", m, s-float64(m*60))
}

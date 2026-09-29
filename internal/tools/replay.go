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
	LapSessionArgs
	Drivers []string `json:"drivers,omitempty" jsonschema:"up to 4 drivers (code, car number or name), compared lap against lap; default: the fastest driver, or the fastest two for format faster"`
	Lap     string   `json:"lap,omitempty" jsonschema:"which lap of each driver: a lap number such as '17', or best (default), first, last (the last timed lap that isn't an out-lap) or last_flying (the last push lap, not an in-lap or cool-down lap)"`
	// Laps gives each driver their own lap.
	Laps    []LapChoice `json:"laps,omitempty" jsonschema:"instead of drivers and lap: up to 4 drivers, each with their own lap, e.g. [{driver: VER, lap: last}, {driver: NOR, lap: '16'}]"`
	Format  string      `json:"format,omitempty" jsonschema:"png (default): a still of the end of the lap; gif: an animation of the whole lap at 3x speed (larger, a few MB); faster: a map of who was faster through each corner and straight, with the gap all round the lap (2 to 4 drivers)"`
	Braking bool        `json:"braking,omitempty" jsonschema:"also mark where each car braked (speed before and at the slowest) and changed gear"`
}

type LapChoice struct {
	Driver string `json:"driver" jsonschema:"driver code (VER), car number or name"`
	Lap    string `json:"lap,omitempty" jsonschema:"a lap number such as '17', or best (default), first, last or last_flying"`
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
	// With format faster: how many stretches this driver was fastest in,
	// and where they gained most on the next fastest.
	StretchesWon *int         `json:"stretches_won,omitempty"`
	BiggestGain  *StretchGain `json:"biggest_gain,omitempty"`
}

type StretchGain struct {
	Stretch string  `json:"stretch"`
	Seconds float64 `json:"seconds"`
}

// StretchSummary is who was fastest through one corner or straight.
type StretchSummary struct {
	Name    string    `json:"name"` // e.g. "Turn 9, Copse", "Hangar Straight"
	Corner  bool      `json:"corner"`
	FromM   float64   `json:"from_m"`
	ToM     float64   `json:"to_m"` // before from_m for the stretch across the line
	Fastest string    `json:"fastest"`
	Margin  float64   `json:"margin_seconds"` // to the next fastest
	Seconds []float64 `json:"seconds"`        // each driver's time, in the order of laps
}

type LapAnimationResult struct {
	Event     f1.Event         `json:"event"`
	Session   string           `json:"session"`
	Laps      []LapSummary     `json:"laps"`
	Stretches []StretchSummary `json:"stretches,omitempty"`
	Weather   *f1.LapWeather   `json:"weather,omitempty"`
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
			"from a session or a part of qualifying (q1, q2, q3); each lap by number, or best, first, last or last_flying, per driver with laps. " +
			"with speed, gear, throttle, brake, tyre and the gap at each point. A PNG still of the end of the lap by default, or an animated GIF. " +
			"With format faster, a map of which driver was faster through each corner and straight, with the gap all round the lap, and those " +
			"times. Also returns each lap's time, gap, tyre compound and age, top speed, and the weather.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a LapAnimationArgs) (*mcp.CallToolResult, LapAnimationResult, error) {
		var picks []f1.LapPick
		switch {
		case len(a.Laps) > 0 && len(a.Drivers) > 0:
			return nil, LapAnimationResult{}, fmt.Errorf("give drivers (with lap) or laps, not both")
		case len(a.Laps) > 0:
			for _, l := range a.Laps {
				picks = append(picks, f1.LapPick{Driver: l.Driver, Lap: l.Lap})
			}
		default:
			for _, d := range a.Drivers {
				picks = append(picks, f1.LapPick{Driver: d, Lap: a.Lap})
			}
			if len(picks) == 0 && a.Lap != "" && !strings.EqualFold(a.Lap, f1.PickBest) {
				return nil, LapAnimationResult{}, fmt.Errorf("lap %q needs drivers", a.Lap)
			}
		}
		if len(picks) > 4 {
			return nil, LapAnimationResult{}, fmt.Errorf("at most 4 drivers, not %d", len(picks))
		}
		format := strings.ToLower(a.Format)
		if format != "" && format != "png" && format != "gif" && format != "faster" {
			return nil, LapAnimationResult{}, fmt.Errorf("format %q: want png, gif or faster", a.Format)
		}
		count := 1
		if format == "faster" {
			if len(picks) == 1 {
				return nil, LapAnimationResult{}, fmt.Errorf("format faster compares 2 to 4 laps")
			}
			count = 2
		}
		e, err := lapSessionEvent(ctx, svc, &a.LapSessionArgs)
		if err != nil {
			return nil, LapAnimationResult{}, err
		}
		if err := f1.ValidLapSession(a.Session); err != nil {
			return nil, LapAnimationResult{}, err
		}
		base, part := f1.SplitSession(a.Session)
		key, err := svc.SessionKey(ctx, e, base)
		if err != nil {
			return nil, LapAnimationResult{}, err
		}
		r, err := svc.ReplayLaps(ctx, key, part, picks, count)
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
		switch format {
		case "gif":
			g, err := tracks.RenderLapGIF(t, r.Laps, o)
			if err != nil {
				return nil, LapAnimationResult{}, err
			}
			var b bytes.Buffer
			if err := gif.EncodeAll(&b, g); err != nil {
				return nil, LapAnimationResult{}, err
			}
			content = append(content, &mcp.ImageContent{Data: b.Bytes(), MIMEType: "image/gif"})
		case "faster":
			o.LapEvents = false
			img, cmp, err := tracks.RenderFaster(t, r.Laps, o)
			if err != nil {
				return nil, LapAnimationResult{}, err
			}
			if content, err = appendPNGWith(content, img, cmp.FasterColors()); err != nil {
				return nil, LapAnimationResult{}, err
			}
			summariseStretches(&out, cmp)
		default:
			img, err := tracks.RenderLapStill(t, r.Laps, o)
			if err != nil {
				return nil, LapAnimationResult{}, err
			}
			if content, err = appendPNGWith(content, img, tracks.LapStillColors(r.Laps)); err != nil {
				return nil, LapAnimationResult{}, err
			}
		}
		return &mcp.CallToolResult{Content: content}, out, nil
	})
}

// summariseStretches adds who was fastest where to a lap comparison.
func summariseStretches(out *LapAnimationResult, cmp tracks.LapComparison) {
	won := make([]int, len(out.Laps))
	gains := make([]*StretchGain, len(out.Laps))
	for _, r := range cmp.Stretches {
		won[r.Fastest]++
		out.Stretches = append(out.Stretches, StretchSummary{
			Name: r.Name, Corner: r.Corner, FromM: math.Round(r.FromM), ToM: math.Round(r.ToM),
			Fastest: out.Laps[r.Fastest].Driver, Margin: r.Margin, Seconds: r.Seconds,
		})
		if g := gains[r.Fastest]; g == nil || r.Margin > g.Seconds {
			gains[r.Fastest] = &StretchGain{Stretch: r.Name, Seconds: r.Margin}
		}
	}
	for i := range out.Laps {
		out.Laps[i].StretchesWon = &won[i]
		out.Laps[i].BiggestGain = gains[i]
	}
}

func appendPNGWith(content []mcp.Content, img image.Image, colors []string) ([]mcp.Content, error) {
	var b bytes.Buffer
	if err := tracks.EncodePNGWith(&b, img, colors); err != nil {
		return content, err
	}
	return append(content, &mcp.ImageContent{Data: b.Bytes(), MIMEType: "image/png"}), nil
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

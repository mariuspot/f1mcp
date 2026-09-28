// Command incidentdemo is a spike: it finds the first red flag in a session,
// works out where the incident was from OpenF1 flags and car positions, and
// draws it on the track map and the nearest corner.
//
//	go run ./cmd/incidentdemo -session 9523 -circuit monaco -year 2024
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tracks"
)

func main() {
	session := flag.Int("session", 9523, "OpenF1 session key")
	circuit := flag.String("circuit", "monaco", "circuit ID")
	year := flag.Int("year", 2024, "season")
	out := flag.String("out", "assets/incidents", "output directory")
	flag.Parse()
	log.SetFlags(0)
	if err := run(context.Background(), *session, *circuit, *year, *out); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, session int, circuit string, year int, out string) error {
	t, err := tracks.Load(circuit, year)
	if err != nil {
		return err
	}
	of1 := openf1.NewClient("", nil)
	key := strconv.Itoa(session)

	// The first red flag, and the sector flags waved just before it.
	msgs, err := of1.RaceControl(ctx, openf1.SessionFilter{SessionKey: key})
	if err != nil {
		return err
	}
	var red *openf1.RaceControl
	for i, m := range msgs {
		if m.Flag != nil && *m.Flag == "RED" {
			red = &msgs[i]
			break
		}
	}
	if red == nil {
		return fmt.Errorf("no red flag in session %d", session)
	}
	lap := 0
	if red.LapNumber != nil {
		lap = *red.LapNumber
	}
	log.Printf("red flag at %s, lap %d", red.Date.Format(time.TimeOnly), lap)

	sectors := map[int]string{} // marshal sector -> worst flag in the minute before
	for _, m := range msgs {
		if m.Date.Before(red.Date.Add(-time.Minute)) || m.Date.After(red.Date) || m.Sector == nil || m.Flag == nil {
			continue
		}
		switch *m.Flag {
		case "DOUBLE YELLOW":
			sectors[*m.Sector] = "DOUBLE YELLOW"
		case "YELLOW":
			if sectors[*m.Sector] == "" {
				sectors[*m.Sector] = "YELLOW"
			}
		}
	}
	marshal, err := marshalSectors(ctx, t.CircuitKey, year)
	if err != nil {
		return err
	}
	o := &tracks.Overlay{Banner: fmt.Sprintf("RED FLAG · Lap %d · %s UTC", lap, red.Date.UTC().Format("15:04"))}
	for n, f := range sectors {
		from, ok1 := marshal[n]
		to, ok2 := marshal[n+1]
		if !ok2 {
			to, ok2 = marshal[1]
		}
		if !ok1 || !ok2 {
			continue
		}
		color, label := "#FFD100", fmt.Sprintf("Yellow · sector %d", n)
		if f == "DOUBLE YELLOW" {
			color, label = "#FF9500", fmt.Sprintf("Double yellow · sector %d", n)
		}
		o.Highlights = append(o.Highlights, tracks.Highlight{From: from, To: to, Color: color, Label: label})
		log.Printf("%s in marshal sector %d", f, n)
	}

	// Cars that stopped in the half minute before the red flag.
	drivers, err := of1.Drivers(ctx, openf1.SessionDriverFilter{SessionKey: key})
	if err != nil {
		return err
	}
	var crash []tracks.Point
	for _, d := range drivers {
		locs, err := of1.Locations(ctx, openf1.WindowFilter{
			SessionKey: key, DriverNumber: d.DriverNumber,
			After: red.Date.Add(-28 * time.Second), Before: red.Date.Add(-3 * time.Second),
		})
		if err != nil {
			return err
		}
		if len(locs) < 2 {
			continue
		}
		a, b := locs[0], locs[len(locs)-1]
		if math.Hypot(float64(b.X-a.X), float64(b.Y-a.Y))/10 > 30 {
			continue
		}
		pt := tracks.Point{float64(b.X), float64(b.Y)}
		crash = append(crash, pt)
		o.Markers = append(o.Markers, tracks.Marker{Position: pt, Label: d.NameAcronym, Color: d.TeamColour})
		log.Printf("%s (%d) stopped", d.NameAcronym, d.DriverNumber)
	}

	opts := tracks.Options{Years: strconv.Itoa(year), Overlay: o}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("%s-%d-red-flag", circuit, year)
	img, err := tracks.Render(t, opts)
	if err != nil {
		return err
	}
	if err := writePNG(filepath.Join(out, name+"-map.png"), img); err != nil {
		return err
	}
	if len(crash) > 0 {
		c := nearestCorner(t, crash)
		img, err := tracks.RenderCorner(t, c.Number, opts)
		if err != nil {
			return err
		}
		if err := writePNG(filepath.Join(out, fmt.Sprintf("%s-turn-%02d.png", name, c.Number)), img); err != nil {
			return err
		}
		log.Printf("nearest corner: turn %d %s", c.Number, c.Name)
	}
	log.Printf("wrote %s", out)
	return nil
}

// marshalSectors returns where each marshal sector starts, from MultiViewer.
func marshalSectors(ctx context.Context, circuitKey, year int) (map[int]tracks.Point, error) {
	u := fmt.Sprintf("https://api.multiviewer.app/api/v1/circuits/%d/%d", circuitKey, year)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "f1mcp-incidentdemo")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var mv struct {
		MarshalSectors []struct {
			Number        int `json:"number"`
			TrackPosition struct {
				X, Y float64
			} `json:"trackPosition"`
		} `json:"marshalSectors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&mv); err != nil {
		return nil, err
	}
	out := map[int]tracks.Point{}
	for _, m := range mv.MarshalSectors {
		out[m.Number] = tracks.Point{m.TrackPosition.X, m.TrackPosition.Y}
	}
	return out, nil
}

func nearestCorner(t *tracks.Track, pts []tracks.Point) tracks.Corner {
	var cx, cy float64
	for _, p := range pts {
		cx += p[0] / float64(len(pts))
		cy += p[1] / float64(len(pts))
	}
	best, bestD := t.Corners[0], math.Inf(1)
	for _, c := range t.Corners {
		if d := math.Hypot(c.Position[0]-cx, c.Position[1]-cy); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func writePNG(file string, img image.Image) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	if err := tracks.EncodePNG(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

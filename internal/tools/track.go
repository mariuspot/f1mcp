package tools

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/f1"
	"github.com/mariuspot/f1mcp/internal/tracks"
)

type TrackArgs struct {
	Circuit string `json:"circuit,omitempty" jsonschema:"circuit ID (e.g. 'spa', from get_schedule) or name, city or country; or give year and round instead"`
	Year    int    `json:"year,omitempty" jsonschema:"season; picks that year's layout (default: the latest)"`
	Round   string `json:"round,omitempty" jsonschema:"the circuit of this round of the year (number, 'last', 'next' or event name)"`
}

type TrackMapArgs struct {
	TrackArgs
	Corner int `json:"corner,omitempty" jsonschema:"draw a close-up of this turn instead of the whole track"`
}

type CornerInfo struct {
	Number     int      `json:"number"`
	Name       string   `json:"name,omitempty"`
	AltName    string   `json:"alt_name,omitempty"`
	DistanceM  float64  `json:"distance_m"` // from the start/finish line
	ElevationM *float64 `json:"elevation_m,omitempty"`
}

type StraightInfo struct {
	Name string `json:"name"`
	From int    `json:"from_turn"`
	To   int    `json:"to_turn"`
}

type TrackInfo struct {
	CircuitID        string         `json:"circuit_id"`
	Name             string         `json:"name"`
	Locality         string         `json:"locality"`
	Country          string         `json:"country"`
	Layout           string         `json:"layout"` // seasons this layout was used, e.g. "2023–2026"
	LengthKM         float64        `json:"length_km"`
	Turns            int            `json:"turns"`
	Corners          []CornerInfo   `json:"corners"`
	Straights        []StraightInfo `json:"straights,omitempty"`
	SectorStartsM    []float64      `json:"sector_starts_m,omitempty"` // where sectors 2 and 3 start
	MarshalSectors   int            `json:"marshal_sectors"`
	ElevationChangeM float64        `json:"elevation_change_m,omitempty"`
	Resource         string         `json:"resource"` // track:// URI of the map
}

// resolveTrack finds the track layout a tool is asked about: by circuit,
// or by the circuit of a year's round.
func resolveTrack(ctx context.Context, svc *f1.Service, a TrackArgs) (*tracks.Track, tracks.Layout, error) {
	id, year := "", a.Year
	if a.Circuit == "" {
		e, err := svc.ResolveEvent(ctx, a.Year, a.Round)
		if err != nil {
			return nil, tracks.Layout{}, err
		}
		id, year = e.Circuit.ID, e.Year
	} else {
		var err error
		if id, err = findCircuit(a.Circuit); err != nil {
			return nil, tracks.Layout{}, err
		}
	}
	layouts, err := tracks.Layouts(id)
	if err != nil {
		return nil, tracks.Layout{}, err
	}
	l := layouts[len(layouts)-1]
	if year != 0 {
		// The layout used that year, or the nearest one.
		for _, c := range layouts {
			if c.From <= year {
				l = c
			}
		}
		if year < layouts[0].From {
			l = layouts[0]
		}
	}
	t, err := tracks.Load(id, l.To)
	return t, l, err
}

// findCircuit matches a circuit ID or part of a circuit's name, city or
// country.
func findCircuit(q string) (string, error) {
	ids, err := tracks.Circuits()
	if err != nil {
		return "", err
	}
	q = strings.ToLower(strings.TrimSpace(q))
	if slices.Contains(ids, q) {
		return q, nil
	}
	var matches []string
	for _, id := range ids {
		years, _ := tracks.Years(id)
		t, err := tracks.Load(id, years[len(years)-1])
		if err != nil {
			continue
		}
		for _, f := range []string{t.Name, t.Locality, t.Country, id} {
			if strings.Contains(strings.ToLower(f), q) {
				matches = append(matches, id)
				break
			}
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("no track data for %q; tracks are available for circuits raced from 2023: %s", q, strings.Join(ids, ", "))
	default:
		return "", fmt.Errorf("%q matches several circuits: %s", q, strings.Join(matches, ", "))
	}
}

func trackURI(t *tracks.Track) string {
	return fmt.Sprintf("track://%s/%d", t.CircuitID, t.Year)
}

func trackInfo(t *tracks.Track, l tracks.Layout) TrackInfo {
	info := TrackInfo{
		CircuitID: t.CircuitID, Name: t.Name, Locality: t.Locality, Country: t.Country,
		Layout: l.Years(), LengthKM: math.Round(t.LengthKM()*1000) / 1000, Turns: len(t.Corners),
		MarshalSectors: len(t.MarshalSectors), ElevationChangeM: t.ElevationChange(), Resource: trackURI(t),
	}
	for _, c := range t.Corners {
		loc := t.Locate(c.Position)
		info.Corners = append(info.Corners, CornerInfo{
			Number: c.Number, Name: c.Name, AltName: c.AltName,
			DistanceM: math.Round(loc.Distance), ElevationM: loc.ElevationM,
		})
	}
	for _, s := range t.Straights {
		info.Straights = append(info.Straights, StraightInfo{Name: s.Name, From: s.From, To: s.To})
	}
	for _, i := range t.SectorStarts {
		info.SectorStartsM = append(info.SectorStartsM, math.Round(t.Locate(t.Outline[i]).Distance))
	}
	return info
}

func pngBytes(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	err := tracks.EncodePNG(&b, img)
	return b.Bytes(), err
}

func registerTrack(s *mcp.Server, svc *f1.Service) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_track",
		Description: "Get a circuit's layout (circuits raced from 2023): length, every turn with its name, distance from the line and " +
			"elevation, named straights, where sectors start, marshal sectors and total climb. Find it by circuit or by year and round.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a TrackArgs) (*mcp.CallToolResult, TrackInfo, error) {
		t, l, err := resolveTrack(ctx, svc, a)
		if err != nil {
			return nil, TrackInfo{}, err
		}
		return nil, trackInfo(t, l), nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_track_map",
		Description: "Draw a circuit map as a PNG image (circuits raced from 2023): the track coloured by timing sector, kerbs, " +
			"start/finish, numbered and named turns and an elevation profile; or, with corner, a close-up of that turn. " +
			"Also returns the track's data as in get_track.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a TrackMapArgs) (*mcp.CallToolResult, TrackInfo, error) {
		t, l, err := resolveTrack(ctx, svc, a.TrackArgs)
		if err != nil {
			return nil, TrackInfo{}, err
		}
		o := tracks.Options{Years: l.Years()}
		var img image.Image
		if a.Corner > 0 {
			img, err = tracks.RenderCorner(t, a.Corner, o)
		} else {
			img, err = tracks.Render(t, o)
		}
		if err != nil {
			return nil, TrackInfo{}, err
		}
		b, err := pngBytes(img)
		if err != nil {
			return nil, TrackInfo{}, err
		}
		info := trackInfo(t, l)
		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.ImageContent{Data: b, MIMEType: "image/png"},
			&mcp.ResourceLink{URI: info.Resource, Name: t.Name, MIMEType: "image/png"},
		}}, info, nil
	})

	// One resource per circuit layout: its map as a PNG.
	ids, _ := tracks.Circuits()
	for _, id := range ids {
		layouts, _ := tracks.Layouts(id)
		for _, l := range layouts {
			t, err := tracks.Load(id, l.To)
			if err != nil {
				continue
			}
			s.AddResource(&mcp.Resource{
				URI: trackURI(t), Name: id + "-" + strconv.Itoa(l.To), Title: fmt.Sprintf("%s map (%s)", t.Name, l.Years()),
				Description: fmt.Sprintf("Map of %s, %s, as raced %s", t.Name, t.Country, l.Years()), MIMEType: "image/png",
			}, trackResource)
		}
	}
	s.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "track://{circuit}/{year}", Name: "track-map", Title: "Circuit map",
		Description: "Map of a circuit as raced in a year, as a PNG", MIMEType: "image/png",
	}, trackResource)
}

// trackResource serves track://{circuit}/{year} as a PNG map.
func trackResource(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	rest, ok := strings.CutPrefix(uri, "track://")
	id, yearStr, ok2 := strings.Cut(rest, "/")
	year, err := strconv.Atoi(yearStr)
	if !ok || !ok2 || err != nil {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	t, err := tracks.Load(id, year)
	if err != nil {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	years := strconv.Itoa(year)
	if layouts, err := tracks.Layouts(id); err == nil {
		for _, l := range layouts {
			if l.From <= t.Year && t.Year <= l.To {
				years = l.Years()
			}
		}
	}
	img, err := tracks.Render(t, tracks.Options{Years: years})
	if err != nil {
		return nil, err
	}
	b, err := pngBytes(img)
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "image/png", Blob: b}}}, nil
}

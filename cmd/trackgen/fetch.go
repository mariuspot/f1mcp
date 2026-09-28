package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/mariuspot/f1mcp/internal/jolpica"
	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tracks"
)

const multiviewerURL = "https://api.multiviewer.app/api/v1/circuits"

// raceMatchWindow is how far apart the OpenF1 and Jolpica race start times
// may be and still count as the same race.
const raceMatchWindow = 6 * time.Hour

type fetchOptions struct {
	from, to     int
	dir          string          // track files
	linesFile    string          // stored timing lines
	elevationDir string          // stored elevation laps
	force        bool            // re-fetch everything
	only         map[string]bool // re-fetch only these circuit IDs
}

func (o fetchOptions) refetch(circuitID string) bool {
	return o.force || o.only[circuitID]
}

// fetch downloads layouts that aren't stored yet, finds timing lines for
// circuits that don't have them, then normalizes every stored track.
func fetch(ctx context.Context, o fetchOptions) error {
	if err := os.MkdirAll(o.dir, 0o755); err != nil {
		return err
	}
	of1 := openf1.NewClient("", nil)
	jol := jolpica.NewClient("", nil)
	httpClient := &http.Client{Timeout: 30 * time.Second}

	races, err := matchRaces(ctx, of1, jol, o.from, o.to)
	if err != nil {
		return err
	}

	for _, r := range races {
		id := r.race.Circuit.CircuitID
		file := filepath.Join(o.dir, tracks.FileName(id, r.year))
		stored, err := readTrack(file)
		if err == nil && !o.refetch(id) && len(stored.MarshalSectors) > 0 {
			continue
		}
		mv, err := fetchMultiviewer(ctx, httpClient, r.session.CircuitKey, r.year)
		if err != nil {
			log.Printf("%d %s: %v, skipping", r.year, id, err)
			continue
		}
		time.Sleep(500 * time.Millisecond) // be polite to MultiViewer
		if stored != nil && !o.refetch(id) {
			// Stored before marshal sectors were kept: add them only.
			stored.MarshalSectors = marshalSectors(mv)
			if err := writeTrack(file, stored); err != nil {
				return err
			}
			log.Printf("%d %-15s added %d marshal sectors", r.year, id, len(stored.MarshalSectors))
			continue
		}
		t := toTrack(r.race, r.session.CircuitKey, r.year, mv)
		if err := writeTrack(file, &t); err != nil {
			return err
		}
		log.Printf("%d %-15s fetched: %4d points %2d corners", r.year, id, len(t.Outline), len(t.Corners))
	}

	lines, err := loadLines(o.linesFile)
	if err != nil {
		return err
	}
	if err := findMissingLines(ctx, of1, o, races, lines); err != nil {
		return err
	}
	if err := writeJSON(o.linesFile, lines); err != nil {
		return err
	}
	if err := findMissingElevation(ctx, of1, o, lines); err != nil {
		return err
	}

	files, err := filepath.Glob(filepath.Join(o.dir, "*.json"))
	if err != nil {
		return err
	}
	for _, file := range files {
		t, err := readTrack(file)
		if err != nil {
			return err
		}
		var l *Lines
		if found, ok := lines[t.CircuitID]; ok {
			l = &found
		}
		e, err := loadElevation(o.elevationDir, t.CircuitID)
		if err != nil {
			return err
		}
		normalize(t, l)
		applyElevation(t, e)
		if err := writeTrack(file, t); err != nil {
			return err
		}
	}
	log.Printf("normalized %d tracks", len(files))
	return nil
}

type matchedRace struct {
	year    int
	session openf1.Session
	race    jolpica.Race
}

// matchRaces pairs each OpenF1 race session with its Jolpica race by start
// time, falling back to the circuit ID seen in other years when start times
// disagree (Jolpica has Las Vegas 2024 a day early).
func matchRaces(ctx context.Context, of1 *openf1.Client, jol *jolpica.Client, from, to int) ([]matchedRace, error) {
	type pending struct {
		year    int
		session openf1.Session
		races   []jolpica.Race
		race    *jolpica.Race
	}
	var all []*pending
	circuitIDs := map[int]string{}
	for year := from; year <= to; year++ {
		races, err := jol.Schedule(ctx, strconv.Itoa(year))
		if err != nil {
			return nil, fmt.Errorf("jolpica schedule %d: %w", year, err)
		}
		sessions, err := of1.Sessions(ctx, openf1.SessionsFilter{Year: year, SessionName: "Race"})
		if err != nil {
			return nil, fmt.Errorf("openf1 sessions %d: %w", year, err)
		}
		for _, s := range sessions {
			if s.IsCancelled {
				continue
			}
			p := &pending{year: year, session: s, races: races}
			if r, ok := matchRace(races, s.DateStart); ok {
				p.race = &r
				circuitIDs[s.CircuitKey] = r.Circuit.CircuitID
			}
			all = append(all, p)
		}
	}

	var out []matchedRace
	for _, p := range all {
		s := p.session
		if p.race == nil {
			i := slices.IndexFunc(p.races, func(r jolpica.Race) bool {
				return circuitIDs[s.CircuitKey] != "" && r.Circuit.CircuitID == circuitIDs[s.CircuitKey]
			})
			if i < 0 {
				log.Printf("%d %s: no matching Jolpica race, skipping", p.year, s.CircuitShortName)
				continue
			}
			p.race = &p.races[i]
			log.Printf("%d %s: matched by circuit, start times differ (OpenF1 %s, Jolpica %s %s)",
				p.year, p.race.Circuit.CircuitID, s.DateStart.Format(time.RFC3339), p.race.Date, p.race.Time)
		}
		out = append(out, matchedRace{p.year, s, *p.race})
	}
	return out, nil
}

// findMissingLines measures timing lines for circuits that have none (or are
// being re-fetched), trying each circuit's most recent race first and older
// ones if its timing data doesn't fit the track.
func findMissingLines(ctx context.Context, of1 *openf1.Client, o fetchOptions, races []matchedRace, lines map[string]Lines) error {
	done := map[string]bool{}
	for _, r := range slices.Backward(races) {
		id := r.race.Circuit.CircuitID
		if done[id] || r.session.DateStart.After(time.Now()) {
			continue
		}
		if _, ok := lines[id]; ok && !o.refetch(id) {
			continue
		}
		t, err := readTrack(filepath.Join(o.dir, tracks.FileName(id, r.year)))
		if err != nil {
			continue // layout not available for this year
		}
		orientOutline(t)
		l, err := findLines(ctx, of1, r.session.SessionKey, func(l Lines) bool { return plausibleLines(t, l) })
		if err != nil {
			log.Printf("%d %s: timing lines: %v, trying an older race", r.year, id, err)
			continue
		}
		l.Year = r.year
		lines[id] = l
		done[id] = true
		log.Printf("%d %-15s timing lines found", r.year, id)
	}
	return nil
}

func matchRace(races []jolpica.Race, start time.Time) (jolpica.Race, bool) {
	for _, r := range races {
		t, err := time.Parse(time.RFC3339, r.Date+"T"+r.Time)
		if err != nil {
			// Some races have a date but no time yet.
			t, err = time.Parse(time.DateOnly, r.Date)
			if err != nil {
				continue
			}
		}
		if d := t.Sub(start); d.Abs() <= raceMatchWindow {
			return r, true
		}
	}
	return jolpica.Race{}, false
}

type multiviewerCircuit struct {
	Rotation float64   `json:"rotation"`
	X        []float64 `json:"x"`
	Y        []float64 `json:"y"`
	Corners  []struct {
		Number        int     `json:"number"`
		Angle         float64 `json:"angle"`
		TrackPosition struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"trackPosition"`
	} `json:"corners"`
	MarshalSectors []struct {
		Number        int `json:"number"`
		TrackPosition struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"trackPosition"`
	} `json:"marshalSectors"`
}

func marshalSectors(mv *multiviewerCircuit) []tracks.MarshalSector {
	var out []tracks.MarshalSector
	for _, m := range mv.MarshalSectors {
		out = append(out, tracks.MarshalSector{
			Number:   m.Number,
			Position: tracks.Point{round(m.TrackPosition.X, 1), round(m.TrackPosition.Y, 1)},
		})
	}
	return out
}

func fetchMultiviewer(ctx context.Context, c *http.Client, circuitKey, year int) (*multiviewerCircuit, error) {
	u := fmt.Sprintf("%s/%d/%d", multiviewerURL, circuitKey, year)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "f1mcp-trackgen (+https://github.com/mariuspot/f1mcp)")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("multiviewer %s: %s", u, resp.Status)
	}
	var mv multiviewerCircuit
	if err := json.NewDecoder(resp.Body).Decode(&mv); err != nil {
		return nil, fmt.Errorf("multiviewer %s: %w", u, err)
	}
	if len(mv.X) == 0 || len(mv.X) != len(mv.Y) {
		return nil, fmt.Errorf("multiviewer %s: no outline", u)
	}
	return &mv, nil
}

func toTrack(r jolpica.Race, circuitKey, year int, mv *multiviewerCircuit) tracks.Track {
	t := tracks.Track{
		CircuitID:  r.Circuit.CircuitID,
		CircuitKey: circuitKey,
		Name:       r.Circuit.CircuitName,
		Locality:   r.Circuit.Location.Locality,
		Country:    r.Circuit.Location.Country,
		Year:       year,
		Rotation:   mv.Rotation,
	}
	for i := range mv.X {
		t.Outline = append(t.Outline, tracks.Point{round(mv.X[i], 0), round(mv.Y[i], 0)})
	}
	for _, c := range mv.Corners {
		t.Corners = append(t.Corners, tracks.Corner{
			Number:   c.Number,
			Position: tracks.Point{round(c.TrackPosition.X, 1), round(c.TrackPosition.Y, 1)},
			Angle:    round(c.Angle, 2),
		})
	}
	t.MarshalSectors = marshalSectors(mv)
	return t
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// pointRE matches an indented [x, y] pair so it can be put on one line.
var pointRE = regexp.MustCompile(`\[\s+(-?[0-9.]+),\s+(-?[0-9.]+)\s+\]`)

func readTrack(file string) (*tracks.Track, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var t tracks.Track
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return &t, nil
}

func writeTrack(file string, t *tracks.Track) error {
	return writeJSON(file, t)
}

// numberListRE matches an indented list of plain numbers (elevation
// profiles, samples) so it can be put on one line.
var (
	numberListRE = regexp.MustCompile(`\[\s+-?[0-9.]+(,\s+-?[0-9.]+)*\s+\]`)
	spaceRE      = regexp.MustCompile(`\s+`)
)

func writeJSON(file string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = pointRE.ReplaceAll(b, []byte("[$1, $2]"))
	b = numberListRE.ReplaceAllFunc(b, func(m []byte) []byte {
		return spaceRE.ReplaceAll(m, []byte(" "))
	})
	return os.WriteFile(file, append(b, '\n'), 0o644)
}

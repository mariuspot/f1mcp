package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/mariuspot/f1mcp/internal/live"
)

// Player runs one replay at a time into a live state, for everyone
// watching: the live page shows a replay the same way it will show a live
// session.
type Player struct {
	root string // where collected sessions are, one folder each

	mu     sync.Mutex
	state  *live.State
	status Status
	cancel context.CancelFunc
	// clock maps session time to wall time while running, to report where
	// the replay is between records.
	started  time.Time // wall time the paced part began
	fromTime time.Time // session time it began at
}

// Status is what the player is doing.
type Status struct {
	Race    string    `json:"race,omitempty"` // folder name, e.g. 2024-brazil-race
	Title   string    `json:"title,omitempty"`
	Speed   float64   `json:"speed,omitempty"`
	Running bool      `json:"running"`
	Time    time.Time `json:"time,omitempty"` // session time now
	Error   string    `json:"error,omitempty"`
	// Run counts replays started; events are numbered from 1 in each.
	Run int `json:"run"`
}

// Race is a collected session that can be replayed.
type Race struct {
	ID    string `json:"id"`
	Title string `json:"title"` // e.g. "2024 Brazil Race"
	Laps  int    `json:"laps"`
}

// NewPlayer returns a player for the sessions collected under root.
func NewPlayer(root string) *Player {
	return &Player{root: root, state: live.NewState()}
}

// Races lists the collected sessions.
func (p *Player) Races() ([]Race, error) {
	if p.root == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(p.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Race
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(p.root, e.Name(), "manifest.json"))
		if err != nil {
			continue
		}
		var m Manifest
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		out = append(out, Race{ID: e.Name(), Title: fmt.Sprintf("%d %s %s", m.Year, m.Country, m.Session), Laps: lastLap(filepath.Join(p.root, e.Name()))})
	}
	slices.SortFunc(out, func(a, b Race) int { return compareStrings(b.ID, a.ID) })
	return out, nil
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// State returns the live state of the current replay.
func (p *Player) State() *live.State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// Status reports what the player is doing.
func (p *Player) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.status
	if st.Running && !p.started.IsZero() {
		st.Time = p.fromTime.Add(time.Duration(float64(time.Since(p.started)) * st.Speed))
	}
	return st
}

// Start replays a race at speed times real time from the start of lap
// fromLap (0: from the start), replacing any replay running.
func (p *Player) Start(race string, speed float64, fromLap int) error {
	if speed <= 0 || speed > 100 {
		return fmt.Errorf("speed %v: want more than 0 and up to 100", speed)
	}
	dir := filepath.Join(p.root, filepath.Base(race))
	src, err := Open(dir, Options{SkipCars: true})
	if err != nil {
		return fmt.Errorf("no replay %q", race)
	}
	from := src.Manifest.Start
	if fromLap > 1 {
		t, err := LapStart(dir, fromLap)
		if err != nil {
			src.Close()
			return err
		}
		from = t
	}
	p.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	state := live.NewState()
	title := fmt.Sprintf("%d %s %s", src.Manifest.Year, src.Manifest.Country, src.Manifest.Session)
	p.mu.Lock()
	p.state, p.cancel = state, cancel
	p.status = Status{Race: filepath.Base(race), Title: title, Speed: speed, Running: true, Time: from, Run: p.status.Run + 1}
	p.started, p.fromTime = time.Time{}, from
	p.mu.Unlock()

	go func() {
		defer src.Close()
		caughtUp := false
		err := Play(ctx, src, from, time.Time{}, speed, func(r live.Record) {
			if !caughtUp && !r.Time.Before(from) {
				caughtUp = true
				p.mu.Lock()
				p.started = time.Now()
				p.mu.Unlock()
			}
			state.Apply(r)
		})
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.state != state {
			return // replaced by another replay
		}
		p.status.Running = false
		p.status.Time = state.Snapshot().Time
		if err != nil && err != context.Canceled {
			p.status.Error = err.Error()
		}
	}()
	return nil
}

// Stop stops the replay, keeping its state.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	if p.status.Running {
		p.status.Running = false
		p.status.Time = p.state.Snapshot().Time
	}
}

// LapStart returns when the first car started lap n of a collected session.
func LapStart(dir string, n int) (time.Time, error) {
	laps, err := readGz(filepath.Join(dir, "laps.json.gz"))
	if err != nil {
		return time.Time{}, err
	}
	var first time.Time
	for _, raw := range laps {
		var l struct {
			Lap   int       `json:"lap_number"`
			Start time.Time `json:"date_start"`
		}
		if json.Unmarshal(raw, &l) == nil && l.Lap == n && !l.Start.IsZero() && (first.IsZero() || l.Start.Before(first)) {
			first = l.Start
		}
	}
	if first.IsZero() {
		return first, fmt.Errorf("no lap %d", n)
	}
	return first, nil
}

func lastLap(dir string) int {
	laps, err := readGz(filepath.Join(dir, "laps.json.gz"))
	if err != nil {
		return 0
	}
	most := 0
	for _, raw := range laps {
		var l struct {
			Lap int `json:"lap_number"`
		}
		if json.Unmarshal(raw, &l) == nil {
			most = max(most, l.Lap)
		}
	}
	return most
}

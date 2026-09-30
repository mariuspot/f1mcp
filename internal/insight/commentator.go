package insight

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mariuspot/f1mcp/internal/live"
)

// Pacing: at most one call to Claude per minWall of real time, and, unless
// something major happened, at least minSession of session time between
// comments.
const (
	minWall    = 8 * time.Second
	minSession = 30 * time.Second
	maxTokens  = 300
	skip       = "SKIP"
)

// Commentator writes insights for sessions, keeping them in dir so a
// replay played again costs nothing.
type Commentator struct {
	claude *Claude
	dir    string
}

// NewCommentator returns a commentator, or nil if claude is nil.
func NewCommentator(claude *Claude, dir string) *Commentator {
	if claude == nil {
		return nil
	}
	return &Commentator{claude: claude, dir: dir}
}

// Run comments on one session as it's played.
type Run struct {
	c     *Commentator
	state *live.State
	key   string // e.g. "replay-2024-brazil-race"

	mu          sync.Mutex
	pending     []live.Event
	lastWall    time.Time
	lastSession time.Time
	said        []string
}

// Start comments on a session's state until ctx is done. key names the
// session for the cache. Feed it events with Add.
func (c *Commentator) Start(ctx context.Context, state *live.State, key string) *Run {
	r := &Run{c: c, state: state, key: key}
	go r.loop(ctx)
	return r
}

// Add queues events to comment on. Only notable ones are kept.
func (r *Run) Add(events []live.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range events {
		if e.Priority >= 2 {
			r.pending = append(r.pending, e)
		}
	}
}

func (r *Run) loop(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		batch := r.due()
		if batch == nil {
			continue
		}
		if err := r.comment(ctx, batch); err != nil && ctx.Err() == nil {
			log.Printf("insight: %v", err)
		}
	}
}

// due returns the events to comment on now, if it's time.
func (r *Run) due() []live.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) == 0 || time.Since(r.lastWall) < minWall {
		return nil
	}
	major := slices.ContainsFunc(r.pending, func(e live.Event) bool { return e.Priority >= 3 })
	now := r.pending[len(r.pending)-1].Time
	if !major && now.Sub(r.lastSession) < minSession {
		return nil
	}
	batch := r.pending
	r.pending = nil
	r.lastWall, r.lastSession = time.Now(), now
	return batch
}

func (r *Run) comment(ctx context.Context, events []live.Event) error {
	snap := r.state.Snapshot()
	var ids []int
	var drivers []string
	for _, e := range events {
		ids = append(ids, e.ID)
		for _, d := range e.Drivers {
			if d != "" && !slices.Contains(drivers, d) {
				drivers = append(drivers, d)
			}
		}
	}
	text, err := r.cached(events)
	if err != nil {
		prompt := Prompt(snap, events, r.state.Recent(5), r.recentlySaid())
		cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		text, err = r.c.claude.Complete(cctx, System, prompt, maxTokens)
		if err != nil {
			return err
		}
		r.keep(events, text)
	}
	if text == "" || strings.EqualFold(strings.TrimSpace(text), skip) {
		return nil
	}
	r.state.AddInsight(live.Insight{Text: text, Drivers: drivers, Events: ids})
	r.mu.Lock()
	r.said = append(r.said, text)
	r.mu.Unlock()
	return nil
}

func (r *Run) recentlySaid() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.said[max(0, len(r.said)-5):])
}

// cacheFile names the insight for a batch of events by what and when they
// were, so the same moment of a replay gives the same file whichever lap
// the replay started from.
func (r *Run) cacheFile(events []live.Event) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s %s\n", r.c.claude.Model(), PromptVersion)
	for _, e := range events {
		fmt.Fprintf(h, "%s %s\n", e.Time.UTC().Format(time.RFC3339Nano), e.Text)
	}
	return filepath.Join(r.c.dir, r.key, hex.EncodeToString(h.Sum(nil)[:12])+".txt")
}

func (r *Run) cached(events []live.Event) (string, error) {
	if r.c.dir == "" {
		return "", os.ErrNotExist
	}
	b, err := os.ReadFile(r.cacheFile(events))
	return string(b), err
}

func (r *Run) keep(events []live.Event, text string) {
	if r.c.dir == "" {
		return
	}
	f := r.cacheFile(events)
	if os.MkdirAll(filepath.Dir(f), 0o755) == nil {
		_ = os.WriteFile(f, []byte(text), 0o644)
	}
}

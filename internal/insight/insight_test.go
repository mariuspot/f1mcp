package insight

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mariuspot/f1mcp/internal/live"
)

// fakeClaude answers every message with reply, counting calls.
func fakeClaude(t *testing.T, reply string, calls *atomic.Int32) *Claude {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/messages" || r.Header.Get("x-api-key") != "test-key" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("request %s, key %q, version %q", r.URL.Path, r.Header.Get("x-api-key"), r.Header.Get("anthropic-version"))
		}
		var body struct {
			Model  string `json:"model"`
			System []struct {
				Text         string            `json:"text"`
				CacheControl map[string]string `json:"cache_control"`
			} `json:"system"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Model != DefaultModel || len(body.System) != 1 || body.System[0].CacheControl["type"] != "ephemeral" || len(body.Messages) != 1 {
			t.Errorf("body: %+v", body)
		}
		json.NewEncoder(w).Encode(map[string]any{"content": []map[string]string{{"type": "text", "text": reply}}})
	}))
	t.Cleanup(srv.Close)
	return NewClaude("test-key", "", srv.Client()).WithBaseURL(srv.URL)
}

func TestClaudeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	defer srv.Close()
	_, err := NewClaude("bad", "", srv.Client()).WithBaseURL(srv.URL).Complete(context.Background(), "s", "u", 10)
	if err == nil || !strings.Contains(err.Error(), "invalid x-api-key") {
		t.Errorf("err = %v", err)
	}
	if NewClaude("", "", nil) != nil || NewCommentator(nil, "") != nil {
		t.Error("no key should mean no client and no commentator")
	}
}

func stateWithCars() *live.State {
	s := live.NewState()
	rec := func(topic string, v any) {
		b, _ := json.Marshal(v)
		s.Apply(live.Record{Topic: topic, Time: time.Date(2024, 11, 3, 16, 0, 0, 0, time.UTC), Data: b})
	}
	rec("drivers", map[string]any{"driver_number": 1, "name_acronym": "VER", "team_name": "Red Bull Racing"})
	rec("position", map[string]any{"driver_number": 1, "position": 1})
	rec("stints", map[string]any{"driver_number": 1, "stint_number": 1, "lap_start": 1, "compound": "INTERMEDIATE"})
	return s
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatal("timed out")
}

func TestCommentAndCache(t *testing.T) {
	var calls atomic.Int32
	dir := t.TempDir()
	event := live.Event{ID: 7, Time: time.Date(2024, 11, 3, 16, 30, 0, 0, time.UTC), Lap: 30, Kind: live.KindFlag, Priority: 3, Text: "Safety car deployed on lap 30"}

	for run := range 2 {
		state := stateWithCars()
		ctx, cancel := context.WithCancel(context.Background())
		c := NewCommentator(fakeClaude(t, "A cheap stop for anyone yet to pit.", &calls), dir)
		c.Start(ctx, state, "replay-test").Add([]live.Event{event, {Priority: 1, Text: "minor, ignored"}})
		waitFor(t, func() bool { return len(state.InsightsSince(0)) == 1 })
		in := state.InsightsSince(0)[0]
		if in.Text != "A cheap stop for anyone yet to pit." || len(in.Events) != 1 || in.Events[0] != 7 {
			t.Errorf("run %d: insight %+v", run, in)
		}
		cancel()
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("Claude called %d times, want 1 (the replay's second run is cached)", n)
	}
}

func TestSkip(t *testing.T) {
	var calls atomic.Int32
	state := stateWithCars()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	NewCommentator(fakeClaude(t, "SKIP", &calls), "").Start(ctx, state, "x").Add([]live.Event{{ID: 1, Priority: 3, Text: "Chequered flag"}})
	waitFor(t, func() bool { return calls.Load() == 1 })
	time.Sleep(100 * time.Millisecond)
	if n := len(state.InsightsSince(0)); n != 0 {
		t.Errorf("%d insights after SKIP, want 0", n)
	}
}

// Notable events wait for 30 s of session time between comments; major
// ones don't.
func TestPacing(t *testing.T) {
	t0 := time.Date(2024, 11, 3, 16, 0, 0, 0, time.UTC)
	r := &Run{}
	r.Add([]live.Event{{Priority: 2, Time: t0, Text: "a"}})
	if r.due() == nil {
		t.Fatal("first notable event not due")
	}
	r.lastWall = time.Time{} // as if 8 s had passed
	r.Add([]live.Event{{Priority: 2, Time: t0.Add(10 * time.Second), Text: "b"}})
	if r.due() != nil {
		t.Error("notable event 10 s later due already")
	}
	r.Add([]live.Event{{Priority: 3, Time: t0.Add(12 * time.Second), Text: "c"}})
	if b := r.due(); len(b) != 2 {
		t.Errorf("major event: batch %v, want both waiting events", b)
	}
	r.Add([]live.Event{{Priority: 3, Time: t0.Add(13 * time.Second), Text: "d"}})
	if r.due() != nil {
		t.Error("due again within 8 s of real time")
	}
}

func TestPrompt(t *testing.T) {
	lane := 22.0
	s := live.Snapshot{Session: "Brazil Race", Lap: 33, Flag: live.Red, Cars: []live.Car{
		{Code: "OCO", Team: "Alpine", Position: 1, Compound: "INTERMEDIATE", TyreAge: 2, RedFlagChanges: 1},
		{Code: "VER", Team: "Red Bull Racing", Position: 2, Compound: "INTERMEDIATE", Stops: []live.PitStop{{Lap: 20, LaneSeconds: &lane}}},
		{Code: "SAI", Team: "Ferrari", Out: "DNF"},
	}}
	p := Prompt(s, []live.Event{{Lap: 32, Text: "Red flag on lap 32"}}, nil, []string{"earlier insight"})
	for _, want := range []string{"Lap 33. Track: red flag.", "about 22 s", "P1 OCO Alpine: leader, intermediate 2 laps, 0 stops (+1 tyre change under red flag)", "SAI Ferrari: DNF", "Red flag on lap 32", "earlier insight", "SKIP"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q:\n%s", want, p)
		}
	}
}

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
	_, err := NewClaude("bad", "", srv.Client()).WithBaseURL(srv.URL).Complete(context.Background(), "s", "u", 10, "")
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

// advance moves the state's session time to t with a weather reading.
func advance(s *live.State, t time.Time) {
	s.Apply(live.Record{Topic: "weather", Time: t, Data: []byte(`{"rainfall":0}`)})
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
		advance(state, event.Time.Add(20*time.Second)) // past the settling wait
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
	at := time.Date(2024, 11, 3, 17, 56, 0, 0, time.UTC)
	NewCommentator(fakeClaude(t, "SKIP", &calls), "").Start(ctx, state, "x").Add([]live.Event{{ID: 1, Priority: 3, Time: at, Text: "Chequered flag"}})
	advance(state, at.Add(20*time.Second))
	waitFor(t, func() bool { return calls.Load() == 1 })
	time.Sleep(100 * time.Millisecond)
	if n := len(state.InsightsSince(0)); n != 0 {
		t.Errorf("%d insights after SKIP, want 0", n)
	}
}

// Events wait 15 s of session time for related ones; notable events also
// wait for 30 s since the last comment, major ones don't.
func TestPacing(t *testing.T) {
	t0 := time.Date(2024, 11, 3, 16, 0, 0, 0, time.UTC)
	state := live.NewState()
	r := &Run{state: state}
	r.Add([]live.Event{{Priority: 2, Time: t0, Text: "a"}})
	advance(state, t0.Add(10*time.Second))
	if r.due() != nil {
		t.Fatal("due before the settling wait")
	}
	advance(state, t0.Add(16*time.Second))
	if b := r.due(); len(b) != 1 {
		t.Fatalf("after settling: %v", b)
	}

	r.lastWall = time.Time{} // as if 8 s of real time had passed
	r.Add([]live.Event{{Priority: 2, Time: t0.Add(20 * time.Second), Text: "b"}})
	advance(state, t0.Add(40*time.Second))
	if r.due() != nil {
		t.Error("notable event due 24 s after the last comment")
	}
	r.Add([]live.Event{{Priority: 3, Time: t0.Add(41 * time.Second), Text: "c"}})
	advance(state, t0.Add(57*time.Second))
	if b := r.due(); len(b) != 2 {
		t.Errorf("major event: batch %v, want both waiting events", b)
	}
	r.Add([]live.Event{{Priority: 3, Time: t0.Add(58 * time.Second), Text: "d"}})
	advance(state, t0.Add(80*time.Second))
	if r.due() != nil {
		t.Error("due again within 8 s of real time")
	}
}

func TestPrompt(t *testing.T) {
	lane, pace := 22.0, 85.412
	s := live.Snapshot{Session: "Brazil Race", Lap: 33, Flag: live.Red, Cars: []live.Car{
		{Code: "OCO", Team: "Alpine", Position: 1, Lap: 32, Compound: "INTERMEDIATE", TyreAge: 2, RedFlagChanges: 1,
			LastLap: &live.LapTime{Lap: 32, Seconds: 158.23, Neutral: true}},
		{Code: "VER", Team: "Red Bull Racing", Position: 2, Lap: 32, Compound: "INTERMEDIATE", TyreAge: 9, Pace: &pace, TrackLimits: 2,
			Stops:   []live.PitStop{{Lap: 20, LaneSeconds: &lane}},
			Penalty: []string{"FIA STEWARDS: 10 SECOND TIME PENALTY FOR CAR 1 (VER) - CAUSING A COLLISION"}},
		{Code: "ALB", Team: "Williams", Position: 19},
		{Code: "SAI", Team: "Ferrari", Out: "DNF"},
	}}
	msgs := []live.Message{{Message: "CAR 1 (VER) TIME 1:24.1 DELETED - TRACK LIMITS AT TURN 4 LAP 20"}, {Message: "CLEAR IN TRACK SECTOR 2"}, {Message: "RED FLAG"}}
	p := Prompt(s, []live.Event{{Lap: 32, Text: "Red flag on lap 32"}}, msgs, []string{"earlier insight"})
	for _, want := range []string{
		"Lap 33. Track: red flag.",
		"about 22 s",
		"P1 OCO (Alpine): leader | intermediates, 2 laps old (changed under the red flag) | stops: none | pace n/a",
		"last lap 2:38.230, neutralised",
		"P2 VER (Red Bull Racing): intermediates, 9 laps old | stops: lap 20 | pace 1:25.412 | 2 lap time(s) deleted for track limits | penalty: 10 second time penalty - causing a collision",
		"ALB (Williams): not running",
		"SAI (Ferrari): out of the race (DNF)",
		"- RED FLAG",
		"Red flag on lap 32",
		"earlier insight",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q:\n%s", want, p)
		}
	}
	for _, not := range []string{"CLEAR IN TRACK SECTOR", "TRACK LIMITS AT TURN"} {
		if strings.Contains(p, not) {
			t.Errorf("prompt has %q", not)
		}
	}
}

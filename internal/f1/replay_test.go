package f1

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tracks"
)

func TestSetTyre(t *testing.T) {
	stints := []openf1.Stint{
		{DriverNumber: 1, LapStart: 1, LapEnd: 20, Compound: "MEDIUM", TyreAgeAtStart: 0},
		{DriverNumber: 1, LapStart: 21, LapEnd: 57, Compound: "HARD", TyreAgeAtStart: 3},
		{DriverNumber: 44, LapStart: 1, LapEnd: 30, Compound: "SOFT", TyreAgeAtStart: 2},
	}
	for _, tc := range []struct {
		number, lap int
		compound    string
		age         int
	}{
		{1, 1, "MEDIUM", 0},
		{1, 20, "MEDIUM", 19},
		{1, 21, "HARD", 3},  // a used set
		{1, 30, "HARD", 12}, // 3 before the stint, 9 laps into it
		{44, 5, "SOFT", 6},
		{16, 5, "", 0}, // no stint
	} {
		l := tracks.LapTrace{Number: tc.number, Lap: tc.lap}
		SetTyre(&l, stints)
		if l.Compound != tc.compound || l.TyreAge != tc.age {
			t.Errorf("car %d lap %d: %s %d laps old, want %s %d", tc.number, tc.lap, l.Compound, l.TyreAge, tc.compound, tc.age)
		}
	}
}

func TestPickLap(t *testing.T) {
	start := time.Date(2026, 7, 4, 15, 0, 0, 0, time.UTC)
	lap := func(n int, secs float64, out bool) openf1.Lap {
		l := openf1.Lap{LapNumber: n, IsPitOutLap: out, DateStart: start.Add(time.Duration(n) * 2 * time.Minute)}
		if secs > 0 {
			l.LapDuration = &secs
		}
		return l
	}
	// Out, push, cool-down, in (untimed), out, push, cool-down.
	laps := []openf1.Lap{
		lap(1, 120, true), lap(2, 88.4, false), lap(3, 110, false), lap(4, 0, false),
		lap(5, 118, true), lap(6, 88.1, false), lap(7, 105, false),
	}
	for _, tc := range []struct {
		choice string
		want   int
	}{
		{"", 6}, {"best", 6}, {"first", 2}, {"last", 7}, {"last_flying", 6}, {"LAST_FLYING", 6}, {"3", 3},
	} {
		l, err := pickLap(laps, tc.choice)
		if err != nil || l.LapNumber != tc.want {
			t.Errorf("pickLap(%q) = lap %d, %v; want lap %d", tc.choice, l.LapNumber, err, tc.want)
		}
	}
	for _, bad := range []string{"4", "9", "fastest"} {
		if l, err := pickLap(laps, bad); err == nil {
			t.Errorf("pickLap(%q) = lap %d, want an error", bad, l.LapNumber)
		}
	}
}

func TestSplitSession(t *testing.T) {
	for _, tc := range []struct {
		in, session string
		part        int
	}{
		{"q1", Qualifying, 1}, {"Q3", Qualifying, 3}, {"sq2", SprintQualifying, 2}, {"race", Race, 0}, {"qualifying", Qualifying, 0},
	} {
		if s, p := SplitSession(tc.in); s != tc.session || p != tc.part {
			t.Errorf("SplitSession(%q) = %q, %d; want %q, %d", tc.in, s, p, tc.session, tc.part)
		}
	}
}

type fakeTranscriber struct {
	mu      sync.Mutex
	prompts []string
}

func (f *fakeTranscriber) Transcribe(_ context.Context, url, prompt string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prompts = append(f.prompts, prompt)
	return "transcript of " + url[strings.LastIndex(url, "/")+1:], nil
}

// Clips get the lap the driver was on when they were broadcast, from the lap
// start times (the fixture has only Verstappen's lap 8, from 12:15:42).
func TestTeamRadio(t *testing.T) {
	s := testService(t, "2023-09-20T00:00:00Z")
	ctx := context.Background()
	e, err := s.ResolveEvent(ctx, 2023, "singapore")
	if err != nil {
		t.Fatal(err)
	}
	clips, err := s.TeamRadio(ctx, e, Race, "VER", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 5 {
		t.Fatalf("got %d VER clips, want 5", len(clips))
	}
	for i, c := range clips {
		if c.Driver.Code != "VER" || c.URL == "" || c.Transcript != "" {
			t.Errorf("clip %d: %+v", i, c)
		}
		if i > 0 && c.Time.Before(clips[i-1].Time) {
			t.Error("clips out of order")
		}
		wantLap := 0
		if !c.Time.Before(time.Date(2023, 9, 17, 12, 15, 42, 0, time.UTC)) {
			wantLap = 8
		}
		if c.Lap != wantLap {
			t.Errorf("clip at %s on lap %d, want %d", c.Time.Format("15:04:05"), c.Lap, wantLap)
		}
	}
	from8, err := s.TeamRadio(ctx, e, Race, "VER", 8, 0)
	if err != nil || len(from8) != 3 {
		t.Errorf("from lap 8: %d clips, %v; want 3", len(from8), err)
	}
	all, err := s.TeamRadio(ctx, e, Race, "", 0, 0)
	if err != nil || len(all) != 65 {
		t.Errorf("all drivers: %d clips, %v; want 65", len(all), err)
	}

	tr := &fakeTranscriber{}
	s.WithTranscriber(tr)
	s.TranscribeClips(ctx, clips)
	for _, c := range clips {
		if !strings.HasPrefix(c.Transcript, "transcript of ") {
			t.Errorf("transcript = %q", c.Transcript)
		}
	}
	if len(tr.prompts) != 5 || !strings.Contains(tr.prompts[0], "Max Verstappen") {
		t.Errorf("prompts = %q", tr.prompts)
	}
}

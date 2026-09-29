package f1

import (
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

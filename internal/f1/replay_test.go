package f1

import (
	"testing"

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

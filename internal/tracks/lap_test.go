package tracks

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// gapJumps returns the largest and average change in the gap between two
// laps from one video frame to the next.
func gapJumps(t *Track, a, b LapTrace) (largest, mean float64) {
	dist, lap := lapDistances(t.Outline)
	pa, pb := lapProgress(t, a, dist, lap), lapProgress(t, b, dist, lap)
	step := lapSpeedUp / followFPS
	prev, n := math.NaN(), 0
	for T := 5.0; T < min(a.Duration, b.Duration)-5; T += step {
		gap := T - timeAt(a, pa, distanceAt(b, pb, T))
		if !math.IsNaN(prev) {
			d := math.Abs(gap - prev)
			largest = max(largest, d)
			mean += d
			n++
		}
		prev = gap
	}
	return largest, mean / float64(n)
}

// Positions worked out from speed make the gap between two cars change
// smoothly from frame to frame, unlike OpenF1's raw positions.
func TestSmoothLapsSteadyGap(t *testing.T) {
	b, err := os.ReadFile("../../cmd/trackgen/data/laps/baku-2026-qualifying-rus-vs-ver.json")
	if err != nil {
		t.Skip("no stored lap sample:", err)
	}
	var f struct {
		CircuitID string     `json:"circuit_id"`
		Year      int        `json:"year"`
		Laps      []LapTrace `json:"laps"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	tr, err := Load(f.CircuitID, f.Year)
	if err != nil {
		t.Fatal(err)
	}
	rawMax, rawMean := gapJumps(tr, f.Laps[0], f.Laps[1])
	smooth := smoothLaps(tr, f.Laps)
	smMax, smMean := gapJumps(tr, smooth[0], smooth[1])
	t.Logf("gap change per frame: raw max %.3f s, mean %.4f s; smoothed max %.3f s, mean %.4f s", rawMax, rawMean, smMax, smMean)
	if smMax > 0.05 {
		t.Errorf("smoothed gap jumps by up to %.3f s between frames, want under 0.05 s", smMax)
	}
	// The smoothed laps still take their real lap times.
	for i, l := range smooth {
		last := l.Positions[len(l.Positions)-1][0]
		if math.Abs(last-f.Laps[i].Duration) > 0.01 {
			t.Errorf("%s smoothed lap ends at %.3f s, want %.3f s", l.Driver, last, f.Laps[i].Duration)
		}
	}
}

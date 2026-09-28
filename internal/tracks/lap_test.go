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

// Braking zones start drawing as soon as braking starts, even when the
// driver changes down during them.
func TestLapEventsInStartOrder(t *testing.T) {
	l := LapTrace{Duration: 10, Telemetry: [][5]float64{
		{0, 300, 100, 0, 8},
		{1, 280, 0, 100, 8}, // brake
		{2, 200, 0, 100, 6}, // downshift while braking
		{3, 150, 0, 100, 4},
		{4, 150, 50, 0, 4}, // release
	}}
	ev := l.events()
	if len(ev) == 0 || !ev[0].braking || ev[0].t != 1 {
		t.Fatalf("first event = %+v, want braking from t=1", ev)
	}
	for i := 1; i < len(ev); i++ {
		if ev[i].t < ev[i-1].t {
			t.Errorf("event %d at %.1f s comes after one at %.1f s", i, ev[i].t, ev[i-1].t)
		}
	}
}

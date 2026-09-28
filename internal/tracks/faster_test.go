package tracks

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// Stretches cover the whole lap, end to end, with the last one running
// across the line.
func TestStretchesCoverLap(t *testing.T) {
	ids, err := Circuits()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		layouts, _ := Layouts(id)
		for _, l := range layouts {
			tr, err := Load(id, l.To)
			if err != nil {
				t.Fatal(err)
			}
			st := tr.Stretches()
			_, lap := lapDistances(tr.Outline)
			if st[0].FromM != 0 && st[len(st)-1].ToM != st[0].FromM {
				t.Errorf("%s %d: lap starts at %.0f m but the last stretch ends at %.0f m", id, l.To, st[0].FromM, st[len(st)-1].ToM)
			}
			total := 0.0
			for i, s := range st {
				if i > 0 && s.FromM != st[i-1].ToM {
					t.Errorf("%s %d: %s starts at %.0f m, but %s ends at %.0f m", id, l.To, s.Name, s.FromM, st[i-1].Name, st[i-1].ToM)
				}
				if s.ToM < s.FromM {
					total += lap - s.FromM + s.ToM
				} else {
					total += s.ToM - s.FromM
				}
			}
			if math.Abs(total-lap) > 1 {
				t.Errorf("%s %d: stretches cover %.0f m of a %.0f m lap", id, l.To, total, lap)
			}
		}
	}
}

func TestStretchNames(t *testing.T) {
	tr, err := Load("silverstone", 2026)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, s := range tr.Stretches() {
		names[s.Name] = true
	}
	for _, want := range []string{"Turn 9, Copse", "Hangar Straight", "Hamilton Straight", "Turn 3, Village", "Turns 10–13, Maggotts and Becketts"} {
		if !names[want] {
			t.Errorf("no stretch %q in %v", want, names)
		}
	}
}

// Each lap's times over the stretches add up to its lap time.
func TestCompareLapsAddsUp(t *testing.T) {
	b, err := os.ReadFile("../../cmd/trackgen/data/laps/silverstone-2026-qualifying-ant-vs-ver.json")
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
	cmp, err := CompareLaps(tr, f.Laps)
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range f.Laps {
		sum := 0.0
		for _, r := range cmp.Stretches {
			sum += r.Seconds[i]
		}
		if math.Abs(sum-l.Duration) > 0.02 {
			t.Errorf("%s stretches add up to %.3f s, lap was %.3f s", l.Driver, sum, l.Duration)
		}
	}
	if _, err := CompareLaps(tr, f.Laps[:1]); err == nil {
		t.Error("comparing one lap: no error")
	}
}

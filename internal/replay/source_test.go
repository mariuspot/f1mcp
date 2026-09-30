package replay

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mariuspot/f1mcp/internal/live"
)

func raws(t *testing.T, vs ...any) []json.RawMessage {
	t.Helper()
	var out []json.RawMessage
	for _, v := range vs {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

type m = map[string]any

// A collected session plays back in the order a live feed would have sent
// it: laps when they end, stints when their first lap starts, pit stops
// when the car leaves the pit lane, car data merged in by time.
func TestSourceOrder(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2024, 11, 3, 15, 30, 0, 0, time.UTC)
	at := func(sec float64) string {
		return start.Add(time.Duration(sec * float64(time.Second))).Format(time.RFC3339Nano)
	}
	files := map[string][]json.RawMessage{
		"sessions": raws(t, m{"session_key": 1, "session_name": "Race"}),
		"laps": raws(t,
			m{"driver_number": 1, "lap_number": 1, "date_start": at(0), "lap_duration": 90.0},
			m{"driver_number": 1, "lap_number": 2, "date_start": at(90), "lap_duration": nil},
			m{"driver_number": 1, "lap_number": 3, "date_start": at(200), "lap_duration": 88.0}),
		"stints":       raws(t, m{"driver_number": 1, "stint_number": 2, "lap_start": 3}),
		"pit":          raws(t, m{"driver_number": 1, "date": at(170), "lane_duration": 25.0}),
		"race_control": raws(t, m{"date": at(10), "message": "GREEN LIGHT"}),
		"car_data/1":   raws(t, m{"date": at(5), "speed": 100}, m{"date": at(95), "speed": 200}),
		"location/1":   raws(t, m{"date": at(6), "x": 1}),
	}
	for name, recs := range files {
		if err := writeGz(filepath.Join(dir, name+".json.gz"), recs); err != nil {
			t.Fatal(err)
		}
	}
	man, _ := json.Marshal(Manifest{SessionKey: 1, Start: start, End: start.Add(300 * time.Second), Drivers: []int{1}})
	os.WriteFile(filepath.Join(dir, "manifest.json"), man, 0o644)

	src, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	var got []string
	var last time.Time
	err = Play(context.Background(), src, time.Time{}, time.Time{}, 0, func(r live.Record) {
		if r.Time.Before(last) {
			t.Errorf("%s at %s before %s", r.Topic, r.Time, last)
		}
		last = r.Time
		got = append(got, r.Topic+"@"+r.Time.Sub(start).String())
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"sessions@-1h0m0s", "car_data@5s", "location@6s", "race_control@10s", "laps@1m30s", "car_data@1m35s", "pit@3m15s", "laps@3m20s", "stints@3m20s", "laps@4m48s"}
	if len(got) != len(want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("record %d = %s, want %s", i, got[i], want[i])
		}
	}
}

// The collected 2024 São Paulo race, if it's here, replays to its result.
func TestReplaySaoPaulo2024(t *testing.T) {
	dir := "../../replays/2024-brazil-race"
	if _, err := os.Stat(dir); err != nil {
		t.Skip("not collected; run trackgen collect -session 9636")
	}
	src, err := Open(dir, Options{SkipCars: true})
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	state := live.NewState()
	if err := Play(context.Background(), src, time.Time{}, time.Time{}, 0, state.Apply); err != nil {
		t.Fatal(err)
	}
	s := state.Snapshot()
	want := []string{"VER", "OCO", "GAS", "RUS", "LEC", "NOR"}
	for i, code := range want {
		if s.Cars[i].Code != code {
			t.Errorf("P%d = %s, want %s", i+1, s.Cars[i].Code, code)
		}
	}
	if s.Flag != live.Ended || s.Lap != 69 || s.BestLap == nil || s.BestLap.Driver != "VER" {
		t.Errorf("flag %s, lap %d, fastest %+v", s.Flag, s.Lap, s.BestLap)
	}
}

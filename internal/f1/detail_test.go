package f1

import (
	"testing"
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
)

func TestOrderAt(t *testing.T) {
	sd := &sessionData{drivers: map[int]DriverRef{
		1: {Code: "AAA", Number: 1}, 2: {Code: "BBB", Number: 2}, 3: {Code: "CCC", Number: 3},
	}, teams: map[int]string{}}
	t0 := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	at := func(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }
	ends := map[int]map[int]time.Time{
		1: {1: at(90), 2: at(180), 3: at(270)},
		2: {1: at(91), 2: at(182.5), 3: at(275)},
		3: {1: at(95), 2: at(200)}, // a lap down after lap 3
	}
	order := orderAt(sd, ends, 3)
	if order[0].Driver.Code != "AAA" || order[0].GapSeconds != nil {
		t.Errorf("leader = %+v", order[0])
	}
	if order[1].Driver.Code != "BBB" || *order[1].GapSeconds != 5 || *order[1].IntervalSeconds != 5 {
		t.Errorf("second = %+v", order[1])
	}
	if order[2].Driver.Code != "CCC" || order[2].LapsDown != 1 || order[2].GapSeconds != nil {
		t.Errorf("third = %+v", order[2])
	}
	// At lap 2 everyone is on the lead lap.
	order = orderAt(sd, ends, 2)
	if *order[2].GapSeconds != 20 || *order[2].IntervalSeconds != 17.5 {
		t.Errorf("lap 2 third = %+v", order[2])
	}
}

func TestSummariseWeather(t *testing.T) {
	t0 := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	ws := []WeatherSample{
		{Time: t0, AirTempC: 20, TrackTempC: 30},
		{Time: t0.Add(time.Minute), AirTempC: 22, TrackTempC: 34, Rain: true},
		{Time: t0.Add(2 * time.Minute), AirTempC: 24, TrackTempC: 32, Rain: true},
	}
	sum, err := SummariseWeather(ws)
	if err != nil {
		t.Fatal(err)
	}
	if sum.AirTempC != (Range{20, 24, 22}) || sum.TrackTempC.Max != 34 {
		t.Errorf("temperatures = %+v %+v", sum.AirTempC, sum.TrackTempC)
	}
	if !sum.Rain || !sum.RainFrom.Equal(t0.Add(time.Minute)) || !sum.RainTo.Equal(t0.Add(2*time.Minute)) {
		t.Errorf("rain = %v from %v to %v", sum.Rain, sum.RainFrom, sum.RainTo)
	}
	if _, err := SummariseWeather(nil); err == nil {
		t.Error("want an error with no readings")
	}
}

func TestKeyMessages(t *testing.T) {
	flag := func(f string) *string { return &f }
	for _, tc := range []struct {
		m    openf1.RaceControl
		want bool
	}{
		{openf1.RaceControl{Category: "SafetyCar", Message: "SAFETY CAR DEPLOYED"}, true},
		{openf1.RaceControl{Category: "Flag", Flag: flag("RED"), Message: "RED FLAG"}, true},
		{openf1.RaceControl{Category: "Other", Message: "FIA STEWARDS: 5 SECOND TIME PENALTY FOR CAR 1"}, true},
		{openf1.RaceControl{Category: "Flag", Flag: flag("YELLOW"), Message: "YELLOW IN TRACK SECTOR 4"}, false},
		{openf1.RaceControl{Category: "Drs", Message: "DRS ENABLED"}, false},
	} {
		if got := keyMessage(tc.m); got != tc.want {
			t.Errorf("keyMessage(%q) = %v, want %v", tc.m.Message, got, tc.want)
		}
	}
}

func TestPaginate(t *testing.T) {
	items := make([]int, 450)
	got, p := Paginate(items, "", 0)
	if len(got) != 200 || p.Total != 450 || p.NextCursor != "200" {
		t.Errorf("first page: %d items, %+v", len(got), p)
	}
	got, p = Paginate(items, "400", 0)
	if len(got) != 50 || p.NextCursor != "" {
		t.Errorf("last page: %d items, %+v", len(got), p)
	}
	if got, _ := Paginate(items, "9999", 10); len(got) != 0 {
		t.Errorf("past the end: %d items", len(got))
	}
}

package insight

import (
	"strings"
	"testing"

	"github.com/mariuspot/f1mcp/internal/f1"
	"github.com/mariuspot/f1mcp/internal/live"
)

func TestParsePreview(t *testing.T) {
	msgs, err := parsePreview("Here you go:\n[{\"topic\":\"welcome\",\"text\":\"Welcome to Interlagos.\"},{\"topic\":\"grid\",\"text\":\" \"}]\nEnjoy!")
	if err != nil || len(msgs) != 1 || msgs[0].Topic != "welcome" {
		t.Errorf("parsePreview = %+v, %v", msgs, err)
	}
	for _, bad := range []string{"no array here", "[]", "[{\"topic\":\"x\"}]"} {
		if _, err := parsePreview(bad); err == nil {
			t.Errorf("parsePreview(%q): no error", bad)
		}
	}
}

func TestPreviewPrompt(t *testing.T) {
	d := f1.PreviewData{
		Event:   f1.Event{Year: 2024, Round: 21, Name: "São Paulo Grand Prix", Circuit: f1.Circuit{Name: "Interlagos", Locality: "São Paulo"}},
		Session: f1.Race,
		Grid: []f1.GridPlace{
			{Position: 1, Driver: f1.DriverRef{Code: "NOR"}, Team: "McLaren"},
			{Position: 17, Driver: f1.DriverRef{Code: "VER"}, Team: "Red Bull", Qualified: 12},
		},
		Standings:  []f1.Standing{{Position: 1, Driver: &f1.DriverRef{Code: "VER"}, Points: 362}},
		AfterRound: 20,
		LastYear: &f1.PastRace{Event: f1.Event{Year: 2023, Name: "São Paulo Grand Prix"}, Laps: 71, RedFlags: 1, PitLaneS: 23.6,
			Stops: map[int]int{2: 13}, Strategies: []string{"VER (P1, from P1): started on SOFT, stop lap 27 to MEDIUM"}},
	}
	p := PreviewPrompt(d, &live.Weather{AirC: 23, TrackC: 28, Humidity: 87, Rain: true})
	for _, want := range []string{
		"2024 São Paulo Grand Prix (round 21), the race",
		"Weather now: raining, air 23 °C, track 28 °C",
		"P17 VER (Red Bull), qualified P12",
		"after round 20: 1. VER 362",
		"Last year here (2023 São Paulo Grand Prix): 71 laps",
		"red flags: 1",
		"Pit lane time about 24 s",
		"2 stop(s): 13 finishers",
		"stop lap 27 to MEDIUM",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q:\n%s", want, p)
		}
	}
}

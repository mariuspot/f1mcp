package live

import (
	"encoding/json"
	"testing"
	"time"
)

var t0 = time.Date(2024, 11, 3, 15, 30, 0, 0, time.UTC)

func rec(topic string, sec int, v any) Record {
	b, _ := json.Marshal(v)
	return Record{Topic: topic, Time: t0.Add(time.Duration(sec) * time.Second), Data: b}
}

type m = map[string]any

func TestState(t *testing.T) {
	s := NewState()
	for _, r := range []Record{
		rec("drivers", 0, m{"driver_number": 1, "name_acronym": "VER", "first_name": "Max", "last_name": "Verstappen", "team_name": "Red Bull Racing", "team_colour": "3671C6"}),
		rec("drivers", 0, m{"driver_number": 4, "name_acronym": "NOR", "team_name": "McLaren", "team_colour": "FF8000"}),
		rec("stints", 1, m{"driver_number": 1, "stint_number": 1, "lap_start": 1, "compound": "INTERMEDIATE", "tyre_age_at_start": 3}),
		rec("position", 2, m{"driver_number": 1, "position": 2}),
		rec("position", 2, m{"driver_number": 4, "position": 1}),
		rec("laps", 90, m{"driver_number": 1, "lap_number": 1, "date_start": t0, "lap_duration": 90.5, "duration_sector_1": 30.1, "duration_sector_2": 30.2, "duration_sector_3": 30.2}),
		rec("laps", 180, m{"driver_number": 1, "lap_number": 2, "date_start": t0.Add(90 * time.Second), "lap_duration": 88.0, "duration_sector_1": 29.0, "duration_sector_2": 29.5, "duration_sector_3": 29.5}),
		rec("intervals", 181, m{"driver_number": 1, "gap_to_leader": 1.25, "interval": 1.25}),
		rec("intervals", 181, m{"driver_number": 4, "gap_to_leader": nil, "interval": nil}),
		rec("race_control", 200, m{"lap_number": 3, "category": "SafetyCar", "message": "SAFETY CAR DEPLOYED"}),
		rec("pit", 230, m{"driver_number": 1, "lap_number": 3, "lane_duration": 22.1, "stop_duration": 2.4}),
		rec("stints", 231, m{"driver_number": 1, "stint_number": 2, "lap_start": 4, "compound": "WET", "tyre_age_at_start": 0}),
		rec("overtakes", 240, m{"overtaking_driver_number": 1, "overtaken_driver_number": 4, "position": 1}),
		rec("session_result", 300, m{"driver_number": 4, "dnf": true}),
	} {
		s.Apply(r)
	}
	snap := s.Snapshot()
	if snap.Flag != SC {
		t.Errorf("flag = %s, want sc", snap.Flag)
	}
	ver := snap.Cars[0]
	if ver.Code != "VER" || ver.Color != "#3671C6" || ver.Lap != 2 || ver.LastLap == nil || ver.LastLap.Seconds != 88.0 {
		t.Fatalf("VER = %+v", ver)
	}
	if ver.GapToLeader == nil || *ver.GapToLeader != 1.25 {
		t.Errorf("VER gap = %v", ver.GapToLeader)
	}
	if ver.Compound != "WET" || ver.TyreAge != 0 || len(ver.Stops) != 1 || ver.Stops[0].CompoundAfter != "WET" {
		t.Errorf("VER tyres = %s %d, stops %+v", ver.Compound, ver.TyreAge, ver.Stops)
	}
	if !ver.LastLap.Purple[0] || snap.BestLap == nil || snap.BestLap.Seconds != 88.0 {
		t.Errorf("fastest: purple %v, best %+v", ver.LastLap.Purple, snap.BestLap)
	}
	nor := snap.Cars[1]
	if nor.Code != "NOR" || nor.Out != "DNF" || nor.GapToLeader != nil {
		t.Errorf("NOR = %+v (out cars last, null gaps unset)", nor)
	}
	if len(snap.Overtakes) != 1 || snap.Overtakes[0].By != "VER" || snap.Overtakes[0].Of != "NOR" {
		t.Errorf("overtakes = %+v", snap.Overtakes)
	}
	if snap.Lap != 3 {
		t.Errorf("lap = %d, want 3", snap.Lap)
	}
}

func TestFlags(t *testing.T) {
	s := NewState()
	steps := []struct {
		msg  m
		want Flag
	}{
		{m{"flag": "YELLOW", "scope": "Sector", "sector": 4, "message": "YELLOW IN TRACK SECTOR 4"}, Yellow},
		{m{"flag": "CLEAR", "scope": "Sector", "sector": 4, "message": "CLEAR IN TRACK SECTOR 4"}, Green},
		{m{"category": "SafetyCar", "message": "VIRTUAL SAFETY CAR DEPLOYED"}, VSC},
		{m{"category": "SafetyCar", "message": "VIRTUAL SAFETY CAR ENDING"}, VSC},
		{m{"flag": "GREEN", "scope": "Track", "message": "TRACK CLEAR"}, Green},
		{m{"flag": "RED", "scope": "Track", "message": "RED FLAG"}, Red},
		{m{"flag": "CHEQUERED", "scope": "Track", "message": "CHEQUERED FLAG"}, Ended},
	}
	for i, st := range steps {
		s.Apply(rec("race_control", i, st.msg))
		if got := s.Snapshot().Flag; got != st.want {
			t.Errorf("after %q: flag %s, want %s", st.msg["message"], got, st.want)
		}
	}
}

package live

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Flag is the state of the track.
type Flag string

const (
	Green  Flag = "green"
	Yellow Flag = "yellow" // somewhere on track
	VSC    Flag = "vsc"
	SC     Flag = "sc"
	Red    Flag = "red"
	Ended  Flag = "chequered"
)

// Car is one driver's running state.
type Car struct {
	Number   int    `json:"number"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Team     string `json:"team"`
	Color    string `json:"color"` // hex, e.g. "#3671C6"
	Position int    `json:"position,omitempty"`
	// GapToLeader and Interval are seconds, or laps behind as e.g.
	// "+1 LAP" in GapText.
	GapToLeader *float64 `json:"gap_to_leader,omitempty"`
	Interval    *float64 `json:"interval,omitempty"`
	GapText     string   `json:"gap_text,omitempty"`

	Lap      int         `json:"lap"` // laps completed
	LastLap  *LapTime    `json:"last_lap,omitempty"`
	BestLap  *LapTime    `json:"best_lap,omitempty"`
	BestSecs [3]*float64 `json:"best_sectors"`

	Compound string    `json:"compound,omitempty"`
	TyreAge  int       `json:"tyre_age"` // laps on this set, including before this stint
	Stint    int       `json:"stint,omitempty"`
	Stops    []PitStop `json:"stops,omitempty"`
	InPit    bool      `json:"in_pit,omitempty"`
	Out      string    `json:"out,omitempty"` // "DNF", "DNS" or "DSQ"
	Penalty  []string  `json:"penalties,omitempty"`

	stintStart, stintAge int
	laps                 []lapRecord
}

// LapTime is a completed lap.
type LapTime struct {
	Lap     int         `json:"lap"`
	Seconds float64     `json:"seconds"`
	Sectors [3]*float64 `json:"sectors"`
	PitOut  bool        `json:"pit_out,omitempty"`
	Started time.Time   `json:"started"`
	Purple  [3]bool     `json:"purple_sectors"` // fastest of anyone so far
}

// PitStop is a stop in the pit lane.
type PitStop struct {
	Lap           int      `json:"lap"`
	LaneSeconds   *float64 `json:"lane_seconds,omitempty"`
	StopSeconds   *float64 `json:"stop_seconds,omitempty"`
	CompoundAfter string   `json:"compound_after,omitempty"`
}

// Message is a race control message.
type Message struct {
	Time    time.Time `json:"time"`
	Lap     int       `json:"lap,omitempty"`
	Flag    string    `json:"flag,omitempty"`
	Message string    `json:"message"`
}

// Overtake is one car passing another.
type Overtake struct {
	Time     time.Time `json:"time"`
	By       string    `json:"by"`
	Of       string    `json:"of"`
	Position int       `json:"position"`
}

// Radio is a team radio clip.
type Radio struct {
	Time   time.Time `json:"time"`
	Driver string    `json:"driver"`
	Lap    int       `json:"lap,omitempty"`
	URL    string    `json:"url"`
}

// Weather is the latest weather reading.
type Weather struct {
	AirC     float64 `json:"air_c"`
	TrackC   float64 `json:"track_c"`
	Humidity float64 `json:"humidity_pct"`
	WindMS   float64 `json:"wind_m_s"`
	Rain     bool    `json:"rain"`
}

// Snapshot is the state of a session at a moment.
type Snapshot struct {
	Time      time.Time  `json:"time"`
	Session   string     `json:"session"`
	Lap       int        `json:"lap"` // the leader's current lap
	TotalLaps int        `json:"total_laps,omitempty"`
	Flag      Flag       `json:"flag"`
	Weather   *Weather   `json:"weather,omitempty"`
	Cars      []Car      `json:"cars"` // in position order
	BestLap   *Best      `json:"best_lap,omitempty"`
	Messages  []Message  `json:"messages,omitempty"`  // the latest first
	Overtakes []Overtake `json:"overtakes,omitempty"` // the latest first
	Radio     []Radio    `json:"radio,omitempty"`     // the latest first
}

// Best is the session's fastest lap so far.
type Best struct {
	Driver  string  `json:"driver"`
	Lap     int     `json:"lap"`
	Seconds float64 `json:"seconds"`
}

// How many recent messages, overtakes and clips a snapshot keeps.
const recent = 20

// State is a session's running state. It is safe for concurrent use.
type State struct {
	mu        sync.RWMutex
	now       time.Time
	session   string
	flag      Flag
	yellows   map[int]bool // marshal sectors under yellow
	weather   *Weather
	cars      map[int]*Car
	best      *Best
	bestSecs  [3]float64
	messages  []Message
	overtakes []Overtake
	radio     []Radio
	leaderLap int
	totalLaps int

	events   []Event
	pending  []Event // events from the record being applied
	nextID   int
	leader   int            // car number
	reported map[string]int // lap each closing/pace pair was last reported
	rain     rainWatch
	started  bool // a car has completed a lap
}

// NewState returns an empty state.
func NewState() *State {
	return &State{cars: map[int]*Car{}, flag: Green, yellows: map[int]bool{}, reported: map[string]int{}}
}

func (s *State) car(n int) *Car {
	c, ok := s.cars[n]
	if !ok {
		c = &Car{Number: n}
		s.cars[n] = c
	}
	return c
}

func (s *State) code(n int) string {
	if c, ok := s.cars[n]; ok && c.Code != "" {
		return c.Code
	}
	return ""
}

// Apply updates the state with a record. Records it doesn't understand are
// ignored.
func (s *State) Apply(r Record) { s.Step(r) }

// Step is Apply, returning the events the record gave rise to.
func (s *State) Step(r Record) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = nil
	s.apply(r)
	return s.pending
}

func (s *State) apply(r Record) {
	if r.Time.After(s.now) {
		s.now = r.Time
	}
	switch r.Topic {
	case "sessions":
		var v struct {
			SessionName string `json:"session_name"`
			CountryName string `json:"country_name"`
			Year        int    `json:"year"`
		}
		if json.Unmarshal(r.Data, &v) == nil {
			s.session = strings.TrimSpace(v.CountryName + " " + v.SessionName)
		}
	case "drivers":
		var v struct {
			Number     int    `json:"driver_number"`
			Acronym    string `json:"name_acronym"`
			FirstName  string `json:"first_name"`
			LastName   string `json:"last_name"`
			TeamName   string `json:"team_name"`
			TeamColour string `json:"team_colour"`
		}
		if json.Unmarshal(r.Data, &v) == nil {
			c := s.car(v.Number)
			c.Code, c.Name, c.Team = v.Acronym, v.FirstName+" "+v.LastName, v.TeamName
			if v.TeamColour != "" {
				c.Color = "#" + v.TeamColour
			}
		}
	case "position":
		var v struct {
			Number   int `json:"driver_number"`
			Position int `json:"position"`
		}
		if json.Unmarshal(r.Data, &v) == nil {
			s.car(v.Number).Position = v.Position
			if v.Position == 1 && v.Number != s.leader {
				if s.leader != 0 {
					s.emit(KindLead, 3, []string{s.code(v.Number), s.code(s.leader)}, "%s takes the lead from %s", s.code(v.Number), s.code(s.leader))
				}
				s.leader = v.Number
			}
		}
	case "intervals":
		var v struct {
			Number   int             `json:"driver_number"`
			Gap      json.RawMessage `json:"gap_to_leader"`
			Interval json.RawMessage `json:"interval"`
		}
		if json.Unmarshal(r.Data, &v) == nil {
			c := s.car(v.Number)
			c.GapToLeader, c.GapText = gapValue(v.Gap)
			c.Interval, _ = gapValue(v.Interval)
		}
	case "laps":
		s.applyLap(r.Data)
	case "stints":
		var v struct {
			Number   int    `json:"driver_number"`
			Stint    int    `json:"stint_number"`
			LapStart int    `json:"lap_start"`
			Compound string `json:"compound"`
			AgeStart int    `json:"tyre_age_at_start"`
		}
		if json.Unmarshal(r.Data, &v) == nil {
			c := s.car(v.Number)
			if v.Stint > c.Stint && c.Stint > 0 {
				s.pitEvent(c, v.Compound, v.AgeStart)
			}
			if v.Stint >= c.Stint {
				c.Stint, c.Compound, c.stintStart, c.stintAge = v.Stint, v.Compound, v.LapStart, v.AgeStart
				c.TyreAge = v.AgeStart + max(0, c.Lap+1-v.LapStart)
				if n := len(c.Stops); n > 0 && c.Stops[n-1].CompoundAfter == "" {
					c.Stops[n-1].CompoundAfter = v.Compound
				}
			}
		}
	case "pit":
		var v struct {
			Number int      `json:"driver_number"`
			Lap    int      `json:"lap_number"`
			Lane   *float64 `json:"lane_duration"`
			Stop   *float64 `json:"stop_duration"`
		}
		if json.Unmarshal(r.Data, &v) == nil {
			c := s.car(v.Number)
			c.Stops = append(c.Stops, PitStop{Lap: v.Lap, LaneSeconds: v.Lane, StopSeconds: v.Stop})
			// The in-lap may already be recorded; it wasn't a racing lap.
			// OpenF1 may number the stop by the in-lap or the out-lap.
			for i := range c.laps {
				if c.laps[i].lap >= v.Lap-1 {
					c.laps[i].racing = false
				}
			}
		}
	case "race_control":
		s.applyMessage(r)
	case "weather":
		var v struct {
			Air      float64 `json:"air_temperature"`
			Track    float64 `json:"track_temperature"`
			Humidity float64 `json:"humidity"`
			Wind     float64 `json:"wind_speed"`
			Rain     float64 `json:"rainfall"`
		}
		if json.Unmarshal(r.Data, &v) == nil {
			s.weather = &Weather{AirC: v.Air, TrackC: v.Track, Humidity: v.Humidity, WindMS: v.Wind, Rain: v.Rain > 0}
			s.rainChange(v.Rain > 0, v.Track)
		}
	case "overtakes":
		var v struct {
			By       int `json:"overtaking_driver_number"`
			Of       int `json:"overtaken_driver_number"`
			Position int `json:"position"`
		}
		if json.Unmarshal(r.Data, &v) == nil {
			s.overtakes = prepend(s.overtakes, Overtake{Time: r.Time, By: s.code(v.By), Of: s.code(v.Of), Position: v.Position})
			priority := 1
			if v.Position <= 10 {
				priority = 2
			}
			s.emit(KindOvertake, priority, []string{s.code(v.By), s.code(v.Of)}, "%s passes %s for P%d", s.code(v.By), s.code(v.Of), v.Position)
		}
	case "team_radio":
		var v struct {
			Number int    `json:"driver_number"`
			URL    string `json:"recording_url"`
		}
		if json.Unmarshal(r.Data, &v) == nil {
			s.radio = prepend(s.radio, Radio{Time: r.Time, Driver: s.code(v.Number), Lap: s.car(v.Number).Lap + 1, URL: v.URL})
			s.emit(KindRadio, 1, []string{s.code(v.Number)}, "Team radio: %s", s.code(v.Number))
		}
	case "session_result":
		var v struct {
			Number int  `json:"driver_number"`
			DNF    bool `json:"dnf"`
			DNS    bool `json:"dns"`
			DSQ    bool `json:"dsq"`
		}
		if json.Unmarshal(r.Data, &v) == nil {
			switch {
			case v.DSQ:
				s.car(v.Number).Out = "DSQ"
			case v.DNS:
				s.car(v.Number).Out = "DNS"
			case v.DNF:
				s.car(v.Number).Out = "DNF"
			}
		}
	}
}

func (s *State) applyLap(data json.RawMessage) {
	var v struct {
		Number   int       `json:"driver_number"`
		Lap      int       `json:"lap_number"`
		Start    time.Time `json:"date_start"`
		Duration *float64  `json:"lap_duration"`
		S1       *float64  `json:"duration_sector_1"`
		S2       *float64  `json:"duration_sector_2"`
		S3       *float64  `json:"duration_sector_3"`
		PitOut   bool      `json:"is_pit_out_lap"`
	}
	if json.Unmarshal(data, &v) != nil {
		return
	}
	c := s.car(v.Number)
	if v.Lap <= c.Lap {
		return
	}
	c.Lap = v.Lap
	s.started = true
	if s.flag != Ended {
		s.leaderLap = max(s.leaderLap, v.Lap+1)
	}
	if c.stintStart > 0 {
		c.TyreAge = c.stintAge + max(0, v.Lap+1-c.stintStart)
	}
	if v.Duration == nil {
		return
	}
	lt := &LapTime{Lap: v.Lap, Seconds: *v.Duration, Sectors: [3]*float64{v.S1, v.S2, v.S3}, PitOut: v.PitOut, Started: v.Start}
	for i, sec := range lt.Sectors {
		if sec == nil {
			continue
		}
		if c.BestSecs[i] == nil || *sec < *c.BestSecs[i] {
			c.BestSecs[i] = sec
		}
		if s.bestSecs[i] == 0 || *sec < s.bestSecs[i] {
			s.bestSecs[i] = *sec
			lt.Purple[i] = true
		}
	}
	c.LastLap = lt
	if !v.PitOut && (c.BestLap == nil || lt.Seconds < c.BestLap.Seconds) {
		c.BestLap = lt
	}
	if !v.PitOut && (s.best == nil || lt.Seconds < s.best.Seconds) {
		if s.best != nil {
			// Small improvements, common as a wet track dries, are minor.
			priority := 1
			if s.best.Seconds-lt.Seconds >= 0.2 {
				priority = 2
			}
			s.emit(KindFastestLap, priority, []string{c.Code}, "Fastest lap: %s, %s on lap %d", c.Code, lapTimeText(lt.Seconds), v.Lap)
		}
		s.best = &Best{Driver: c.Code, Lap: v.Lap, Seconds: lt.Seconds}
	}
	s.lapDone(c, lt)
}

func (s *State) applyMessage(r Record) {
	var v struct {
		Lap      *int    `json:"lap_number"`
		Category string  `json:"category"`
		Flag     *string `json:"flag"`
		Scope    *string `json:"scope"`
		Sector   *int    `json:"sector"`
		Number   *int    `json:"driver_number"`
		Message  string  `json:"message"`
	}
	if json.Unmarshal(r.Data, &v) != nil {
		return
	}
	m := Message{Time: r.Time, Message: v.Message}
	if v.Lap != nil {
		m.Lap = *v.Lap
		if s.flag != Ended {
			s.leaderLap = max(s.leaderLap, *v.Lap)
		}
	}
	if v.Flag != nil {
		m.Flag = *v.Flag
	}
	s.messages = prepend(s.messages, m)

	msg := v.Message
	prev := s.flag
	defer func() {
		if s.flag != prev {
			s.flagEvent(prev, m.Lap)
		}
	}()
	switch {
	case strings.Contains(msg, "VIRTUAL SAFETY CAR DEPLOYED"):
		s.flag = VSC
	case strings.Contains(msg, "SAFETY CAR DEPLOYED"):
		s.flag = SC
	case strings.Contains(msg, "VIRTUAL SAFETY CAR ENDING"), strings.Contains(msg, "SAFETY CAR IN THIS LAP"):
		// Racing resumes at the line; until then it's still neutralised.
	case m.Flag == "RED":
		s.flag = Red
	case m.Flag == "CHEQUERED":
		s.flag = Ended
	case (m.Flag == "GREEN" || m.Flag == "CLEAR") && v.Scope != nil && *v.Scope == "Track",
		m.Flag == "GREEN" && v.Scope == nil,
		s.flag == Red && (msg == "SESSION STARTED" || msg == "ROLLING START"):
		s.flag = Green
		clear(s.yellows)
	case (m.Flag == "YELLOW" || m.Flag == "DOUBLE YELLOW") && v.Sector != nil:
		s.yellows[*v.Sector] = true
		if s.flag == Green {
			s.flag = Yellow
		}
	case m.Flag == "CLEAR" && v.Sector != nil:
		delete(s.yellows, *v.Sector)
		if len(s.yellows) == 0 && s.flag == Yellow {
			s.flag = Green
		}
	}
	if v.Number == nil {
		if sm := carInMessage.FindStringSubmatch(msg); sm != nil {
			n, _ := strconv.Atoi(sm[1])
			v.Number = &n
		}
	}
	if v.Number != nil && strings.Contains(msg, "PENALTY") && !strings.Contains(msg, "NO FURTHER") {
		c := s.car(*v.Number)
		c.Penalty = append(c.Penalty, msg)
		s.emit(KindPenalty, 3, []string{c.Code}, "%s", penaltyText(msg, c.Code))
	}
}

// carInMessage finds the car a race control message is about, e.g.
// "CAR 81 (PIA)".
var carInMessage = regexp.MustCompile(`\bCAR (\d+) \(`)

// Snapshot returns a copy of the state.
func (s *State) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := Snapshot{Time: s.now, Session: s.session, Lap: s.leaderLap, TotalLaps: s.totalLaps, Flag: s.flag, Weather: s.weather, BestLap: s.best}
	for _, c := range s.cars {
		cc := *c
		cc.Stops = slices.Clone(c.Stops)
		cc.Penalty = slices.Clone(c.Penalty)
		snap.Cars = append(snap.Cars, cc)
	}
	// Running cars by position, then those out of the race.
	slices.SortFunc(snap.Cars, func(a, b Car) int {
		if (a.Out == "") != (b.Out == "") {
			if a.Out == "" {
				return -1
			}
			return 1
		}
		pa, pb := a.Position, b.Position
		if pa == 0 {
			pa = 999
		}
		if pb == 0 {
			pb = 999
		}
		if pa != pb {
			return pa - pb
		}
		return a.Number - b.Number
	})
	snap.Messages = slices.Clone(s.messages)
	snap.Overtakes = slices.Clone(s.overtakes)
	snap.Radio = slices.Clone(s.radio)
	return snap
}

// gapValue reads a gap: seconds, or text such as "+1 LAP".
func gapValue(raw json.RawMessage) (*float64, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ""
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return &f, ""
	}
	var t string
	if json.Unmarshal(raw, &t) == nil {
		return nil, t
	}
	return nil, ""
}

// prepend adds v at the front, keeping the most recent.
func prepend[T any](list []T, v T) []T {
	list = append([]T{v}, list...)
	if len(list) > recent {
		list = list[:recent]
	}
	return list
}

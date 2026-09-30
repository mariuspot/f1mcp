package replay

import (
	"compress/gzip"
	"container/heap"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/mariuspot/f1mcp/internal/live"
)

// Options adjust what a replay plays.
type Options struct {
	// SkipCars leaves out car_data and location, several records a second
	// for each car, when only the timing is needed.
	SkipCars bool
}

// Source plays a collected session back as the records a live feed would
// have sent, in the order they became known. Car data and location are
// read from their files as they're needed, not all at once.
type Source struct {
	Manifest Manifest
	heads    recordHeap
	files    []*os.File
}

// Open opens a session collected into dir.
func Open(dir string, o Options) (*Source, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("%s is not a collected session: %w", dir, err)
	}
	s := &Source{}
	if err := json.Unmarshal(b, &s.Manifest); err != nil {
		return nil, err
	}
	timed, err := timedRecords(dir, s.Manifest)
	if err != nil {
		return nil, err
	}
	s.push(&sliceStream{recs: timed})
	if !o.SkipCars {
		for _, name := range carEndpoints {
			for _, d := range s.Manifest.Drivers {
				st, f, err := openStream(filepath.Join(dir, name, strconv.Itoa(d)+".json.gz"), name)
				if err != nil {
					s.Close()
					return nil, err
				}
				s.files = append(s.files, f)
				s.push(st)
			}
		}
	}
	return s, nil
}

// Close closes the files being read.
func (s *Source) Close() error {
	for _, f := range s.files {
		f.Close()
	}
	return nil
}

func (s *Source) push(st stream) {
	if r, ok := st.next(); ok {
		heap.Push(&s.heads, head{r, st})
	}
}

// Next returns the next record in time order.
func (s *Source) Next() (live.Record, bool, error) {
	if s.heads.Len() == 0 {
		return live.Record{}, false, nil
	}
	h := heap.Pop(&s.heads).(head)
	if r, ok := h.st.next(); ok {
		heap.Push(&s.heads, head{r, h.st})
	}
	if err := h.st.err(); err != nil {
		return live.Record{}, false, err
	}
	return h.r, true, nil
}

// timedRecords reads the endpoints fetched for the whole session and gives
// each record the time a live feed would have sent it.
func timedRecords(dir string, m Manifest) ([]live.Record, error) {
	var out []live.Record
	add := func(topic string, t time.Time, data json.RawMessage) {
		if !t.IsZero() {
			out = append(out, live.Record{Topic: topic, Time: t, Data: data})
		}
	}
	before := m.Start.Add(-time.Hour) // what's known before the session
	read := func(name string) ([]json.RawMessage, error) {
		recs, err := readGz(filepath.Join(dir, name+".json.gz"))
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return recs, err
	}

	// Laps are known when they end; lap starts tell when stints begin.
	laps, err := read("laps")
	if err != nil {
		return nil, err
	}
	type lap struct {
		Number   int       `json:"driver_number"`
		Lap      int       `json:"lap_number"`
		Start    time.Time `json:"date_start"`
		Duration *float64  `json:"lap_duration"`
		raw      json.RawMessage
	}
	byCar := map[int][]lap{}
	for _, raw := range laps {
		var l lap
		if json.Unmarshal(raw, &l) == nil {
			l.raw = raw
			byCar[l.Number] = append(byCar[l.Number], l)
		}
	}
	lapStart := map[[2]int]time.Time{}
	for n, ls := range byCar {
		slices.SortFunc(ls, func(a, b lap) int { return a.Lap - b.Lap })
		for i, l := range ls {
			lapStart[[2]int{n, l.Lap}] = l.Start
			var end time.Time
			switch {
			case !l.Start.IsZero() && l.Duration != nil:
				end = l.Start.Add(time.Duration(*l.Duration * float64(time.Second)))
			case i+1 < len(ls):
				end = ls[i+1].Start
			}
			add("laps", end, l.raw)
		}
	}

	for _, name := range []string{"sessions", "meetings", "drivers"} {
		recs, err := read(name)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			add(name, before, r)
		}
	}

	stints, err := read("stints")
	if err != nil {
		return nil, err
	}
	for _, raw := range stints {
		var st struct {
			Number   int `json:"driver_number"`
			LapStart int `json:"lap_start"`
		}
		if json.Unmarshal(raw, &st) != nil {
			continue
		}
		t := lapStart[[2]int{st.Number, st.LapStart}]
		if t.IsZero() {
			t = m.Start
		}
		add("stints", t, raw)
	}

	pits, err := read("pit")
	if err != nil {
		return nil, err
	}
	for _, raw := range pits {
		var p struct {
			Date time.Time `json:"date"`
			Lane *float64  `json:"lane_duration"`
		}
		if json.Unmarshal(raw, &p) == nil {
			t := p.Date
			if p.Lane != nil {
				t = t.Add(time.Duration(*p.Lane * float64(time.Second)))
			}
			add("pit", t, raw)
		}
	}

	results, err := read("session_result")
	if err != nil {
		return nil, err
	}
	for _, raw := range results {
		add("session_result", m.End.Add(30*time.Second), raw)
	}

	for _, name := range []string{"position", "intervals", "race_control", "weather", "team_radio", "overtakes"} {
		recs, err := read(name)
		if err != nil {
			return nil, err
		}
		for _, raw := range recs {
			var d struct {
				Date time.Time `json:"date"`
			}
			if json.Unmarshal(raw, &d) == nil {
				add(name, d.Date, raw)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b live.Record) int { return a.Time.Compare(b.Time) })
	return out, nil
}

// A stream gives records in time order.
type stream interface {
	next() (live.Record, bool)
	err() error
}

type sliceStream struct {
	recs []live.Record
	i    int
}

func (s *sliceStream) next() (live.Record, bool) {
	if s.i >= len(s.recs) {
		return live.Record{}, false
	}
	s.i++
	return s.recs[s.i-1], true
}

func (s *sliceStream) err() error { return nil }

// fileStream reads a gzipped JSON array of dated records one at a time.
type fileStream struct {
	topic string
	dec   *json.Decoder
	e     error
}

func openStream(path, topic string) (*fileStream, *os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	dec := json.NewDecoder(zr)
	if _, err := dec.Token(); err != nil { // [
		f.Close()
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return &fileStream{topic: topic, dec: dec}, f, nil
}

func (s *fileStream) next() (live.Record, bool) {
	if s.e != nil || !s.dec.More() {
		return live.Record{}, false
	}
	var raw json.RawMessage
	if err := s.dec.Decode(&raw); err != nil {
		if err != io.EOF {
			s.e = err
		}
		return live.Record{}, false
	}
	var d struct {
		Date time.Time `json:"date"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		s.e = err
		return live.Record{}, false
	}
	return live.Record{Topic: s.topic, Time: d.Date, Data: raw}, true
}

func (s *fileStream) err() error { return s.e }

type head struct {
	r  live.Record
	st stream
}

type recordHeap []head

func (h recordHeap) Len() int           { return len(h) }
func (h recordHeap) Less(i, j int) bool { return h[i].r.Time.Before(h[j].r.Time) }
func (h recordHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *recordHeap) Push(x any)        { *h = append(*h, x.(head)) }
func (h *recordHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}

// Play feeds a source's records to apply in time order. Records before
// from are applied at once, to catch up; after it they're paced at speed
// times real time (0: as fast as possible). It stops at until, if set, or
// when ctx is done.
func Play(ctx context.Context, src live.Source, from, until time.Time, speed float64, apply func(live.Record)) error {
	var startWall time.Time
	for {
		r, ok, err := src.Next()
		if err != nil || !ok {
			return err
		}
		if !until.IsZero() && r.Time.After(until) {
			return nil
		}
		if speed > 0 && !r.Time.Before(from) {
			if startWall.IsZero() {
				startWall = time.Now()
			}
			due := startWall.Add(time.Duration(float64(r.Time.Sub(from)) / speed))
			if wait := time.Until(due); wait > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(wait):
				}
			}
		} else if err := ctx.Err(); err != nil {
			return err
		}
		apply(r)
	}
}

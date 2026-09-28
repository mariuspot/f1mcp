package openf1

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// Golden files in testdata/ hold real OpenF1 responses. Tests replay them by
// default; run `go test ./internal/openf1 -update` to re-fetch them from the
// live API. Fixtures use the 2023 Singapore Grand Prix (meeting 1219, race
// session 9165) and Max Verstappen (driver 1).
var update = flag.Bool("update", false, "re-fetch golden files from the live OpenF1 API")

// goldenClient returns a Client backed by a server that serves
// testdata/<name>.json, and fails the test if the client requests anything
// other than path?query. Query parameters may be sent in any order.
func goldenClient(t *testing.T, name, path, query string) *Client {
	t.Helper()
	file := filepath.Join("testdata", name+".json")
	if *update {
		fetchGolden(t, file, DefaultBaseURL+path+"?"+query)
	}
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading golden file (run with -update to create it): %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("path = %q, want %q", r.URL.Path, path)
		}
		if got, want := normalizeQuery(t, r.URL.RawQuery), normalizeQuery(t, query); !slices.Equal(got, want) {
			t.Errorf("query = %v, want %v", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, srv.Client())
}

func fetchGolden(t *testing.T, file, rawURL string) {
	t.Helper()
	resp, err := http.Get(rawURL)
	if err != nil {
		t.Fatalf("fetching %s: %v", rawURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading %s: %v", rawURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fetching %s: status %s: %s", rawURL, resp.Status, body)
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		t.Fatalf("indenting %s: %v", rawURL, err)
	}
	pretty.WriteByte('\n')
	if err := os.WriteFile(file, pretty.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	// Be polite to the public API when refreshing many files.
	time.Sleep(500 * time.Millisecond)
}

// normalizeQuery splits a raw query into sorted, unescaped parts. OpenF1
// filters like "date>2023-09-17T12:30:00Z" have no "=", so url.ParseQuery
// can't be used.
func normalizeQuery(t *testing.T, raw string) []string {
	t.Helper()
	var parts []string
	for p := range strings.SplitSeq(raw, "&") {
		u, err := url.QueryUnescape(p)
		if err != nil {
			t.Fatalf("unescaping %q: %v", p, err)
		}
		parts = append(parts, u)
	}
	slices.Sort(parts)
	return parts
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return tm
}

func TestMeetings(t *testing.T) {
	c := goldenClient(t, "meetings", "/meetings", "year=2023&country_name=Singapore")
	got, err := c.Meetings(context.Background(), MeetingsFilter{Year: 2023, CountryName: "Singapore"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d meetings, want 1", len(got))
	}
	m := got[0]
	if m.MeetingKey != 1219 || m.MeetingName != "Singapore Grand Prix" || m.CountryName != "Singapore" || m.Year != 2023 {
		t.Errorf("unexpected meeting: %+v", m)
	}
}

func TestSessions(t *testing.T) {
	c := goldenClient(t, "sessions", "/sessions", "meeting_key=1219")
	got, err := c.Sessions(context.Background(), SessionsFilter{MeetingKey: "1219"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d sessions, want 5", len(got))
	}
	race := got[4]
	if race.SessionKey != 9165 || race.SessionName != "Race" || !race.DateStart.Equal(mustTime(t, "2023-09-17T12:00:00Z")) {
		t.Errorf("unexpected race session: %+v", race)
	}
}

func TestDrivers(t *testing.T) {
	c := goldenClient(t, "drivers", "/drivers", "session_key=9165&driver_number=1")
	got, err := c.Drivers(context.Background(), SessionDriverFilter{SessionKey: "9165", DriverNumber: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d drivers, want 1", len(got))
	}
	d := got[0]
	if d.DriverNumber != 1 || d.FullName != "Max VERSTAPPEN" || d.NameAcronym != "VER" || d.TeamName != "Red Bull Racing" {
		t.Errorf("unexpected driver: %+v", d)
	}
}

func TestLaps(t *testing.T) {
	c := goldenClient(t, "laps", "/laps", "session_key=9165&driver_number=1&lap_number=8")
	got, err := c.Laps(context.Background(), LapsFilter{SessionKey: "9165", DriverNumber: 1, LapNumber: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d laps, want 1", len(got))
	}
	l := got[0]
	if l.LapNumber != 8 || l.LapDuration == nil || *l.LapDuration != 100.177 {
		t.Errorf("unexpected lap: %+v", l)
	}
	if l.DurationSector1 == nil || *l.DurationSector1 != 29.375 {
		t.Errorf("sector 1 = %v, want 29.375", l.DurationSector1)
	}
}

func TestStints(t *testing.T) {
	c := goldenClient(t, "stints", "/stints", "session_key=9165&driver_number=1")
	got, err := c.Stints(context.Background(), SessionDriverFilter{SessionKey: "9165", DriverNumber: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d stints, want 2", len(got))
	}
	s := got[0]
	if s.StintNumber != 1 || s.Compound != "HARD" || s.LapStart != 1 || s.LapEnd != 40 || s.TyreAgeAtStart != 0 {
		t.Errorf("unexpected stint: %+v", s)
	}
}

func TestPits(t *testing.T) {
	c := goldenClient(t, "pit", "/pit", "session_key=9165&driver_number=1")
	got, err := c.Pits(context.Background(), SessionDriverFilter{SessionKey: "9165", DriverNumber: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d pit stops, want 1", len(got))
	}
	p := got[0]
	if p.LapNumber != 40 || p.PitDuration == nil || *p.PitDuration != 29.6 {
		t.Errorf("unexpected pit stop: %+v", p)
	}
}

func TestRaceControl(t *testing.T) {
	c := goldenClient(t, "race_control", "/race_control", "session_key=9165")
	got, err := c.RaceControl(context.Background(), SessionFilter{SessionKey: "9165"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 76 {
		t.Fatalf("got %d messages, want 76", len(got))
	}
	i := slices.IndexFunc(got, func(m RaceControl) bool { return m.Message == "SAFETY CAR DEPLOYED" })
	if i < 0 {
		t.Fatal("no SAFETY CAR DEPLOYED message")
	}
	if m := got[i]; m.Category != "SafetyCar" || m.LapNumber == nil || *m.LapNumber != 20 {
		t.Errorf("unexpected safety car message: %+v", m)
	}
}

func TestWeather(t *testing.T) {
	c := goldenClient(t, "weather", "/weather", "session_key=9165")
	got, err := c.Weather(context.Background(), SessionFilter{SessionKey: "9165"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 176 {
		t.Fatalf("got %d weather samples, want 176", len(got))
	}
	w := got[0]
	if w.AirTemperature != 30.0 || w.TrackTemperature != 38.3 || w.Humidity != 68.0 {
		t.Errorf("unexpected weather: %+v", w)
	}
}

func TestIntervals(t *testing.T) {
	c := goldenClient(t, "intervals", "/intervals",
		"session_key=9165&driver_number=1&date>2023-09-17T12:30:00Z&date<2023-09-17T12:30:30Z")
	got, err := c.Intervals(context.Background(), WindowFilter{
		SessionKey:   "9165",
		DriverNumber: 1,
		After:        mustTime(t, "2023-09-17T12:30:00Z"),
		Before:       mustTime(t, "2023-09-17T12:30:30Z"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 {
		t.Fatalf("got %d intervals, want 6", len(got))
	}
	// TODO: assert gap values once we decide how to model "+1 LAP" style gaps.
	if i := got[0]; i.DriverNumber != 1 || !i.Date.Equal(mustTime(t, "2023-09-17T12:30:00.656Z")) {
		t.Errorf("unexpected interval: %+v", i)
	}
}

func TestPositions(t *testing.T) {
	c := goldenClient(t, "position", "/position", "session_key=9165&driver_number=1")
	got, err := c.Positions(context.Background(), SessionDriverFilter{SessionKey: "9165", DriverNumber: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 30 {
		t.Fatalf("got %d positions, want 30", len(got))
	}
	if p := got[0]; p.Position != 11 || !p.Date.Equal(mustTime(t, "2023-09-17T11:01:04.137Z")) {
		t.Errorf("unexpected position: %+v", p)
	}
}

func TestCarData(t *testing.T) {
	c := goldenClient(t, "car_data", "/car_data",
		"session_key=9165&driver_number=1&date>2023-09-17T12:30:00Z&date<2023-09-17T12:30:02Z")
	got, err := c.CarData(context.Background(), WindowFilter{
		SessionKey:   "9165",
		DriverNumber: 1,
		After:        mustTime(t, "2023-09-17T12:30:00Z"),
		Before:       mustTime(t, "2023-09-17T12:30:02Z"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 7 {
		t.Fatalf("got %d samples, want 7", len(got))
	}
	s := got[0]
	if s.Speed != 139 || s.NGear != 4 || s.RPM != 8397 || s.Throttle != 54 || s.Brake != 0 || s.DRS != 0 {
		t.Errorf("unexpected car data: %+v", s)
	}
}

// OpenF1 answers queries that match nothing with 404 {"detail":"No results found."}.
func TestNoResultsIsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"detail":"No results found."}`)
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.Client()).Laps(context.Background(), LapsFilter{SessionKey: "9165", LapNumber: 999})
	if err != nil {
		t.Fatalf("want no error for no results, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d laps, want 0", len(got))
	}
}

func TestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client()).Meetings(context.Background(), MeetingsFilter{Year: 2023}); err == nil {
		t.Fatal("want error for 500 response, got nil")
	}
}

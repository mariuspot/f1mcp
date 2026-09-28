package jolpica

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/mariuspot/f1mcp/internal/golden"
)

// Fixtures use the 2023 season, mostly round 15 (Singapore) and round 17
// (Qatar, a sprint weekend). See package golden for -update.

// goldenClient returns a Client backed by testdata/<name>.json that expects
// a request for path?limit=100.
func goldenClient(t *testing.T, name, path string) *Client {
	t.Helper()
	srv := golden.Serve(t, name, DefaultBaseURL, path, "limit=100")
	return NewClient(srv.URL, srv.Client())
}

func TestSchedule(t *testing.T) {
	c := goldenClient(t, "schedule_2023", "/2023/races/")
	got, err := c.Schedule(context.Background(), "2023")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 22 {
		t.Fatalf("got %d races, want 22", len(got))
	}
	sg := got[14]
	if sg.Round != "15" || sg.RaceName != "Singapore Grand Prix" || sg.Date != "2023-09-17" || sg.Time != "12:00:00Z" {
		t.Errorf("unexpected race: %+v", sg)
	}
	if sg.Circuit.CircuitID != "marina_bay" {
		t.Errorf("circuit = %q, want marina_bay", sg.Circuit.CircuitID)
	}
	if sg.Sprint != nil {
		t.Error("Singapore 2023 should have no sprint")
	}
	qatar := got[16]
	if qatar.Sprint == nil || qatar.SprintShootout == nil {
		t.Errorf("Qatar 2023 should have Sprint and SprintShootout: %+v", qatar)
	}
}

// 2024 renamed SprintShootout to SprintQualifying.
func TestScheduleSprintQualifying(t *testing.T) {
	c := goldenClient(t, "schedule_2024", "/2024/races/")
	got, err := c.Schedule(context.Background(), "2024")
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(got, func(r Race) bool { return r.Sprint != nil })
	if i < 0 {
		t.Fatal("no sprint weekend in 2024")
	}
	if r := got[i]; r.SprintQualifying == nil || r.SprintShootout != nil {
		t.Errorf("2024 sprint weekend should have SprintQualifying only: %+v", r)
	}
}

func TestRaceResults(t *testing.T) {
	c := goldenClient(t, "results_2023_15", "/2023/15/results/")
	got, err := c.RaceResults(context.Background(), "2023", "15")
	if err != nil {
		t.Fatal(err)
	}
	if got.RaceName != "Singapore Grand Prix" || len(got.Results) != 20 {
		t.Fatalf("got %s with %d results, want Singapore Grand Prix with 20", got.RaceName, len(got.Results))
	}
	win := got.Results[0]
	if win.Driver.Code != "SAI" || win.Constructor.ConstructorID != "ferrari" || win.Time == nil || win.Status != "Finished" {
		t.Errorf("unexpected winner: %+v", win)
	}
	last := got.Results[19]
	if last.PositionText != "W" || last.Status != "Withdrew" || last.Time != nil {
		t.Errorf("unexpected last entry: %+v", last)
	}
}

func TestQualifyingResults(t *testing.T) {
	c := goldenClient(t, "qualifying_2023_15", "/2023/15/qualifying/")
	got, err := c.QualifyingResults(context.Background(), "2023", "15")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.QualifyingResults) != 20 {
		t.Fatalf("got %d qualifying results, want 20", len(got.QualifyingResults))
	}
	pole := got.QualifyingResults[0]
	if pole.Driver.Code != "SAI" || pole.Q3 == "" {
		t.Errorf("unexpected pole: %+v", pole)
	}
}

func TestSprintResults(t *testing.T) {
	c := goldenClient(t, "sprint_2023_17", "/2023/17/sprint/")
	got, err := c.SprintResults(context.Background(), "2023", "17")
	if err != nil {
		t.Fatal(err)
	}
	if got.RaceName != "Qatar Grand Prix" || len(got.SprintResults) == 0 {
		t.Fatalf("got %s with %d sprint results", got.RaceName, len(got.SprintResults))
	}
	win := got.SprintResults[0]
	if win.Driver.Code != "PIA" || win.Points != "8" || win.Time == nil || win.Time.Time != "35:01.297" {
		t.Errorf("unexpected sprint winner: %+v", win)
	}
}

func TestPitStops(t *testing.T) {
	c := goldenClient(t, "pitstops_2023_15", "/2023/15/pitstops/")
	got, err := c.PitStops(context.Background(), "2023", "15")
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(got.PitStops, func(p PitStop) bool { return p.DriverID == "max_verstappen" })
	if i < 0 {
		t.Fatal("no pit stop for max_verstappen")
	}
	if p := got.PitStops[i]; p.Lap != "40" || p.Stop != "1" || p.Duration != "29.698" {
		t.Errorf("unexpected pit stop: %+v", p)
	}
}

func TestDriverStandings(t *testing.T) {
	c := goldenClient(t, "driverstandings_2023_15", "/2023/15/driverstandings/")
	got, err := c.DriverStandings(context.Background(), "2023", "15")
	if err != nil {
		t.Fatal(err)
	}
	if got.Round != "15" || len(got.DriverStandings) == 0 {
		t.Fatalf("unexpected standings list: round %s, %d entries", got.Round, len(got.DriverStandings))
	}
	lead := got.DriverStandings[0]
	if lead.Driver.Code != "VER" || lead.Points != "374" || lead.Wins != "12" || lead.Constructors[0].ConstructorID != "red_bull" {
		t.Errorf("unexpected leader: %+v", lead)
	}
}

func TestConstructorStandings(t *testing.T) {
	c := goldenClient(t, "constructorstandings_2023_15", "/2023/15/constructorstandings/")
	got, err := c.ConstructorStandings(context.Background(), "2023", "15")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ConstructorStandings) != 10 {
		t.Fatalf("got %d constructor standings, want 10", len(got.ConstructorStandings))
	}
	if lead := got.ConstructorStandings[0]; lead.Constructor.ConstructorID != "red_bull" || lead.Position != "1" {
		t.Errorf("unexpected leader: %+v", lead)
	}
}

func TestDrivers(t *testing.T) {
	c := goldenClient(t, "drivers_2023_15", "/2023/15/drivers/")
	got, err := c.Drivers(context.Background(), "2023", "15")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 20 {
		t.Fatalf("got %d drivers, want 20", len(got))
	}
	i := slices.IndexFunc(got, func(d Driver) bool { return d.DriverID == "max_verstappen" })
	if i < 0 {
		t.Fatal("no max_verstappen")
	}
	if d := got[i]; d.Code != "VER" || d.PermanentNumber != "3" {
		t.Errorf("unexpected driver: %+v", d)
	}
}

func TestConstructors(t *testing.T) {
	c := goldenClient(t, "constructors_2023", "/2023/constructors/")
	got, err := c.Constructors(context.Background(), "2023", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 10 {
		t.Fatalf("got %d constructors, want 10", len(got))
	}
	if !slices.ContainsFunc(got, func(c Constructor) bool { return c.ConstructorID == "red_bull" && c.Name == "Red Bull" }) {
		t.Error("no red_bull constructor")
	}
}

func TestSeasonDefaultsToCurrent(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		io.WriteString(w, `{"MRData":{"total":"0","RaceTable":{"Races":[]}}}`)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client()).Schedule(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if path != "/current/races/" {
		t.Errorf("path = %q, want /current/races/", path)
	}
}

// Jolpica answers unknown rounds with 200 and an empty list.
func TestUnknownRoundIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"MRData":{"total":"0","RaceTable":{"season":"2023","round":"99","Races":[]}}}`)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, srv.Client()).RaceResults(context.Background(), "2023", "99")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRoundRequired(t *testing.T) {
	if _, err := NewClient("http://unused", nil).RaceResults(context.Background(), "2023", ""); err == nil {
		t.Fatal("want error for empty round, got nil")
	}
}

func TestTooManyRowsIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"MRData":{"total":"250","DriverTable":{"Drivers":[]}}}`)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client()).Drivers(context.Background(), "2023", ""); err == nil {
		t.Fatal("want error when total exceeds page size, got nil")
	}
}

func TestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client()).Schedule(context.Background(), "abc"); err == nil {
		t.Fatal("want error for 404 response, got nil")
	}
}

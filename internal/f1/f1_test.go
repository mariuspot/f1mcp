package f1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mariuspot/f1mcp/internal/jolpica"
	"github.com/mariuspot/f1mcp/internal/openf1"
)

// fakeAPI serves the clients' saved responses by path, whatever the query,
// and 404s anything else. Fixtures are the 2023 season around round 15
// (Singapore) and round 17 (Qatar, a sprint weekend).
func fakeAPI(t *testing.T, dir string, routes map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile(filepath.Join(dir, file+".json"))
		if err != nil {
			t.Errorf("fixture %s: %v", file, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testService(t *testing.T, now string) *Service {
	t.Helper()
	jol := fakeAPI(t, "../jolpica/testdata", map[string]string{
		"/2023/races/":                   "schedule_2023",
		"/2023/15/results/":              "results_2023_15",
		"/2023/15/qualifying/":           "qualifying_2023_15",
		"/2023/17/sprint/":               "sprint_2023_17",
		"/2023/15/driverstandings/":      "driverstandings_2023_15",
		"/2023/15/constructorstandings/": "constructorstandings_2023_15",
		"/2023/15/drivers/":              "drivers_2023_15",
	})
	of1 := fakeAPI(t, "../openf1/testdata", map[string]string{
		"/meetings":   "meetings",
		"/sessions":   "sessions",
		"/drivers":    "drivers",
		"/team_radio": "team_radio",
		"/laps":       "laps",
	})
	s := New(jolpica.NewClient(jol.URL, jol.Client()), openf1.NewClient(of1.URL, of1.Client()))
	at, err := time.Parse(time.RFC3339, now)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return at }
	return s
}

func TestResolveEvent(t *testing.T) {
	s := testService(t, "2023-09-20T00:00:00Z") // after Singapore, before Japan
	ctx := context.Background()
	for _, tc := range []struct {
		round string
		want  int
	}{
		{"15", 15}, {"singapore", 15}, {"Marina Bay", 15}, {"last", 15}, {"", 15}, {"next", 16}, {"monaco", 6}, // Imola was cancelled in 2023
		{"spa", 12}, {"Spa-Francorchamps", 12}, {"barcelona", 7}, {"catalunya", 7}, {"silver", 10}, // a whole name or word beats part of a word
	} {
		e, err := s.ResolveEvent(ctx, 2023, tc.round)
		if err != nil {
			t.Errorf("round %q: %v", tc.round, err)
			continue
		}
		if e.Round != tc.want {
			t.Errorf("round %q = %d %s, want round %d", tc.round, e.Round, e.Name, tc.want)
		}
	}
	for _, bad := range []string{"99", "atlantis", "grand prix"} {
		if _, err := s.ResolveEvent(ctx, 2023, bad); err == nil {
			t.Errorf("round %q: want an error", bad)
		}
	}
}

func TestSchedule(t *testing.T) {
	s := testService(t, "2023-09-20T00:00:00Z")
	events, err := s.Schedule(context.Background(), 2023)
	if err != nil {
		t.Fatal(err)
	}
	sg := events[14]
	race, _ := sg.Start(Race)
	if sg.Circuit.ID != "marina_bay" || !race.Equal(time.Date(2023, 9, 17, 12, 0, 0, 0, time.UTC)) || sg.Sprint {
		t.Errorf("unexpected Singapore event: %+v", sg)
	}
	if sg.CountryFlagURL == "" {
		t.Error("Singapore has no flag link")
	}
	if !events[16].Sprint {
		t.Error("Qatar 2023 should be a sprint weekend")
	}
	if _, ok := events[16].Start(SprintQualifying); !ok {
		t.Error("Qatar 2023 should have a sprint qualifying (sprint shootout) time")
	}
}

func TestRaceResults(t *testing.T) {
	s := testService(t, "2023-09-20T00:00:00Z")
	e, _ := s.ResolveEvent(context.Background(), 2023, "15")
	rs, err := s.SessionResults(context.Background(), e, Race)
	if err != nil {
		t.Fatal(err)
	}
	win := rs[0]
	if win.Position != 1 || win.Driver.Code != "SAI" || win.Team != "Ferrari" || win.Points != 25 || win.TimeSeconds == nil {
		t.Errorf("unexpected winner: %+v", win)
	}
	if rs[1].GapSeconds == nil || *rs[1].GapSeconds <= 0 {
		t.Errorf("second place should have a gap: %+v", rs[1])
	}
	last := rs[len(rs)-1]
	if last.Position != 0 || last.Classified != "W" {
		t.Errorf("a withdrawn driver shouldn't have a position: %+v", last)
	}
}

func TestQualifyingAndSprintResults(t *testing.T) {
	s := testService(t, "2023-10-20T00:00:00Z")
	ctx := context.Background()
	sg, _ := s.ResolveEvent(ctx, 2023, "15")
	q, err := s.SessionResults(ctx, sg, Qualifying)
	if err != nil {
		t.Fatal(err)
	}
	if q[0].Driver.Code != "SAI" || q[0].Q3 == nil || *q[0].Q3 < 60 || *q[0].Q3 > 120 {
		t.Errorf("unexpected pole: %+v", q[0])
	}
	qatar, _ := s.ResolveEvent(ctx, 2023, "17")
	sp, err := s.SessionResults(ctx, qatar, Sprint)
	if err != nil {
		t.Fatal(err)
	}
	if sp[0].Driver.Code != "PIA" || sp[0].TimeSeconds == nil || *sp[0].TimeSeconds != 2101.297 {
		t.Errorf("unexpected sprint winner: %+v", sp[0])
	}
	if _, err := s.SessionResults(ctx, sg, Sprint); err == nil {
		t.Error("Singapore 2023 had no sprint; want an error")
	}
	if _, err := s.SessionResults(ctx, sg, "warmup"); err == nil {
		t.Error("want an error for an unknown session")
	}
}

func TestStandings(t *testing.T) {
	s := testService(t, "2023-09-20T00:00:00Z")
	ctx := context.Background()
	d, after, err := s.Standings(ctx, 2023, 15, DriversChampionship)
	if err != nil {
		t.Fatal(err)
	}
	if after != 15 || d[0].Driver.Code != "VER" || d[0].Points != 374 || d[0].Wins != 12 || d[0].Team != "Red Bull" {
		t.Errorf("unexpected leader after round %d: %+v", after, d[0])
	}
	teams, _, err := s.Standings(ctx, 2023, 15, TeamsChampionship)
	if err != nil {
		t.Fatal(err)
	}
	if teams[0].Team != "Red Bull" || teams[0].Driver != nil {
		t.Errorf("unexpected teams leader: %+v", teams[0])
	}
}

func TestDrivers(t *testing.T) {
	s := testService(t, "2023-09-20T00:00:00Z")
	ctx := context.Background()
	e, _ := s.ResolveEvent(ctx, 2023, "15")
	ds, err := s.Drivers(ctx, 2023, &e)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 20 {
		t.Fatalf("got %d drivers, want 20", len(ds))
	}
	for _, d := range ds {
		if d.Code != "VER" {
			continue
		}
		// Team name from Jolpica, colour and race number from OpenF1.
		if d.Team == nil || d.Team.ID != "red_bull" || d.Team.Name != "Red Bull" || d.Team.Color == "" || d.Number != 1 || d.HeadshotURL == "" {
			t.Errorf("unexpected Verstappen: %+v team %+v", d, d.Team)
		}
		return
	}
	t.Error("no VER")
}

func TestSeconds(t *testing.T) {
	for in, want := range map[string]float64{
		"1:30:58.421": 5458.421, "1:34.183": 94.183, "29.698": 29.698, "+5.366": 5.366,
	} {
		got := seconds(in)
		if got == nil || *got != want {
			t.Errorf("seconds(%q) = %v, want %v", in, got, want)
		}
	}
	for _, in := range []string{"", "+1 Lap", "DNF"} {
		if got := seconds(in); got != nil {
			t.Errorf("seconds(%q) = %v, want nil", in, *got)
		}
	}
}

func TestCacheKeepsPastSeasons(t *testing.T) {
	s := testService(t, "2025-01-01T00:00:00Z")
	ctx := context.Background()
	if _, err := s.Schedule(ctx, 2023); err != nil {
		t.Fatal(err)
	}
	for k, c := range s.cache {
		if strings.HasPrefix(k, "schedule/2023") && !c.expires.IsZero() {
			t.Errorf("a past season's schedule should be kept, but expires %v", c.expires)
		}
	}
}

// Package golden serves saved API responses to client tests.
//
// Golden files in a package's testdata/ hold real API responses. Tests replay
// them by default; run `go test ./internal/<pkg> -update -count=1` to
// re-fetch them from the live API.
package golden

import (
	"bytes"
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

var update = flag.Bool("update", false, "re-fetch golden files from the live API")

// Serve returns a test server that serves testdata/<name>.json, and fails the
// test if anything other than path?query is requested. Query parameters may
// be sent in any order. With -update, the file is first re-fetched from
// liveBaseURL+path?query.
func Serve(t *testing.T, name, liveBaseURL, path, query string) *httptest.Server {
	t.Helper()
	file := filepath.Join("testdata", name+".json")
	if *update {
		fetch(t, file, liveBaseURL+path+"?"+query)
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
	return srv
}

func fetch(t *testing.T, file, rawURL string) {
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
	// Be polite to public APIs when refreshing many files.
	time.Sleep(700 * time.Millisecond)
}

// normalizeQuery splits a raw query into sorted, unescaped parts. Filters
// like OpenF1's "date>2023-09-17T12:30:00Z" have no "=", so url.ParseQuery
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

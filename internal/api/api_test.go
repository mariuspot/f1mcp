package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariuspot/f1mcp/internal/f1"
	"github.com/mariuspot/f1mcp/internal/jolpica"
	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/tools"
)

// testAPI serves the API with clients that can't reach anything, which is
// enough for the tools that use only embedded track data.
func testAPI(t *testing.T) *httptest.Server {
	t.Helper()
	down := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, http.ErrServerClosed
	})}
	svc := f1.New(jolpica.NewClient("http://offline", down), openf1.NewClient("http://offline", down))
	r := tools.Register(mcp.NewServer(&mcp.Implementation{Name: "test"}, nil), svc)
	srv := httptest.NewServer(Handler(r, 1<<20))
	t.Cleanup(srv.Close)
	return srv
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func post(t *testing.T, url, body string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestListTools(t *testing.T) {
	srv := testAPI(t)
	resp, err := http.Get(srv.URL + "/api/tools")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var ts []struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		InputSchema map[string]any `json:"input_schema"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ts); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range ts {
		names[tl.Name] = true
		if tl.Description == "" || tl.InputSchema["type"] != "object" {
			t.Errorf("%s: description %q, schema %v", tl.Name, tl.Description, tl.InputSchema)
		}
	}
	for _, want := range []string{"get_schedule", "get_track_map", "get_lap_animation"} {
		if !names[want] {
			t.Errorf("no tool %s in %v", want, names)
		}
	}
}

func TestCallToolWithImage(t *testing.T) {
	srv := testAPI(t)
	resp, out := post(t, srv.URL+"/api/tools/get_track_map", `{"circuit": "monza", "corner": 1}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %v", resp.StatusCode, out)
	}
	if out["result"].(map[string]any)["circuit_id"] != "monza" {
		t.Errorf("result = %v", out["result"])
	}
	imgs := out["images"].([]any)
	if len(imgs) != 1 {
		t.Fatalf("images = %v", imgs)
	}
	img := imgs[0].(map[string]any)
	ir, err := http.Get(srv.URL + img["url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	ir.Body.Close()
	if ir.StatusCode != http.StatusOK || ir.Header.Get("Content-Type") != "image/png" {
		t.Errorf("image %s: status %d, type %s", img["url"], ir.StatusCode, ir.Header.Get("Content-Type"))
	}
}

func TestCallToolErrors(t *testing.T) {
	srv := testAPI(t)
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"/api/tools/nope", `{}`, http.StatusNotFound},
		{"/api/tools/get_track", `{"circuit": 5}`, http.StatusBadRequest},
		{"/api/tools/get_track", `{"circut": "spa"}`, http.StatusBadRequest},
		{"/api/tools/get_track", `{"circuit": "atlantis"}`, http.StatusUnprocessableEntity},
	} {
		resp, out := post(t, srv.URL+tc.path, tc.body)
		if resp.StatusCode != tc.status || out["error"] == "" {
			t.Errorf("%s %s: status %d %v, want %d with an error", tc.path, tc.body, resp.StatusCode, out, tc.status)
		}
	}
}

func TestImageStoreDropsOldest(t *testing.T) {
	s := newImageStore(10)
	a := s.put([]byte("aaaaaa"), "image/png")
	b := s.put([]byte("bbbbbb"), "image/png")
	if _, ok := s.get(a); ok {
		t.Error("oldest image kept past the limit")
	}
	if _, ok := s.get(b); !ok {
		t.Error("newest image dropped")
	}
	if s.put([]byte("bbbbbb"), "image/png") != b || len(s.order) != 1 {
		t.Error("same image stored twice")
	}
}

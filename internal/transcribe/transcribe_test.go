package transcribe

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTranscribeOnceAndKeep(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/clip.mp3":
			w.Write([]byte("fake mp3"))
		case "/audio/transcriptions":
			calls.Add(1)
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Errorf("authorization = %q", r.Header.Get("Authorization"))
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			f, _, err := r.FormFile("file")
			if err != nil {
				t.Fatal(err)
			}
			audio, _ := io.ReadAll(f)
			if string(audio) != "fake mp3" || r.FormValue("model") != DefaultModel || !strings.Contains(r.FormValue("prompt"), "Verstappen") {
				t.Errorf("request: audio %q, model %q, prompt %q", audio, r.FormValue("model"), r.FormValue("prompt"))
			}
			w.Write([]byte(`{"text": " Box, box. "}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	tr := New("test-key", dir, srv.Client()).WithBaseURL(srv.URL)
	for range 2 {
		text, err := tr.Transcribe(context.Background(), srv.URL+"/clip.mp3", "Max Verstappen")
		if err != nil || text != "Box, box." {
			t.Fatalf("Transcribe = %q, %v", text, err)
		}
	}
	// A new Transcriber with the same directory reads the kept transcript.
	again := New("test-key", dir, srv.Client()).WithBaseURL(srv.URL)
	if text, err := again.Transcribe(context.Background(), srv.URL+"/clip.mp3", ""); err != nil || text != "Box, box." {
		t.Fatalf("after restart: %q, %v", text, err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("transcribed %d times, want 1", n)
	}
}

func TestTranscribeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/clip.mp3" {
			w.Write([]byte("x"))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": {"message": "Incorrect API key"}}`))
	}))
	defer srv.Close()
	tr := New("bad", "", srv.Client()).WithBaseURL(srv.URL)
	if _, err := tr.Transcribe(context.Background(), srv.URL+"/clip.mp3", ""); err == nil || !strings.Contains(err.Error(), "Incorrect API key") {
		t.Errorf("err = %v", err)
	}
	if New("", "", nil) != nil {
		t.Error("New with no key should return nil")
	}
}

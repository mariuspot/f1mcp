// Package transcribe turns team radio clips into text with OpenAI's speech
// to text, keeping each transcript so a clip is only paid for once.
package transcribe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	DefaultBaseURL = "https://api.openai.com/v1"
	// DefaultModel is whisper-1: on team radio it transcribes the whole
	// clip, where gpt-4o-transcribe tends to stop at the first pause.
	DefaultModel = "whisper-1"
	maxClipBytes = 10 << 20
)

// Transcriber transcribes audio clips by URL. Transcripts are kept in memory
// and, if Dir is set, in files there, so they survive restarts.
type Transcriber struct {
	key     string
	model   string
	baseURL string
	dir     string
	http    *http.Client

	mu   sync.Mutex
	done map[string]string
}

// New returns a Transcriber using the OpenAI API key, or nil if key is
// empty. dir, if not empty, is where transcripts are kept.
func New(key, dir string, httpClient *http.Client) *Transcriber {
	if key == "" {
		return nil
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Transcriber{key: key, model: DefaultModel, baseURL: DefaultBaseURL, dir: dir, http: httpClient, done: map[string]string{}}
}

// WithBaseURL returns t calling a different API address, e.g. in tests.
func (t *Transcriber) WithBaseURL(u string) *Transcriber {
	t.baseURL = strings.TrimSuffix(u, "/")
	return t
}

// Transcribe returns what is said in the clip at audioURL. prompt primes
// the model with names and terms it is likely to hear.
func (t *Transcriber) Transcribe(ctx context.Context, audioURL, prompt string) (string, error) {
	id := cacheID(t.model, audioURL)
	t.mu.Lock()
	text, ok := t.done[id]
	t.mu.Unlock()
	if ok {
		return text, nil
	}
	if t.dir != "" {
		if b, err := os.ReadFile(filepath.Join(t.dir, id+".txt")); err == nil {
			return t.keep(id, string(b), false), nil
		}
	}
	audio, err := t.download(ctx, audioURL)
	if err != nil {
		return "", err
	}
	text, err = t.transcribe(ctx, audio, filepath.Base(audioURL), prompt)
	if err != nil {
		return "", err
	}
	return t.keep(id, text, true), nil
}

func (t *Transcriber) keep(id, text string, write bool) string {
	t.mu.Lock()
	t.done[id] = text
	t.mu.Unlock()
	if write && t.dir != "" {
		if err := os.MkdirAll(t.dir, 0o755); err == nil {
			// A failed write only costs a second transcription later.
			_ = os.WriteFile(filepath.Join(t.dir, id+".txt"), []byte(text), 0o644)
		}
	}
	return text
}

// cacheID names a clip's transcript by model, so changing the model
// transcribes clips again.
func cacheID(model, audioURL string) string {
	sum := sha256.Sum256([]byte(model + " " + audioURL))
	return hex.EncodeToString(sum[:16])
}

func (t *Transcriber) download(ctx context.Context, audioURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: %s", audioURL, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxClipBytes))
}

func (t *Transcriber) transcribe(ctx context.Context, audio []byte, name, prompt string) (string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("file", name)
	if err != nil {
		return "", err
	}
	fw.Write(audio)
	w.WriteField("model", t.model)
	w.WriteField("language", "en")
	w.WriteField("response_format", "json")
	if prompt != "" {
		w.WriteField("prompt", prompt)
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+t.key)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := t.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Text  string `json:"text"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("transcribing: %s: %w", resp.Status, err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := resp.Status
		if out.Error != nil {
			msg = out.Error.Message
		}
		return "", errors.New("transcribing: " + msg)
	}
	return strings.TrimSpace(out.Text), nil
}

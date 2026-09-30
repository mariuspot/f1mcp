// Package insight turns what's happening in a session into short
// commentary with Claude: why a car pitted, what an undercut means, who is
// quicker on which tyres.
package insight

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://api.anthropic.com/v1"
	DefaultModel   = "claude-sonnet-5"
	apiVersion     = "2023-06-01"
)

// Claude calls Anthropic's Messages API.
type Claude struct {
	key, model, baseURL string
	http                *http.Client
}

// NewClaude returns a client using the API key, or nil if it's empty.
// model may be empty for DefaultModel.
func NewClaude(key, model string, httpClient *http.Client) *Claude {
	if key == "" {
		return nil
	}
	if model == "" {
		model = DefaultModel
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &Claude{key: key, model: model, baseURL: DefaultBaseURL, http: httpClient}
}

// WithBaseURL returns c calling a different API address, e.g. in tests.
func (c *Claude) WithBaseURL(u string) *Claude {
	c.baseURL = strings.TrimSuffix(u, "/")
	return c
}

// Model is the model c uses.
func (c *Claude) Model() string { return c.model }

// Complete sends one user message with a system prompt and returns the
// text of the reply.
func (c *Claude) Complete(ctx context.Context, system, user string, maxTokens int) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":      c.model,
		"max_tokens": maxTokens,
		// The system prompt is the same for every call, so cache it.
		"system":   []map[string]any{{"type": "text", "text": system, "cache_control": map[string]string{"type": "ephemeral"}}},
		"messages": []map[string]any{{"role": "user", "content": user}},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", c.key)
	req.Header.Set("anthropic-version", apiVersion)
	req.Header.Set("content-type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("claude: %s: %w", resp.Status, err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := resp.Status
		if out.Error != nil {
			msg = out.Error.Message
		}
		return "", errors.New("claude: " + msg)
	}
	var text strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	return strings.TrimSpace(text.String()), nil
}

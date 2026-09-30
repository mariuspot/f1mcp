// Package openf1 is a client for the OpenF1 API (https://openf1.org).
package openf1

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const DefaultBaseURL = "https://api.openf1.org/v1"

// OpenF1 allows 3 requests per second and 30 per minute. The per-minute
// limiter allows a burst of 10 then one request every 3 seconds, so no
// 60-second window exceeds 30.
const (
	secondInterval = 350 * time.Millisecond
	minuteInterval = 3 * time.Second
	minuteBurst    = 10
	maxRetries     = 3
)

type Client struct {
	baseURL  string
	http     *http.Client
	perSec   *rate.Limiter
	perMin   *rate.Limiter
	retryGap time.Duration // wait before retrying a 429 without Retry-After
}

// NewClient returns a client for baseURL, or DefaultBaseURL if empty. A nil
// httpClient gets a default with a timeout.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		http:     httpClient,
		perSec:   rate.NewLimiter(rate.Every(secondInterval), 1),
		perMin:   rate.NewLimiter(rate.Every(minuteInterval), minuteBurst),
		retryGap: 5 * time.Second,
	}
}

// Filters. Zero values are left out of the query. Session and meeting keys
// are strings so callers can pass "latest".

type MeetingsFilter struct {
	Year        int
	CountryName string
}

type SessionsFilter struct {
	Year        int
	MeetingKey  string
	SessionKey  string
	SessionName string
}

type SessionFilter struct {
	SessionKey string
}

type SessionDriverFilter struct {
	SessionKey   string
	DriverNumber int
}

type LapsFilter struct {
	SessionKey   string
	DriverNumber int
	LapNumber    int
}

// WindowFilter limits high-frequency endpoints to a time range.
type WindowFilter struct {
	SessionKey   string
	DriverNumber int
	After        time.Time
	Before       time.Time
}

func (c *Client) Meetings(ctx context.Context, f MeetingsFilter) ([]Meeting, error) {
	var q query
	q.int("year", f.Year)
	q.str("country_name", f.CountryName)
	return get[Meeting](ctx, c, "/meetings", q)
}

func (c *Client) Sessions(ctx context.Context, f SessionsFilter) ([]Session, error) {
	var q query
	q.int("year", f.Year)
	q.str("meeting_key", f.MeetingKey)
	q.str("session_key", f.SessionKey)
	q.str("session_name", f.SessionName)
	return get[Session](ctx, c, "/sessions", q)
}

func (c *Client) Drivers(ctx context.Context, f SessionDriverFilter) ([]Driver, error) {
	return get[Driver](ctx, c, "/drivers", f.query())
}

func (c *Client) Laps(ctx context.Context, f LapsFilter) ([]Lap, error) {
	var q query
	q.str("session_key", f.SessionKey)
	q.int("driver_number", f.DriverNumber)
	q.int("lap_number", f.LapNumber)
	return get[Lap](ctx, c, "/laps", q)
}

func (c *Client) Stints(ctx context.Context, f SessionDriverFilter) ([]Stint, error) {
	return get[Stint](ctx, c, "/stints", f.query())
}

func (c *Client) Pits(ctx context.Context, f SessionDriverFilter) ([]Pit, error) {
	return get[Pit](ctx, c, "/pit", f.query())
}

func (c *Client) RaceControl(ctx context.Context, f SessionFilter) ([]RaceControl, error) {
	var q query
	q.str("session_key", f.SessionKey)
	return get[RaceControl](ctx, c, "/race_control", q)
}

// StartingGrid is the grid for the race that follows a qualifying session,
// given the qualifying session's key.
func (c *Client) StartingGrid(ctx context.Context, f SessionFilter) ([]GridSlot, error) {
	var q query
	q.str("session_key", f.SessionKey)
	return get[GridSlot](ctx, c, "/starting_grid", q)
}

func (c *Client) TeamRadio(ctx context.Context, f SessionDriverFilter) ([]TeamRadio, error) {
	return get[TeamRadio](ctx, c, "/team_radio", f.query())
}

func (c *Client) Weather(ctx context.Context, f SessionFilter) ([]Weather, error) {
	var q query
	q.str("session_key", f.SessionKey)
	return get[Weather](ctx, c, "/weather", q)
}

func (c *Client) Intervals(ctx context.Context, f WindowFilter) ([]Interval, error) {
	return get[Interval](ctx, c, "/intervals", f.query())
}

func (c *Client) Positions(ctx context.Context, f SessionDriverFilter) ([]Position, error) {
	return get[Position](ctx, c, "/position", f.query())
}

func (c *Client) CarData(ctx context.Context, f WindowFilter) ([]CarData, error) {
	return get[CarData](ctx, c, "/car_data", f.query())
}

func (c *Client) SessionResults(ctx context.Context, f SessionFilter) ([]SessionResult, error) {
	var q query
	q.str("session_key", f.SessionKey)
	return get[SessionResult](ctx, c, "/session_result", q)
}

func (c *Client) Locations(ctx context.Context, f WindowFilter) ([]Location, error) {
	return get[Location](ctx, c, "/location", f.query())
}

// RawFilter picks what Raw fetches: a session, and optionally a meeting
// instead, one car and a time window. Zero values don't filter.
type RawFilter struct {
	SessionKey   int
	MeetingKey   int
	DriverNumber int
	After        time.Time
	Before       time.Time
}

// Raw fetches an endpoint, e.g. "/car_data", and returns each record as
// OpenF1 sent it, with every field.
func (c *Client) Raw(ctx context.Context, path string, f RawFilter) ([]json.RawMessage, error) {
	var q query
	q.int("session_key", f.SessionKey)
	q.int("meeting_key", f.MeetingKey)
	q.int("driver_number", f.DriverNumber)
	q.date(">", f.After)
	q.date("<", f.Before)
	return get[json.RawMessage](ctx, c, path, q)
}

func (f SessionDriverFilter) query() query {
	var q query
	q.str("session_key", f.SessionKey)
	q.int("driver_number", f.DriverNumber)
	return q
}

func (f WindowFilter) query() query {
	var q query
	q.str("session_key", f.SessionKey)
	q.int("driver_number", f.DriverNumber)
	q.date(">", f.After)
	q.date("<", f.Before)
	return q
}

// query collects raw query parts. OpenF1 comparison filters such as
// "date>2023-09-17T12:30:00Z" aren't key=value pairs, so url.Values can't
// express them.
type query []string

func (q *query) str(key, v string) {
	if v != "" {
		*q = append(*q, key+"="+url.QueryEscape(v))
	}
}

func (q *query) int(key string, v int) {
	if v != 0 {
		*q = append(*q, key+"="+strconv.Itoa(v))
	}
}

func (q *query) date(op string, t time.Time) {
	if !t.IsZero() {
		*q = append(*q, "date"+op+t.UTC().Format(time.RFC3339Nano))
	}
}

// get fetches an endpoint and decodes its JSON array. OpenF1 answers queries
// that match nothing with 404 {"detail":"No results found."}; that is
// returned as an empty result, not an error.
func get[T any](ctx context.Context, c *Client, path string, q query) ([]T, error) {
	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + strings.Join(q, "&")
	}
	resp, err := c.do(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("openf1 %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if resp.StatusCode == http.StatusNotFound && isNoResults(body) {
			return nil, nil
		}
		return nil, fmt.Errorf("openf1 %s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}

	var out []T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("openf1 %s: decoding response: %w", path, err)
	}
	return out, nil
}

// do sends a rate-limited GET, retrying when OpenF1 answers 429.
func (c *Client) do(ctx context.Context, u string) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		if err := c.perSec.Wait(ctx); err != nil {
			return nil, err
		}
		if err := c.perMin.Wait(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.http.Do(req)
		if err != nil || resp.StatusCode != http.StatusTooManyRequests || attempt == maxRetries {
			return resp, err
		}
		wait := c.retryGap * time.Duration(attempt+1)
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
			wait = time.Duration(s) * time.Second
		}
		resp.Body.Close()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}

func isNoResults(body []byte) bool {
	var e struct {
		Detail string `json:"detail"`
	}
	return json.Unmarshal(body, &e) == nil && e.Detail == "No results found."
}

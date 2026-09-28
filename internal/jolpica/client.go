// Package jolpica is a client for the Jolpica F1 API
// (https://github.com/jolpica/jolpica-f1), the successor to Ergast.
package jolpica

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.jolpi.ca/ergast/f1"

// pageSize is the largest page Jolpica allows. Every endpoint this client
// calls fits in one page; get fails rather than silently truncating.
const pageSize = 100

// ErrNotFound is returned when a season, round or standings list doesn't
// exist. Jolpica reports this as 200 with an empty list.
var ErrNotFound = errors.New("jolpica: not found")

type Client struct {
	baseURL string
	http    *http.Client
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
	return &Client{baseURL: strings.TrimSuffix(baseURL, "/"), http: httpClient}
}

// Season is a year such as "2024" or "current". Round is a number such as
// "15" or "last".

// Schedule returns the race weekends of a season.
func (c *Client) Schedule(ctx context.Context, season string) ([]Race, error) {
	mr, err := c.get(ctx, seasonPath(season, "")+"/races/")
	if err != nil {
		return nil, err
	}
	return mr.RaceTable.Races, nil
}

func (c *Client) RaceResults(ctx context.Context, season, round string) (*Race, error) {
	return c.race(ctx, season, round, "results")
}

func (c *Client) QualifyingResults(ctx context.Context, season, round string) (*Race, error) {
	return c.race(ctx, season, round, "qualifying")
}

func (c *Client) SprintResults(ctx context.Context, season, round string) (*Race, error) {
	return c.race(ctx, season, round, "sprint")
}

func (c *Client) PitStops(ctx context.Context, season, round string) (*Race, error) {
	return c.race(ctx, season, round, "pitstops")
}

// DriverStandings returns the drivers' championship after round, or the
// latest standings of the season if round is empty.
func (c *Client) DriverStandings(ctx context.Context, season, round string) (*StandingsList, error) {
	return c.standings(ctx, season, round, "driverstandings")
}

// ConstructorStandings returns the constructors' championship after round,
// or the latest standings of the season if round is empty.
func (c *Client) ConstructorStandings(ctx context.Context, season, round string) (*StandingsList, error) {
	return c.standings(ctx, season, round, "constructorstandings")
}

// Drivers returns the drivers entered in a season, or in one round if round
// is not empty.
func (c *Client) Drivers(ctx context.Context, season, round string) ([]Driver, error) {
	mr, err := c.get(ctx, seasonPath(season, round)+"/drivers/")
	if err != nil {
		return nil, err
	}
	return mr.DriverTable.Drivers, nil
}

// Constructors returns the constructors entered in a season, or in one round
// if round is not empty.
func (c *Client) Constructors(ctx context.Context, season, round string) ([]Constructor, error) {
	mr, err := c.get(ctx, seasonPath(season, round)+"/constructors/")
	if err != nil {
		return nil, err
	}
	return mr.ConstructorTable.Constructors, nil
}

func (c *Client) race(ctx context.Context, season, round, endpoint string) (*Race, error) {
	if round == "" {
		return nil, fmt.Errorf("jolpica %s: round is required", endpoint)
	}
	mr, err := c.get(ctx, seasonPath(season, round)+"/"+endpoint+"/")
	if err != nil {
		return nil, err
	}
	if len(mr.RaceTable.Races) == 0 {
		return nil, ErrNotFound
	}
	return &mr.RaceTable.Races[0], nil
}

func (c *Client) standings(ctx context.Context, season, round, endpoint string) (*StandingsList, error) {
	mr, err := c.get(ctx, seasonPath(season, round)+"/"+endpoint+"/")
	if err != nil {
		return nil, err
	}
	if len(mr.StandingsTable.StandingsLists) == 0 {
		return nil, ErrNotFound
	}
	return &mr.StandingsTable.StandingsLists[0], nil
}

// seasonPath builds "/{season}[/{round}]", defaulting season to "current".
func seasonPath(season, round string) string {
	if season == "" {
		season = "current"
	}
	p := "/" + url.PathEscape(season)
	if round != "" {
		p += "/" + url.PathEscape(round)
	}
	return p
}

func (c *Client) get(ctx context.Context, path string) (*mrData, error) {
	u := c.baseURL + path + "?limit=" + strconv.Itoa(pageSize)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jolpica %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("jolpica %s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}

	var out struct {
		MRData mrData `json:"MRData"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("jolpica %s: decoding response: %w", path, err)
	}
	if total, _ := strconv.Atoi(out.MRData.Total); total > pageSize {
		return nil, fmt.Errorf("jolpica %s: %d rows exceeds page size %d", path, total, pageSize)
	}
	return &out.MRData, nil
}

package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/mariuspot/f1mcp/internal/f1"
	"github.com/mariuspot/f1mcp/internal/openf1"
	"github.com/mariuspot/f1mcp/internal/replay"
)

// collect saves everything OpenF1 has about a session into dir, or
// replays/<year>-<country>-<session>, for replaying it as if live.
func collect(ctx context.Context, sessionKey int, dir string) error {
	c := openf1.NewClient("", nil)
	if dir == "" {
		sessions, err := c.Sessions(ctx, openf1.SessionsFilter{SessionKey: strconv.Itoa(sessionKey)})
		if err != nil || len(sessions) == 0 {
			return fmt.Errorf("session %d: %v", sessionKey, err)
		}
		s := sessions[0]
		dir = fmt.Sprintf("replays/%d-%s-%s", s.Year, f1.Slug(s.CountryName), f1.Slug(s.SessionName))
	}
	start := time.Now()
	m, err := replay.Collect(ctx, c, sessionKey, dir, log.Printf)
	if err != nil {
		return err
	}
	total := 0
	for _, n := range m.Records {
		total += n
	}
	log.Printf("%s: %d %s %s, %d drivers, %d records in %s", dir, m.Year, m.Country, m.Session, len(m.Drivers), total, time.Since(start).Round(time.Second))
	return nil
}

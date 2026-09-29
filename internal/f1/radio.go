package f1

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mariuspot/f1mcp/internal/openf1"
)

// RadioClip is a clip of team radio that was broadcast during a session.
type RadioClip struct {
	Time   time.Time `json:"time"`
	Lap    int       `json:"lap,omitempty"` // the driver's lap when it was broadcast
	Driver DriverRef `json:"driver"`
	// URL is Formula 1's recording, an MP3. It's linked to, not copied.
	URL string `json:"url"`
	// Transcript is what was said, by speech to text, when a transcriber
	// is set up.
	Transcript string `json:"transcript,omitempty"`
}

// Transcriber turns a recording into text; see package transcribe.
type Transcriber interface {
	Transcribe(ctx context.Context, audioURL, prompt string) (string, error)
}

// WithTranscriber sets the transcriber for team radio clips.
func (s *Service) WithTranscriber(t Transcriber) *Service {
	s.transcriber = t
	return s
}

// RadioFilter narrows down clips. Zero values don't filter.
type RadioFilter struct {
	Driver         int // car number
	FromLap, ToLap int
	After, Before  time.Time
}

// TeamRadio returns the clips broadcast in a session or a part of
// qualifying, oldest first, without transcripts (see TranscribeClips).
func (s *Service) TeamRadio(ctx context.Context, e Event, session, driver string, fromLap, toLap int) ([]RadioClip, error) {
	sd, err := s.lapSession(ctx, e, session)
	if err != nil {
		return nil, err
	}
	f := RadioFilter{FromLap: fromLap, ToLap: toLap}
	if driver != "" {
		if f.Driver, err = sd.driver(driver); err != nil {
			return nil, err
		}
	}
	return s.radio(ctx, sd, f)
}

// RadioBetween returns the clips of an OpenF1 session broadcast between two
// times, for one car or (driver 0) all, without transcripts.
func (s *Service) RadioBetween(ctx context.Context, sessionKey int, from, to time.Time, driver int) ([]RadioClip, error) {
	sd, err := s.sessionByKey(ctx, sessionKey)
	if err != nil {
		return nil, err
	}
	return s.radio(ctx, sd, RadioFilter{Driver: driver, After: from, Before: to})
}

// sessionByKey is session for an OpenF1 session key, with its drivers.
func (s *Service) sessionByKey(ctx context.Context, sessionKey int) (*sessionData, error) {
	key := strconv.Itoa(sessionKey)
	sessions, err := memo(s, "openf1/session/"+key, 0, func() ([]openf1.Session, error) {
		return s.of1.Sessions(ctx, openf1.SessionsFilter{SessionKey: key})
	})
	if err != nil || len(sessions) == 0 {
		return nil, fmt.Errorf("session %d: %v", sessionKey, err)
	}
	sess := sessions[0]
	ds, err := memo(s, "openf1/drivers/"+key, sess.Year, func() ([]openf1.Driver, error) {
		return s.of1.Drivers(ctx, openf1.SessionDriverFilter{SessionKey: key})
	})
	if err != nil {
		return nil, err
	}
	sd := &sessionData{Event: Event{Year: sess.Year, Name: sess.CountryName}, Session: sess.SessionName, key: key, year: sess.Year, drivers: map[int]DriverRef{}, teams: map[int]string{}}
	for _, d := range ds {
		sd.drivers[d.DriverNumber] = DriverRef{Code: d.NameAcronym, Number: d.DriverNumber, Name: d.FirstName + " " + d.LastName}
		sd.teams[d.DriverNumber] = d.TeamName
	}
	return sd, nil
}

func (s *Service) radio(ctx context.Context, sd *sessionData, f RadioFilter) ([]RadioClip, error) {
	clips, err := memo(s, "openf1/team_radio/"+sd.key, sd.year, func() ([]openf1.TeamRadio, error) {
		return s.of1.TeamRadio(ctx, openf1.SessionDriverFilter{SessionKey: sd.key})
	})
	if err != nil {
		return nil, err
	}
	// Every lap of the session, to tell which lap each clip was on.
	laps, err := memo(s, "openf1/laps/"+sd.key, sd.year, func() ([]openf1.Lap, error) {
		return s.of1.Laps(ctx, openf1.LapsFilter{SessionKey: sd.key})
	})
	if err != nil {
		return nil, err
	}
	starts := map[int][]openf1.Lap{}
	for _, l := range laps {
		if !l.DateStart.IsZero() {
			starts[l.DriverNumber] = append(starts[l.DriverNumber], l)
		}
	}
	for _, ls := range starts {
		slices.SortFunc(ls, func(a, b openf1.Lap) int { return a.DateStart.Compare(b.DateStart) })
	}

	var out []RadioClip
	for _, c := range clips {
		if f.Driver != 0 && c.DriverNumber != f.Driver {
			continue
		}
		if sd.part > 0 && (c.Date.Before(sd.partFrom) || (!sd.partTo.IsZero() && !c.Date.Before(sd.partTo))) {
			continue
		}
		if (!f.After.IsZero() && c.Date.Before(f.After)) || (!f.Before.IsZero() && c.Date.After(f.Before)) {
			continue
		}
		lap := 0
		for _, l := range starts[c.DriverNumber] {
			if l.DateStart.After(c.Date) {
				break
			}
			lap = l.LapNumber
		}
		if (f.FromLap > 0 && lap < f.FromLap) || (f.ToLap > 0 && lap > f.ToLap) {
			continue
		}
		out = append(out, RadioClip{Time: c.Date.UTC(), Lap: lap, Driver: sd.ref(c.DriverNumber), URL: c.RecordingURL})
	}
	slices.SortFunc(out, func(a, b RadioClip) int { return a.Time.Compare(b.Time) })
	return out, nil
}

// maxTranscribe is the most clips transcribed at once.
const maxTranscribe = 4

// TranscribeClips adds transcripts to clips, if a transcriber is set up. A
// clip that can't be transcribed is left without one.
func (s *Service) TranscribeClips(ctx context.Context, clips []RadioClip) {
	if s.transcriber == nil || len(clips) == 0 {
		return
	}
	prompt := radioPrompt(clips)
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxTranscribe)
	for i := range clips {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			text, err := s.transcriber.Transcribe(ctx, clips[i].URL, prompt)
			if err != nil {
				log.Printf("transcribing %s: %v", clips[i].URL, err)
				return
			}
			clips[i].Transcript = text
		}()
	}
	wg.Wait()
}

// radioPrompt primes speech to text with the drivers heard and F1 terms.
func radioPrompt(clips []RadioClip) string {
	var names []string
	for _, c := range clips {
		if n := c.Driver.Name; n != "" && !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	return "Formula 1 team radio between a driver and their race engineer. Drivers: " + strings.Join(names, ", ") +
		". Terms: box, pit, DRS, safety car, VSC, softs, mediums, hards, inters, undercut, delta, push, plan A, plan B."
}

package insight

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mariuspot/f1mcp/internal/live"
)

// PromptVersion is part of each cached insight's key: change it when the
// prompt changes, so replays get fresh insights rather than old ones.
const PromptVersion = "7"

// System is the insight agent's instructions.
const System = `You are the strategy analyst on a Formula 1 pit wall. Fans are following the live timing screen; when something happens, you add one sharp insight they can't read off the screen.

You get the timing tower now, the events since your last comment, recent race control messages, and your recent insights.

How to read the data:
- Gaps: "to leader" and "to car ahead" in seconds. Behind a safety car or VSC, or under a red flag, the field is neutralised: gaps shrink or freeze and say nothing about pace, and nobody may overtake on track. A change of places then comes from pit stops.
- Pace: the average of a driver's last 3 racing laps (green flag, not in or out of the pits). Compare drivers only on pace. A last lap marked neutralised, in-lap or out-lap is not a pace lap; never compare those.
- Tyres: the compound and how many laps that set has done (a set may have been used before). What matters for pace and strategy is tyre age, not the number of stops.
- Stops: pit stops, with the laps they were made on. Tyres changed while the race was stopped under a red flag are shown separately; they cost no time and aren't pit stops.
- Pit lane time: the typical time a green-flag stop costs this race. Under a VSC a stop costs noticeably less, and behind a safety car much less, because everyone else is slowed.
- "passes" is a place gained while racing. "moves ahead" is a place gained because the other car pitted or the field was neutralised: not a pass, and it says nothing about pace.
- An undercut is pitting before a rival and gaining on fresh tyres while they stay out; two cars pitting together is not an undercut.
- Under a red flag, every team may change tyres for free before the restart, so tyre age before a red flag says nothing about the restart.
- Track limits: a deleted lap time only removes that lap from the records. It costs no time or position and says nothing about pace. Repeat offences can bring a warning and then a time penalty, but only race control decides that.
- Penalties: a time penalty is served at the driver's next pit stop or added to their race time at the end. Don't say which, or when, unless race control does.
- Don't state restart procedures (standing or rolling), how a penalty will be served, or how many laps remain unless race control says so.

What to write:
- Why something probably happened: a stop timed to a VSC or safety car, an undercut or overcut, worn tyres, rain or a drying track, a penalty.
- Or what it means: who gains or loses, who still needs fresher tyres, who is exposed to the car behind.
- Or who is quicker, on what tyres and how old, from the pace figures.
- One or two sentences, at most 45 words. No preamble, lists or emoji. Plain words; no jargon or terms you'd need to explain.
- Name drivers only by their three-letter codes in capitals, exactly as given (VER, NOR), never by name. Gaps and times in seconds.

Be accurate:
- Every fact must come from the data. Before writing "everyone", "only", "all" or "still", check each car in the tower.
- Before saying anything about a driver (tyres, stops, penalty, pace), check that driver's own line. Only the drivers listed under "Penalties so far" have penalties.
- A pit event says whether the stop was under a VSC or safety car. Events arrive in order but in batches: a driver's stop may be reported just after a flag changes.
- Before saying a driver has or hasn't stopped, or is on old or fresh tyres, check their tyre age and stops.
- Mark anything inferred with "likely" or "looks like". Never state a prediction as certain.
- A tyre age difference of a lap or two means little, and one pass says little about a car's or team's pace; don't draw conclusions from either.
- Add to the events rather than restating them. Don't return to a point from your recent insights (the same driver's pace, the same threat) unless something has changed; find a different angle or SKIP.
- If there's nothing worth adding, answer exactly SKIP. Routine midfield moves rarely need a comment.`

// Prompt describes the moment for the insight agent.
func Prompt(s live.Snapshot, events []live.Event, messages []live.Message, said []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Session: %s. Lap %d. Track: %s.", s.Session, s.Lap, flagText(s.Flag))
	if s.Weather != nil {
		rain := "dry"
		if s.Weather.Rain {
			rain = "raining"
		}
		fmt.Fprintf(&b, " Weather: %s, track %.0f °C.", rain, s.Weather.TrackC)
	}
	if loss, n := pitLoss(s.Cars); n > 0 {
		fmt.Fprintf(&b, " Pit lane time this race: about %.0f s (%d stops).", loss, n)
	}
	if s.BestLap != nil {
		fmt.Fprintf(&b, " Fastest lap: %s %s.", s.BestLap.Driver, lapText(s.BestLap.Seconds))
	}
	var penalties, notRunning []string
	for _, c := range s.Cars {
		for _, p := range c.Penalty {
			penalties = append(penalties, c.Code+" ("+penaltyText(p)+")")
		}
		if why := notRunningText(c, s); why != "" {
			notRunning = append(notRunning, c.Code+" ("+why+")")
		}
	}
	if len(penalties) > 0 {
		fmt.Fprintf(&b, "\nPenalties so far, and only these: %s.", strings.Join(penalties, "; "))
	}
	if len(notRunning) > 0 {
		fmt.Fprintf(&b, "\nNot running: %s.", strings.Join(notRunning, "; "))
	}
	b.WriteString("\n\nTiming tower:\n")
	for _, c := range s.Cars {
		b.WriteString(towerLine(c, s))
		b.WriteString("\n")
	}
	b.WriteString("\nNew events:\n")
	for _, e := range events {
		fmt.Fprintf(&b, "- lap %d %s: %s\n", e.Lap, e.Time.UTC().Format("15:04:05"), e.Text)
	}
	if rc := notable(messages, 6); len(rc) > 0 {
		b.WriteString("\nRecent race control (oldest first):\n")
		for _, m := range slices.Backward(rc) {
			fmt.Fprintf(&b, "- %s\n", m.Message)
		}
	}
	if len(said) > 0 {
		b.WriteString("\nYou said recently:\n")
		for _, t := range said {
			fmt.Fprintf(&b, "- %s\n", t)
		}
	}
	b.WriteString("\nYour insight (or SKIP):")
	return b.String()
}

func flagText(f live.Flag) string {
	switch f {
	case live.SC:
		return "safety car"
	case live.VSC:
		return "virtual safety car"
	case live.Red:
		return "red flag"
	case live.Yellow:
		return "yellow flag in a sector"
	case live.Ended:
		return "chequered flag"
	}
	return "green"
}

// pitLoss is the median time in the pit lane of the stops so far, and how
// many there were. Stops of over two minutes (red flags, repairs) are left
// out.
func pitLoss(cars []live.Car) (float64, int) {
	var lanes []float64
	for _, c := range cars {
		for _, st := range c.Stops {
			if st.LaneSeconds != nil && *st.LaneSeconds < 120 {
				lanes = append(lanes, *st.LaneSeconds)
			}
		}
	}
	if len(lanes) == 0 {
		return 0, 0
	}
	slices.Sort(lanes)
	return lanes[len(lanes)/2], len(lanes)
}

func lapText(s float64) string {
	m := int(s) / 60
	return fmt.Sprintf("%d:%06.3f", m, s-float64(m*60))
}

// towerLine describes one car for the insight agent, e.g. "P3 OCO (Alpine):
// +12.9 to leader, +2.1 to car ahead | intermediates, 28 laps old | stops:
// none | pace 1:25.412 | last lap 2:38.230, neutralised".
func towerLine(c live.Car, s live.Snapshot) string {
	name := fmt.Sprintf("%s (%s)", c.Code, c.Team)
	if why := notRunningText(c, s); why != "" {
		return fmt.Sprintf("%s: %s", name, why)
	}
	parts := []string{}
	switch {
	case c.Position == 1:
		parts = append(parts, "leader")
	case c.GapText != "":
		parts = append(parts, c.GapText+" to leader")
	case c.GapToLeader != nil:
		gap := fmt.Sprintf("+%.1f to leader", *c.GapToLeader)
		if c.Interval != nil {
			gap += fmt.Sprintf(", +%.1f to car ahead", *c.Interval)
		}
		parts = append(parts, gap)
	}
	if c.Compound != "" {
		tyre := fmt.Sprintf("%s, %d laps old", compoundName(c.Compound), c.TyreAge)
		if c.RedFlagChanges > 0 {
			tyre += " (changed under the red flag)"
		}
		parts = append(parts, tyre)
	}
	stops := "stops: none"
	if len(c.Stops) > 0 {
		var laps []string
		for _, st := range c.Stops {
			laps = append(laps, fmt.Sprintf("lap %d", st.Lap))
		}
		stops = "stops: " + strings.Join(laps, ", ")
	}
	parts = append(parts, stops)
	if c.Pace != nil {
		parts = append(parts, "pace "+lapText(*c.Pace))
	} else {
		parts = append(parts, "pace n/a (under 3 racing laps)")
	}
	if l := c.LastLap; l != nil {
		last := "last lap " + lapText(l.Seconds)
		switch {
		case l.Neutral:
			last += ", neutralised"
		case l.PitOut:
			last += ", out-lap"
		case slices.ContainsFunc(c.Stops, func(st live.PitStop) bool { return st.Lap == l.Lap }):
			last += ", in-lap"
		}
		parts = append(parts, last)
	}
	if c.TrackLimits > 0 {
		parts = append(parts, fmt.Sprintf("%d lap time(s) deleted for track limits", c.TrackLimits))
	}
	for _, p := range c.Penalty {
		parts = append(parts, "penalty: "+penaltyText(p))
	}
	return fmt.Sprintf("P%d %s: %s", c.Position, name, strings.Join(parts, " | "))
}

func compoundName(c string) string {
	switch c {
	case "INTERMEDIATE":
		return "intermediates"
	case "WET":
		return "wets"
	}
	return strings.ToLower(c) + "s"
}

// penaltyText tidies a stewards' penalty message.
func penaltyText(msg string) string {
	msg = strings.TrimPrefix(msg, "FIA STEWARDS: ")
	if i := strings.Index(msg, " FOR CAR "); i >= 0 {
		rest := msg[i:]
		if j := strings.Index(rest, " - "); j >= 0 {
			msg = msg[:i] + " -" + rest[j+2:]
		} else {
			msg = msg[:i]
		}
	}
	return strings.ToLower(msg)
}

// notable returns up to n race control messages worth knowing, dropping
// sector yellows and clears and deleted lap times, which the tower already
// covers.
func notable(messages []live.Message, n int) []live.Message {
	var out []live.Message
	for _, m := range messages {
		if strings.Contains(m.Message, "IN TRACK SECTOR") || strings.Contains(m.Message, "TRACK LIMITS AT TURN") {
			continue
		}
		out = append(out, m)
		if len(out) == n {
			break
		}
	}
	return out
}

// stoppedAfter is how long a car can go without completing a lap, with the
// race running, before it's taken to have stopped.
const stoppedAfter = 4 * time.Minute

// notRunningText says why a car isn't racing, or "" if it is.
func notRunningText(c live.Car, s live.Snapshot) string {
	switch {
	case c.Out != "":
		return "out of the race (" + c.Out + ")"
	case c.Lap == 0 && s.Lap > 2:
		return "not running"
	case c.LastLap != nil && s.Flag != live.Red && s.Flag != live.Ended:
		end := c.LastLap.Started.Add(time.Duration(c.LastLap.Seconds * float64(time.Second)))
		if idle := s.Time.Sub(end); !c.LastLap.Started.IsZero() && idle > stoppedAfter {
			return fmt.Sprintf("no lap completed for %.0f min, likely stopped or retired", idle.Minutes())
		}
	}
	return ""
}

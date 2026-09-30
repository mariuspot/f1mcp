package insight

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mariuspot/f1mcp/internal/live"
)

// PromptVersion is part of each cached insight's key: change it when the
// prompt changes, so replays get fresh insights rather than old ones.
const PromptVersion = "2"

// System is the insight agent's instructions.
const System = `You are the analyst on a Formula 1 pit wall, writing live insight for fans following the timing screen.

You get the timing tower as it stands, the events since your last comment, recent race control messages, and what you said recently. Write what a sharp engineer would add that the screen doesn't say:
- why something probably happened: a stop timed to a safety car or VSC (cheaper stop), an undercut or overcut, tyres too old, a penalty to serve, a change of weather;
- what it means: who gains or loses, the pit window it opens, the threat from behind, how the order could shake out;
- pace and tyres: who is faster, on what, and whether it will last.

Rules:
- One or two short sentences, at most 45 words. No preamble, no lists, no emoji.
- Use drivers' three-letter codes as given. Gaps and times in seconds.
- Only use facts in the data. When you infer, say "likely" or "looks like".
- Don't restate an event as it's already shown; add to it. Don't repeat what you said recently.
- If nothing is worth adding, answer exactly SKIP.`

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
	b.WriteString("\n\nTiming tower:\n")
	for _, c := range s.Cars {
		if c.Out != "" {
			fmt.Fprintf(&b, "%s %s: %s\n", c.Code, c.Team, c.Out)
			continue
		}
		gap := "leader"
		switch {
		case c.Position != 1 && c.GapText != "":
			gap = c.GapText
		case c.Position != 1 && c.GapToLeader != nil:
			gap = fmt.Sprintf("+%.1f", *c.GapToLeader)
			if c.Interval != nil {
				gap += fmt.Sprintf(" (int %.1f)", *c.Interval)
			}
		}
		tyre := "?"
		if c.Compound != "" {
			tyre = fmt.Sprintf("%s %d laps", strings.ToLower(c.Compound), c.TyreAge)
		}
		last := ""
		if c.LastLap != nil {
			last = ", last " + lapText(c.LastLap.Seconds)
		}
		pen := ""
		if len(c.Penalty) > 0 {
			pen = fmt.Sprintf(", %d penalty", len(c.Penalty))
		}
		stops := fmt.Sprintf("%d stops", len(c.Stops))
		if c.RedFlagChanges > 0 {
			stops += fmt.Sprintf(" (+%d tyre change under red flag)", c.RedFlagChanges)
		}
		fmt.Fprintf(&b, "P%d %s %s: %s, %s, %s%s%s\n", c.Position, c.Code, c.Team, gap, tyre, stops, last, pen)
	}
	b.WriteString("\nNew events:\n")
	for _, e := range events {
		fmt.Fprintf(&b, "- lap %d %s: %s\n", e.Lap, e.Time.UTC().Format("15:04:05"), e.Text)
	}
	if len(messages) > 0 {
		b.WriteString("\nRecent race control:\n")
		for _, m := range slices.Backward(messages) {
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

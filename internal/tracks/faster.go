package tracks

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/fogleman/gg"
)

// Stretch is a part of the lap: a corner, or corners close together, or the
// straight between them.
type Stretch struct {
	Name   string  `json:"name"` // e.g. "Turn 9, Copse", "Hangar Straight"
	Corner bool    `json:"corner"`
	FromM  float64 `json:"from_m"` // distance from the line
	// ToM is where it ends. The stretch across the line ends before it
	// starts, running on into the next lap.
	ToM float64 `json:"to_m"`
}

const (
	stretchBefore = 120.0 // metres before a turn's apex its stretch starts
	stretchAfter  = 60.0  // and after, where it ends
	minStraight   = 50.0  // the shortest straight between two corners
	chicane       = 120.0 // turns with apexes closer than this are one stretch
	nearTie       = 0.02  // seconds; closer stretches are drawn faded
)

// Stretches splits the lap into its corners and the straights between them.
// Turns with apexes under 120 m apart are one stretch, e.g. a chicane, as
// are the turns of a named complex with no straight between them; other
// turns too close for 50 m of straight between them meet halfway.
// The stretch across the start/finish line is last.
func (t *Track) Stretches() []Stretch {
	dist, lap := lapDistances(t.Outline)
	type group struct {
		from, to float64
		corners  []Corner
	}
	var groups []group
	lastApex := 0.0
	for _, c := range t.Corners {
		d := dist[nearestIndex(t.Outline, c.Position)]
		from, to := max(d-stretchBefore, 0), min(d+stretchAfter, lap)
		if n := len(groups); n > 0 {
			prev := &groups[n-1]
			sameComplex := c.Name != "" && c.Name == prev.corners[len(prev.corners)-1].Name && from-prev.to < minStraight
			switch {
			case d-lastApex < chicane || sameComplex:
				// A chicane, or a named complex such as the Esses: one
				// stretch.
				prev.to = max(prev.to, to)
				prev.corners = append(prev.corners, c)
				lastApex = d
				continue
			case from-prev.to < minStraight:
				// Too close for a straight between them: split the
				// difference.
				mid := min(max((prev.to+from)/2, lastApex), d)
				prev.to, from = mid, mid
			}
		}
		groups = append(groups, group{from, to, []Corner{c}})
		lastApex = d
	}
	if len(groups) == 0 {
		return []Stretch{{Name: "Lap", FromM: 0, ToM: lap}}
	}
	var out []Stretch
	for i, g := range groups {
		out = append(out, Stretch{Name: cornersName(g.corners), Corner: true, FromM: g.from, ToM: g.to})
		if i+1 < len(groups) && groups[i+1].from > g.to {
			next := groups[i+1]
			out = append(out, Stretch{Name: t.straightName(g.corners[len(g.corners)-1], next.corners[0]), FromM: g.to, ToM: next.from})
		}
	}
	first, last := groups[0], groups[len(groups)-1]
	if first.from+lap-last.to > 1 {
		name := t.straightName(last.corners[len(last.corners)-1], first.corners[0])
		if strings.HasPrefix(name, "Turn ") {
			name = "Start/finish straight"
		}
		out = append(out, Stretch{Name: name, FromM: last.to, ToM: first.from})
	}
	return out
}

// cornersName names a group of turns, e.g. "Turn 9, Copse" or "Turns
// 10–11, Maggotts".
func cornersName(cs []Corner) string {
	name := fmt.Sprintf("Turn %d", cs[0].Number)
	if len(cs) > 1 {
		name = fmt.Sprintf("Turns %d–%d", cs[0].Number, cs[len(cs)-1].Number)
	}
	var names []string
	for _, c := range cs {
		if c.Name != "" && (len(names) == 0 || names[len(names)-1] != c.Name) {
			names = append(names, c.Name)
		}
	}
	switch n := len(names); {
	case n == 1:
		name += ", " + names[0]
	case n > 1:
		name += ", " + strings.Join(names[:n-1], ", ") + " and " + names[n-1]
	}
	return name
}

// straightName is the name of the straight between two turns, or e.g.
// "Turn 3 to Turn 4".
func (t *Track) straightName(from, to Corner) string {
	for _, st := range t.Straights {
		if st.From == from.Number && st.To == to.Number {
			return st.Name
		}
	}
	return fmt.Sprintf("Turn %d to Turn %d", from.Number, to.Number)
}

// StretchResult is how long each lap took over a stretch.
type StretchResult struct {
	Stretch
	Seconds []float64 `json:"seconds"` // per lap, in the order given
	Fastest int       `json:"fastest"` // index of the fastest lap here
	Margin  float64   `json:"margin"`  // seconds to the next fastest
}

// LapComparison is where each of several laps was faster.
type LapComparison struct {
	Stretches []StretchResult
	// Colors are the laps' colours on the map: their team colours, made
	// lighter for a second car of the same team.
	Colors []string
}

// CompareLaps times each lap over each stretch of the track.
func CompareLaps(t *Track, laps []LapTrace) (LapComparison, error) {
	if len(laps) < 2 {
		return LapComparison{}, fmt.Errorf("need at least 2 laps to compare, got %d", len(laps))
	}
	laps = smoothLaps(t, laps)
	dist, lap := lapDistances(t.Outline)
	progress := make([][]float64, len(laps))
	for i, l := range laps {
		progress[i] = lapProgress(t, l, dist, lap)
	}
	cmp := LapComparison{Colors: lapColors(laps)}
	for _, st := range t.Stretches() {
		r := StretchResult{Stretch: st}
		for i, l := range laps {
			var s float64
			if st.ToM < st.FromM { // across the line
				s = l.Duration - timeAt(l, progress[i], st.FromM) + timeAt(l, progress[i], st.ToM)
			} else {
				s = timeAt(l, progress[i], st.ToM) - timeAt(l, progress[i], st.FromM)
			}
			r.Seconds = append(r.Seconds, math.Round(s*1000)/1000)
		}
		second := math.Inf(1)
		for i, s := range r.Seconds {
			if s < r.Seconds[r.Fastest] {
				r.Fastest = i
			}
		}
		for i, s := range r.Seconds {
			if i != r.Fastest {
				second = min(second, s)
			}
		}
		r.Margin = math.Round((second-r.Seconds[r.Fastest])*1000) / 1000
		cmp.Stretches = append(cmp.Stretches, r)
	}
	return cmp, nil
}

// lapColors gives each lap its team colour, lightened for a second (or
// third) car in the same colour so they can be told apart.
func lapColors(laps []LapTrace) []string {
	var out []string
	seen := map[string]int{}
	for _, l := range laps {
		c := strings.ToUpper(hex(l.Color))
		n := seen[c]
		seen[c]++
		out = append(out, blendHex(c, "#FFFFFF", 0.45*float64(n)))
	}
	return out
}

// blendHex mixes f of colour b into colour a.
func blendHex(a, b string, f float64) string {
	ca, cb := hexColor(a), hexColor(b)
	mix := func(x, y uint8) uint8 { return uint8(math.Round(float64(x)*(1-f) + float64(y)*f)) }
	return fmt.Sprintf("#%02X%02X%02X", mix(ca.R, cb.R), mix(ca.G, cb.G), mix(ca.B, cb.B))
}

// LapStillColors are the colours RenderLapStill draws with beyond the
// track's own, for EncodePNGWith.
func LapStillColors(laps []LapTrace) []string {
	out := append([]string{}, sectorColors[:]...)
	out = append(out, "#2ECC71", "#2C2C3A")
	for _, l := range laps {
		out = append(out, strings.ToUpper(hex(l.Color)))
		if c, ok := compoundColors[l.Compound]; ok {
			out = append(out, c)
		}
	}
	return out
}

// FasterColors are the colours RenderFaster draws with beyond the track's
// own, for EncodePNGWith.
func (c LapComparison) FasterColors() []string {
	var out []string
	for _, col := range c.Colors {
		out = append(out, col, blendHex(col, asphaltColor, 0.6))
	}
	return out
}

// RenderFaster draws the track map with each stretch of the lap edged in
// the colour of the driver who was fastest there (faded where it was
// within 0.02 s), and under it the gap to the first lap all round the lap.
func RenderFaster(t *Track, laps []LapTrace, o Options) (image.Image, LapComparison, error) {
	cmp, err := CompareLaps(t, laps)
	if err != nil {
		return nil, LapComparison{}, err
	}
	if len(t.Outline) < 2 {
		return nil, LapComparison{}, fmt.Errorf("track %s %d has no outline", t.CircuitID, t.Year)
	}
	p := mapProjection(t)
	dc := gg.NewContext(mapWidth, mapHeight+mapElevationHeight)
	dc.SetHexColor(bgColor)
	dc.Clear()

	dist, lap := lapDistances(t.Outline)
	var edges []trackEdge
	for _, r := range cmp.Stretches {
		col := cmp.Colors[r.Fastest]
		if r.Margin < nearTie {
			col = blendHex(col, asphaltColor, 0.6)
		}
		from, to := indexAtDistance(dist, r.FromM), indexAtDistance(dist, r.ToM)
		if r.ToM < r.FromM {
			to += len(t.Outline)
		}
		edges = append(edges, trackEdge{from, to, col})
	}
	drawTrackEdges(dc, p, t, 14, edges)
	var taken []rect
	for _, st := range t.Straights {
		if r, ok := drawStraightName(dc, p, t, st, 14+2*3); ok {
			taken = append(taken, r)
		}
	}
	for _, c := range t.Corners {
		drawCornerMarker(dc, p, c, 40, 14, false)
	}
	drawCornerNames(dc, p, t, 40, 14, 20, taken)
	drawTitle(dc, t.Name, fmt.Sprintf("%s, %s · %s", t.Locality, t.Country, o.years(t)))
	drawBanner(dc, o.Overlay)
	drawFasterLegend(dc, laps, cmp, mapHeight-40)
	drawDelta(dc, t, smoothLaps(t, laps), cmp, dist, lap, mapElevationPanel())
	drawCredit(dc, mapWidth, float64(mapHeight+mapElevationHeight))
	return dc.Image(), cmp, nil
}

// indexAtDistance is the first outline point at least d metres round the
// lap.
func indexAtDistance(dist []float64, d float64) int {
	for i, x := range dist {
		if x >= d {
			return i
		}
	}
	return len(dist)
}

// drawFasterLegend shows each driver's colour, lap time and how many
// stretches they were fastest in.
func drawFasterLegend(dc *gg.Context, laps []LapTrace, cmp LapComparison, y float64) {
	won := make([]int, len(laps))
	for _, r := range cmp.Stretches {
		won[r.Fastest]++
	}
	dc.SetFontFace(face(regularFont, 16))
	x := 48.0
	for i, l := range laps {
		dc.SetHexColor(cmp.Colors[i])
		dc.DrawRoundedRectangle(x, y-10, 28, 8, 3)
		dc.Fill()
		dc.SetHexColor(subtleColor)
		label := fmt.Sprintf("%s %s · fastest in %d of %d", l.Driver, formatLapTime(l.Duration), won[i], len(cmp.Stretches))
		dc.DrawStringAnchored(label, x+36, y-6, 0, 0.35)
		w, _ := dc.MeasureString(label)
		x += 36 + w + 28
	}
}

// drawDelta draws each lap's gap to the first lap against distance round
// the lap, over a strip coloured by who was fastest in each stretch.
func drawDelta(dc *gg.Context, t *Track, laps []LapTrace, cmp LapComparison, dist []float64, lap float64, e elevationPanel) {
	progress := make([][]float64, len(laps))
	for i, l := range laps {
		progress[i] = lapProgress(t, l, dist, lap)
	}
	const step = 5.0
	deltas := make([][]float64, len(laps))
	lo, hi := 0.0, 0.0
	for i := 1; i < len(laps); i++ {
		for d := 0.0; d <= lap; d += step {
			g := timeAt(laps[i], progress[i], d) - timeAt(laps[0], progress[0], d)
			deltas[i] = append(deltas[i], g)
			lo, hi = min(lo, g), max(hi, g)
		}
	}
	span := max(hi-lo, 0.2)
	lo, hi = lo-span*0.1, hi+span*0.1

	dc.SetHexColor(insetColor)
	dc.DrawRoundedRectangle(e.x, e.y, e.w, e.h, 10)
	dc.Fill()
	dc.SetFontFace(face(boldFont, 16))
	dc.SetHexColor(textColor)
	title := "Gap to " + laps[0].Driver
	dc.DrawString(title, e.x+16, e.y+26)
	tw, _ := dc.MeasureString(title)
	dc.SetFontFace(face(regularFont, 15))
	dc.SetHexColor(subtleColor)
	dc.DrawString("above the line: behind · below: ahead", e.x+16+tw+12, e.y+26)

	px, py, pw, ph := e.plotArea()
	xAt := func(d float64) float64 { return px + d/lap*pw }
	yAt := func(g float64) float64 { return py + (hi-g)/(hi-lo)*ph }

	// Who was fastest in each stretch, along the bottom.
	for _, r := range cmp.Stretches {
		col := cmp.Colors[r.Fastest]
		if r.Margin < nearTie {
			col = blendHex(col, asphaltColor, 0.6)
		}
		dc.SetHexColor(col)
		spans := [][2]float64{{r.FromM, r.ToM}}
		if r.ToM < r.FromM {
			spans = [][2]float64{{r.FromM, lap}, {0, r.ToM}}
		}
		for _, s := range spans {
			dc.DrawRectangle(xAt(s[0]), py+ph+6, xAt(s[1])-xAt(s[0]), 6)
			dc.Fill()
		}
	}

	// Gridlines: the reference lap at zero, and the extremes.
	dc.SetFontFace(face(regularFont, 13))
	for _, g := range []float64{hi, 0, lo} {
		y := yAt(g)
		if g != 0 && math.Abs(y-yAt(0)) < 24 {
			continue // too close to the zero line to label
		}
		dc.SetLineWidth(1)
		dc.SetHexColor("#2C2C3A")
		if g == 0 {
			dc.SetHexColor(cmp.Colors[0])
			dc.SetLineWidth(2)
		}
		dc.DrawLine(px, y, px+pw, y)
		dc.Stroke()
		dc.SetHexColor(subtleColor)
		dc.DrawStringAnchored(fmt.Sprintf("%+.2f s", g), px-10, y, 1, 0.35)
	}
	for km := 0; float64(km)*1000 <= lap; km++ {
		dc.SetHexColor(subtleColor)
		dc.DrawStringAnchored(fmt.Sprintf("%d km", km), xAt(float64(km)*1000), py+ph+26, 0.5, 0.35)
	}

	// Turn numbers above the plot, with a faint line down through it.
	dc.SetFontFace(face(boldFont, 12))
	prevX, row := math.Inf(-1), 0
	for _, c := range t.Corners {
		x := xAt(dist[nearestIndex(t.Outline, c.Position)])
		if x-prevX < 22 && row == 0 {
			row = 1
		} else {
			row = 0
		}
		prevX = x
		ly := py - 14 - float64(row)*22
		dc.SetHexColor("#FFFFFF20")
		dc.SetLineWidth(1)
		dc.DrawLine(x, ly+10, x, py+ph)
		dc.Stroke()
		dc.SetHexColor(textColor)
		dc.DrawCircle(x, ly, 10)
		dc.Fill()
		dc.SetHexColor(bgColor)
		dc.DrawStringAnchored(fmt.Sprint(c.Number), x, ly, 0.5, 0.35)
	}

	// Each other lap's gap.
	dc.SetLineWidth(3)
	dc.SetLineJoin(gg.LineJoinRound)
	for i := 1; i < len(laps); i++ {
		dc.NewSubPath()
		for j, g := range deltas[i] {
			dc.LineTo(xAt(float64(j)*step), yAt(g))
		}
		dc.SetHexColor(cmp.Colors[i])
		dc.Stroke()
		last := deltas[i][len(deltas[i])-1]
		dc.SetFontFace(face(boldFont, 13))
		dc.DrawStringAnchored(laps[i].Driver, px+pw+4, yAt(last), 0, 0.35)
	}
}

// paletteWith is a palette for images drawn in the track's colours plus
// extra ones, e.g. team colours. The colours come first, then blends of
// each with the background, asphalt and text for anti-aliased edges, then
// blends between the rest as room allows.
func paletteWith(extra []string) color.Palette {
	base := []string{
		bgColor, asphaltColor, edgeColor, kerbRed, kerbWhite, textColor,
		subtleColor, leaderColor, insetColor, "#000000",
	}
	var cols []color.RGBA
	for _, h := range append(base, extra...) {
		cols = append(cols, hexColor(h))
	}
	p := color.Palette{}
	seen := map[color.RGBA]bool{}
	add := func(c color.RGBA) {
		if !seen[c] && len(p) < 256 {
			seen[c] = true
			p = append(p, c)
		}
	}
	blend := func(a, b color.RGBA, steps int) {
		for s := 1; s <= steps; s++ {
			f := float64(s) / float64(steps+1)
			add(color.RGBA{
				R: uint8(float64(a.R)*(1-f) + float64(b.R)*f),
				G: uint8(float64(a.G)*(1-f) + float64(b.G)*f),
				B: uint8(float64(a.B)*(1-f) + float64(b.B)*f),
				A: 255,
			})
		}
	}
	for _, c := range cols {
		add(c)
	}
	for _, target := range []string{bgColor, asphaltColor, insetColor, textColor} {
		for _, c := range cols {
			blend(c, hexColor(target), 3)
		}
	}
	for i, a := range cols {
		for _, b := range cols[i+1:] {
			blend(a, b, 1)
		}
	}
	return p
}

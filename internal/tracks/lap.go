package tracks

import (
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"math"

	"github.com/fogleman/gg"
	xdraw "golang.org/x/image/draw"
)

// LapTrace is one driver's lap: positions and telemetry against time from
// the start of the lap.
type LapTrace struct {
	Driver   string  `json:"driver"` // e.g. "NOR"
	Number   int     `json:"number"`
	Color    string  `json:"color"` // team colour, hex
	Lap      int     `json:"lap"`
	Duration float64 `json:"duration"` // lap time in seconds
	// Positions are [t, x, y]: seconds from the start of the lap and track
	// coordinates.
	Positions [][3]float64 `json:"positions"`
	// Telemetry is [t, speed km/h, throttle %, brake %, gear].
	Telemetry [][5]float64 `json:"telemetry"`
}

const (
	lapGIFWidth   = 800
	lapSpeedUp    = 3.0 // playback speed relative to real time
	lapFrameDelay = 8   // hundredths of a second per frame
	lapTrail      = 3.0 // seconds of trail behind each car
	lapHold       = 1.5 // seconds to hold the last frame
	lapRowHeight  = 40
)

func lapBandHeight(drivers int) int { return 44 + lapRowHeight*drivers }

// RenderLapGIF animates one or more laps over the track map, synchronised by
// lap time: each car with a fading trail, and a panel with the lap timer and
// each driver's speed, gear, throttle and brake. With several laps the panel
// also shows each driver's gap to the first at the same point on the lap.
func RenderLapGIF(t *Track, laps []LapTrace, o Options) (*gif.GIF, error) {
	if len(laps) == 0 {
		return nil, fmt.Errorf("no laps to draw")
	}
	full, err := Render(t, o)
	if err != nil {
		return nil, err
	}
	// The map, scaled down, under a band for the timing panel so the panel
	// never hides the track.
	scale := float64(lapGIFWidth) / float64(full.Bounds().Dx())
	band := lapBandHeight(len(laps))
	h := int(float64(full.Bounds().Dy())*scale) + band
	base := image.NewRGBA(image.Rect(0, 0, lapGIFWidth, h))
	for i := 0; i < len(base.Pix); i += 4 {
		c := hexColor(bgColor)
		base.Pix[i], base.Pix[i+1], base.Pix[i+2], base.Pix[i+3] = c.R, c.G, c.B, 255
	}
	xdraw.CatmullRom.Scale(base, image.Rect(0, band, lapGIFWidth, h), full, full.Bounds(), xdraw.Src, nil)

	p := mapProjection(t)
	p.scale, p.ox, p.oy = p.scale*scale, p.ox*scale, p.oy*scale+float64(band)

	outward := -insideSide(p, t.Outline)
	pal := lapPalette(laps)
	// A transparent entry, used for pixels unchanged since the last frame.
	withClear := append(append(color.Palette{}, pal...), color.RGBA{})
	clear := uint8(len(pal))
	dist, lap := lapDistances(t.Outline)
	progress := make([][]float64, len(laps))
	for i, l := range laps {
		progress[i] = lapProgress(t, l, dist, lap)
	}

	longest := 0.0
	for _, l := range laps {
		longest = max(longest, l.Duration)
	}
	step := lapSpeedUp * float64(lapFrameDelay) / 100

	g := &gif.GIF{}
	var prev *image.Paletted
	cache := map[color.RGBA]uint8{}
	for T := 0.0; ; T += step {
		T = min(T, longest)
		frame := image.NewRGBA(base.Bounds())
		copy(frame.Pix, base.Pix)
		dc := gg.NewContextForRGBA(frame)
		if o.LapEvents {
			for i, l := range laps {
				drawLapEvents(dc, p, l, min(T, l.Duration), i, len(laps), outward)
			}
		}
		var placed [][2]float64
		for _, l := range laps {
			placed = append(placed, drawCar(dc, p, l, min(T, l.Duration), placed))
		}
		drawElevationDots(dc, t, laps, progress, dist, lap, T, scale, float64(band))
		drawLapHUD(dc, laps, progress, T)

		pf := quantize(frame, withClear[:len(pal)], cache)
		pf.Palette = withClear
		delay := lapFrameDelay
		if T >= longest {
			delay = int(lapHold * 100)
		}
		if prev == nil {
			g.Image = append(g.Image, pf)
		} else {
			// Store only the part of the frame that changed, with unchanged
			// pixels in it transparent so they compress to almost nothing.
			r := changed(prev, pf)
			if r.Empty() {
				r = image.Rect(0, 0, 1, 1)
			}
			sub := image.NewPaletted(r, withClear)
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					i := pf.PixOffset(x, y)
					if pf.Pix[i] == prev.Pix[i] {
						sub.Pix[sub.PixOffset(x, y)] = clear
					} else {
						sub.Pix[sub.PixOffset(x, y)] = pf.Pix[i]
					}
				}
			}
			g.Image = append(g.Image, sub)
		}
		g.Delay = append(g.Delay, delay)
		g.Disposal = append(g.Disposal, gif.DisposalNone)
		prev = pf
		if T >= longest {
			break
		}
	}
	return g, nil
}

// lapPalette is the track palette plus the drivers' team colours and the
// panel's throttle green.
func lapPalette(laps []LapTrace) color.Palette {
	pal := append(color.Palette{}, palette...)
	bg := hexColor(bgColor)
	add := func(c color.RGBA) {
		if len(pal) < 256 {
			pal = append(pal, c)
		}
	}
	add(hexColor("#2ECC71"))
	for _, l := range laps {
		c := hexColor(hex(l.Color))
		add(c)
		add(color.RGBA{uint8((int(c.R) + int(bg.R)) / 2), uint8((int(c.G) + int(bg.G)) / 2), uint8((int(c.B) + int(bg.B)) / 2), 255})
	}
	return pal
}

func quantize(img *image.RGBA, pal color.Palette, cache map[color.RGBA]uint8) *image.Paletted {
	out := image.NewPaletted(img.Bounds(), pal)
	for i := 0; i < len(img.Pix); i += 4 {
		c := color.RGBA{img.Pix[i], img.Pix[i+1], img.Pix[i+2], 255}
		idx, ok := cache[c]
		if !ok {
			idx = uint8(pal.Index(c))
			cache[c] = idx
		}
		out.Pix[i/4] = idx
	}
	return out
}

// changed returns the smallest rectangle containing every pixel that
// differs between two frames.
func changed(a, b *image.Paletted) image.Rectangle {
	r := image.Rectangle{}
	w := a.Bounds().Dx()
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			pt := image.Pt(i%w, i/w)
			r = r.Union(image.Rectangle{pt, pt.Add(image.Pt(1, 1))})
		}
	}
	return r
}

// position interpolates a lap's position at time T.
func (l LapTrace) position(T float64) Point {
	ps := l.Positions
	if len(ps) == 0 {
		return Point{}
	}
	if T <= ps[0][0] {
		return Point{ps[0][1], ps[0][2]}
	}
	for i := 1; i < len(ps); i++ {
		if ps[i][0] >= T {
			a, b := ps[i-1], ps[i]
			f := (T - a[0]) / max(b[0]-a[0], 1e-9)
			return Point{a[1] + f*(b[1]-a[1]), a[2] + f*(b[2]-a[2])}
		}
	}
	last := ps[len(ps)-1]
	return Point{last[1], last[2]}
}

// telemetry returns the telemetry sample nearest time T.
func (l LapTrace) telemetry(T float64) [5]float64 {
	var best [5]float64
	bestD := math.Inf(1)
	for _, s := range l.Telemetry {
		if d := math.Abs(s[0] - T); d < bestD {
			best, bestD = s, d
		}
	}
	return best
}

// lapProgress returns the distance round the lap, in metres, of each of a
// lap's positions, never going backwards.
func lapProgress(t *Track, l LapTrace, dist []float64, lap float64) []float64 {
	out := make([]float64, len(l.Positions))
	furthest := 0.0
	for i, s := range l.Positions {
		d := dist[nearestIndex(t.Outline, Point{s[1], s[2]})]
		// Near the line a point may match the end of the outline.
		if i < len(l.Positions)/2 && d > lap/2 {
			d -= lap
		}
		furthest = max(furthest, d)
		out[i] = furthest
	}
	return out
}

// distanceAt interpolates how far round the lap a car was at time T.
func distanceAt(l LapTrace, prog []float64, T float64) float64 {
	ps := l.Positions
	for i := 1; i < len(ps); i++ {
		if ps[i][0] >= T {
			f := (T - ps[i-1][0]) / max(ps[i][0]-ps[i-1][0], 1e-9)
			return prog[i-1] + f*(prog[i]-prog[i-1])
		}
	}
	return prog[len(prog)-1]
}

// timeAt interpolates when a car reached a distance round the lap.
func timeAt(l LapTrace, prog []float64, d float64) float64 {
	ps := l.Positions
	for i := 1; i < len(ps); i++ {
		if prog[i] >= d {
			f := (d - prog[i-1]) / max(prog[i]-prog[i-1], 1e-9)
			return ps[i-1][0] + f*(ps[i][0]-ps[i-1][0])
		}
	}
	return l.Duration
}

// drawCar draws a car at lap time T with a fading trail behind it, and
// returns where it drew it. Its label moves down if a car already drawn is
// close by.
func drawCar(dc *gg.Context, p projection, l LapTrace, T float64, others [][2]float64) [2]float64 {
	c := hexColor(hex(l.Color))
	const steps = 15
	dc.SetLineCap(gg.LineCapRound)
	for k := steps; k > 0; k-- {
		t0 := T - lapTrail*float64(k)/steps
		t1 := T - lapTrail*float64(k-1)/steps
		if t1 <= 0 {
			continue
		}
		x0, y0 := p.point(l.position(max(t0, 0)))
		x1, y1 := p.point(l.position(t1))
		a := 1 - float64(k)/steps
		dc.SetRGBA255(int(c.R), int(c.G), int(c.B), int(80+175*a))
		dc.SetLineWidth(2 + 4*a)
		dc.DrawLine(x0, y0, x1, y1)
		dc.Stroke()
	}
	x, y := p.point(l.position(T))
	dc.SetHexColor(textColor)
	dc.DrawCircle(x, y, 8)
	dc.Fill()
	dc.SetRGB255(int(c.R), int(c.G), int(c.B))
	dc.DrawCircle(x, y, 6)
	dc.Fill()
	ly := y
	for _, o := range others {
		if math.Hypot(o[0]-x, o[1]-y) < 24 {
			ly = max(ly, o[1]+20)
		}
	}
	dc.SetFontFace(face(boldFont, 12))
	w, fh := dc.MeasureString(l.Driver)
	dc.SetHexColor("#000000C0")
	dc.DrawRoundedRectangle(x+11, ly-fh/2-3, w+8, fh+6, 3)
	dc.Fill()
	dc.SetRGB255(int(c.R), int(c.G), int(c.B))
	dc.DrawRectangle(x+11, ly-fh/2-3, 3, fh+6)
	dc.Fill()
	dc.SetHexColor(textColor)
	dc.DrawStringAnchored(l.Driver, x+16, ly, 0, 0.35)
	return [2]float64{x, y}
}

// lapEvent is a moment in a lap worth marking on the track.
type lapEvent struct {
	t, until float64 // until is the end of a braking zone
	gear     int     // new gear, for a gear change
	braking  bool
	speed    float64 // km/h as braking starts
}

// events returns a lap's braking zones and gear changes, in time order.
func (l LapTrace) events() []lapEvent {
	var out []lapEvent
	var brakeFrom, brakeSpeed float64
	braking := false
	for i, s := range l.Telemetry {
		on := s[3] > 0
		switch {
		case on && !braking:
			brakeFrom, brakeSpeed, braking = s[0], s[1], true
		case !on && braking:
			out = append(out, lapEvent{t: brakeFrom, until: s[0], braking: true, speed: brakeSpeed})
			braking = false
		}
		if i > 0 && s[4] != l.Telemetry[i-1][4] && s[4] > 0 {
			out = append(out, lapEvent{t: s[0], gear: int(s[4])})
		}
	}
	if braking {
		out = append(out, lapEvent{t: brakeFrom, until: l.Duration, braking: true, speed: brakeSpeed})
	}
	return out
}

// drawLapEvents marks where a car braked (red along the track) and changed
// gear, up to lap time T. The braking speed and new gear are drawn off the
// track, each joined to its point by a thin line: on the driver's own side
// with several drivers, else on outward (+1 right, -1 left of travel).
func drawLapEvents(dc *gg.Context, p projection, l LapTrace, T float64, driver, drivers int, outward float64) {
	side, dir := 0.0, outward
	if drivers > 1 {
		side = 7 * (float64(driver)*2 - float64(drivers-1))
		dir = math.Copysign(1, side)
	}
	// at returns the point offset sideways by off pixels from the car's
	// position at time t (positive is right of travel).
	at := func(t, off float64) (float64, float64) {
		x, y := p.point(l.position(t))
		ax, ay := p.point(l.position(t - 0.1))
		bx, by := p.point(l.position(t + 0.1))
		dx, dy := bx-ax, by-ay
		if n := math.Hypot(dx, dy); n > 0 {
			// Right of travel is (-dy, dx) in image space.
			x, y = x-dy/n*off, y+dx/n*off
		}
		return x, y
	}
	c := hexColor(hex(l.Color))
	leader := func(x0, y0, x1, y1 float64) {
		dc.SetRGBA255(int(c.R), int(c.G), int(c.B), 200)
		dc.SetLineWidth(1)
		dc.DrawLine(x0, y0, x1, y1)
		dc.Stroke()
		dc.DrawCircle(x0, y0, 1.8)
		dc.Fill()
	}
	gears := 0
	for _, e := range l.events() {
		if e.t > T {
			break
		}
		if e.braking {
			dc.NewSubPath()
			for t := e.t; t <= min(e.until, T); t += 0.1 {
				dc.LineTo(at(t, side))
			}
			dc.SetHexColor(kerbRed)
			dc.SetLineWidth(5)
			dc.SetLineCap(gg.LineCapRound)
			dc.Stroke()

			// Speed as braking started.
			x0, y0 := at(e.t, side)
			lx, ly := at(e.t, side+dir*52)
			leader(x0, y0, lx, ly)
			label := fmt.Sprintf("%.0f", e.speed)
			dc.SetFontFace(face(boldFont, 10))
			w, h := dc.MeasureString(label)
			dc.SetRGB255(int(c.R), int(c.G), int(c.B))
			dc.DrawRoundedRectangle(lx-w/2-3, ly-h/2-2, w+6, h+4, 3)
			dc.Fill()
			dc.SetHexColor(bgColor)
			dc.DrawStringAnchored(label, lx, ly, 0.5, 0.35)
			continue
		}
		// New gear, alternating between two distances so neighbours
		// don't overlap.
		x0, y0 := at(e.t, side)
		d := 24.0 + float64(gears%2)*12
		gears++
		gx, gy := at(e.t, side+dir*d)
		leader(x0, y0, gx, gy)
		dc.SetRGB255(int(c.R), int(c.G), int(c.B))
		dc.DrawCircle(gx, gy, 6)
		dc.Fill()
		dc.SetFontFace(face(boldFont, 9))
		dc.SetHexColor(textColor)
		dc.DrawStringAnchored(fmt.Sprint(e.gear), gx, gy, 0.5, 0.4)
	}
}

// drawElevationDots marks each car on the map's elevation profile. scale and
// dy convert full-map coordinates to the animation's.
func drawElevationDots(dc *gg.Context, t *Track, laps []LapTrace, progress [][]float64, dist []float64, lap, T, scale, dy float64) {
	if len(t.Elevation) != len(t.Outline) {
		return
	}
	xAt, yAt := mapElevationPanel().axes(lap, max(t.ElevationChange(), 10))
	for i, l := range laps {
		d := min(max(distanceAt(l, progress[i], min(T, l.Duration)), 0), lap)
		x, y := xAt(d)*scale, yAt(elevationAt(t, dist, d))*scale+dy
		c := hexColor(hex(l.Color))
		dc.SetHexColor(textColor)
		dc.DrawCircle(x, y, 6)
		dc.Fill()
		dc.SetRGB255(int(c.R), int(c.G), int(c.B))
		dc.DrawCircle(x, y, 4.5)
		dc.Fill()
	}
}

// drawLapHUD draws the timing band across the top: the lap timer, then a
// row per driver with speed, gear, throttle, brake and gap to the first.
func drawLapHUD(dc *gg.Context, laps []LapTrace, progress [][]float64, T float64) {
	const pad = 16.0
	W := float64(dc.Width())
	dc.SetFontFace(face(boldFont, 22))
	dc.SetHexColor(textColor)
	dc.DrawString(formatLapTime(min(T, laps[0].Duration)), pad, 30)
	dc.SetFontFace(face(regularFont, 12))
	dc.SetHexColor(subtleColor)
	dc.DrawStringAnchored(fmt.Sprintf("Lap time · played at %.0f× speed", lapSpeedUp), W-pad, 26, 1, 0)

	for i, l := range laps {
		ry := 44 + float64(lapRowHeight*i)
		lt := min(T, l.Duration)
		tel := l.telemetry(lt)
		dc.SetHexColor(hex(l.Color))
		dc.DrawRoundedRectangle(pad, ry+4, 6, lapRowHeight-12, 2)
		dc.Fill()

		dc.SetFontFace(face(boldFont, 16))
		dc.SetHexColor(textColor)
		dc.DrawString(l.Driver, pad+14, ry+22)
		dc.SetFontFace(face(regularFont, 15))
		dc.DrawString(fmt.Sprintf("%3.0f km/h", tel[1]), pad+64, ry+22)
		dc.DrawString(fmt.Sprintf("G%.0f", tel[4]), pad+150, ry+22)

		// Throttle and brake bars.
		const bw = 140.0
		bx, by := pad+228, ry+13
		dc.SetFontFace(face(regularFont, 11))
		dc.SetHexColor(subtleColor)
		dc.DrawStringAnchored("THR", bx, by+4, 1.15, 0.35)
		dc.DrawStringAnchored("BRK", bx+bw+44, by+4, 1.15, 0.35)
		dc.SetHexColor("#2C2C3A")
		dc.DrawRectangle(bx, by, bw, 9)
		dc.DrawRectangle(bx+bw+44, by, bw, 9)
		dc.Fill()
		dc.SetHexColor("#2ECC71")
		dc.DrawRectangle(bx, by, bw*min(tel[2], 100)/100, 9)
		dc.Fill()
		if tel[3] > 0 {
			dc.SetHexColor(kerbRed)
			dc.DrawRectangle(bx+bw+44, by, bw*min(tel[3], 100)/100, 9)
			dc.Fill()
		}

		// Gap to the first driver at the same point on the lap.
		switch {
		case i > 0:
			d := distanceAt(l, progress[i], lt)
			gap := lt - timeAt(laps[0], progress[0], d)
			if lt >= l.Duration {
				// Finished: the gap is the difference in lap time.
				gap = l.Duration - laps[0].Duration
			}
			dc.SetFontFace(face(boldFont, 15))
			if gap >= 0 {
				dc.SetHexColor(kerbRed)
				dc.DrawStringAnchored(fmt.Sprintf("+%.3f", gap), W-pad, ry+22, 1, 0)
			} else {
				dc.SetHexColor("#2ECC71")
				dc.DrawStringAnchored(fmt.Sprintf("%.3f", gap), W-pad, ry+22, 1, 0)
			}
		case len(laps) > 1:
			dc.SetFontFace(face(regularFont, 13))
			dc.SetHexColor(subtleColor)
			dc.DrawStringAnchored("reference", W-pad, ry+22, 1, 0)
		}
	}
}

func formatLapTime(s float64) string {
	m := int(s) / 60
	return fmt.Sprintf("%d:%06.3f", m, s-float64(m*60))
}

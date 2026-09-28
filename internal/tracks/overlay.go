package tracks

import (
	"math"
	"strings"

	"github.com/fogleman/gg"
)

// Overlay adds session information on top of a track: stretches of track
// such as yellow-flag sectors, points such as cars, and a banner such as
// "RED FLAG".
type Overlay struct {
	Highlights []Highlight `json:"highlights,omitempty"`
	Markers    []Marker    `json:"markers,omitempty"`
	// Banner is shown under the title, in BannerColor (default red), with
	// Caption on the line below it.
	Banner      string `json:"banner,omitempty"`
	BannerColor string `json:"banner_color,omitempty"`
	Caption     string `json:"caption,omitempty"`
}

// HighlightStyle is how a highlighted stretch of track is drawn.
type HighlightStyle int

const (
	// Band is a solid band over the track, e.g. a yellow flag.
	Band HighlightStyle = iota
	// DoubleBand is two bands along either edge of the track, e.g. a double
	// yellow flag.
	DoubleBand
)

// Highlight marks the track from one point to another, in lap order.
type Highlight struct {
	From  Point          `json:"from"`
	To    Point          `json:"to"`
	Color string         `json:"color"` // hex colour
	Style HighlightStyle `json:"style"`
	// Faint draws the highlight lightly and without its label, for context
	// that isn't the focus, such as flags elsewhere on the track.
	Faint bool   `json:"faint,omitempty"`
	Label string `json:"label,omitempty"` // e.g. "Double yellow · sector 4"
}

// Marker marks a point on or near the track, e.g. a car.
type Marker struct {
	Position Point  `json:"position"`
	Label    string `json:"label"` // e.g. "PER"
	Color    string `json:"color"` // hex colour, e.g. the team colour
}

func hex(c string) string { return "#" + strings.TrimPrefix(c, "#") }

// drawHighlights draws each highlight along the track. width is the drawn
// track width in pixels.
func drawHighlights(dc *gg.Context, p projection, t *Track, o *Overlay, width float64) {
	if o == nil {
		return
	}
	n := len(t.Outline)
	// Faint ones first so the focus is drawn over them.
	for _, faint := range []bool{true, false} {
		for _, h := range o.Highlights {
			if h.Faint != faint {
				continue
			}
			from, to := nearestIndex(t.Outline, h.From), nearestIndex(t.Outline, h.To)
			if to < from {
				to += n
			}
			alpha := "D0"
			if h.Faint {
				alpha = "50"
			}
			color := hex(h.Color) + alpha
			dc.SetLineCap(gg.LineCapButt)
			switch h.Style {
			case DoubleBand:
				for _, side := range []float64{-1, 1} {
					traceOffset(dc, p, t.Outline, from, to, side*width*0.95)
					dc.SetHexColor(color)
					dc.SetLineWidth(width * 0.55)
					dc.Stroke()
				}
			default:
				tracePath(dc, p, t.Outline, from, to)
				dc.SetHexColor(color)
				dc.SetLineWidth(width * 1.9)
				dc.Stroke()
			}
			dc.SetLineCap(gg.LineCapRound)
			if h.Label != "" && !h.Faint {
				drawHighlightLabel(dc, p, t, h, from, to, width)
			}
		}
	}
}

// traceOffset adds outline[from:to] as a path shifted sideways by offset
// pixels (positive is right of the direction of travel).
func traceOffset(dc *gg.Context, p projection, outline []Point, from, to int, offset float64) {
	n := len(outline)
	dc.NewSubPath()
	for i := from; i <= to; i++ {
		x, y := p.point(outline[i%n])
		ax, ay := p.point(outline[(i-1+n)%n])
		bx, by := p.point(outline[(i+1)%n])
		tx, ty := bx-ax, by-ay
		l := math.Hypot(tx, ty)
		if l == 0 {
			continue
		}
		// In image space (y down), (-ty, tx) points right of travel.
		dc.LineTo(x-ty/l*offset, y+tx/l*offset)
	}
}

func drawHighlightLabel(dc *gg.Context, p projection, t *Track, h Highlight, from, to int, width float64) {
	n := len(t.Outline)
	x, y := p.point(t.Outline[((from+to)/2)%n])
	cx, cy := outlineCentre(p, t.Outline)
	dx, dy := x-cx, y-cy
	if l := math.Hypot(dx, dy); l > 0 {
		dx, dy = dx/l, dy/l
	}
	lx, ly := x+dx*(width*1.5+22), y+dy*(width*1.5+22)
	ax := 0.0
	if dx < 0 {
		ax = 1
	}
	dc.SetFontFace(face(boldFont, 15))
	w, fh := dc.MeasureString(h.Label)
	bx := lx - ax*w
	dc.SetHexColor("#000000B0")
	dc.DrawRoundedRectangle(bx-6, ly-fh/2-5, w+12, fh+10, 4)
	dc.Fill()
	dc.SetHexColor(hex(h.Color))
	dc.DrawStringAnchored(h.Label, lx, ly, ax, 0.35)
}

// drawMarkers draws each marker as a coloured dot with its label beside it.
// Markers too close together to tell apart are spread round their shared
// spot, each with a thin line back to its exact position.
func drawMarkers(dc *gg.Context, p projection, o *Overlay, radius float64) {
	if o == nil {
		return
	}
	type placed struct {
		m      Marker
		x, y   float64 // exact position
		dx, dy float64 // drawn position
	}
	var ms []placed
	for _, m := range o.Markers {
		x, y := p.point(m.Position)
		ms = append(ms, placed{m: m, x: x, y: y, dx: x, dy: y})
	}
	// Group markers that would overlap, then fan each group out.
	used := make([]bool, len(ms))
	for i := range ms {
		if used[i] {
			continue
		}
		group := []int{i}
		used[i] = true
		for j := i + 1; j < len(ms); j++ {
			if !used[j] && math.Hypot(ms[j].x-ms[i].x, ms[j].y-ms[i].y) < radius*2.5 {
				group = append(group, j)
				used[j] = true
			}
		}
		if len(group) < 2 {
			continue
		}
		var cx, cy float64
		for _, k := range group {
			cx += ms[k].x / float64(len(group))
			cy += ms[k].y / float64(len(group))
		}
		spread := radius * 3.2
		for n, k := range group {
			a := -math.Pi/2 + 2*math.Pi*float64(n)/float64(len(group))
			ms[k].dx, ms[k].dy = cx+math.Cos(a)*spread, cy+math.Sin(a)*spread
		}
	}

	for _, m := range ms {
		if m.dx != m.x || m.dy != m.y {
			dc.SetHexColor(textColor)
			dc.SetLineWidth(1.5)
			dc.DrawLine(m.x, m.y, m.dx, m.dy)
			dc.Stroke()
			dc.DrawCircle(m.x, m.y, 2.5)
			dc.Fill()
		}
		dc.SetHexColor(textColor)
		dc.DrawCircle(m.dx, m.dy, radius+2.5)
		dc.Fill()
		dc.SetHexColor(hex(m.m.Color))
		dc.DrawCircle(m.dx, m.dy, radius)
		dc.Fill()
	}
	dc.SetFontFace(face(boldFont, radius*1.3))
	for _, m := range ms {
		w, h := dc.MeasureString(m.m.Label)
		lx, ly := m.dx+radius+8, m.dy
		dc.SetHexColor("#000000B0")
		dc.DrawRoundedRectangle(lx-4, ly-h/2-4, w+8, h+8, 4)
		dc.Fill()
		dc.SetHexColor(textColor)
		dc.DrawStringAnchored(m.m.Label, lx, ly, 0, 0.35)
	}
}

// drawBanner draws the overlay's banner as a pill under the title, with the
// caption below it.
func drawBanner(dc *gg.Context, o *Overlay) {
	if o == nil || o.Banner == "" {
		return
	}
	color := o.BannerColor
	if color == "" {
		color = kerbRed
	}
	dc.SetFontFace(face(boldFont, 18))
	w, h := dc.MeasureString(o.Banner)
	x, y := 48.0, 116.0
	dc.SetHexColor(hex(color))
	dc.DrawRoundedRectangle(x, y, w+24, h+14, 6)
	dc.Fill()
	dc.SetHexColor(textColor)
	dc.DrawStringAnchored(o.Banner, x+12, y+(h+14)/2, 0, 0.35)
	if o.Caption != "" {
		dc.SetFontFace(face(regularFont, 17))
		dc.SetHexColor(textColor)
		dc.DrawString(o.Caption, x, y+h+14+26)
	}
}

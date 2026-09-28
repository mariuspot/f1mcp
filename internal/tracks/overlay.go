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
	Highlights []Highlight
	Markers    []Marker
	// Banner is shown under the title, in BannerColor (default red).
	Banner      string
	BannerColor string
}

// Highlight marks the track from one point to another, in lap order.
type Highlight struct {
	From, To Point
	Color    string // hex colour
	Label    string // drawn beside the stretch, e.g. "Double yellow · sector 4"
}

// Marker marks a point on or near the track, e.g. a car.
type Marker struct {
	Position Point
	Label    string // e.g. "PER"
	Color    string // hex colour, e.g. the team colour
}

// drawHighlights draws each highlight as a translucent band along the track.
// width is the drawn track width in pixels.
func drawHighlights(dc *gg.Context, p projection, t *Track, o *Overlay, width float64) {
	if o == nil {
		return
	}
	n := len(t.Outline)
	for _, h := range o.Highlights {
		from, to := nearestIndex(t.Outline, h.From), nearestIndex(t.Outline, h.To)
		if to < from {
			to += n
		}
		color := strings.TrimPrefix(h.Color, "#")
		tracePath(dc, p, t.Outline, from, to)
		dc.SetHexColor("#" + color + "70")
		dc.SetLineWidth(width * 2.4)
		dc.SetLineCap(gg.LineCapButt)
		dc.Stroke()
		dc.SetLineCap(gg.LineCapRound)

		if h.Label != "" {
			mid := t.Outline[((from+to)/2)%n]
			x, y := p.point(mid)
			cx, cy := outlineCentre(p, t.Outline)
			dx, dy := x-cx, y-cy
			l := math.Hypot(dx, dy)
			if l > 0 {
				dx, dy = dx/l, dy/l
			}
			lx, ly := x+dx*(width*1.6+18), y+dy*(width*1.6+18)
			ax := 0.0
			if dx < 0 {
				ax = 1
			}
			dc.SetFontFace(face(boldFont, 15))
			dc.SetHexColor("#" + color)
			dc.DrawStringAnchored(h.Label, lx, ly, ax, 0.35)
		}
	}
}

// drawMarkers draws each marker as a coloured dot with its label beside it.
// Labels of markers close together are stacked so they don't overlap.
func drawMarkers(dc *gg.Context, p projection, o *Overlay, radius float64) {
	if o == nil {
		return
	}
	dc.SetFontFace(face(boldFont, radius*1.3))
	var placed [][2]float64
	for _, m := range o.Markers {
		x, y := p.point(m.Position)
		dc.SetHexColor(textColor)
		dc.DrawCircle(x, y, radius+2.5)
		dc.Fill()
		dc.SetHexColor("#" + strings.TrimPrefix(m.Color, "#"))
		dc.DrawCircle(x, y, radius)
		dc.Fill()

		// Stack the label below any label already near this point.
		lx, ly := x+radius+8, y
		for _, q := range placed {
			if math.Abs(q[0]-lx) < 60 && math.Abs(q[1]-ly) < radius*1.6 {
				ly = q[1] + radius*1.8
			}
		}
		placed = append(placed, [2]float64{lx, ly})
		w, h := dc.MeasureString(m.Label)
		dc.SetHexColor("#000000B0")
		dc.DrawRoundedRectangle(lx-4, ly-h/2-4, w+8, h+8, 4)
		dc.Fill()
		dc.SetHexColor(textColor)
		dc.DrawStringAnchored(m.Label, lx, ly, 0, 0.35)
	}
}

// drawBanner draws the overlay's banner as a pill under the title.
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
	dc.SetHexColor(color)
	dc.DrawRoundedRectangle(x, y, w+24, h+14, 6)
	dc.Fill()
	dc.SetHexColor(textColor)
	dc.DrawStringAnchored(o.Banner, x+12, y+(h+14)/2, 0, 0.35)
}

package tracks

import (
	"fmt"
	"math"

	"github.com/fogleman/gg"
)

// Marshal sectors are drawn as a thin strip along the inside of the track,
// alternating light and dark, with each sector's number at its start.

// insideSide returns +1 if the inside of the circuit is to the right of the
// direction of travel in the image, or -1 if it is to the left.
func insideSide(p projection, outline []Point) float64 {
	area := 0.0
	pts := screenTrack(p, outline)
	for i, a := range pts {
		b := pts[(i+1)%len(pts)]
		area += a[0]*b[1] - b[0]*a[1]
	}
	// In image space (y down) a positive area runs clockwise, so the inside
	// is on the right.
	if area > 0 {
		return 1
	}
	return -1
}

// drawMarshalStrip draws the marshal sectors along the inside edge of the
// track. width and edge are the drawn asphalt and edge widths in pixels.
func drawMarshalStrip(dc *gg.Context, p projection, t *Track, width, edge float64) {
	ms := t.MarshalSectors
	if len(ms) < 2 {
		return
	}
	n := len(t.Outline)
	strip := max(3, width*0.22)
	offset := insideSide(p, t.Outline) * (width/2 + edge + strip/2 + 2)
	dc.SetLineCap(gg.LineCapButt)
	dc.SetLineWidth(strip)
	for k, m := range ms {
		from := nearestIndex(t.Outline, m.Position)
		to := nearestIndex(t.Outline, ms[(k+1)%len(ms)].Position)
		if to < from {
			to += n
		}
		traceOffset(dc, p, t.Outline, from, to, offset)
		if k%2 == 0 {
			dc.SetHexColor("#FFFFFF70")
		} else {
			dc.SetHexColor("#FFFFFF20")
		}
		dc.Stroke()
	}
	dc.SetLineCap(gg.LineCapRound)
}

// drawMarshalNumbers writes each marshal sector's number just inside the
// strip at its start, and returns the boxes it used.
func drawMarshalNumbers(dc *gg.Context, p projection, t *Track, width float64) []rect {
	ms := t.MarshalSectors
	if len(ms) < 2 {
		return nil
	}
	n := len(t.Outline)
	side := insideSide(p, t.Outline)
	dc.SetFontFace(face(boldFont, max(10, width*0.55)))
	var rects []rect
	for k, m := range ms {
		from := nearestIndex(t.Outline, m.Position)
		to := nearestIndex(t.Outline, ms[(k+1)%len(ms)].Position)
		if to < from {
			to += n
		}
		// A little way into the sector, so the number sits beside it.
		i := from + min(3, (to-from)/2)
		x, y := p.point(t.Outline[i%n])
		ax, ay := p.point(t.Outline[(i-1+n)%n])
		bx, by := p.point(t.Outline[(i+1)%n])
		tx, ty := bx-ax, by-ay
		l := max(1e-9, math.Hypot(tx, ty))
		// Right of travel is (-ty, tx) in image space.
		nx, ny := -ty/l*side, tx/l*side
		d := width*1.2 + 10
		lx, ly := x+nx*d, y+ny*d
		label := fmt.Sprint(m.Number)
		w, h := dc.MeasureString(label)
		dc.SetHexColor(subtleColor)
		dc.DrawStringAnchored(label, lx, ly, 0.5, 0.35)
		rects = append(rects, rect{lx - w/2 - 2, ly - h/2 - 2, lx + w/2 + 2, ly + h/2 + 2})
	}
	return rects
}

package tracks

import (
	"fmt"
	"math"

	"github.com/fogleman/gg"
)

const (
	// Extra canvas height for the elevation panel under a map, and the
	// strip under a corner image.
	mapElevationHeight    = 260
	cornerElevationHeight = 170
)

// lapDistances returns the distance in metres from the start/finish line to
// each outline point, and the lap length.
func lapDistances(outline []Point) ([]float64, float64) {
	d := make([]float64, len(outline))
	for i := 1; i < len(outline); i++ {
		d[i] = d[i-1] + math.Hypot(outline[i][0]-outline[i-1][0], outline[i][1]-outline[i-1][1])/10
	}
	last, first := outline[len(outline)-1], outline[0]
	return d, d[len(d)-1] + math.Hypot(first[0]-last[0], first[1]-last[1])/10
}

// ElevationChange returns the difference between the highest and lowest
// points of the lap in metres, or 0 if elevation is unknown.
func (t *Track) ElevationChange() float64 {
	hi := 0.0
	for _, z := range t.Elevation {
		hi = max(hi, z)
	}
	return hi
}

// elevationPanel is where an elevation profile is drawn.
type elevationPanel struct {
	x, y, w, h float64
	// detailed adds turn numbers, sector colours and distance labels.
	detailed bool
	// focus, if set, is shaded in the profile with a dot at the corner.
	focus *Corner
}

// drawElevation draws the track's height against distance round the lap.
func drawElevation(dc *gg.Context, t *Track, e elevationPanel) {
	if len(t.Elevation) != len(t.Outline) {
		return
	}
	dist, lap := lapDistances(t.Outline)
	change := t.ElevationChange()
	// Show at least 10 m of height so nearly flat tracks look flat.
	top := max(change, 10)

	dc.SetHexColor(insetColor)
	dc.DrawRoundedRectangle(e.x, e.y, e.w, e.h, 10)
	dc.Fill()

	// Title and total change.
	dc.SetFontFace(face(boldFont, 16))
	dc.SetHexColor(textColor)
	dc.DrawString("Elevation", e.x+16, e.y+26)
	tw, _ := dc.MeasureString("Elevation")
	dc.SetFontFace(face(regularFont, 15))
	dc.SetHexColor(subtleColor)
	summary := fmt.Sprintf("%.0f m change over %.2f km", change, lap/1000)
	if e.focus != nil {
		i := nearestIndex(t.Outline, e.focus.Position)
		summary += fmt.Sprintf(" · Turn %d is %.0f m above the lowest point", e.focus.Number, t.Elevation[i])
	}
	dc.DrawString(summary, e.x+16+tw+12, e.y+26)

	// Plot area.
	left, right, topPad, bottom := 64.0, 20.0, 44.0, 16.0
	if e.detailed {
		topPad, bottom = 84, 34
	}
	px, py := e.x+left, e.y+topPad
	pw, ph := e.w-left-right, e.h-topPad-bottom
	xAt := func(d float64) float64 { return px + d/lap*pw }
	yAt := func(z float64) float64 { return py + ph - z/top*ph }

	// Height gridlines and labels.
	dc.SetFontFace(face(regularFont, 13))
	for _, z := range []float64{0, top / 2, top} {
		y := yAt(z)
		dc.SetHexColor("#2C2C3A")
		dc.SetLineWidth(1)
		dc.DrawLine(px, y, px+pw, y)
		dc.Stroke()
		dc.SetHexColor(subtleColor)
		dc.DrawStringAnchored(fmt.Sprintf("%.0f m", z), px-10, y, 1, 0.35)
	}
	// Distance labels, every kilometre.
	if e.detailed {
		for km := 0; float64(km)*1000 <= lap; km++ {
			x := xAt(float64(km) * 1000)
			dc.SetHexColor(subtleColor)
			dc.DrawStringAnchored(fmt.Sprintf("%d km", km), x, py+ph+20, 0.5, 0.35)
		}
	}

	// Filled profile, closed back to the start.
	dc.MoveTo(xAt(0), yAt(0))
	for i, z := range t.Elevation {
		dc.LineTo(xAt(dist[i]), yAt(z))
	}
	dc.LineTo(xAt(lap), yAt(t.Elevation[0]))
	dc.LineTo(xAt(lap), yAt(0))
	dc.ClosePath()
	dc.SetHexColor(asphaltColor)
	dc.Fill()

	// Highlight the part of the lap shown in a corner image, over the fill
	// so it stands out above and below the line.
	if e.focus != nil {
		ci := nearestIndex(t.Outline, e.focus.Position)
		from, to := dist[ci]-cornerRadius/10, dist[ci]+cornerRadius/10
		for _, r := range [][2]float64{{from, to}, {from + lap, to + lap}, {from - lap, to - lap}} {
			a, b := max(r[0], 0), min(r[1], lap)
			if b <= a {
				continue
			}
			x0, x1 := xAt(a), xAt(b)
			dc.SetHexColor("#E1060030")
			dc.DrawRectangle(x0, py, x1-x0, ph)
			dc.Fill()
			dc.SetHexColor("#E1060090")
			dc.SetLineWidth(1)
			for _, x := range []float64{x0, x1} {
				if x > px && x < px+pw {
					dc.DrawLine(x, py, x, py+ph)
					dc.Stroke()
				}
			}
		}
	}

	// Profile line, coloured by sector on the full map.
	dc.SetLineWidth(3)
	dc.SetLineJoin(gg.LineJoinRound)
	for s, r := range sectorRanges(t) {
		dc.NewSubPath()
		for i := r[0]; i <= r[1]; i++ {
			d := lap
			if i < len(dist) {
				d = dist[i]
			}
			dc.LineTo(xAt(d), yAt(t.Elevation[i%len(t.Elevation)]))
		}
		switch {
		case !e.detailed || len(t.SectorStarts) != 2:
			dc.SetHexColor(edgeColor)
		default:
			dc.SetHexColor(sectorColors[s])
		}
		dc.Stroke()
	}

	// Turn numbers above the profile, with a faint line down to it.
	if e.detailed {
		// Turns too close to the previous one move up a row.
		dc.SetFontFace(face(boldFont, 12))
		prevX, row := math.Inf(-1), 0
		for _, c := range t.Corners {
			i := nearestIndex(t.Outline, c.Position)
			x := xAt(dist[i])
			if x-prevX < 22 && row == 0 {
				row = 1
			} else {
				row = 0
			}
			prevX = x
			ly := py - 14 - float64(row)*22
			dc.SetHexColor("#FFFFFF30")
			dc.SetLineWidth(1)
			dc.DrawLine(x, ly+10, x, yAt(t.Elevation[i]))
			dc.Stroke()
			dc.SetHexColor(textColor)
			dc.DrawCircle(x, ly, 10)
			dc.Fill()
			dc.SetHexColor(bgColor)
			dc.DrawStringAnchored(fmt.Sprint(c.Number), x, ly, 0.5, 0.35)
		}
	}

	// The focused corner, with the corners before and after it.
	if e.focus != nil {
		dc.SetFontFace(face(boldFont, 11))
		i := nearestIndex(t.Outline, e.focus.Position)
		x, y := xAt(dist[i]), yAt(t.Elevation[i])
		for _, c := range neighbours(t, e.focus.Number) {
			j := nearestIndex(t.Outline, c.Position)
			nx, ny := xAt(dist[j]), yAt(t.Elevation[j])
			// Lift a neighbour that would overlap the focused corner.
			if lifted := math.Abs(nx-x) < 24; lifted {
				dc.SetHexColor("#FFFFFF60")
				dc.SetLineWidth(1)
				dc.DrawLine(nx, ny, nx, ny-24)
				dc.Stroke()
				ny -= 24
			}
			dc.SetHexColor(textColor)
			dc.DrawCircle(nx, ny, 9)
			dc.Fill()
			dc.SetHexColor(bgColor)
			dc.DrawStringAnchored(fmt.Sprint(c.Number), nx, ny, 0.5, 0.35)
		}
		dc.SetHexColor(kerbRed)
		dc.DrawCircle(x, y, 11)
		dc.Fill()
		dc.SetHexColor(textColor)
		dc.DrawStringAnchored(fmt.Sprint(e.focus.Number), x, y, 0.5, 0.35)
	}
}

// neighbours returns the corners before and after a corner in lap order,
// wrapping round the lap.
func neighbours(t *Track, number int) []Corner {
	n := len(t.Corners)
	for i, c := range t.Corners {
		if c.Number == number && n > 1 {
			prev, next := t.Corners[(i-1+n)%n], t.Corners[(i+1)%n]
			if prev.Number == next.Number {
				return []Corner{prev}
			}
			return []Corner{prev, next}
		}
	}
	return nil
}

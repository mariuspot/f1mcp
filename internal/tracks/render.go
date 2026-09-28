package tracks

import (
	"fmt"
	"image"
	"math"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

const (
	mapWidth, mapHeight       = 1600, 1200
	cornerWidth, cornerHeight = 1200, 900

	header = 110 // room for the title

	// trackWidth is the asphalt width in track units (decimetres), used
	// when zoomed in. The full map uses a fixed pixel width instead.
	trackWidth = 110.0
	// cornerRadius is how much track, in track units, a corner image shows
	// around the corner.
	cornerRadius = 1800.0
	// kerbLength is how far a kerb extends either side of a corner, in
	// track units.
	kerbLength = 200.0
)

var (
	bgColor      = "#15151E"
	asphaltColor = "#3A3A44"
	edgeColor    = "#F0F0F0"
	kerbRed      = "#E10600"
	kerbWhite    = "#F0F0F0"
	textColor    = "#FFFFFF"
	subtleColor  = "#9A9AA8"
	leaderColor  = "#6A6A78"
	insetColor   = "#1F1F2A"
	// Timing sector colours, as used on F1 graphics.
	sectorColors = [3]string{"#E10600", "#00A0DE", "#FFD100"}

	regularFont = mustParse(goregular.TTF)
	boldFont    = mustParse(gobold.TTF)
)

func mustParse(ttf []byte) *truetype.Font {
	f, err := truetype.Parse(ttf)
	if err != nil {
		panic(err)
	}
	return f
}

func face(f *truetype.Font, size float64) font.Face {
	return truetype.NewFace(f, &truetype.Options{Size: size})
}

// projection maps track coordinates to image pixels: rotated by the track's
// rotation, scaled, and with y flipped so up is up.
type projection struct {
	sin, cos float64
	scale    float64
	cx, cy   float64 // centre, in rotated track coordinates
	ox, oy   float64 // centre, in pixels
}

func newProjection(rotation float64) projection {
	rad := rotation * math.Pi / 180
	return projection{sin: math.Sin(rad), cos: math.Cos(rad), scale: 1}
}

// fit centres pts in the pixel rectangle (x, y, w, h), scaled to fit.
func (p projection) fit(pts []Point, x, y, w, h float64) projection {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, pt := range pts {
		rx, ry := p.rotate(pt)
		minX, maxX = min(minX, rx), max(maxX, rx)
		minY, maxY = min(minY, ry), max(maxY, ry)
	}
	p.scale = min(w/(maxX-minX), h/(maxY-minY))
	p.cx, p.cy = (minX+maxX)/2, (minY+maxY)/2
	p.ox, p.oy = x+w/2, y+h/2
	return p
}

// centre centres pt at pixel (x, y) with the given scale.
func (p projection) centre(pt Point, x, y, scale float64) projection {
	p.cx, p.cy = p.rotate(pt)
	p.ox, p.oy = x, y
	p.scale = scale
	return p
}

func (p projection) rotate(pt Point) (float64, float64) {
	return pt[0]*p.cos - pt[1]*p.sin, pt[0]*p.sin + pt[1]*p.cos
}

func (p projection) point(pt Point) (float64, float64) {
	x, y := p.rotate(pt)
	return p.ox + (x-p.cx)*p.scale, p.oy - (y-p.cy)*p.scale
}

// direction converts an angle in track coordinates to a unit vector in image
// space.
func (p projection) direction(deg float64) (float64, float64) {
	rad := deg * math.Pi / 180
	x, y := p.rotate(Point{math.Cos(rad), math.Sin(rad)})
	return x, -y
}

// Options adjust how a track is drawn.
type Options struct {
	// Years labels the seasons shown, e.g. "2023–2026". Defaults to the
	// track's year.
	Years string
}

func (o Options) years(t *Track) string {
	if o.Years != "" {
		return o.Years
	}
	return fmt.Sprint(t.Year)
}

// Render draws the full circuit map: the track with timing sectors, kerbs,
// the start/finish line, and numbered (and, where known, named) corners.
func Render(t *Track, o Options) (image.Image, error) {
	if len(t.Outline) < 2 {
		return nil, fmt.Errorf("track %s %d has no outline", t.CircuitID, t.Year)
	}
	const margin = 150 // room for corner labels around the track
	p := newProjection(t.Rotation).fit(t.Outline, margin, header+margin,
		mapWidth-2*margin, mapHeight-header-2*margin)
	dc := gg.NewContext(mapWidth, mapHeight)
	dc.SetHexColor(bgColor)
	dc.Clear()

	drawTrack(dc, p, t, 14)
	for _, c := range t.Corners {
		drawCornerMarker(dc, p, c, 40, 14, false, true)
	}
	drawTitle(dc, t.Name, fmt.Sprintf("%s, %s · %s", t.Locality, t.Country, o.years(t)))
	drawSectorLegend(dc, t, mapHeight-40)
	drawCredit(dc, mapWidth, mapHeight)
	return dc.Image(), nil
}

// RenderCorner draws a close-up of one corner, with a small map of the whole
// circuit showing where it is.
func RenderCorner(t *Track, number int, o Options) (image.Image, error) {
	i := -1
	for j, c := range t.Corners {
		if c.Number == number {
			i = j
		}
	}
	if i < 0 {
		return nil, fmt.Errorf("track %s %d has no turn %d", t.CircuitID, t.Year, number)
	}
	c := t.Corners[i]
	scale := float64(cornerHeight-header) / (2 * cornerRadius)
	p := newProjection(t.Rotation).centre(c.Position, cornerWidth/2, header+float64(cornerHeight-header)/2, scale)

	dc := gg.NewContext(cornerWidth, cornerHeight)
	dc.SetHexColor(bgColor)
	dc.Clear()

	drawTrack(dc, p, t, trackWidth*scale)
	drawDirectionChevrons(dc, p, t, c, trackWidth*scale)
	for _, other := range t.Corners {
		if other.Number != number {
			drawCornerMarker(dc, p, other, 70, 16, false, false)
		}
	}
	drawCornerMarker(dc, p, c, 80, 24, true, false)

	title := fmt.Sprintf("Turn %d", c.Number)
	if c.Name != "" {
		title += " · " + c.Name
	}
	drawTitle(dc, title, fmt.Sprintf("%s · %s", t.Name, o.years(t)))
	drawInset(dc, t, c)
	drawCredit(dc, cornerWidth, cornerHeight)
	return dc.Image(), nil
}

// drawTrack draws the asphalt with sector-coloured edges, kerbs at each
// corner, and the start/finish line. width is the asphalt width in pixels.
func drawTrack(dc *gg.Context, p projection, t *Track, width float64) {
	dc.SetLineJoin(gg.LineJoinRound)
	dc.SetLineCap(gg.LineCapRound)
	edge := max(3, width*0.2)

	// Edges, coloured by sector when sectors are known.
	sectors := sectorRanges(t)
	for s, r := range sectors {
		tracePath(dc, p, t.Outline, r[0], r[1])
		if len(sectors) == 1 {
			dc.SetHexColor(edgeColor)
		} else {
			dc.SetHexColor(sectorColors[s])
		}
		dc.SetLineWidth(width + 2*edge)
		dc.Stroke()
	}
	tracePath(dc, p, t.Outline, 0, len(t.Outline))
	dc.SetHexColor(asphaltColor)
	dc.SetLineWidth(width)
	dc.Stroke()

	for _, c := range t.Corners {
		drawKerb(dc, p, t.Outline, c, width, edge)
	}
	drawStartFinish(dc, p, t.Outline, width+2*edge)
}

// sectorRanges returns the outline index ranges [from, to) of each sector,
// or the whole lap as one range if sectors are unknown.
func sectorRanges(t *Track) [][2]int {
	n := len(t.Outline)
	if len(t.SectorStarts) != 2 {
		return [][2]int{{0, n}}
	}
	s2, s3 := t.SectorStarts[0], t.SectorStarts[1]
	return [][2]int{{0, s2}, {s2, s3}, {s3, n}}
}

// tracePath adds outline[from:to] as a path, closing the loop at the end of
// the lap.
func tracePath(dc *gg.Context, p projection, outline []Point, from, to int) {
	dc.NewSubPath()
	for i := from; i <= to; i++ {
		dc.LineTo(p.point(outline[i%len(outline)]))
	}
}

// drawKerb draws a red and white kerb on the inside of a corner.
func drawKerb(dc *gg.Context, p projection, outline []Point, c Corner, width, edge float64) {
	n := len(outline)
	ic := nearestIndex(outline, c.Position)
	from, to := indexWithin(outline, ic, -1), indexWithin(outline, ic, 1)

	// Which side is inside: the side the track turns towards.
	ax, ay := p.point(outline[(from+n)%n])
	bx, by := p.point(outline[ic])
	cx, cy := p.point(outline[(to+n)%n])
	right := (bx-ax)*(cy-by)-(by-ay)*(cx-bx) > 0

	offset := width/2 + edge/2
	kerb := max(2.5, edge)
	var pts [][2]float64
	for i := from; i <= to; i++ {
		x, y := p.point(outline[(i+n)%n])
		nx, ny := p.point(outline[(i+1+n)%n])
		px, py := p.point(outline[(i-1+n)%n])
		tx, ty := nx-px, ny-py
		l := math.Hypot(tx, ty)
		if l == 0 {
			continue
		}
		tx, ty = tx/l, ty/l
		// In image space (y down), (-ty, tx) points right of travel.
		sx, sy := ty, -tx
		if right {
			sx, sy = -ty, tx
		}
		pts = append(pts, [2]float64{x + sx*offset, y + sy*offset})
	}
	trace := func() {
		dc.NewSubPath()
		for _, pt := range pts {
			dc.LineTo(pt[0], pt[1])
		}
	}

	dc.SetLineCap(gg.LineCapButt)
	dc.SetLineWidth(kerb)
	trace()
	dc.SetHexColor(kerbWhite)
	dc.Stroke()
	trace()
	dc.SetHexColor(kerbRed)
	dc.SetDash(kerb*1.6, kerb*1.6)
	dc.Stroke()
	dc.SetDash()
	dc.SetLineCap(gg.LineCapRound)
}

// indexWithin walks from outline index i in direction dir (+1 or -1) until
// kerbLength of track is covered, and returns the (unwrapped) index reached.
func indexWithin(outline []Point, i, dir int) int {
	n := len(outline)
	dist := 0.0
	j := i
	for steps := 0; steps < n/4; steps++ {
		a, b := outline[(j+n)%n], outline[(j+dir+n)%n]
		dist += math.Hypot(b[0]-a[0], b[1]-a[1])
		j += dir
		if dist >= kerbLength {
			break
		}
	}
	return j
}

// drawStartFinish draws a chequered line across the track at the start of
// the outline, with an arrow beside it showing the direction of travel.
func drawStartFinish(dc *gg.Context, p projection, outline []Point, width float64) {
	x0, y0 := p.point(outline[0])
	x1, y1 := p.point(outline[min(3, len(outline)-1)])
	dx, dy := x1-x0, y1-y0
	l := math.Hypot(dx, dy)
	dx, dy = dx/l, dy/l
	nx, ny := -dy, dx

	const across = 6
	sq := width / across
	for row := range 2 {
		for col := range across {
			if (row+col)%2 == 0 {
				dc.SetHexColor("#FFFFFF")
			} else {
				dc.SetHexColor("#000000")
			}
			// Corner of this square.
			ox := x0 + nx*(float64(col)*sq-width/2) + dx*float64(row-1)*sq
			oy := y0 + ny*(float64(col)*sq-width/2) + dy*float64(row-1)*sq
			dc.MoveTo(ox, oy)
			dc.LineTo(ox+nx*sq, oy+ny*sq)
			dc.LineTo(ox+nx*sq+dx*sq, oy+ny*sq+dy*sq)
			dc.LineTo(ox+dx*sq, oy+dy*sq)
			dc.ClosePath()
			dc.Fill()
		}
	}

	// Arrow beside the track, pointing along it.
	size := max(10, width*0.5)
	ax, ay := x0+nx*(width/2+size*1.4)+dx*size*2, y0+ny*(width/2+size*1.4)+dy*size*2
	dc.SetHexColor(textColor)
	dc.MoveTo(ax+dx*size, ay+dy*size)
	dc.LineTo(ax-dx*size*0.7+nx*size*0.6, ay-dy*size*0.7+ny*size*0.6)
	dc.LineTo(ax-dx*size*0.7-nx*size*0.6, ay-dy*size*0.7-ny*size*0.6)
	dc.ClosePath()
	dc.Fill()
}

// drawDirectionChevrons draws arrows on the asphalt before and after a
// corner, showing which way cars travel through it.
func drawDirectionChevrons(dc *gg.Context, p projection, t *Track, c Corner, width float64) {
	n := len(t.Outline)
	ic := nearestIndex(t.Outline, c.Position)
	for _, i := range []int{indexWithin(t.Outline, ic, -1) - 4, indexWithin(t.Outline, ic, 1) + 4} {
		x, y := p.point(t.Outline[(i+n)%n])
		nx, ny := p.point(t.Outline[(i+2+n)%n])
		dx, dy := nx-x, ny-y
		l := math.Hypot(dx, dy)
		if l == 0 {
			continue
		}
		dx, dy = dx/l, dy/l
		s := width * 0.4
		dc.SetHexColor(textColor)
		dc.SetLineWidth(max(3, width*0.12))
		dc.MoveTo(x-dx*s-dy*s, y-dy*s+dx*s)
		dc.LineTo(x, y)
		dc.LineTo(x-dx*s+dy*s, y-dy*s-dx*s)
		dc.Stroke()
	}
}

// drawCornerMarker draws a corner's number in a circle away from the track,
// linked to it by a short line. The highlighted corner is drawn in red.
func drawCornerMarker(dc *gg.Context, p projection, c Corner, distance, radius float64, highlight, withName bool) {
	x, y := p.point(c.Position)
	dx, dy := p.direction(c.Angle)
	cx, cy := x+dx*distance, y+dy*distance

	dc.SetHexColor(leaderColor)
	dc.SetLineWidth(2)
	dc.DrawLine(x+dx*distance*0.35, y+dy*distance*0.35, cx, cy)
	dc.Stroke()

	if highlight {
		dc.SetHexColor(kerbRed)
	} else {
		dc.SetHexColor("#FFFFFF")
	}
	dc.DrawCircle(cx, cy, radius)
	dc.Fill()
	dc.SetFontFace(face(boldFont, radius*1.05))
	if highlight {
		dc.SetHexColor("#FFFFFF")
	} else {
		dc.SetHexColor(bgColor)
	}
	dc.DrawStringAnchored(fmt.Sprint(c.Number), cx, cy, 0.5, 0.35)

	if withName && c.Name != "" {
		dc.SetFontFace(face(regularFont, 16))
		dc.SetHexColor(textColor)
		if dx >= 0 {
			dc.DrawStringAnchored(c.Name, cx+radius+8, cy, 0, 0.35)
		} else {
			dc.DrawStringAnchored(c.Name, cx-radius-8, cy, 1, 0.35)
		}
	}
}

// drawInset draws a small map of the whole circuit in the top right corner,
// with the given corner marked.
func drawInset(dc *gg.Context, t *Track, c Corner) {
	const w, h, pad = 280.0, 210.0, 16.0
	x, y := float64(dc.Width())-w-24, 24.0
	dc.SetHexColor(insetColor)
	dc.DrawRoundedRectangle(x, y, w, h, 10)
	dc.Fill()

	p := newProjection(t.Rotation).fit(t.Outline, x+pad, y+pad, w-2*pad, h-2*pad)
	tracePath(dc, p, t.Outline, 0, len(t.Outline))
	dc.SetHexColor(edgeColor)
	dc.SetLineWidth(3)
	dc.Stroke()

	cx, cy := p.point(c.Position)
	dc.SetHexColor(kerbRed)
	dc.DrawCircle(cx, cy, 8)
	dc.Fill()
}

func drawTitle(dc *gg.Context, title, subtitle string) {
	dc.SetFontFace(face(boldFont, 36))
	dc.SetHexColor(textColor)
	dc.DrawString(title, 48, 64)
	dc.SetFontFace(face(regularFont, 20))
	dc.SetHexColor(subtleColor)
	dc.DrawString(subtitle, 48, 96)
}

func drawSectorLegend(dc *gg.Context, t *Track, y float64) {
	if len(t.SectorStarts) != 2 {
		return
	}
	dc.SetFontFace(face(regularFont, 16))
	x := 48.0
	for s, color := range sectorColors {
		dc.SetHexColor(color)
		dc.DrawRoundedRectangle(x, y-10, 28, 8, 3)
		dc.Fill()
		dc.SetHexColor(subtleColor)
		label := fmt.Sprintf("Sector %d", s+1)
		dc.DrawStringAnchored(label, x+36, y-6, 0, 0.35)
		w, _ := dc.MeasureString(label)
		x += 36 + w + 28
	}
}

func drawCredit(dc *gg.Context, width, height float64) {
	dc.SetFontFace(face(regularFont, 14))
	dc.SetHexColor(subtleColor)
	dc.DrawStringAnchored("Track data: MultiViewer · OpenF1 · Jolpica", width-32, height-24, 1, 0)
}

func nearestIndex(pts []Point, p Point) int {
	best, bestD := 0, math.Inf(1)
	for i, q := range pts {
		if d := (q[0]-p[0])*(q[0]-p[0]) + (q[1]-p[1])*(q[1]-p[1]); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

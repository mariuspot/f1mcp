package tracks

import (
	"fmt"
	"image"
	"math"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goitalic"
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
	italicFont  = mustParse(goitalic.TTF)
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
	// Overlay adds flags, cars and incidents on top of the track.
	Overlay *Overlay
	// LapEvents makes RenderLapGIF leave marks on the track where each car
	// braked and changed gear.
	LapEvents bool
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
	p := mapProjection(t)
	height := mapHeight
	if len(t.Elevation) > 0 {
		height += mapElevationHeight
	}
	dc := gg.NewContext(mapWidth, height)
	dc.SetHexColor(bgColor)
	dc.Clear()

	drawTrack(dc, p, t, 14)
	var taken []rect
	for _, st := range t.Straights {
		if r, ok := drawStraightName(dc, p, t, st, 14+2*3); ok {
			taken = append(taken, r)
		}
	}
	drawHighlights(dc, p, t, o.Overlay, 14)
	for _, c := range t.Corners {
		drawCornerMarker(dc, p, c, 40, 14, false)
	}
	taken = append(taken, drawMarshalNumbers(dc, p, t, 14+2*3)...)
	taken = drawCornerNames(dc, p, t, 40, 14, 20, taken)
	taken = append(taken, drawMarkers(dc, p, o.Overlay, 9)...)
	drawHighlightLabels(dc, p, t, o.Overlay, 14, taken)
	drawTitle(dc, t.Name, fmt.Sprintf("%s, %s · %s", t.Locality, t.Country, o.years(t)))
	drawBanner(dc, o.Overlay)
	drawSectorLegend(dc, t, mapHeight-40)
	drawElevation(dc, t, mapElevationPanel())
	drawCredit(dc, mapWidth, float64(height))
	return dc.Image(), nil
}

// mapProjection is how Render places a track on the full map.
func mapProjection(t *Track) projection {
	const margin = 150 // room for corner labels around the track
	return newProjection(t.Rotation).fit(t.Outline, margin, header+margin,
		mapWidth-2*margin, mapHeight-header-2*margin)
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

	height := cornerHeight
	if len(t.Elevation) > 0 {
		height += cornerElevationHeight
	}
	dc := gg.NewContext(cornerWidth, height)
	dc.SetHexColor(bgColor)
	dc.Clear()

	drawTrack(dc, p, t, trackWidth*scale)
	drawDirectionChevrons(dc, p, t, c, trackWidth*scale)
	for _, st := range t.Straights {
		drawStraightName(dc, p, t, st, trackWidth*scale*1.4)
	}

	drawHighlights(dc, p, t, o.Overlay, trackWidth*scale)
	for _, other := range t.Corners {
		if other.Number != number {
			drawCornerMarker(dc, p, other, 70, 16, false)
		}
	}
	drawCornerMarker(dc, p, c, 80, 24, true)
	taken := markerRects(p, t, 70, 16)
	taken = append(taken, drawMarshalNumbers(dc, p, t, trackWidth*scale)...)
	taken = append(taken, drawMarkers(dc, p, o.Overlay, 13)...)
	drawHighlightLabels(dc, p, t, o.Overlay, trackWidth*scale, taken)

	title := fmt.Sprintf("Turn %d", c.Number)
	if c.Name != "" {
		title += " · " + c.Name
	}
	subtitle := fmt.Sprintf("%s · %s", t.Name, o.years(t))
	if c.AltName != "" {
		subtitle = "Also known as " + c.AltName + " · " + subtitle
	}
	drawTitle(dc, title, subtitle)
	drawBanner(dc, o.Overlay)
	drawInset(dc, t, c)
	if len(t.Elevation) > 0 {
		// Cover track that runs past the close-up, then draw the profile.
		dc.SetHexColor(bgColor)
		dc.DrawRectangle(0, cornerHeight, cornerWidth, cornerElevationHeight)
		dc.Fill()
		drawElevation(dc, t, elevationPanel{x: 24, y: cornerHeight + 6, w: cornerWidth - 48, h: cornerElevationHeight - 44, focus: &c})
	}
	drawCredit(dc, cornerWidth, float64(height))
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
	drawMarshalStrip(dc, p, t, width, edge)
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
	// Heading over a stretch of track, so one odd point can't flip it.
	x1, y1 := p.point(outline[min(8, len(outline)-1)])
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

// drawStraightName writes a straight's name alongside it, halfway between
// its corners, turned to follow the track and kept upright. width is the
// drawn track width in pixels.
func drawStraightName(dc *gg.Context, p projection, t *Track, st Straight, width float64) (rect, bool) {
	from, okFrom := cornerIndex(t, st.From)
	to, okTo := cornerIndex(t, st.To)
	if !okFrom || !okTo {
		return rect{}, false
	}
	n := len(t.Outline)
	if to < from {
		to += n // the straight crosses the start/finish line
	}
	mid := (from + to) / 2
	x, y := p.point(t.Outline[mid%n])
	ax, ay := p.point(t.Outline[(mid-3+n)%n])
	bx, by := p.point(t.Outline[(mid+3)%n])
	dx, dy := bx-ax, by-ay
	l := math.Hypot(dx, dy)
	if l == 0 {
		return rect{}, false
	}
	dx, dy = dx/l, dy/l

	// Put the name on the outside of the circuit, away from its centre.
	nx, ny := -dy, dx
	cx, cy := outlineCentre(p, t.Outline)
	if (x-cx)*nx+(y-cy)*ny < 0 {
		nx, ny = -nx, -ny
	}
	offset := width/2 + 16
	lx, ly := x+nx*offset, y+ny*offset

	angle := math.Atan2(dy, dx)
	if angle > math.Pi/2 {
		angle -= math.Pi
	} else if angle < -math.Pi/2 {
		angle += math.Pi
	}
	dc.SetFontFace(face(italicFont, 17))
	w, h := dc.MeasureString(st.Name)
	dc.Push()
	dc.RotateAbout(angle, lx, ly)
	dc.SetHexColor(subtleColor)
	dc.DrawStringAnchored(st.Name, lx, ly, 0.5, 0.35)
	dc.Pop()

	// Bounding box of the rotated text.
	cos, sin := math.Abs(math.Cos(angle)), math.Abs(math.Sin(angle))
	bw, bh := w*cos+h*sin, w*sin+h*cos
	return rect{lx - bw/2, ly - bh/2, lx + bw/2, ly + bh/2}, true
}

func cornerIndex(t *Track, number int) (int, bool) {
	for _, c := range t.Corners {
		if c.Number == number {
			return nearestIndex(t.Outline, c.Position), true
		}
	}
	return 0, false
}

// outlineCentre returns the centre of the outline's bounding box in pixels.
func outlineCentre(p projection, outline []Point) (float64, float64) {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, pt := range outline {
		x, y := p.point(pt)
		minX, maxX = min(minX, x), max(maxX, x)
		minY, maxY = min(minY, y), max(maxY, y)
	}
	return (minX + maxX) / 2, (minY + maxY) / 2
}

// drawCornerMarker draws a corner's number in a circle away from the track,
// linked to it by a short line. The highlighted corner is drawn in red.
func drawCornerMarker(dc *gg.Context, p projection, c Corner, distance, radius float64, highlight bool) {
	x, y := p.point(c.Position)
	dx, dy := p.direction(c.Angle)
	cx, cy := markerCentre(p, c, distance)

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

}

func markerCentre(p projection, c Corner, distance float64) (float64, float64) {
	x, y := p.point(c.Position)
	dx, dy := p.direction(c.Angle)
	return x + dx*distance, y + dy*distance
}

type rect struct{ x0, y0, x1, y1 float64 }

// bestPlacement returns the candidate label box that overlaps least with
// labels already placed, the track and the edges of the image.
func bestPlacement(candidates, taken []rect, track [][2]float64, trackWidth, W, H float64) rect {
	best, bestScore := candidates[0], math.Inf(1)
	for _, r := range candidates {
		score := 0.0
		for _, o := range taken {
			if r.overlaps(o) {
				score += 10
			}
		}
		for _, pt := range track {
			if r.contains(pt[0], pt[1], trackWidth/2) {
				score += 2
			}
		}
		if r.x0 < 0 || r.y0 < header || r.x1 > W || r.y1 > H {
			score += 100
		}
		if score < bestScore {
			best, bestScore = r, score
		}
	}
	return best
}

// screenTrack returns the outline in pixels.
func screenTrack(p projection, outline []Point) [][2]float64 {
	out := make([][2]float64, len(outline))
	for i, pt := range outline {
		x, y := p.point(pt)
		out[i] = [2]float64{x, y}
	}
	return out
}

// markerRects returns the boxes of the corner markers.
func markerRects(p projection, t *Track, distance, radius float64) []rect {
	var out []rect
	for _, c := range t.Corners {
		cx, cy := markerCentre(p, c, distance)
		out = append(out, rect{cx - radius, cy - radius, cx + radius, cy + radius})
	}
	return out
}

func (r rect) overlaps(o rect) bool {
	return r.x0 < o.x1 && o.x0 < r.x1 && r.y0 < o.y1 && o.y0 < r.y1
}

func (r rect) contains(x, y, pad float64) bool {
	return x > r.x0-pad && x < r.x1+pad && y > r.y0-pad && y < r.y1+pad
}

// drawCornerNames writes corner names beside their markers. Each name goes
// on whichever side of its marker overlaps least with other markers, names
// already placed and the track. A complex's name is shown once, at its first
// turn.
func drawCornerNames(dc *gg.Context, p projection, t *Track, distance, radius, trackWidth float64, taken []rect) []rect {
	nameFace, altFace := face(regularFont, 16), face(italicFont, 14)
	dc.SetFontFace(nameFace)
	for _, c := range t.Corners {
		cx, cy := markerCentre(p, c, distance)
		taken = append(taken, rect{cx - radius, cy - radius, cx + radius, cy + radius})
	}
	track := screenTrack(p, t.Outline)
	W, H := float64(dc.Width()), float64(dc.Height())

	for i, c := range t.Corners {
		if c.Name == "" || (i > 0 && t.Corners[i-1].Name == c.Name) {
			continue
		}
		cx, cy := markerCentre(p, c, distance)
		dx, _ := p.direction(c.Angle)
		w, h := dc.MeasureString(c.Name)
		lineH := h
		if c.AltName != "" {
			dc.SetFontFace(altFace)
			aw, ah := dc.MeasureString(c.AltName)
			dc.SetFontFace(nameFace)
			w = max(w, aw)
			h += ah + 4
		}
		gap := radius + 8
		right := rect{cx + gap, cy - h/2, cx + gap + w, cy + h/2}
		left := rect{cx - gap - w, cy - h/2, cx - gap, cy + h/2}
		above := rect{cx - w/2, cy - gap - h, cx + w/2, cy - gap}
		below := rect{cx - w/2, cy + gap, cx + w/2, cy + gap + h}
		d := gap * 0.7
		upRight := rect{cx + d, cy - d - h, cx + d + w, cy - d}
		upLeft := rect{cx - d - w, cy - d - h, cx - d, cy - d}
		downRight := rect{cx + d, cy + d, cx + d + w, cy + d + h}
		downLeft := rect{cx - d - w, cy + d, cx - d, cy + d + h}
		candidates := []rect{right, left, above, below, upRight, upLeft, downRight, downLeft}
		if dx < 0 {
			candidates = []rect{left, right, above, below, upLeft, upRight, downLeft, downRight}
		}

		best := bestPlacement(candidates, taken, track, trackWidth, W, H)
		taken = append(taken, best)
		// Align text to the marker side of the box.
		ax, tx := 0.0, best.x0
		if best.x1 <= cx-radius {
			ax, tx = 1, best.x1
		} else if best.x0 < cx && best.x1 > cx {
			ax, tx = 0.5, (best.x0+best.x1)/2
		}
		dc.SetHexColor(textColor)
		dc.DrawStringAnchored(c.Name, tx, best.y0+lineH/2, ax, 0.35)
		if c.AltName != "" {
			dc.SetFontFace(altFace)
			dc.SetHexColor(subtleColor)
			dc.DrawStringAnchored(c.AltName, tx, best.y1-(h-lineH-4)/2, ax, 0.35)
			dc.SetFontFace(nameFace)
		}
	}
	return taken
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
	dc.SetLineWidth(3)
	dc.SetLineCap(gg.LineCapRound)
	sectors := sectorRanges(t)
	for s, r := range sectors {
		tracePath(dc, p, t.Outline, r[0], r[1])
		if len(sectors) == 1 {
			dc.SetHexColor(edgeColor)
		} else {
			dc.SetHexColor(sectorColors[s])
		}
		dc.Stroke()
	}

	drawInsetStart(dc, p, t.Outline)

	// White ring so the dot stands out on the red first sector.
	cx, cy := p.point(c.Position)
	dc.SetHexColor(textColor)
	dc.DrawCircle(cx, cy, 10)
	dc.Fill()
	dc.SetHexColor(kerbRed)
	dc.DrawCircle(cx, cy, 7)
	dc.Fill()
}

// drawInsetStart marks the start/finish line on the inset map with a bar
// across the track and an arrow beside it showing the direction of travel.
func drawInsetStart(dc *gg.Context, p projection, outline []Point) {
	x0, y0 := p.point(outline[0])
	x1, y1 := p.point(outline[min(8, len(outline)-1)])
	dx, dy := x1-x0, y1-y0
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	dx, dy = dx/l, dy/l
	nx, ny := -dy, dx

	const half = 9.0
	dc.SetLineCap(gg.LineCapButt)
	dc.SetHexColor("#000000")
	dc.SetLineWidth(6)
	dc.DrawLine(x0-nx*half, y0-ny*half, x0+nx*half, y0+ny*half)
	dc.Stroke()
	dc.SetHexColor(textColor)
	dc.SetLineWidth(3)
	dc.DrawLine(x0-nx*half, y0-ny*half, x0+nx*half, y0+ny*half)
	dc.Stroke()
	dc.SetLineCap(gg.LineCapRound)

	const size = 7.0
	ax, ay := x0+nx*(half+size+2)+dx*size, y0+ny*(half+size+2)+dy*size
	dc.MoveTo(ax+dx*size, ay+dy*size)
	dc.LineTo(ax-dx*size*0.7+nx*size*0.6, ay-dy*size*0.7+ny*size*0.6)
	dc.LineTo(ax-dx*size*0.7-nx*size*0.6, ay-dy*size*0.7-ny*size*0.6)
	dc.ClosePath()
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

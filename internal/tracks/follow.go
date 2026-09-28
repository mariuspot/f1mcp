package tracks

import (
	"fmt"
	"image"
	"image/draw"
	"math"

	"github.com/fogleman/gg"
)

const (
	// Follow-camera video frames: 4:5 portrait for phones. From the top: a
	// header naming the circuit and laps, the timing band, the camera view,
	// and the elevation profile.
	followWidth, followHeight = 720, 900
	followHeader              = 64
	followElevation           = 150
	followFPS                 = 25.0
	// followPad is extra space around the pre-drawn track, so the camera
	// never runs off its edge.
	followPad = 900
)

// FollowFPS is the frame rate RenderLapFollow draws at.
const FollowFPS = followFPS

// FollowSize returns the size of RenderLapFollow's frames.
func FollowSize() (int, int) { return followWidth, followHeight }

// RenderLapFollow animates laps with a camera that follows the first car,
// zoomed in like the corner images, and passes each frame to sink. The
// track is drawn once, larger than the view, and each frame is a crop of it
// with the cars, their braking and gear changes, the timing band and a mini
// map of the whole circuit on top. Frames come at FollowFPS, played at the
// same speed-up as the GIFs.
func RenderLapFollow(t *Track, laps []LapTrace, o Options, sink func(*image.RGBA) error) error {
	if len(laps) == 0 {
		return fmt.Errorf("no laps to draw")
	}
	laps = smoothLaps(t, laps)
	scale := float64(cornerHeight-header) / (2 * cornerRadius)
	big, p := drawBigTrack(t, scale, o)

	band := lapBandHeight(len(laps))
	top := followHeader + band
	view := image.Rect(0, top, followWidth, followHeight-followElevation)
	outward := -insideSide(p, t.Outline)
	dist, lap := lapDistances(t.Outline)
	progress := make([][]float64, len(laps))
	events := make([][]lapEvent, len(laps))
	longest := 0.0
	for i, l := range laps {
		progress[i] = lapProgress(t, l, dist, lap)
		events[i] = l.events()
		longest = max(longest, l.Duration)
	}

	// Everything that doesn't move is drawn once: the header and the
	// elevation profile.
	blank := image.NewRGBA(image.Rect(0, 0, followWidth, followHeight))
	bc := gg.NewContextForRGBA(blank)
	bc.SetHexColor(bgColor)
	bc.Clear()
	bc.SetFontFace(face(boldFont, 24))
	bc.SetHexColor(textColor)
	bc.DrawString(fmt.Sprintf("%s · %s", t.Name, o.years(t)), 16, 32)
	if o.Overlay != nil && o.Overlay.Banner != "" {
		bc.SetFontFace(face(regularFont, 14))
		bc.SetHexColor(subtleColor)
		bc.DrawString(o.Overlay.Banner, 16, 54)
	}
	elev := elevationPanel{x: 12, y: followHeight - followElevation + 6, w: followWidth - 24, h: followElevation - 12}
	drawElevation(bc, t, elev)
	// How much of the lap the camera view spans, in metres.
	viewSpan := float64(followWidth) / scale / 10

	step := lapSpeedUp / followFPS
	holdFrames := int(math.Round(lapHold * followFPS))
	var last *image.RGBA
	for T := 0.0; ; T += step {
		T = min(T, longest)
		// Camera on the first car, smoothed over a moment either side.
		var cx, cy float64
		for _, dt := range []float64{-0.3, -0.15, 0, 0.15, 0.3} {
			x, y := p.point(laps[0].position(min(max(T+dt, 0), laps[0].Duration)))
			cx, cy = cx+x/5, cy+y/5
		}
		crop := image.Pt(int(cx)-followWidth/2, int(cy)-top-view.Dy()/2)

		frame := image.NewRGBA(blank.Rect)
		copy(frame.Pix, blank.Pix)
		draw.Draw(frame, view, big, crop.Add(image.Pt(0, top)), draw.Src)

		// The same projection, shifted into the frame.
		pc := p
		pc.ox -= float64(crop.X)
		pc.oy -= float64(crop.Y)
		dc := gg.NewContextForRGBA(frame)
		for _, l := range laps {
			drawTrail(dc, pc, l, min(T, l.Duration))
		}
		if o.LapEvents {
			for i, l := range laps {
				drawLapEvents(dc, pc, l, events[i], min(T, l.Duration), i, len(laps), outward)
			}
		}
		var placed [][2]float64
		for _, l := range laps {
			placed = append(placed, drawCar(dc, pc, l, min(T, l.Duration), placed))
		}
		// Put back the header, band and elevation areas over anything drawn
		// past the view, then draw the panels.
		draw.Draw(frame, image.Rect(0, 0, followWidth, top), blank, image.Point{}, draw.Src)
		draw.Draw(frame, image.Rect(0, view.Max.Y, followWidth, followHeight), blank, image.Pt(0, view.Max.Y), draw.Src)
		// Mini map to the right of the header and timing band.
		const mapW = 170
		drawFollowInset(dc, t, laps, T, followWidth-mapW-12, 12, mapW, float64(top)-24)
		drawLapHUD(dc, laps, progress, T, followHeader, followWidth-mapW-28)
		drawFollowElevation(dc, t, laps, progress, dist, lap, T, elev, viewSpan)

		if err := sink(frame); err != nil {
			return err
		}
		last = frame
		if T >= longest {
			break
		}
	}
	for range holdFrames {
		if err := sink(last); err != nil {
			return err
		}
	}
	return nil
}

// drawBigTrack draws the whole track at scale, with room around it, and
// returns the image and the projection used.
func drawBigTrack(t *Track, scale float64, o Options) (*image.RGBA, projection) {
	p := newProjection(t.Rotation)
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, pt := range t.Outline {
		x, y := p.rotate(pt)
		minX, maxX = min(minX, x), max(maxX, x)
		minY, maxY = min(minY, y), max(maxY, y)
	}
	w := int((maxX-minX)*scale) + 2*followPad
	h := int((maxY-minY)*scale) + 2*followPad
	p = p.centre(Point{}, 0, 0, scale)
	p.cx, p.cy = (minX+maxX)/2, (minY+maxY)/2
	p.ox, p.oy = float64(w)/2, float64(h)/2

	dc := gg.NewContext(w, h)
	dc.SetHexColor(bgColor)
	dc.Clear()
	width := trackWidth * scale
	drawTrack(dc, p, t, width)
	drawHighlights(dc, p, t, o.Overlay, width)
	for _, st := range t.Straights {
		drawStraightName(dc, p, t, st, width*1.4)
	}
	for _, c := range t.Corners {
		drawCornerMarker(dc, p, c, 70, 16, false)
	}
	taken := drawMarshalNumbers(dc, p, t, width)
	drawCornerNames(dc, p, t, 70, 16, width, taken)
	return dc.Image().(*image.RGBA), p
}

// drawFollowElevation marks each car on the elevation profile, and shades
// the stretch of lap the camera shows.
func drawFollowElevation(dc *gg.Context, t *Track, laps []LapTrace, progress [][]float64, dist []float64, lap, T float64, e elevationPanel, span float64) {
	if len(t.Elevation) != len(t.Outline) {
		return
	}
	xAt, yAt := e.axes(lap, max(t.ElevationChange(), 10))
	_, py, _, ph := e.plotArea()
	d0 := distanceAt(laps[0], progress[0], min(T, laps[0].Duration))
	dc.SetHexColor("#E1060030")
	for _, r := range [][2]float64{{d0 - span/2, d0 + span/2}, {d0 - span/2 + lap, d0 + span/2 + lap}, {d0 - span/2 - lap, d0 + span/2 - lap}} {
		a, b := max(r[0], 0), min(r[1], lap)
		if b > a {
			dc.DrawRectangle(xAt(a), py, xAt(b)-xAt(a), ph)
			dc.Fill()
		}
	}
	for i, l := range laps {
		d := min(max(distanceAt(l, progress[i], min(T, l.Duration)), 0), lap)
		x, y := xAt(d), yAt(elevationAt(t, dist, d))
		c := hexColor(hex(l.Color))
		dc.SetHexColor(textColor)
		dc.DrawCircle(x, y, 6)
		dc.Fill()
		dc.SetRGB255(int(c.R), int(c.G), int(c.B))
		dc.DrawCircle(x, y, 4.5)
		dc.Fill()
	}
}

// drawFollowInset draws a mini map of the whole circuit in the box (x, y,
// w, h), coloured by sector, with a dot for each car.
func drawFollowInset(dc *gg.Context, t *Track, laps []LapTrace, T, x, y, w, h float64) {
	const pad = 12.0
	dc.SetHexColor(bgColor)
	dc.DrawRoundedRectangle(x, y, w, h, 10)
	dc.FillPreserve()
	dc.SetHexColor(leaderColor)
	dc.SetLineWidth(1.5)
	dc.Stroke()

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
	for _, l := range laps {
		cx, cy := p.point(l.position(min(T, l.Duration)))
		c := hexColor(hex(l.Color))
		dc.SetHexColor(textColor)
		dc.DrawCircle(cx, cy, 7)
		dc.Fill()
		dc.SetRGB255(int(c.R), int(c.G), int(c.B))
		dc.DrawCircle(cx, cy, 5)
		dc.Fill()
	}
}

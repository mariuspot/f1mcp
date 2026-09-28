package tracks

import (
	"fmt"
	"image"
	"image/draw"
	"math"

	"github.com/fogleman/gg"
)

const (
	// Follow-camera video frames: 720p, with the timing band at the top.
	followWidth, followHeight = 1280, 720
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
	scale := float64(cornerHeight-header) / (2 * cornerRadius)
	big, p := drawBigTrack(t, scale, o)

	band := lapBandHeight(len(laps))
	view := image.Rect(0, band, followWidth, followHeight)
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
	bg := hexColor(bgColor)
	blank := image.NewRGBA(image.Rect(0, 0, followWidth, followHeight))
	for i := 0; i < len(blank.Pix); i += 4 {
		blank.Pix[i], blank.Pix[i+1], blank.Pix[i+2], blank.Pix[i+3] = bg.R, bg.G, bg.B, 255
	}

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
		crop := image.Pt(int(cx)-followWidth/2, int(cy)-band-view.Dy()/2)

		frame := image.NewRGBA(blank.Rect)
		copy(frame.Pix, blank.Pix)
		draw.Draw(frame, view, big, crop.Add(image.Pt(0, band)), draw.Src)

		// The same projection, shifted into the frame.
		pc := p
		pc.ox -= float64(crop.X)
		pc.oy -= float64(crop.Y)
		dc := gg.NewContextForRGBA(frame)
		if o.LapEvents {
			for i, l := range laps {
				drawLapEvents(dc, pc, l, events[i], min(T, l.Duration), i, len(laps), outward)
			}
		}
		var placed [][2]float64
		for _, l := range laps {
			placed = append(placed, drawCar(dc, pc, l, min(T, l.Duration), placed))
		}
		// Clear anything drawn over the timing band, then draw the panels.
		dc.SetHexColor(bgColor)
		dc.DrawRectangle(0, 0, followWidth, float64(band))
		dc.Fill()
		drawFollowInset(dc, t, laps, T, float64(band))
		drawLapHUD(dc, laps, progress, T)

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

// drawFollowInset draws a mini map of the whole circuit in the top right of
// the view, coloured by sector, with a dot for each car.
func drawFollowInset(dc *gg.Context, t *Track, laps []LapTrace, T, top float64) {
	const w, h, pad = 300.0, 220.0, 16.0
	x, y := float64(dc.Width())-w-16, top+16
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

package tracks

import (
	"image"
	"image/color"
	"image/png"
	"io"
	"strconv"
)

// palette holds the colours track images are drawn with, plus blends between
// every pair of them for anti-aliased edges. Encoding to it instead of full
// colour makes images several times smaller with no visible difference.
var palette = buildPalette()

func buildPalette() color.Palette {
	base := []string{
		bgColor, asphaltColor, edgeColor, kerbRed, kerbWhite, textColor,
		subtleColor, leaderColor, insetColor, "#000000",
		sectorColors[0], sectorColors[1], sectorColors[2],
	}
	var cols []color.RGBA
	for _, h := range base {
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
	for _, c := range cols {
		add(c)
	}
	const steps = 3
	for i, a := range cols {
		for _, b := range cols[i+1:] {
			for s := 1; s <= steps; s++ {
				f := float64(s) / (steps + 1)
				add(color.RGBA{
					R: uint8(float64(a.R)*(1-f) + float64(b.R)*f),
					G: uint8(float64(a.G)*(1-f) + float64(b.G)*f),
					B: uint8(float64(a.B)*(1-f) + float64(b.B)*f),
					A: 255,
				})
			}
		}
	}
	return p
}

func hexColor(h string) color.RGBA {
	v, _ := strconv.ParseUint(h[1:7], 16, 32)
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

// EncodePNG writes a track image as a palette PNG.
func EncodePNG(w io.Writer, img image.Image) error {
	b := img.Bounds()
	out := image.NewPaletted(b, palette)
	// Images have few distinct colours, so cache the nearest-colour lookup.
	cache := map[color.RGBA]uint8{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			c := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), 255}
			i, ok := cache[c]
			if !ok {
				i = uint8(palette.Index(c))
				cache[c] = i
			}
			out.SetColorIndex(x, y, i)
		}
	}
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	return enc.Encode(w, out)
}

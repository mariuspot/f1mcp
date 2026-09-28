package main

import (
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"

	"github.com/mariuspot/f1mcp/internal/tracks"
)

// render writes a map and one image per corner for each layout of each
// circuit, into <out>/<circuit>-<first year>[-<last year>]/. When a circuit
// changes, each version gets its own directory.
func render(out string) error {
	ids, err := tracks.Circuits()
	if err != nil {
		return err
	}
	for _, id := range ids {
		layouts, err := tracks.Layouts(id)
		if err != nil {
			return err
		}
		for _, l := range layouts {
			t, err := tracks.Load(id, l.To)
			if err != nil {
				return err
			}
			name := fmt.Sprintf("%s-%d", id, l.From)
			if l.To != l.From {
				name += fmt.Sprintf("-%d", l.To)
			}
			dir := filepath.Join(out, name)
			if err := renderLayout(dir, t, tracks.Options{Years: l.Years()}); err != nil {
				return err
			}
			log.Printf("wrote %s (map and %d corners)", dir, len(t.Corners))
		}
	}
	return nil
}

func renderLayout(dir string, t *tracks.Track, o tracks.Options) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	img, err := tracks.Render(t, o)
	if err != nil {
		return err
	}
	if err := writePNG(filepath.Join(dir, "map.png"), img); err != nil {
		return err
	}
	for _, c := range t.Corners {
		img, err := tracks.RenderCorner(t, c.Number, o)
		if err != nil {
			return err
		}
		if err := writePNG(filepath.Join(dir, fmt.Sprintf("turn-%02d.png", c.Number)), img); err != nil {
			return err
		}
	}
	return nil
}

func writePNG(file string, img image.Image) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	if err := tracks.EncodePNG(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

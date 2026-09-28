package main

import (
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mariuspot/f1mcp/internal/tracks"
)

// render writes a map and one image per corner for each layout of each
// circuit, into <out>/<circuit>-<first year>[-<last year>]/. When a circuit
// changes, each version gets its own directory. It also writes an HTML
// gallery of them: index.html and one page per layout.
func render(out, incidentsDir string) error {
	ids, err := tracks.Circuits()
	if err != nil {
		return err
	}
	var pages []galleryLayout
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
			pages = append(pages, galleryLayout{Dir: name, Years: l.Years(), Track: t})
		}
	}
	incidents, err := loadIncidents(incidentsDir)
	if err != nil {
		return err
	}
	var incidentPages []galleryIncident
	for _, inc := range incidents {
		page, err := renderIncident(out, inc)
		if err != nil {
			return fmt.Errorf("incident %s: %w", inc.Name, err)
		}
		incidentPages = append(incidentPages, page)
		log.Printf("wrote incident %s", inc.Name)
	}
	return writeGallery(out, pages, incidentPages)
}

// renderIncident draws an incident on its track map and nearest corner, into
// <out>/incidents/<name>/.
func renderIncident(out string, inc Incident) (galleryIncident, error) {
	t, err := tracks.Load(inc.CircuitID, inc.Year)
	if err != nil {
		return galleryIncident{}, err
	}
	dir := filepath.Join(out, "incidents", inc.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return galleryIncident{}, err
	}
	o := tracks.Options{Years: strconv.Itoa(inc.Year), Overlay: &inc.Overlay}
	img, err := tracks.Render(t, o)
	if err != nil {
		return galleryIncident{}, err
	}
	if err := writePNG(filepath.Join(dir, "map.png"), img); err != nil {
		return galleryIncident{}, err
	}
	page := galleryIncident{Dir: "incidents/" + inc.Name, Incident: inc, Track: t}
	if inc.Corner > 0 {
		img, err := tracks.RenderCorner(t, inc.Corner, o)
		if err != nil {
			return galleryIncident{}, err
		}
		page.CornerFile = fmt.Sprintf("turn-%02d.png", inc.Corner)
		if err := writePNG(filepath.Join(dir, page.CornerFile), img); err != nil {
			return galleryIncident{}, err
		}
	}
	return page, nil
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

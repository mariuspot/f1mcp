package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mariuspot/f1mcp/internal/jolpica"
)

// Headshot is a freely licensed driver photo from Wikimedia Commons, with
// what's needed to credit it.
type Headshot struct {
	Driver     string `json:"driver"` // e.g. "VER"
	Name       string `json:"name"`
	File       string `json:"file"`        // stored image, relative to the headshots directory
	Page       string `json:"page"`        // Wikipedia page the photo is from
	Commons    string `json:"commons"`     // Commons file page
	License    string `json:"license"`     // e.g. "CC BY-SA 4.0"
	LicenseURL string `json:"license_url"` // e.g. "https://creativecommons.org/licenses/by-sa/4.0"
	Artist     string `json:"artist"`
	Country    string `json:"country"` // ISO 3166 alpha-2, e.g. "gb", for the flag
	// Crop is where the face is, as fractions of the photo's width and
	// height, and how big a square to cut, as a fraction of its width.
	Crop Crop `json:"crop"`
}

type Crop struct {
	X, Y, Size float64
}

// nationalities maps Jolpica nationalities to ISO 3166 alpha-2 codes, for
// flags.
var nationalities = map[string]string{
	"American": "us", "Argentine": "ar", "Argentinian": "ar", "Australian": "au", "Austrian": "at",
	"Belgian": "be", "Brazilian": "br", "British": "gb", "Canadian": "ca", "Chinese": "cn",
	"Danish": "dk", "Dutch": "nl", "Finnish": "fi", "French": "fr", "German": "de",
	"Italian": "it", "Japanese": "jp", "Mexican": "mx", "Monegasque": "mc", "New Zealander": "nz",
	"Polish": "pl", "Spanish": "es", "Swedish": "se", "Swiss": "ch", "Thai": "th",
}

// Credit returns the attribution line for the photo.
func (h Headshot) Credit() string {
	return fmt.Sprintf("%s: photo by %s, %s, via Wikimedia Commons", h.Name, h.Artist, h.License)
}

const wikiUserAgent = "f1mcp-trackgen (https://github.com/mariuspot/f1mcp)"

// freeLicenseRE matches licences that allow reuse in the gallery.
var freeLicenseRE = regexp.MustCompile(`^(CC BY(-SA)? [0-9.]+|CC0|Public domain)`)

// findHeadshots stores a Commons photo for each driver code in a season,
// from the photo on their Wikipedia page, if its licence allows reuse.
func findHeadshots(ctx context.Context, year int, codes []string, dir string) error {
	jol := jolpica.NewClient("", nil)
	drivers, err := jol.Drivers(ctx, strconv.Itoa(year), "")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	index, err := loadHeadshots(dir)
	if err != nil {
		return err
	}
	for _, code := range codes {
		var d *jolpica.Driver
		for i := range drivers {
			if strings.EqualFold(drivers[i].Code, code) {
				d = &drivers[i]
			}
		}
		if d == nil {
			log.Printf("%s: not a %d driver, skipping", code, year)
			continue
		}
		h, err := fetchHeadshot(ctx, *d, dir)
		if err != nil {
			log.Printf("%s: %v", d.Code, err)
			continue
		}
		if old, ok := index[h.Driver]; ok && old.Crop != (Crop{}) {
			h.Crop = old.Crop // keep a crop set by hand
		}
		if h.Country != "" {
			if err := fetchFlag(ctx, h.Country, filepath.Join(dir, "..", "flags")); err != nil {
				log.Printf("%s flag: %v", h.Driver, err)
			}
		}
		index[h.Driver] = h
		log.Printf("%s: %s", h.Driver, h.Credit())
	}
	return writeJSON(filepath.Join(dir, "headshots.json"), index)
}

func loadHeadshots(dir string) (map[string]Headshot, error) {
	index := map[string]Headshot{}
	b, err := os.ReadFile(filepath.Join(dir, "headshots.json"))
	if errors.Is(err, os.ErrNotExist) {
		return index, nil
	}
	if err != nil {
		return nil, err
	}
	return index, json.Unmarshal(b, &index)
}

func fetchHeadshot(ctx context.Context, d jolpica.Driver, dir string) (Headshot, error) {
	title, err := url.PathUnescape(d.URL[strings.LastIndex(d.URL, "/")+1:])
	if err != nil {
		return Headshot{}, err
	}
	// The photo on the driver's Wikipedia page.
	var page struct {
		Query struct {
			Pages map[string]struct {
				PageImage string `json:"pageimage"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := wikiGet(ctx, "https://en.wikipedia.org/w/api.php", url.Values{
		"action": {"query"}, "titles": {title}, "prop": {"pageimages"}, "redirects": {"1"}, "format": {"json"},
	}, &page); err != nil {
		return Headshot{}, err
	}
	var file string
	for _, p := range page.Query.Pages {
		file = p.PageImage
	}
	if file == "" {
		return Headshot{}, errors.New("no photo on the Wikipedia page")
	}

	// Its licence, author and a 500 px copy, from Commons.
	var info struct {
		Query struct {
			Pages map[string]struct {
				ImageInfo []struct {
					ThumbURL       string `json:"thumburl"`
					DescriptionURL string `json:"descriptionurl"`
					ExtMetadata    map[string]struct {
						Value any `json:"value"` // usually text, sometimes a number
					} `json:"extmetadata"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := wikiGet(ctx, "https://commons.wikimedia.org/w/api.php", url.Values{
		"action": {"query"}, "titles": {"File:" + file}, "prop": {"imageinfo"},
		"iiprop": {"extmetadata|url"}, "iiurlwidth": {"500"}, "format": {"json"},
	}, &info); err != nil {
		return Headshot{}, err
	}
	for _, p := range info.Query.Pages {
		if len(p.ImageInfo) == 0 {
			break
		}
		ii := p.ImageInfo[0]
		meta := func(k string) string {
			v := ii.ExtMetadata[k].Value
			if v == nil {
				return ""
			}
			return stripTags(fmt.Sprint(v))
		}
		license := meta("LicenseShortName")
		if !freeLicenseRE.MatchString(license) {
			return Headshot{}, fmt.Errorf("photo licence %q doesn't allow reuse", license)
		}
		name := strings.ToLower(d.Code) + ".jpg"
		if err := wikiDownload(ctx, ii.ThumbURL, filepath.Join(dir, name)); err != nil {
			return Headshot{}, err
		}
		return Headshot{
			Driver: d.Code, Name: d.GivenName + " " + d.FamilyName, File: name,
			Page: d.URL, Commons: ii.DescriptionURL,
			License: license, LicenseURL: meta("LicenseUrl"), Artist: meta("Artist"),
			Country: nationalities[d.Nationality],
			Crop:    Crop{X: 0.5, Y: 0.3, Size: 0.5},
		}, nil
	}
	return Headshot{}, errors.New("no image info on Commons")
}

// fetchFlag stores a country's flag (public domain) from flagcdn.com, if
// it isn't stored yet.
func fetchFlag(ctx context.Context, country, dir string) error {
	file := filepath.Join(dir, country+".png")
	if _, err := os.Stat(file); err == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return wikiDownload(ctx, "https://flagcdn.com/w80/"+country+".png", file)
}

var tagRE = regexp.MustCompile(`<[^>]+>`)

func stripTags(s string) string {
	return strings.TrimSpace(tagRE.ReplaceAllString(s, ""))
}

// wikiGet calls a MediaWiki API, waiting a second first to be polite.
func wikiGet(ctx context.Context, base string, q url.Values, v any) error {
	resp, err := wikiRequest(ctx, base+"?"+q.Encode())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

func wikiDownload(ctx context.Context, u, file string) error {
	resp, err := wikiRequest(ctx, u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func wikiRequest(ctx context.Context, u string) (*http.Response, error) {
	time.Sleep(time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", wikiUserAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: %s", u, resp.Status)
	}
	return resp, nil
}

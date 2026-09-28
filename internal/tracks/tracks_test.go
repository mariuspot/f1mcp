package tracks

import (
	"errors"
	"slices"
	"testing"
)

func TestLoad(t *testing.T) {
	tr, err := Load("monaco", 2024)
	if err != nil {
		t.Fatal(err)
	}
	if tr.CircuitID != "monaco" || tr.Year != 2024 || tr.Name != "Circuit de Monaco" {
		t.Errorf("unexpected track: %s %d %q", tr.CircuitID, tr.Year, tr.Name)
	}
	if len(tr.Outline) < 100 || len(tr.Corners) != 19 {
		t.Errorf("got %d outline points and %d corners", len(tr.Outline), len(tr.Corners))
	}
}

func TestLoadFallsBackToNearestYear(t *testing.T) {
	years, err := Years("imola")
	if err != nil {
		t.Fatal(err)
	}
	// Imola was on the calendar in 2024 and 2025 only.
	if !slices.Equal(years, []int{2024, 2025}) {
		t.Fatalf("imola years = %v, want [2024 2025]", years)
	}
	for _, tc := range []struct{ year, want int }{{2023, 2024}, {2025, 2025}, {2030, 2025}} {
		tr, err := Load("imola", tc.year)
		if err != nil {
			t.Fatal(err)
		}
		if tr.Year != tc.want {
			t.Errorf("Load(imola, %d) gave year %d, want %d", tc.year, tr.Year, tc.want)
		}
	}
}

func TestLayouts(t *testing.T) {
	got, err := Layouts("imola")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (Layout{2024, 2025}) || got[0].Years() != "2024–2025" {
		t.Errorf("imola layouts = %+v", got)
	}
}

func TestGroupLayoutsSplitsOnChange(t *testing.T) {
	old := &Track{Outline: []Point{{0, 0}, {1, 0}}}
	changed := &Track{Outline: []Point{{0, 0}, {2, 0}}}
	byYear := map[int]*Track{2023: old, 2024: old, 2025: changed, 2026: changed}
	got, err := groupLayouts([]int{2023, 2024, 2025, 2026}, func(y int) (*Track, error) { return byYear[y], nil })
	if err != nil {
		t.Fatal(err)
	}
	want := []Layout{{2023, 2024}, {2025, 2026}}
	if !slices.Equal(got, want) {
		t.Errorf("layouts = %v, want %v", got, want)
	}
}

func TestLoadUnknownCircuit(t *testing.T) {
	if _, err := Load("nurburgring", 2024); !errors.Is(err, ErrUnknownCircuit) {
		t.Fatalf("err = %v, want ErrUnknownCircuit", err)
	}
}

// Outlines start at the start/finish line and run in driving direction, so
// corners appear in order along them.
func TestCornersInLapOrder(t *testing.T) {
	ids, err := Circuits()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		years, _ := Years(id)
		tr, err := Load(id, years[len(years)-1])
		if err != nil {
			t.Fatal(err)
		}
		prev := -1
		for _, c := range tr.Corners {
			i := nearestIndex(tr.Outline, c.Position)
			if i < prev {
				t.Errorf("%s: corner %d is before the previous corner along the outline", id, c.Number)
				break
			}
			prev = i
		}
	}
}

func TestRender(t *testing.T) {
	tr, err := Load("marina_bay", 2023)
	if err != nil {
		t.Fatal(err)
	}
	img, err := Render(tr, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != mapWidth || b.Dy() != mapHeight {
		t.Errorf("image is %dx%d, want %dx%d", b.Dx(), b.Dy(), mapWidth, mapHeight)
	}
}

func TestRenderCorner(t *testing.T) {
	tr, err := Load("monaco", 2024)
	if err != nil {
		t.Fatal(err)
	}
	img, err := RenderCorner(tr, 6, Options{Years: "2023–2026"})
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != cornerWidth || b.Dy() != cornerHeight {
		t.Errorf("image is %dx%d, want %dx%d", b.Dx(), b.Dy(), cornerWidth, cornerHeight)
	}
	if _, err := RenderCorner(tr, 99, Options{}); err == nil {
		t.Error("want error for a corner that doesn't exist")
	}
}

// Corner numbers are unique after the generator drops duplicates.
func TestCornerNumbersUnique(t *testing.T) {
	ids, err := Circuits()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		years, _ := Years(id)
		for _, y := range years {
			tr, err := Load(id, y)
			if err != nil {
				t.Fatal(err)
			}
			seen := map[int]bool{}
			for _, c := range tr.Corners {
				if seen[c.Number] {
					t.Errorf("%s %d: turn %d appears twice", id, y, c.Number)
				}
				seen[c.Number] = true
			}
		}
	}
}

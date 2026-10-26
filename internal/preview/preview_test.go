package preview

import (
	"image/color"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

func repoPath(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(append([]string{filepath.Dir(file), "..", ".."}, parts...)...)
}

// A dense wood and a clearing must not look alike: density is half of what a
// map describes, and a GM decides from the preview whether to paste it.
func TestDenseWoodLooksDifferentFromClearing(t *testing.T) {
	doc, err := ir.Parse([]byte(`{
		"map":{"width":40,"length":20,"name":"t","biome":"temperate_forest"},
		"zones":[
			{"id":"wood","anchor":{"x":10,"y":10},"relative_size":0.5,"elevation":"flat","density_overrides":{"vegetation":1.0}},
			{"id":"clearing","anchor":{"x":30,"y":10},"relative_size":0.5,"elevation":"flat","density_overrides":{"vegetation":0.0}}
		]}`))
	if err != nil {
		t.Fatal(err)
	}
	gen, err := generator.New(repoPath("configs", "biomes.json"), repoPath("configs", "props.json"))
	if err != nil {
		t.Fatal(err)
	}
	mask := spatial.Resolve(doc)
	res, err := gen.Generate(doc, mask, 1)
	if err != nil {
		t.Fatal(err)
	}

	img := Render(doc, mask, res.Height, Options{Scale: 8})
	canopyPixels := func(x0, x1 int) int {
		n := 0
		for x := x0 * 8; x < x1*8; x++ {
			for y := 0; y < 20*8; y++ {
				if c := img.RGBAAt(x, y); luminance(c) < luminance(canopy(reliefPalette["ground"]))*1.2 {
					n++
				}
			}
		}
		return n
	}
	// Compare the cores of the two zones, away from the border band.
	wood, clearing := canopyPixels(2, 16), canopyPixels(24, 38)
	if wood < 10*max(clearing, 1) {
		t.Errorf("the wood shows %d canopy pixels against %d in the clearing: they should look clearly different", wood, clearing)
	}

	var trees int
	for _, e := range Legend(doc, res.Height) {
		if e.Key == "tree" {
			trees = e.Count
		}
	}
	if trees == 0 {
		t.Error("the legend should count the trees drawn")
	}
}

func luminance(c color.RGBA) float64 {
	return 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)
}

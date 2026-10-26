package generator

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/johnfercher/talescoder/pkg/decoder"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

// configPath resolves the repo-root configs regardless of the test's cwd.
func configPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "configs", name)
}

func newTestGen(t *testing.T) *Generator {
	t.Helper()
	g, err := New(configPath("biomes.json"), configPath("props.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

const twoZoneIR = `{
	"map":{"width":40,"length":40,"name":"t","biome":"temperate_forest"},
	"zones":[
		{"id":"a","anchor":{"x":10,"y":20},"relative_size":0.5,"elevation":"flat"},
		{"id":"b","anchor":{"x":30,"y":20},"relative_size":0.5,"elevation":"hill"}
	]}`

func TestGenerateProducesDecodableSlab(t *testing.T) {
	doc, err := ir.Parse([]byte(twoZoneIR))
	if err != nil {
		t.Fatal(err)
	}
	mask := spatial.Resolve(doc)
	res, err := newTestGen(t).Generate(doc, mask, 42)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.AssetCount == 0 {
		t.Fatal("no assets generated")
	}

	// Round-trip through the real TaleSpire decoder to confirm the code is
	// structurally valid.
	slab, err := decoder.NewDecoder().Decode(res.Code)
	if err != nil {
		t.Fatalf("decoding generated slab failed: %v", err)
	}
	if len(slab.Assets) == 0 {
		t.Fatal("decoded slab has no assets")
	}
}

func TestGenerateDeterministic(t *testing.T) {
	doc, err := ir.Parse([]byte(twoZoneIR))
	if err != nil {
		t.Fatal(err)
	}
	g := newTestGen(t)
	a, err := g.Generate(doc, spatial.Resolve(doc), 7)
	if err != nil {
		t.Fatal(err)
	}
	b, err := g.Generate(doc, spatial.Resolve(doc), 7)
	if err != nil {
		t.Fatal(err)
	}
	if a.Code != b.Code {
		t.Error("same seed produced different output")
	}

	c, err := g.Generate(doc, spatial.Resolve(doc), 8)
	if err != nil {
		t.Fatal(err)
	}
	if a.Code == c.Code {
		t.Error("different seeds produced identical output")
	}
}

func TestUnknownPOIProducesWarning(t *testing.T) {
	doc, err := ir.Parse([]byte(`{
		"map":{"width":30,"length":30,"name":"t","biome":"temperate_forest"},
		"zones":[{
			"id":"a","anchor":{"x":15,"y":15},"relative_size":0.9,
			"points_of_interest":[{"prop":"definitely_not_a_prop","position":{"x":15,"y":15}}]
		}]}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := newTestGen(t).Generate(doc, spatial.Resolve(doc), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) == 0 {
		t.Error("expected a warning for unknown POI prop")
	}
}

func TestPOIAliasResolves(t *testing.T) {
	if got := resolvePropAlias("broken_pillar"); got != "big_stone_wall" {
		t.Errorf("alias broken_pillar -> %q, want big_stone_wall", got)
	}
	if got := resolvePropAlias("already_an_id"); got != "already_an_id" {
		t.Errorf("passthrough failed, got %q", got)
	}
}

func TestWaterInDepression(t *testing.T) {
	doc, err := ir.Parse([]byte(`{
		"map":{"width":30,"length":30,"name":"t","biome":"temperate_forest"},
		"zones":[{"id":"pond","anchor":{"x":15,"y":15},"relative_size":0.9,"elevation":"depression"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	mask := spatial.Resolve(doc)
	res, err := newTestGen(t).Generate(doc, mask, 1)
	if err != nil {
		t.Fatal(err)
	}
	// The centre of a large depression should be water.
	if !res.Height.IsWaterAt(15, 15) {
		t.Error("expected water at the centre of a depression zone")
	}
}

// TestSlicingProducesDecodableGrid checks that a sliced map yields the right
// grid, every slice decodes, and no placement is lost or duplicated.
func TestSlicingProducesDecodableGrid(t *testing.T) {
	doc, err := ir.Parse([]byte(`{
		"map":{"width":60,"length":60,"name":"t","biome":"temperate_forest"},
		"zones":[{"id":"a","anchor":{"x":30,"y":30},"relative_size":1.0,"elevation":"hill","density_overrides":{"vegetation":0.2}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	g := newTestGen(t)
	g.SetSliceSize(25) // 60/25 -> 3 slices per side

	res, err := g.Generate(doc, spatial.Resolve(doc), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Slices) != 3 || len(res.Slices[0]) != 3 {
		t.Fatalf("expected a 3x3 slice grid, got %dx%d", len(res.Slices), len(res.Slices[0]))
	}

	dec := decoder.NewDecoder()
	totalLayouts := 0
	for sx := range res.Slices {
		for sy := range res.Slices[sx] {
			slab, err := dec.Decode(res.Slices[sx][sy])
			if err != nil {
				t.Fatalf("slice [%d,%d] did not decode: %v", sx, sy, err)
			}
			for _, a := range slab.Assets {
				totalLayouts += len(a.Layouts)
			}
		}
	}
	// Every placed asset lands in exactly one slice: the layouts summed across
	// slices must equal the whole-map asset count.
	if totalLayouts != res.AssetCount {
		t.Errorf("slices hold %d layouts, whole map has %d assets", totalLayouts, res.AssetCount)
	}
}

// TestOversizedMapIsSlicedAutomatically checks that with no slice size set, a
// map too big for one TaleSpire slab is cut into slabs that each fit, without
// losing a placement, while a small map stays a single slab.
func TestOversizedMapIsSlicedAutomatically(t *testing.T) {
	doc, err := ir.Parse([]byte(`{
		"map":{"width":110,"length":110,"name":"t","biome":"temperate_forest"},
		"zones":[{"id":"a","anchor":{"x":55,"y":55},"relative_size":1.0,"elevation":"hill"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := newTestGen(t).Generate(doc, spatial.Resolve(doc), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Code) <= TaleSpireSlabLimitBytes {
		t.Fatalf("map not large enough to exceed the limit (%d bytes)", len(res.Code))
	}
	if res.Slices == nil || res.SliceSize <= 0 {
		t.Fatalf("an oversized map must be sliced, got %d slices of %d tiles", len(res.Slices), res.SliceSize)
	}
	dec := decoder.NewDecoder()
	layouts := 0
	for sx := range res.Slices {
		for sy, code := range res.Slices[sx] {
			if len(code) > TaleSpireSlabLimitBytes {
				t.Errorf("slice [%d,%d] is %d bytes, over the limit", sx, sy, len(code))
			}
			slab, err := dec.Decode(code)
			if err != nil {
				t.Fatalf("slice [%d,%d] did not decode: %v", sx, sy, err)
			}
			for _, a := range slab.Assets {
				layouts += len(a.Layouts)
			}
		}
	}
	if layouts != res.AssetCount {
		t.Errorf("slices hold %d layouts, whole map has %d assets", layouts, res.AssetCount)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("no warning expected once every slice fits, got %v", res.Warnings)
	}
	// Tiles are spread evenly: no thin sliver of a last column to place.
	if last := 110 - (len(res.Slices)-1)*res.SliceSize; last*2 < res.SliceSize {
		t.Errorf("last column is %d tiles for slices of %d: slices should be even", last, res.SliceSize)
	}

	small, err := ir.Parse([]byte(`{
		"map":{"width":30,"length":30,"name":"t","biome":"temperate_forest"},
		"zones":[{"id":"a","anchor":{"x":15,"y":15},"relative_size":1.0,"elevation":"flat"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err = newTestGen(t).Generate(small, spatial.Resolve(small), 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Slices != nil || res.SliceSize != 0 {
		t.Errorf("a map that fits must stay one slab, got %d slices", len(res.Slices))
	}
}

// TestConnectionCarvesRampedPath verifies Phase 3 connection carving: path
// tiles become bare ground and their heights ramp between the two zones.
func TestConnectionCarvesRampedPath(t *testing.T) {
	const doc = `{
		"map":{"width":40,"length":40,"name":"t","biome":"temperate_forest"},
		"zones":[
			{"id":"low","anchor":{"x":8,"y":20},"relative_size":0.5,"elevation":"flat"},
			{"id":"high","anchor":{"x":32,"y":20},"relative_size":0.5,"elevation":"mountain"}
		],
		"connections":[{"from":"low","to":"high","type":"path","width":3}]}`
	parsed, err := ir.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	res, err := newTestGen(t).Generate(parsed, spatial.Resolve(parsed), 1)
	if err != nil {
		t.Fatal(err)
	}

	paths := spatial.BuildPaths(parsed)
	pathTiles := 0
	westH, eastH := -1, -1
	for x := 0; x < res.Height.Width; x++ {
		for y := 0; y < res.Height.Length; y++ {
			if !paths.On(x, y) {
				continue
			}
			pathTiles++
			if res.Height.ReliefAt(x, y) != "base_ground" {
				t.Fatalf("path tile (%d,%d) not carved to base_ground: %q", x, y, res.Height.ReliefAt(x, y))
			}
			if y == 20 && x <= 9 {
				westH = res.Height.HeightAt(x, y)
			}
			if y == 20 && x >= 31 {
				eastH = res.Height.HeightAt(x, y)
			}
		}
	}
	if pathTiles == 0 {
		t.Fatal("no path tiles carved")
	}
	if westH >= 0 && eastH >= 0 && eastH <= westH {
		t.Errorf("path did not ramp upward toward the mountain: west=%d east=%d", westH, eastH)
	}
}

func TestReliefOverrideResolves(t *testing.T) {
	doc, err := ir.Parse([]byte(`{
		"map":{"width":30,"length":30,"name":"t","biome":"temperate_forest"},
		"zones":[{"id":"ruins","anchor":{"x":15,"y":15},"relative_size":0.9,"elevation":"hill","relief_override":"ruins"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := newTestGen(t).Generate(doc, spatial.Resolve(doc), 1)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// The zone centre must resolve to the ruins relief, and the material must
	// win over the height-derived relief.
	if got := res.Height.ReliefAt(15, 15); got != "ruins" {
		t.Errorf("relief_override not applied at centre, got %q", got)
	}
}

func TestUnknownReliefOverrideFails(t *testing.T) {
	// relief_override that is not a relief of the map biome must fail loudly at
	// generation time (config-aware validation).
	doc, err := ir.Parse([]byte(`{
		"map":{"width":30,"length":30,"name":"t","biome":"desert"},
		"zones":[{"id":"z","anchor":{"x":15,"y":15},"relative_size":0.9,"relief_override":"ruins"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newTestGen(t).Generate(doc, spatial.Resolve(doc), 1); err == nil {
		t.Error("expected error for relief_override absent from the biome, got none")
	}
}

// maxSeamHeightDelta returns the largest height jump across any border between
// two different zones — the "cliff" that stitching is meant to soften.
func maxSeamHeightDelta(h *HeightField, mask *spatial.Mask) int {
	max := 0
	for x := 0; x < h.Width; x++ {
		for y := 0; y < h.Length; y++ {
			z := mask.ZoneIndexAt(x, y)
			for _, n := range [][2]int{{x + 1, y}, {x, y + 1}} {
				nx, ny := n[0], n[1]
				if nx >= h.Width || ny >= h.Length {
					continue
				}
				if mask.ZoneIndexAt(nx, ny) == z {
					continue
				}
				d := h.HeightAt(x, y) - h.HeightAt(nx, ny)
				if d < 0 {
					d = -d
				}
				if d > max {
					max = d
				}
			}
		}
	}
	return max
}

// TestStitchingSoftensBorderCliff verifies the Phase 3 headline: a flat zone
// next to a mountainous one has a gentler height step across the border with
// stitching on than with it off.
func TestStitchingSoftensBorderCliff(t *testing.T) {
	const doc = `{
		"map":{"width":40,"length":40,"name":"t","biome":"temperate_forest"},
		"zones":[
			{"id":"plain","anchor":{"x":10,"y":20},"relative_size":0.5,"elevation":"flat"},
			{"id":"peaks","anchor":{"x":30,"y":20},"relative_size":0.5,"elevation":"mountain"}
		]}`
	parsed, err := ir.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	g := newTestGen(t)

	g.SetTransitionHalfWidth(0) // hard borders
	hard, err := g.Generate(parsed, spatial.Resolve(parsed), 1)
	if err != nil {
		t.Fatal(err)
	}
	hardDelta := maxSeamHeightDelta(hard.Height, spatial.Resolve(parsed))

	g.SetTransitionHalfWidth(3) // stitched
	soft, err := g.Generate(parsed, spatial.Resolve(parsed), 1)
	if err != nil {
		t.Fatal(err)
	}
	softDelta := maxSeamHeightDelta(soft.Height, spatial.Resolve(parsed))

	if hardDelta == 0 {
		t.Skip("no cliff to soften with these anchors")
	}
	t.Logf("seam height cliff: hard=%d soft=%d", hardDelta, softDelta)
	if softDelta >= hardDelta {
		t.Errorf("stitching did not soften the border: hard=%d soft=%d", hardDelta, softDelta)
	}
}

// TestReliefOverrideBeatsElevationRelief confirms the precedence rule: a deep
// depression that would normally become water stays non-water when the zone
// overrides the relief to something else.
func TestReliefOverrideBeatsElevationRelief(t *testing.T) {
	doc, err := ir.Parse([]byte(`{
		"map":{"width":30,"length":30,"name":"t","biome":"temperate_forest"},
		"zones":[{"id":"pit","anchor":{"x":15,"y":15},"relative_size":0.9,"elevation":"depression","relief_override":"ground"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := newTestGen(t).Generate(doc, spatial.Resolve(doc), 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Height.IsWaterAt(15, 15) {
		t.Error("relief_override 'ground' should prevent water in a depression")
	}
}

// A path from a courtyard to ruins must not pave the pond nested at the
// courtyard's centre: the courtyard's anchor lies in the pond, and the path
// used to run from there as a dry causeway.
func TestPathStopsAtWater(t *testing.T) {
	doc, err := ir.Parse([]byte(`{
		"map":{"width":50,"length":50,"name":"t","biome":"temperate_forest"},
		"zones":[
			{"id":"courtyard","anchor":{"x":25,"y":28},"relative_size":0.4,"elevation":"flat"},
			{"id":"pond","anchor":{"x":25,"y":28},"relative_size":0.1,"elevation":"depression","relief_override":"water"},
			{"id":"ruins","anchor":{"x":25,"y":7},"relative_size":0.2,"elevation":"hill","relief_override":"ruins"}
		],
		"connections":[{"from":"courtyard","to":"ruins","type":"path","width":3}]}`))
	if err != nil {
		t.Fatal(err)
	}
	mask := spatial.Resolve(doc)
	res, err := newTestGen(t).Generate(doc, mask, 1)
	if err != nil {
		t.Fatal(err)
	}
	for x := 0; x < 50; x++ {
		for y := 0; y < 50; y++ {
			if mask.ZoneIndexAt(x, y) == 1 && !res.Height.IsWaterAt(x, y) {
				t.Fatalf("pond tile (%d,%d) is %q, not water", x, y, res.Height.ReliefAt(x, y))
			}
		}
	}
}

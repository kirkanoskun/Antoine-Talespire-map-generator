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

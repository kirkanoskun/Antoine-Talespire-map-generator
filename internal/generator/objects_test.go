package generator

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/slab"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

// square builds a one-zone 40x40 temperate forest with the given points of
// interest and a dense wood, so a footprint left unreserved would catch trees.
func square(t *testing.T, pois string) *ir.IR {
	t.Helper()
	doc, err := ir.Parse([]byte(fmt.Sprintf(`{"map":{"width":40,"length":40,"biome":"temperate_forest"},
		"zones":[{"id":"place","anchor":{"x":20,"y":20},"relative_size":1,"elevation":"hill",
		"density_overrides":{"vegetation":0.5,"stones":0.2,"misc":0.2},"points_of_interest":[%s]}]}`, pois)))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func generate(t *testing.T, doc *ir.IR) *Result {
	t.Helper()
	res, err := newTestGen(t).Generate(doc, spatial.Resolve(doc), 7)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// layoutsWithID returns the instances of the asset with that UUID prefix
// (the first four bytes of a layout id are the UUID's first group, reversed).
func instancesOf(t *testing.T, code, uuid string) []slab.Instance {
	t.Helper()
	cat, err := Assets()
	if err != nil {
		t.Fatal(err)
	}
	a, ok := cat.ByUUID(uuid)
	if !ok {
		t.Fatalf("no asset %s", uuid)
	}
	id, _ := a.LayoutID()
	s, err := slab.DecodeGenerated(code)
	if err != nil {
		t.Fatal(err)
	}
	var out []slab.Instance
	for _, l := range s.Layouts {
		if l.ID == id {
			out = append(out, l.Instances...)
		}
	}
	return out
}

const fountainUUID = "08545429-752c-41c5-9758-1279d82b70eb"

func TestCatalogueObjectIsPlacedExactly(t *testing.T) {
	res := generate(t, square(t, `{"prop":"fountain_01","position":{"x":20,"y":20}},{"prop":"stall_fruit_01","position":{"x":5,"y":6}}`))
	f := res.Height

	fountain := instancesOf(t, res.Code, fountainUUID)
	if len(fountain) != 1 {
		t.Fatalf("the slab holds %d fountains, want 1", len(fountain))
	}
	// A 2x2 prop whose bounds are centred on its origin: the origin goes to the
	// centre of the footprint (21,21), on the levelled surface, unrotated.
	h := f.HeightAt(20, 20)
	want := slab.Instance{X: 2100, Y: surface(h), Z: 2100}
	if got := fountain[0]; got.X != want.X || got.Z != want.Z || got.Y != want.Y || got.Rotation != 0 {
		t.Errorf("fountain = %+v, want %+v", got, want)
	}
	for _, p := range [][2]int{{20, 20}, {21, 20}, {20, 21}, {21, 21}} {
		if f.HeightAt(p[0], p[1]) != h {
			t.Errorf("footprint not levelled: height at %v is %d, want %d", p, f.HeightAt(p[0], p[1]), h)
		}
	}
	// A tile has its origin at its corner: it sits exactly on its cell.
	stall := instancesOf(t, res.Code, "554ac0fb-4489-41dd-80ce-aec63f0a8649")
	if len(stall) != 1 || stall[0].X != 500 || stall[0].Z != 600 || stall[0].Y != surface(f.HeightAt(5, 6)) {
		t.Errorf("stall = %+v, want X 500, Z 600 on the surface", stall)
	}
	if objs := f.Objects(); len(objs) != 2 || objs[0] != (PlacedObject{X: 20, Y: 20, W: 2, D: 2, Slug: "fountain_01"}) {
		t.Errorf("objects = %+v", objs)
	}
}

func TestCatalogueObjectReservesItsFootprint(t *testing.T) {
	res := generate(t, square(t, `{"prop":"fountain_01","position":{"x":20,"y":20}},{"prop":"tree","position":{"x":21,"y":21}},{"prop":"well_01","position":{"x":21,"y":19}}`))
	f := res.Height
	for x := 20; x < 22; x++ {
		for y := 20; y < 22; y++ {
			if !f.ObjectAt(x, y) || f.PropAt(x, y) != PropNone {
				t.Errorf("tile (%d,%d) under the fountain: reserved %v, scatter %v", x, y, f.ObjectAt(x, y), f.PropAt(x, y))
			}
		}
	}
	joined := strings.Join(res.Warnings, "\n")
	if !strings.Contains(joined, `POI "tree" overlaps another object`) || !strings.Contains(joined, `POI "well_01" overlaps another object`) {
		t.Errorf("overlapping points of interest should be skipped with a warning, got:\n%s", joined)
	}
	if len(f.Objects()) != 1 {
		t.Errorf("objects = %+v, want the fountain only", f.Objects())
	}
}

// Adding an object changes its footprint and the ground fill next to it, and
// nothing else: the random sequence of the scatter is untouched.
func TestCatalogueObjectLeavesTheRestOfTheMap(t *testing.T) {
	without := generate(t, square(t, ``))
	with := generate(t, square(t, `{"prop":"fountain_01","position":{"x":20,"y":20}}`))
	near := func(x, y int) bool { return x >= 19 && x <= 22 && y >= 19 && y <= 22 }
	lines := func(code string) []string {
		s, err := slab.DecodeGenerated(code)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, l := range s.Layouts {
			for _, p := range l.Instances {
				if !near(p.X/100, p.Z/100) {
					out = append(out, fmt.Sprintf("%x %d %d %d %d", l.ID, p.X, p.Y, p.Z, p.Rotation))
				}
			}
		}
		sort.Strings(out)
		return out
	}
	a, b := lines(without.Code), lines(with.Code)
	if strings.Join(a, "\n") != strings.Join(b, "\n") {
		t.Errorf("the map away from the fountain changed (%d vs %d placements)", len(a), len(b))
	}
}

func TestPOIResolutionOrder(t *testing.T) {
	g := newTestGen(t)
	for name, want := range map[string]string{
		"fire":             "old",     // a props.json id
		"campfire":         "old",     // an alias, even though the catalogue has a "campfire" slug
		"bonfire":          "old",     // idem
		"fountain_01":      "catalog", // a catalogue slug
		"gravestone_01":    "catalog",
		"dragon_altar_xyz": "unknown",
	} {
		prop, asset := g.resolvePOI(name)
		got := "unknown"
		if prop != nil {
			got = "old"
		} else if asset != nil {
			got = "catalog"
		}
		if got != want {
			t.Errorf("%s resolves to %s, want %s", name, got, want)
		}
		if g.KnownProp(name) != (want != "unknown") {
			t.Errorf("KnownProp(%s) = %v", name, g.KnownProp(name))
		}
	}
}

func TestUnknownPropsAreListedOnce(t *testing.T) {
	res := generate(t, square(t, `{"prop":"dragon_altar","position":{"x":3,"y":3}},{"prop":"dragon_altar","position":{"x":8,"y":3}},{"prop":"tombstone","position":{"x":12,"y":3}}`))
	if strings.Join(res.UnknownProps, ",") != "dragon_altar,tombstone" {
		t.Errorf("UnknownProps = %v", res.UnknownProps)
	}
}

func TestScatteredCatalogueObjects(t *testing.T) {
	res := generate(t, square(t, `{"prop":"gravestone_01","position":"scattered","count":12},{"prop":"wagon_01","position":"scattered","count":2}`))
	objs := res.Height.Objects()
	if len(objs) != 14 {
		t.Fatalf("placed %d objects, want 14: %v", len(objs), res.Warnings)
	}
	taken := map[[2]int]bool{}
	for _, o := range objs {
		for x := o.X; x < o.X+o.W; x++ {
			for y := o.Y; y < o.Y+o.D; y++ {
				if taken[[2]int{x, y}] {
					t.Fatalf("objects overlap at (%d,%d)", x, y)
				}
				taken[[2]int{x, y}] = true
			}
		}
	}
	if len(instancesOf(t, res.Code, "89e6faca-3b0c-41e6-bb2f-80c5c6eb0b03")) != 12 {
		t.Error("the slab should hold 12 gravestones")
	}
	again := generate(t, square(t, `{"prop":"gravestone_01","position":"scattered","count":12},{"prop":"wagon_01","position":"scattered","count":2}`))
	if again.Code != res.Code {
		t.Error("scattered objects are not reproducible")
	}
}

func TestCatalogueObjectOutsideTheMapIsSkipped(t *testing.T) {
	res := generate(t, square(t, `{"prop":"wagon_01","position":{"x":39,"y":38}}`))
	if len(res.Height.Objects()) != 0 || !strings.Contains(strings.Join(res.Warnings, "\n"), "outside the map") {
		t.Errorf("objects %v, warnings %v", res.Height.Objects(), res.Warnings)
	}
}

func TestSlicedMapKeepsCatalogueObjects(t *testing.T) {
	g := newTestGen(t)
	g.SetSliceSize(20)
	doc := square(t, `{"prop":"fountain_01","position":{"x":20,"y":20}}`)
	res, err := g.Generate(doc, spatial.Resolve(doc), 7)
	if err != nil {
		t.Fatal(err)
	}
	// The fountain's anchor (20,20) is the corner of slice [1][1]: it is
	// there, in that slice's own coordinates.
	got := instancesOf(t, res.Slices[1][1], fountainUUID)
	if len(got) != 1 || got[0].X != 100 || got[0].Z != 100 {
		t.Errorf("fountain in slice [1][1] = %+v, want X 100, Z 100", got)
	}
}

func TestPavingAndSwampFloorReliefs(t *testing.T) {
	for _, c := range []struct{ biome, relief, uuid string }{
		{"temperate_forest", "paving", "a2eaf2e2-01d8-4548-a517-50df8551057a"},
		{"swamp", "swamp_floor", "503f0799-b652-4d81-b202-d4473caecee4"},
	} {
		doc, err := ir.Parse([]byte(fmt.Sprintf(`{"map":{"width":20,"length":20,"biome":%q},"zones":[{"id":"z","anchor":{"x":10,"y":10},"relative_size":1,"elevation":"flat","relief_override":%q}]}`, c.biome, c.relief)))
		if err != nil {
			t.Fatal(err)
		}
		res := generate(t, doc)
		if n := len(instancesOf(t, res.Code, c.uuid)); n < 400 {
			t.Errorf("%s: %d floor blocks, want one per tile at least (400)", c.relief, n)
		}
	}
}

// The reference scenes of the lot 1 spec (US5), as an AI might write them:
// every object named is placed, with no warning, and the code decodes.
func TestReferenceScenes(t *testing.T) {
	for _, c := range []struct {
		file    string
		objects int
	}{
		{"scene_place_village.json", 8},
		{"scene_cimetiere_marais.json", 17},
	} {
		raw, err := os.ReadFile(testdataPath(c.file))
		if err != nil {
			t.Fatal(err)
		}
		doc, err := ir.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		res := generate(t, doc)
		if len(res.Warnings) > 0 || len(res.UnknownProps) > 0 {
			t.Errorf("%s: warnings %v, unknown %v", c.file, res.Warnings, res.UnknownProps)
		}
		if n := len(res.Height.Objects()); n != c.objects {
			t.Errorf("%s: %d objects placed, want %d", c.file, n, c.objects)
		}
		if _, err := slab.DecodeGenerated(res.Code); err != nil {
			t.Errorf("%s: %v", c.file, err)
		}
	}
}

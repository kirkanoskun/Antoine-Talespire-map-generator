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
	// centre of the footprint, the point (21,21), on the levelled surface,
	// unrotated. North-south is mirrored on the way out: y=21 of 40 is Z=19.
	h := f.HeightAt(20, 20)
	want := slab.Instance{X: 2100, Y: surface(h), Z: (40 - 21) * 100}
	if got := fountain[0]; got.X != want.X || got.Z != want.Z || got.Y != want.Y || got.Rotation != 0 {
		t.Errorf("fountain = %+v, want %+v", got, want)
	}
	for _, p := range [][2]int{{20, 20}, {21, 20}, {20, 21}, {21, 21}} {
		if f.HeightAt(p[0], p[1]) != h {
			t.Errorf("footprint not levelled: height at %v is %d, want %d", p, f.HeightAt(p[0], p[1]), h)
		}
	}
	// A tile has its origin at its corner: it sits exactly on its cell, row 6,
	// which is row 33 once mirrored.
	stall := instancesOf(t, res.Code, "554ac0fb-4489-41dd-80ce-aec63f0a8649")
	if len(stall) != 1 || stall[0].X != 500 || stall[0].Z != (40-1-6)*100 || stall[0].Y != surface(f.HeightAt(5, 6)) {
		t.Errorf("stall = %+v, want X 500, Z 3300 on the surface", stall)
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
	// (Decoded positions: rows 19..22 of the map are rows 17..21 once mirrored.)
	near := func(x, z int) bool { return x >= 19 && x <= 22 && z >= 17 && z <= 21 }
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

// An object whose footprint would cross the east or south edge is slid back
// inside the map; one larger than the map itself is skipped.
func TestCatalogueObjectAtTheEdgeIsSlidInside(t *testing.T) {
	res := generate(t, square(t, `{"prop":"wagon_01","position":{"x":39,"y":38}}`))
	objs := res.Height.Objects()
	if len(objs) != 1 || objs[0].X != 38 || objs[0].Y != 35 || !strings.Contains(strings.Join(res.Warnings, "\n"), "moved from (39,38) to (38,35)") {
		t.Errorf("objects %v, warnings %v", objs, res.Warnings)
	}
	doc, err := ir.Parse([]byte(`{"map":{"width":2,"length":2,"biome":"temperate_forest"},"zones":[{"id":"a","anchor":{"x":0,"y":0},"relative_size":1,
		"points_of_interest":[{"prop":"wagon_01","position":{"x":0,"y":0}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	small := generate(t, doc)
	if len(small.Height.Objects()) != 0 || !strings.Contains(strings.Join(small.Warnings, "\n"), "outside the map") {
		t.Errorf("objects %v, warnings %v", small.Height.Objects(), small.Warnings)
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
	// The fountain's footprint is the north-west corner of slice [1][1] (rows
	// 20..21 of its rows 20..39): it is there, in that slice's own coordinates,
	// at the far end of its mirrored north-south axis.
	got := instancesOf(t, res.Slices[1][1], fountainUUID)
	if len(got) != 1 || got[0].X != 100 || got[0].Z != (20-1)*100 {
		t.Errorf("fountain in slice [1][1] = %+v, want X 100, Z 1900", got)
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

// The game's Z grows to the north, the map's y to the south: what the preview
// draws in the north must come out at high Z, and east-west must not move.
// (In game, two imports had shown the map mirrored north-south.)
func TestNorthOfTheMapIsNorthInTheGame(t *testing.T) {
	doc, err := ir.Parse([]byte(`{"map":{"width":20,"length":30,"biome":"temperate_forest"},
		"zones":[
			{"id":"place","anchor":{"x":10,"y":5},"relative_size":0.5,"elevation":"flat","relief_override":"paving",
			 "density_overrides":{"vegetation":0,"stones":0,"misc":0},
			 "points_of_interest":[{"prop":"fountain_01","position":{"x":2,"y":3}},{"prop":"campfire","position":{"x":16,"y":2}}]},
			{"id":"bois","anchor":{"x":10,"y":25},"relative_size":0.5,"elevation":"flat"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	g := newTestGen(t)
	g.SetTransitionHalfWidth(0)
	res, err := g.Generate(doc, spatial.Resolve(doc), 3)
	if err != nil {
		t.Fatal(err)
	}
	// The paved zone is the northern half (the border wanders a row or two):
	// its floor tiles are in the far half, up to the last row, 29 of 0..29.
	paving := instancesOf(t, res.Code, "a2eaf2e2-01d8-4548-a517-50df8551057a")
	rows := map[int]bool{}
	for _, p := range paving {
		rows[p.Z/100] = true
		if p.Z < 1200 {
			t.Fatalf("a paving tile of the northern zone is at Z=%d, in the southern half", p.Z)
		}
	}
	if !rows[29] || rows[30] {
		t.Errorf("the map's northern edge should be row 29 of 0..29, got rows %v", rows)
	}
	// A catalogue object in the north-west: west stays west, north goes far.
	fountain := instancesOf(t, res.Code, fountainUUID)
	if len(fountain) != 1 || fountain[0].X != 300 || fountain[0].Z != (30-4)*100 {
		t.Errorf("fountain = %+v, want X 300 (west), Z 2600 (north)", fountain)
	}
	// An original-catalogue prop in the north-east: east stays east. Its
	// origin is its centre, so the point y=2 becomes the point 30-2.
	fire := instancesOf(t, res.Code, "cf3fc034-c793-4aec-9345-98308dc91d4b")
	if len(fire) != 1 || fire[0].X != 1600 || fire[0].Z != (30-2)*100 {
		t.Errorf("campfire = %+v, want X 1600 (east), Z 2800 (north)", fire)
	}
}

// A sliced map is the same map cut up: every piece, put back where the preview
// shows it (columns west to east, rows north to south), gives the whole-map
// placements. Uneven on purpose: 40 tiles in slices of 16 is 16 + 16 + 8.
func TestSlicesReassembleIntoTheWholeMap(t *testing.T) {
	doc := square(t, `{"prop":"fountain_01","position":{"x":15,"y":15}},{"prop":"wagon_01","position":{"x":30,"y":28}},{"prop":"campfire","position":{"x":16,"y":32}}`)
	g := newTestGen(t)
	g.SetSliceSize(16)
	res, err := g.Generate(doc, spatial.Resolve(doc), 7)
	if err != nil {
		t.Fatal(err)
	}
	lines := func(code string, dx, dz int) []string {
		s, err := slab.DecodeGenerated(code)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, l := range s.Layouts {
			for _, p := range l.Instances {
				out = append(out, fmt.Sprintf("%x %d %d %d", l.ID, p.X+dx, p.Y, p.Z+dz))
			}
		}
		return out
	}
	whole := lines(res.Code, 0, 0)
	var pieces []string
	for sx := range res.Slices {
		for sy, code := range res.Slices[sx] {
			rows := min(16, 40-sy*16)
			// The piece's southern edge is its Z origin: 40 minus its last row.
			pieces = append(pieces, lines(code, sx*1600, (40-sy*16-rows)*100)...)
		}
	}
	sort.Strings(whole)
	sort.Strings(pieces)
	if strings.Join(whole, "\n") != strings.Join(pieces, "\n") {
		t.Errorf("the pieces do not reassemble into the whole map (%d vs %d placements)", len(pieces), len(whole))
	}
}

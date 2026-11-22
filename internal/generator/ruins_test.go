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

// ruinedVillage is a 40x40 forest whose northern two thirds are ruins.
func ruinedVillage(t *testing.T) *ir.IR {
	t.Helper()
	doc, err := ir.Parse([]byte(`{"map":{"width":40,"length":40,"biome":"temperate_forest"},
		"zones":[
		{"id":"ruines","anchor":{"x":20,"y":16},"relative_size":0.65,"elevation":"flat","relief_override":"ruins"},
		{"id":"foret","anchor":{"x":20,"y":36},"relative_size":0.35,"elevation":"flat"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestRuinsAreBuiltInsideTheirZone(t *testing.T) {
	doc := ruinedVillage(t)
	mask := spatial.Resolve(doc)
	res, err := newTestGen(t).Generate(doc, mask, 3)
	if err != nil {
		t.Fatal(err)
	}
	ruins := res.Height.Ruins()
	if len(ruins) < 4 {
		t.Fatalf("%d ruined buildings, want a village", len(ruins))
	}
	for i, r := range ruins {
		if r.W < 6 || r.D < 6 || r.W > 10 || r.D > 10 {
			t.Errorf("ruin %d is %dx%d, want 6 to 10 tiles a side (walls included)", i, r.W, r.D)
		}
		for x := r.X; x < r.X+r.W; x++ {
			for y := r.Y; y < r.Y+r.D; y++ {
				if mask.ZoneIndexAt(x, y) != 0 {
					t.Fatalf("ruin %d reaches (%d,%d), outside the ruins zone", i, x, y)
				}
			}
		}
		for j, o := range ruins[:i] {
			// A free tile between two ruins: they never share a wall.
			if r.X <= o.X+o.W && o.X <= r.X+r.W && r.Y <= o.Y+o.D && o.Y <= r.Y+r.D {
				t.Errorf("ruins %d and %d touch: %+v %+v", j, i, o, r)
			}
		}
	}
}

// In the game, a wall stands on the edge of its floor whichever way it faces:
// the rule measured on the community ruins (a tile's position is the corner of
// its turned footprint), applied after the north-south mirror.
func TestRuinWallsStandOnTheirFloorsEdgeInTheGame(t *testing.T) {
	doc := ruinedVillage(t)
	g := newTestGen(t)
	res, err := g.Generate(doc, spatial.Resolve(doc), 3)
	if err != nil {
		t.Fatal(err)
	}
	wall, _ := g.assets.BySlug(ruinWall)
	wallID, _ := wall.LayoutID()
	const L = 40
	// Where each side's walls may stand, in Slab units (game space, Z north).
	want := map[uint8]map[int]bool{rotNorth: {}, rotSouth: {}, rotWest: {}, rotEast: {}}
	for _, r := range res.Height.Ruins() {
		x0, y0, fx, fy := r.X+1, r.Y+1, r.X+r.W-1, r.Y+r.D-1
		want[rotNorth][(L-y0)*100] = true    // just north of the floor
		want[rotSouth][(L-fy)*100-50] = true // half a tile south of it
		want[rotWest][x0*100-50] = true      // half a tile west of it
		want[rotEast][fx*100] = true         // just east of it
	}
	s, err := slab.DecodeGenerated(res.Code)
	if err != nil {
		t.Fatal(err)
	}
	walls := 0
	for _, l := range s.Layouts {
		if l.ID != wallID {
			continue
		}
		for _, p := range l.Instances {
			walls++
			at := p.Z
			if p.Rotation == rotWest || p.Rotation == rotEast {
				at = p.X
			}
			if !want[p.Rotation][at] {
				t.Fatalf("a wall turned %d stands at %d, off every floor edge", p.Rotation, at)
			}
		}
	}
	if walls < 50 {
		t.Fatalf("%d wall pieces, want a village's worth", walls)
	}
}

// The top line of a ruined wall is jagged: about half its pieces are sunk into
// the ground, as on the community ruins.
func TestRuinWallsAreJagged(t *testing.T) {
	doc := ruinedVillage(t)
	g := newTestGen(t)
	res, err := g.Generate(doc, spatial.Resolve(doc), 3)
	if err != nil {
		t.Fatal(err)
	}
	wall, _ := g.assets.BySlug(ruinWall)
	wallID, _ := wall.LayoutID()
	s, err := slab.DecodeGenerated(res.Code)
	if err != nil {
		t.Fatal(err)
	}
	ground := surface(res.Height.HeightAt(res.Height.Ruins()[0].X, res.Height.Ruins()[0].Y))
	levels := map[int]int{}
	total := 0
	for _, l := range s.Layouts {
		if l.ID == wallID {
			for _, p := range l.Instances {
				levels[ground-p.Y]++
				total++
			}
		}
	}
	sunk := levels[50] + levels[100] + levels[150]
	if f := float64(sunk) / float64(total); f < 0.3 || f > 0.65 {
		t.Errorf("%.0f%% of the wall pieces are sunk (%v), want about half", f*100, levels)
	}
}

func TestRuinedVillageSlicesReassemble(t *testing.T) {
	doc := ruinedVillage(t)
	g := newTestGen(t)
	g.SetSliceSize(16)
	res, err := g.Generate(doc, spatial.Resolve(doc), 3)
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
				out = append(out, fmt.Sprintf("%x %d %d %d %d", l.ID, p.X+dx, p.Y, p.Z+dz, p.Rotation))
			}
		}
		return out
	}
	whole := lines(res.Code, 0, 0)
	var pieces []string
	for sx := range res.Slices {
		for sy, code := range res.Slices[sx] {
			rows := min(16, 40-sy*16)
			pieces = append(pieces, lines(code, sx*1600, (40-sy*16-rows)*100)...)
		}
	}
	sort.Strings(whole)
	sort.Strings(pieces)
	if strings.Join(whole, "\n") != strings.Join(pieces, "\n") {
		t.Errorf("the pieces do not reassemble into the whole map (%d vs %d placements)", len(pieces), len(whole))
	}
}

func TestRuinsAreReproducible(t *testing.T) {
	doc := ruinedVillage(t)
	a, err := newTestGen(t).Generate(doc, spatial.Resolve(doc), 5)
	if err != nil {
		t.Fatal(err)
	}
	b, err := newTestGen(t).Generate(doc, spatial.Resolve(doc), 5)
	if err != nil {
		t.Fatal(err)
	}
	if a.Code != b.Code {
		t.Error("the same seed gave two different ruined villages")
	}
}

// Ruins never push out a point of interest: the castle example's altar, placed
// by hand among the ruins, and its scattered broken columns all stay.
func TestRuinsLeaveThePointsOfInterest(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/castle.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ir.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for seed := int64(1); seed <= 12; seed++ {
		res, err := newTestGen(t).Generate(doc, spatial.Resolve(doc), seed)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Height.Ruins()) == 0 {
			t.Fatalf("seed %d: no ruined building in the castle's ruins", seed)
		}
		for _, w := range res.Warnings {
			if strings.Contains(w, "overlaps") {
				t.Errorf("seed %d: %s", seed, w)
			}
		}
	}
}

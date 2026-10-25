package generator

import (
	"os"
	"reflect"
	"testing"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/prefab"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/slab"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

func buildingSetup(t *testing.T) (*Generator, *ir.IR) {
	t.Helper()
	data, err := os.ReadFile("../../testdata/slabs/precision-v2.txt")
	if err != nil {
		t.Fatal(err)
	}
	store, err := prefab.Open("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(prefab.Entry{ID: "house", Name: "House", Code: string(data), Width: 6}); err != nil {
		t.Fatal(err)
	}
	doc, err := ir.Parse([]byte(`{"map":{"width":20,"length":20,"biome":"temperate_forest"},"zones":[{"id":"land","anchor":{"x":10,"y":10},"relative_size":1}],"buildings":[{"prefab":"house","position":{"x":8,"y":7},"rotation":0}]}`))
	if err != nil {
		t.Fatal(err)
	}
	g := newTestGen(t)
	g.SetPrefabs(store)
	return g, doc
}

func importedInstances(t *testing.T, code string) []slab.Instance {
	t.Helper()
	s, err := slab.DecodeGenerated(code)
	if err != nil {
		t.Fatal(err)
	}
	var out []slab.Instance
	for _, l := range s.Layouts {
		if l.ID[0] == 0 && l.ID[1] == 1 && l.ID[15] == 15 {
			if l.Reserved != 7 {
				t.Fatal("lost reserved layout bits")
			}
			out = append(out, l.Instances...)
		}
	}
	return out
}

func TestBuildingPrecisionRotationAndSlicing(t *testing.T) {
	for _, angle := range []int{0, 90, 180, 270} {
		t.Run(string(rune('a'+angle/90)), func(t *testing.T) {
			g, doc := buildingSetup(t)
			doc.Buildings[0].Rotation = angle
			res, err := g.Generate(doc, spatial.Resolve(doc), 1)
			if err != nil {
				t.Fatal(err)
			}
			got := importedInstances(t, res.Code)
			pairs := map[int][][2]int{0: {{0, 0}, {125, 125}}, 90: {{0, 125}, {125, 0}}, 180: {{125, 125}, {0, 0}}, 270: {{125, 0}, {0, 125}}}[angle]
			want := []slab.Instance{{X: 900 + pairs[0][0], Y: 100, Z: 800 + pairs[0][1], Rotation: uint8((3 + angle/15) % 24), Extra: 17}, {X: 900 + pairs[1][0], Y: 225, Z: 800 + pairs[1][1], Rotation: uint8((19 + angle/15) % 24), Extra: 5}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("angle %d: got %+v, want %+v", angle, got, want)
			}
			if !res.Height.BuildingAt(8, 7) {
				t.Fatal("footprint was not reserved")
			}
			if angle == 90 && (!res.Height.BuildingAt(11, 12) || res.Height.BuildingAt(12, 12)) {
				t.Fatal("rotated footprint dimensions are wrong")
			}
			g.SetSliceSize(5)
			sliced, err := g.Generate(doc, spatial.Resolve(doc), 1)
			if err != nil {
				t.Fatal(err)
			}
			if sliced.Code != res.Code {
				t.Fatal("slicing changed whole-map output")
			}
			count := 0
			for x := range sliced.Slices {
				for y, code := range sliced.Slices[x] {
					parts := importedInstances(t, code)
					count += len(parts)
					for i := range parts {
						parts[i].X += x * 500
						parts[i].Z += y * 500
					}
					if len(parts) > 0 && !reflect.DeepEqual(parts, want) {
						t.Fatal("slicing lost or transformed building parts")
					}
				}
			}
			if count != 2 {
				t.Fatalf("slices contain %d building assets", count)
			}
		})
	}
}

func TestBuildingRejectsUnknownOverlapWaterAndBounds(t *testing.T) {
	for _, kind := range []string{"unknown", "overlap", "water", "bounds"} {
		t.Run(kind, func(t *testing.T) {
			g, doc := buildingSetup(t)
			switch kind {
			case "unknown":
				doc.Buildings[0].Prefab = "missing"
			case "overlap":
				doc.Buildings = append(doc.Buildings, doc.Buildings[0])
			case "water":
				doc.Zones[0].ReliefOverride = "water"
			case "bounds":
				doc.Buildings[0].Position.X = 19
			}
			if _, err := g.Generate(doc, spatial.Resolve(doc), 1); err == nil {
				t.Fatal("invalid building placement accepted")
			}
		})
	}
}

// cellarBuilding imports a 5x5 building whose ground floor sits 400 units (8
// terrain steps) above two cellar pieces — the shape that made a real
// community inn float in game when builds were seated by their lowest piece.
func cellarBuilding(t *testing.T, groundY *int) (*Generator, *ir.IR) {
	t.Helper()
	var floor []slab.Instance
	for x := 0; x < 5; x++ {
		for z := 0; z < 5; z++ {
			floor = append(floor, slab.Instance{X: x * 100, Y: 400, Z: z * 100})
		}
	}
	cellar := []slab.Instance{{X: 100, Y: 0, Z: 100}, {X: 200, Y: 0, Z: 100}}
	code, err := slab.Encode(&slab.Slab{Layouts: []slab.Layout{
		{ID: [16]byte{0xF1}, Instances: floor},
		{ID: [16]byte{0xF2}, Instances: cellar},
	}})
	if err != nil {
		t.Fatal(err)
	}
	store, err := prefab.Open("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(prefab.Entry{ID: "inn", Name: "Inn", Code: code, GroundY: groundY}); err != nil {
		t.Fatal(err)
	}
	g := newTestGen(t)
	g.SetPrefabs(store)
	return g, cellarScene(t, true)
}

func cellarScene(t *testing.T, withBuilding bool) *ir.IR {
	t.Helper()
	buildings := ""
	if withBuilding {
		buildings = `,"buildings":[{"prefab":"inn","position":{"x":5,"y":5}}]`
	}
	doc, err := ir.Parse([]byte(`{"map":{"width":20,"length":20,"biome":"temperate_forest"},"zones":[{"id":"land","anchor":{"x":10,"y":10},"relative_size":1}]` + buildings + `}`))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// layoutsByFirstByte splits a generated code into the fixture's floor and
// cellar pieces, and everything else (the map's own terrain and scatter).
func layoutsByFirstByte(t *testing.T, code string) (floor, cellar, terrain []slab.Instance) {
	t.Helper()
	s, err := slab.DecodeGenerated(code)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range s.Layouts {
		switch {
		case l.ID == [16]byte{0xF1}:
			floor = append(floor, l.Instances...)
		case l.ID == [16]byte{0xF2}:
			cellar = append(cellar, l.Instances...)
		default:
			terrain = append(terrain, l.Instances...)
		}
	}
	return floor, cellar, terrain
}

func TestBuildingSeatsOnItsGroundLevel(t *testing.T) {
	g, doc := cellarBuilding(t, nil)
	res, err := g.Generate(doc, spatial.Resolve(doc), 1)
	if err != nil {
		t.Fatal(err)
	}
	floor, cellar, terrain := layoutsByFirstByte(t, res.Code)

	// The ground floor rests exactly on the terrain's surface…
	top := surface(res.Height.HeightAt(6, 6))
	for _, p := range floor {
		if p.Y != top {
			t.Fatalf("ground floor at Y=%d, want the terrain surface %d", p.Y, top)
		}
	}
	// …and the cellar keeps its depth below it, underground, never below zero.
	for _, p := range cellar {
		if p.Y != top-400 || p.Y < 0 {
			t.Fatalf("cellar at Y=%d, want %d (400 below the ground floor, >= 0)", p.Y, top-400)
		}
	}

	// The cellar needs 400 units of room but the terrain surface was only 100
	// up: the whole scene rose by the missing 300, i.e. 6 terrain steps.
	base, err := g.Generate(cellarScene(t, false), spatial.Resolve(cellarScene(t, false)), 1)
	if err != nil {
		t.Fatal(err)
	}
	if lift := res.Height.HeightAt(15, 15) - base.Height.HeightAt(15, 15); lift != 6 {
		t.Errorf("scene raised by %d steps away from the building, want 6", lift)
	}

	// The map's own ground is dug out where the cellar is (tiles (7,7) and
	// (8,7)), and only there.
	for _, tile := range [][2]int{{7, 7}, {8, 7}} {
		if !res.Height.dugAt(tile[0], tile[1]) {
			t.Errorf("tile %v under the cellar was not dug", tile)
		}
		for _, p := range terrain {
			if p.X == tile[0]*100 && p.Z == tile[1]*100 {
				t.Fatalf("terrain still fills the cellar at tile %v (Y=%d)", tile, p.Y)
			}
		}
	}
	if res.Height.dugAt(6, 6) {
		t.Error("a tile with no underground piece was dug")
	}
	groundAt66 := false
	for _, p := range terrain {
		if p.X == 600 && p.Z == 600 {
			groundAt66 = true
		}
	}
	if !groundAt66 {
		t.Error("no terrain under the building where nothing is underground")
	}
}

// An explicit ground_y of 0 seats the lowest piece on the terrain: no lift and
// no digging — the escape hatch for a build whose busiest level is not its
// ground floor.
func TestGroundLevelOverrideSeatsTheChosenLevel(t *testing.T) {
	zero := 0
	g, doc := cellarBuilding(t, &zero)
	res, err := g.Generate(doc, spatial.Resolve(doc), 1)
	if err != nil {
		t.Fatal(err)
	}
	floor, cellar, _ := layoutsByFirstByte(t, res.Code)
	top := surface(res.Height.HeightAt(6, 6))
	if cellar[0].Y != top || floor[0].Y != top+400 {
		t.Errorf("cellar at %d and floor at %d, want %d and %d", cellar[0].Y, floor[0].Y, top, top+400)
	}
	base, err := g.Generate(cellarScene(t, false), spatial.Resolve(cellarScene(t, false)), 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Height.HeightAt(15, 15) != base.Height.HeightAt(15, 15) {
		t.Error("the scene was raised although nothing goes underground")
	}
	if res.Height.dugAt(7, 7) {
		t.Error("ground was dug although nothing goes underground")
	}
}

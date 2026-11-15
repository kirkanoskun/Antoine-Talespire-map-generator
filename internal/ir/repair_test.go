package ir

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

const lenientBase = `{"map":{"width":40,"length":30,"name":"t","biome":"temperate_forest"},
	"zones":[{"id":"a","anchor":{"x":20,"y":15},"relative_size":0.8,"elevation":"flat"}]}`

func testCatalogue() *Catalogue {
	return &Catalogue{
		Reliefs: func(biome string) []string {
			if biome == "temperate_forest" {
				return []string{"base_ground", "ground", "mountain", "paving", "ruins", "water"}
			}
			return nil
		},
		KnownProp:   func(name string) bool { return name == "altar" || name == "fountain_01" },
		KnownPrefab: func(id string) bool { return id == "inn" },
	}
}

func codes(repairs []Repair) string {
	var out []string
	for _, r := range repairs {
		out = append(out, r.Code)
	}
	return strings.Join(out, ",")
}

// A valid document must come back exactly as the strict reader returns it,
// with nothing reported: leniency never changes a correct map.
func TestParseLenientLeavesValidDocumentsAlone(t *testing.T) {
	for _, file := range []string{"castle.json", "twozones.json", "transition.json", "nested.json", "scene_place_village.json", "scene_cimetiere_marais.json"} {
		raw, err := os.ReadFile("../../testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		strict, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		lenient, repairs, err := ParseLenient(raw, nil)
		if err != nil || len(repairs) != 0 || !reflect.DeepEqual(strict, lenient) {
			t.Errorf("%s: err %v, repairs %v, same %v", file, err, repairs, reflect.DeepEqual(strict, lenient))
		}
	}
}

func TestParseLenientRepairs(t *testing.T) {
	zone := func(body string) string {
		return `{"map":{"width":40,"length":30,"biome":"temperate_forest"},"zones":[{` + body + `}]}`
	}
	for _, c := range []struct {
		name, in, wantCodes string
		check               func(*IR) bool
	}{
		{"decimal anchor", zone(`"id":"a","anchor":{"x":20.6,"y":15},"relative_size":0.8`), RepairRounded,
			func(d *IR) bool { return d.Zones[0].Anchor.X == 21 }},
		{"number as string", `{"map":{"width":"40","length":30,"biome":"temperate_forest"},"zones":[{"id":"a","anchor":{"x":1,"y":1},"relative_size":0.5}]}`, "",
			func(d *IR) bool { return d.Map.Width == 40 }},
		{"unknown fields", `{"comment":"x","map":{"width":40,"length":30,"biome":"temperate_forest","style":"dark"},"zones":[{"id":"a","anchor":{"x":1,"y":1},"relative_size":0.5,"notes":"n"}]}`,
			RepairUnknownField + "," + RepairUnknownField + "," + RepairUnknownField, nil},
		{"zone name becomes its description", zone(`"id":"a","name":"La cour","anchor":{"x":1,"y":1},"relative_size":0.5`), "",
			func(d *IR) bool { return d.Zones[0].Description == "La cour" }},
		{"biome in French", `{"map":{"width":40,"length":30,"biome":"Forêt tempérée"},"zones":[{"id":"a","anchor":{"x":1,"y":1},"relative_size":0.5}]}`, RepairBiome,
			func(d *IR) bool { return d.Map.Biome == "temperate_forest" }},
		{"elevation synonym", zone(`"id":"a","anchor":{"x":1,"y":1},"relative_size":0.5,"elevation":"Colline"`), RepairElevation,
			func(d *IR) bool { return d.Zones[0].Elevation == ElevationHill }},
		{"unknown elevation is flat", zone(`"id":"a","anchor":{"x":1,"y":1},"relative_size":0.5,"elevation":"wavy"`), RepairElevation,
			func(d *IR) bool { return d.Zones[0].Elevation == ElevationFlat }},
		{"size as a percentage", zone(`"id":"a","anchor":{"x":1,"y":1},"relative_size":60`), RepairZoneSize,
			func(d *IR) bool { return d.Zones[0].RelativeSize == 0.6 }},
		{"size just over one", zone(`"id":"a","anchor":{"x":1,"y":1},"relative_size":1.2`), RepairZoneSize,
			func(d *IR) bool { return d.Zones[0].RelativeSize == 1 }},
		{"size missing", zone(`"id":"a","anchor":{"x":1,"y":1}`), RepairZoneSize,
			func(d *IR) bool { return d.Zones[0].RelativeSize == 0.3 }},
		{"anchor one past the edge", zone(`"id":"a","anchor":{"x":40,"y":30},"relative_size":0.5`), RepairAnchor,
			func(d *IR) bool { return d.Zones[0].Anchor.X == 39 && d.Zones[0].Anchor.Y == 29 }},
		// (A document valid apart from a missing anchor keeps the strict reader's
		// default, the map's corner; with another slip to repair, it is centred.)
		{"anchor missing", zone(`"id":"a","relative_size":0.5,"elevation":"Flat"`), RepairAnchor + "," + RepairElevation,
			func(d *IR) bool { return d.Zones[0].Anchor.X == 20 && d.Zones[0].Anchor.Y == 15 }},
		{"anchor as a pair", zone(`"id":"a","anchor":[7,8],"relative_size":0.5`), "",
			func(d *IR) bool { return d.Zones[0].Anchor.X == 7 && d.Zones[0].Anchor.Y == 8 }},
		{"id missing", zone(`"anchor":{"x":1,"y":1},"relative_size":0.5`), RepairZoneID,
			func(d *IR) bool { return d.Zones[0].ID == "zone_1" }},
		{"density words and ranges", zone(`"id":"a","anchor":{"x":1,"y":1},"relative_size":0.5,"density_overrides":{"trees":50,"stones":1.5,"fog":0.2}`),
			"", func(d *IR) bool {
				o := d.Zones[0].DensityOverrides
				return o["vegetation"] == 0.5 && o["stones"] == 1 && len(o) == 2
			}},
		{"position keyword", zone(`"id":"a","anchor":{"x":9,"y":8},"relative_size":0.5,"points_of_interest":[{"prop":"altar","position":"centre"}]`), RepairPosition,
			func(d *IR) bool {
				p := d.Zones[0].PointsOfInterest[0].Position
				return !p.Scattered && p.X == 9 && p.Y == 8
			}},
		{"position as a pair, outside", zone(`"id":"a","anchor":{"x":9,"y":8},"relative_size":0.5,"points_of_interest":[{"prop":"altar","position":[40,2]}]`), RepairPosition,
			func(d *IR) bool { p := d.Zones[0].PointsOfInterest[0].Position; return p.X == 39 && p.Y == 2 }},
		{"scattered without a count", zone(`"id":"a","anchor":{"x":9,"y":8},"relative_size":0.5,"points_of_interest":[{"prop":"altar","position":"scattered"}]`), RepairCount,
			func(d *IR) bool { return d.Zones[0].PointsOfInterest[0].Count == 1 }},
		{"prop under another key, extra field", zone(`"id":"a","anchor":{"x":9,"y":8},"relative_size":0.5,"points_of_interest":[{"name":"altar","position":{"x":1,"y":1},"rotation":90}]`), RepairUnknownField,
			func(d *IR) bool { return d.Zones[0].PointsOfInterest[0].Prop == "altar" }},
		{"point of interest with no prop", zone(`"id":"a","anchor":{"x":9,"y":8},"relative_size":0.5,"points_of_interest":[{"position":{"x":1,"y":1}}]`), RepairPOIDropped,
			func(d *IR) bool { return len(d.Zones[0].PointsOfInterest) == 0 }},
		{"comments and trailing commas", "{\"map\":{\"width\":40,\"length\":30,\"biome\":\"temperate_forest\",}, // la carte\n\"zones\":[{\"id\":\"a // b\",\"anchor\":{\"x\":1,\"y\":1},\"relative_size\":0.5,},] /* fin */}", RepairSyntax,
			func(d *IR) bool { return d.Zones[0].ID == "a // b" }},
	} {
		doc, repairs, err := ParseLenient([]byte(c.in), nil)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if c.wantCodes != "" && codes(repairs) != c.wantCodes {
			t.Errorf("%s: repairs %s, want %s (%+v)", c.name, codes(repairs), c.wantCodes, repairs)
		}
		if c.check != nil && !c.check(doc) {
			out, _ := json.Marshal(doc)
			t.Errorf("%s: repaired to %s", c.name, out)
		}
		if err := doc.Validate(); err != nil {
			t.Errorf("%s: the repaired document is not valid: %v", c.name, err)
		}
	}
}

func TestParseLenientZonesConnectionsBuildings(t *testing.T) {
	in := `{"map":{"width":40,"length":30,"biome":"temperate_forest"},
		"zones":[{"id":"a","anchor":{"x":5,"y":5},"relative_size":0.5},{"id":"a","anchor":{"x":30,"y":20},"relative_size":0.5}],
		"connections":[{"from":"a","to":"a_2","width":500},{"from":"a","to":"nowhere"},{"from":"a","to":"a"}],
		"buildings":[{"prefab":"inn","position":{"x":50,"y":2},"rotation":100},{"prefab":"castle","position":{"x":1,"y":1}},{"prefab":"inn"}]}`
	doc, repairs, err := ParseLenient([]byte(in), testCatalogue())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Zones[1].ID != "a_2" {
		t.Errorf("duplicate id not renamed: %q", doc.Zones[1].ID)
	}
	if len(doc.Connections) != 1 || doc.Connections[0].Width != maxPathWidth {
		t.Errorf("connections = %+v", doc.Connections)
	}
	if len(doc.Buildings) != 1 || doc.Buildings[0].Prefab != "inn" || doc.Buildings[0].Position.X != 39 || doc.Buildings[0].Rotation != 90 {
		t.Errorf("buildings = %+v", doc.Buildings)
	}
	for _, want := range []string{RepairZoneID, RepairPathWidth, RepairConnection, RepairBuilding, RepairBuildingDropped} {
		if !strings.Contains(codes(repairs), want) {
			t.Errorf("missing repair %s in %s", want, codes(repairs))
		}
	}
}

// An oversize map is scaled down as a whole, not cropped: what was at 3/4 of
// the width still is.
func TestParseLenientScalesAnOversizeMap(t *testing.T) {
	in := `{"map":{"width":400,"length":100,"biome":"temperate_forest"},
		"zones":[{"id":"a","anchor":{"x":300,"y":50},"relative_size":0.5,"points_of_interest":[{"prop":"altar","position":{"x":399,"y":99}}]}]}`
	doc, repairs, err := ParseLenient([]byte(in), nil)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Map.Width != 200 || doc.Map.Length != 100 || doc.Zones[0].Anchor.X != 150 || doc.Zones[0].Anchor.Y != 50 {
		t.Errorf("map %dx%d, anchor %+v", doc.Map.Width, doc.Map.Length, doc.Zones[0].Anchor)
	}
	if p := doc.Zones[0].PointsOfInterest[0].Position; p.X != 199 || p.Y != 99 {
		t.Errorf("point of interest at %+v", p)
	}
	if codes(repairs) != RepairMapSize {
		t.Errorf("repairs = %+v", repairs)
	}
}

// The catalogue fallbacks also apply to a document the strict reader accepts.
func TestParseLenientCatalogueFallbacks(t *testing.T) {
	in := `{"map":{"width":40,"length":30,"biome":"temperate_forest"},
		"zones":[
			{"id":"a","anchor":{"x":5,"y":5},"relative_size":0.5,"relief_override":"Cobblestone","points_of_interest":[{"prop":"Fountain 01","position":{"x":1,"y":1}},{"prop":"dragon","position":{"x":2,"y":2}}]},
			{"id":"b","anchor":{"x":30,"y":20},"relative_size":0.5,"relief_override":"lava_lake"},
			{"id":"c","anchor":{"x":10,"y":20},"relative_size":0.2,"relief_override":"ruins"}],
		"connections":[{"from":"a","to":"b","width":-4}],
		"buildings":[{"prefab":"castle","position":{"x":1,"y":1}}]}`
	doc, repairs, err := ParseLenient([]byte(in), testCatalogue())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Zones[0].ReliefOverride != "paving" || doc.Zones[1].ReliefOverride != "" || doc.Zones[2].ReliefOverride != "ruins" {
		t.Errorf("reliefs = %q %q %q", doc.Zones[0].ReliefOverride, doc.Zones[1].ReliefOverride, doc.Zones[2].ReliefOverride)
	}
	if pois := doc.Zones[0].PointsOfInterest; pois[0].Prop != "fountain_01" || pois[1].Prop != "dragon" {
		t.Errorf("props = %+v", pois)
	}
	if doc.Buildings != nil || doc.Connections[0].Width != 0 {
		t.Errorf("buildings %+v, path width %d", doc.Buildings, doc.Connections[0].Width)
	}
	if got := codes(repairs); got != strings.Join([]string{RepairPathWidth, RepairRelief, RepairReliefDropped, RepairProp, RepairBuildingDropped}, ",") {
		t.Errorf("repairs = %s", got)
	}
}

// What would mean guessing the scene is still refused, with a message an AI
// can act on.
func TestParseLenientStillRefuses(t *testing.T) {
	for name, in := range map[string]string{
		"no zones":      `{"map":{"width":40,"length":30,"biome":"temperate_forest"},"zones":[]}`,
		"unknown biome": `{"map":{"width":40,"length":30,"biome":"underwater_city"},"zones":[{"id":"a","anchor":{"x":1,"y":1},"relative_size":0.5}]}`,
		"no biome":      `{"map":{"width":40,"length":30},"zones":[{"id":"a","anchor":{"x":1,"y":1},"relative_size":0.5}]}`,
		"not json":      `{"map": {"width": 40} "zones": []}`,
		"an array":      `[1,2,3]`,
		"empty":         ``,
	} {
		if doc, _, err := ParseLenient([]byte(in), testCatalogue()); err == nil {
			t.Errorf("%s: accepted as %+v", name, doc)
		}
	}
	_, _, err := ParseLenient([]byte(`{"map":{"width":40,"length":30,"biome":"mars"},"zones":[{"id":"a","anchor":{"x":1,"y":1},"relative_size":0.5}]}`), nil)
	if err == nil || !strings.Contains(err.Error(), "temperate_forest") {
		t.Errorf("an unknown biome should list the valid ones, got %v", err)
	}
}

// Whatever the input, the lenient reader must not panic, and what it accepts
// must be a valid document.
func FuzzParseLenient(f *testing.F) {
	f.Add([]byte(lenientBase))
	f.Add([]byte(`{"map":{"width":40.5,"length":"30","biome":"Forêt"},"zones":[{"anchor":[1,2],"relative_size":60,"elevation":"Hill","density_overrides":{"trees":50},"points_of_interest":[{"name":"altar","position":"centre"},{"prop":"x","position":[1]}]}],"connections":[{"from":"zone_1","to":"zone_1","width":1e30}],"buildings":[{"prefab":"inn","position":[1,2],"rotation":-1e30}]}`))
	f.Add([]byte(`{"map":null,"zones":[null,1,"a",{"id":3,"anchor":null,"points_of_interest":[null,{}]}],"connections":[null],"buildings":[null,{}]}`))
	f.Add([]byte(`{"map":{"width":-1e400,"length":1e400},"zones":[{"id":"a","relative_size":"NaN","anchor":{"x":"Infinity","y":-0}}]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, _, err := ParseLenient(data, testCatalogue())
		if err != nil {
			return
		}
		if verr := doc.Validate(); verr != nil {
			t.Fatalf("accepted an invalid document: %v", verr)
		}
	})
}

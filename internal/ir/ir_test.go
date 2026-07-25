package ir

import "testing"

func TestParseValid(t *testing.T) {
	doc, err := Parse([]byte(`{
		"map": {"width": 40, "length": 40, "name": "t", "biome": "temperate_forest"},
		"zones": [
			{"id":"a","anchor":{"x":10,"y":20},"relative_size":0.5,"elevation":"flat"},
			{"id":"b","anchor":{"x":30,"y":20},"relative_size":0.5,"relief_override":"mountain"}
		]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.Map.Biome != "temperate_forest" {
		t.Fatalf("map biome not parsed, got %q", doc.Map.Biome)
	}
	if len(doc.Zones) != 2 {
		t.Fatalf("want 2 zones, got %d", len(doc.Zones))
	}
	// Elevation defaults to flat when omitted.
	if doc.Zones[1].Elevation != ElevationFlat {
		t.Errorf("want default flat elevation, got %q", doc.Zones[1].Elevation)
	}
	if doc.Zones[1].ReliefOverride != "mountain" {
		t.Errorf("relief_override not parsed, got %q", doc.Zones[1].ReliefOverride)
	}
}

func TestPositionUnmarshal(t *testing.T) {
	doc, err := Parse([]byte(`{
		"map": {"width": 40, "length": 40, "name": "t", "biome": "dead_forest"},
		"zones": [{
			"id":"a","anchor":{"x":10,"y":10},"relative_size":0.5,
			"points_of_interest":[
				{"prop":"altar","position":{"x":5,"y":5}},
				{"prop":"broken_pillar","position":"scattered","count":3}
			]
		}]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pois := doc.Zones[0].PointsOfInterest
	if pois[0].Position.Scattered || pois[0].Position.X != 5 {
		t.Errorf("explicit position not parsed: %+v", pois[0].Position)
	}
	if !pois[1].Position.Scattered || pois[1].Count != 3 {
		t.Errorf("scattered position not parsed: %+v", pois[1])
	}
}

func TestValidationErrors(t *testing.T) {
	cases := map[string]string{
		"zero dimensions": `{"map":{"width":0,"length":10,"name":"x","biome":"desert"},"zones":[{"id":"a","anchor":{"x":0,"y":0},"relative_size":0.5}]}`,
		"no zones":        `{"map":{"width":10,"length":10,"name":"x","biome":"desert"},"zones":[]}`,
		"unknown biome":   `{"map":{"width":10,"length":10,"name":"x","biome":"nope"},"zones":[{"id":"a","anchor":{"x":0,"y":0},"relative_size":0.5}]}`,
		"missing biome":   `{"map":{"width":10,"length":10,"name":"x"},"zones":[{"id":"a","anchor":{"x":0,"y":0},"relative_size":0.5}]}`,
		"bad size":        `{"map":{"width":10,"length":10,"name":"x","biome":"desert"},"zones":[{"id":"a","anchor":{"x":0,"y":0},"relative_size":2}]}`,
		"anchor oob":      `{"map":{"width":10,"length":10,"name":"x","biome":"desert"},"zones":[{"id":"a","anchor":{"x":50,"y":0},"relative_size":0.5}]}`,
		"duplicate id":    `{"map":{"width":10,"length":10,"name":"x","biome":"desert"},"zones":[{"id":"a","anchor":{"x":0,"y":0},"relative_size":0.5},{"id":"a","anchor":{"x":1,"y":1},"relative_size":0.5}]}`,
		"unknown field":   `{"map":{"width":10,"length":10,"name":"x","biome":"desert"},"zones":[{"id":"a","anchor":{"x":0,"y":0},"relative_size":0.5}],"bogus":1}`,
		"bad connection":  `{"map":{"width":10,"length":10,"name":"x","biome":"desert"},"zones":[{"id":"a","anchor":{"x":0,"y":0},"relative_size":0.5}],"connections":[{"from":"a","to":"ghost"}]}`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(in)); err == nil {
				t.Errorf("expected error for %s, got none", name)
			}
		})
	}
}

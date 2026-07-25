package spatial

import "testing"

// A small zone whose anchor coincides with a larger zone's centre is swallowed
// by pure Voronoi; stampNestedZones must carve it back as a disc.
func TestNestedZoneIsStamped(t *testing.T) {
	doc := mustParse(t, `{
		"map":{"width":40,"length":40,"name":"t","biome":"temperate_forest"},
		"zones":[
			{"id":"clearing","anchor":{"x":20,"y":20},"relative_size":0.7,"elevation":"flat"},
			{"id":"pond","anchor":{"x":20,"y":20},"relative_size":0.12,"elevation":"depression","relief_override":"water"}
		]}`)
	m := Resolve(doc)
	counts := m.TileCounts()

	if counts[1] == 0 {
		t.Fatal("nested pond captured no tiles; stamping did not run")
	}
	if m.ZoneIndexAt(20, 20) != 1 {
		t.Errorf("pond should own its own centre tile, got zone %d", m.ZoneIndexAt(20, 20))
	}
	// The pond should form a compact disc, clearly smaller than the container
	// but not negligible.
	if counts[1] < 50 {
		t.Errorf("nested pond disc too small: %d tiles", counts[1])
	}
	if counts[1] >= counts[0] {
		t.Errorf("nested pond (%d) should be smaller than its container (%d)", counts[1], counts[0])
	}
}

// Well-separated zones must not be treated as nested.
func TestSeparateZonesNotStamped(t *testing.T) {
	doc := mustParse(t, `{
		"map":{"width":40,"length":40,"name":"t","biome":"temperate_forest"},
		"zones":[
			{"id":"a","anchor":{"x":10,"y":20},"relative_size":0.5,"elevation":"flat"},
			{"id":"b","anchor":{"x":30,"y":20},"relative_size":0.5,"elevation":"flat"}
		]}`)
	m := Resolve(doc)
	// Each zone owns its own anchor and roughly half the map (no disc stamped
	// over the other).
	if m.ZoneIndexAt(10, 20) != 0 || m.ZoneIndexAt(30, 20) != 1 {
		t.Error("separated zones should each own their own anchor")
	}
	counts := m.TileCounts()
	if counts[0] < 600 || counts[1] < 600 {
		t.Errorf("expected a roughly even split, got %v", counts)
	}
}

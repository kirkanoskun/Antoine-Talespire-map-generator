package spatial

import "testing"

func TestBuildPathsCarvesSegment(t *testing.T) {
	doc := mustParse(t, `{
		"map":{"width":40,"length":40,"name":"t","biome":"temperate_forest"},
		"zones":[
			{"id":"south","anchor":{"x":20,"y":30},"relative_size":0.5,"elevation":"flat"},
			{"id":"north","anchor":{"x":20,"y":8},"relative_size":0.5,"elevation":"flat"}
		],
		"connections":[{"from":"south","to":"north","type":"path","width":3}]}`)
	p := BuildPaths(doc)

	if !p.Any() {
		t.Fatal("expected a carved path")
	}
	// The straight vertical corridor at x=20 is on the path all the way.
	for _, y := range []int{8, 15, 20, 25, 30} {
		if !p.On(20, y) {
			t.Errorf("tile (20,%d) should be on the path", y)
		}
	}
	// A tile far from the segment is not.
	if p.On(2, 2) {
		t.Error("far tile should not be on the path")
	}
	// Param runs from ~0 at the 'from' anchor to ~1 at the 'to' anchor.
	if p.Param(20, 30) > 0.05 {
		t.Errorf("param at 'from' anchor = %g, want ~0", p.Param(20, 30))
	}
	if p.Param(20, 8) < 0.95 {
		t.Errorf("param at 'to' anchor = %g, want ~1", p.Param(20, 8))
	}
	if p.Conn(20, 20) != 0 {
		t.Errorf("mid-path connection index = %d, want 0", p.Conn(20, 20))
	}
}

func TestBuildPathsNoConnections(t *testing.T) {
	doc := mustParse(t, `{
		"map":{"width":20,"length":20,"name":"t","biome":"desert"},
		"zones":[{"id":"a","anchor":{"x":10,"y":10},"relative_size":1.0}]}`)
	if BuildPaths(doc).Any() {
		t.Error("expected no path tiles without connections")
	}
}

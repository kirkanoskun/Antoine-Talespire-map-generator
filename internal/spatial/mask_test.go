package spatial

import (
	"testing"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
)

func mustParse(t *testing.T, s string) *ir.IR {
	t.Helper()
	doc, err := ir.Parse([]byte(s))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return doc
}

func TestResolveCoversEveryTile(t *testing.T) {
	doc := mustParse(t, `{
		"map":{"width":40,"length":40,"name":"t"},
		"zones":[
			{"id":"a","biome":"temperate_forest","anchor":{"x":10,"y":20},"relative_size":0.5},
			{"id":"b","biome":"desert","anchor":{"x":30,"y":20},"relative_size":0.5}
		]}`)
	m := Resolve(doc)
	total := 0
	for _, c := range m.TileCounts() {
		if c == 0 {
			t.Error("a zone captured no tiles")
		}
		total += c
	}
	if total != 40*40 {
		t.Errorf("tiles not fully partitioned: got %d want %d", total, 40*40)
	}
}

func TestLargerRelativeSizeWinsMoreArea(t *testing.T) {
	// Same anchors distance apart; zone "big" has a much larger relative size
	// and must claim strictly more tiles than "small".
	doc := mustParse(t, `{
		"map":{"width":60,"length":60,"name":"t"},
		"zones":[
			{"id":"big","biome":"temperate_forest","anchor":{"x":20,"y":30},"relative_size":0.8},
			{"id":"small","biome":"desert","anchor":{"x":40,"y":30},"relative_size":0.1}
		]}`)
	m := Resolve(doc)
	counts := m.TileCounts()
	if counts[0] <= counts[1] {
		t.Errorf("expected big zone to win more area, got big=%d small=%d", counts[0], counts[1])
	}
}

func TestBoundaryDetection(t *testing.T) {
	doc := mustParse(t, `{
		"map":{"width":40,"length":40,"name":"t"},
		"zones":[
			{"id":"a","biome":"temperate_forest","anchor":{"x":10,"y":20},"relative_size":0.5},
			{"id":"b","biome":"desert","anchor":{"x":30,"y":20},"relative_size":0.5}
		]}`)
	m := Resolve(doc)
	found := false
	for x := 0; x < m.Width && !found; x++ {
		for y := 0; y < m.Length; y++ {
			if m.IsBoundary(x, y) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("expected at least one boundary tile between two zones")
	}
}

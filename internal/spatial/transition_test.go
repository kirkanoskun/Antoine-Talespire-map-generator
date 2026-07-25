package spatial

import "testing"

const twoZoneStraightBorder = `{
	"map":{"width":40,"length":40,"name":"t","biome":"temperate_forest"},
	"zones":[
		{"id":"west","anchor":{"x":10,"y":20},"relative_size":0.5},
		{"id":"east","anchor":{"x":30,"y":20},"relative_size":0.5}
	]}`

func TestTransitionsSeamAndBand(t *testing.T) {
	doc := mustParse(t, twoZoneStraightBorder)
	m := Resolve(doc)
	tr := m.Transitions(3)

	// Find a seam tile: one whose 4-neighbour is a different zone.
	seamX, seamY, found := -1, -1, false
	for x := 0; x < m.Width && !found; x++ {
		for y := 0; y < m.Length; y++ {
			if m.foreignNeighbour(x, y) >= 0 {
				seamX, seamY, found = x, y, true
				break
			}
		}
	}
	if !found {
		t.Fatal("no seam tile between two zones")
	}

	if got := tr.Dist(seamX, seamY); got != 1 {
		t.Errorf("seam tile distance = %d, want 1", got)
	}
	other, blend := tr.At(seamX, seamY)
	if other != m.foreignNeighbour(seamX, seamY) {
		t.Errorf("seam tile foreign zone = %d, want %d", other, m.foreignNeighbour(seamX, seamY))
	}
	if blend != 0.5 {
		t.Errorf("seam blend = %g, want 0.5", blend)
	}
}

func TestTransitionsBlendFadesWithDistance(t *testing.T) {
	doc := mustParse(t, twoZoneStraightBorder)
	tr := Resolve(doc).Transitions(3)

	// Blend must be strictly decreasing in distance and zero outside the band.
	seen := map[int]float64{}
	for x := 0; x < tr.Width; x++ {
		for y := 0; y < tr.Length; y++ {
			d := tr.Dist(x, y)
			_, b := tr.At(x, y)
			if d == 0 {
				if b != 0 {
					t.Fatalf("out-of-band tile has non-zero blend %g", b)
				}
				continue
			}
			if d > 3 {
				t.Fatalf("distance %d exceeds half-width 3", d)
			}
			if prev, ok := seen[d]; ok && prev != b {
				t.Fatalf("inconsistent blend for distance %d: %g vs %g", d, prev, b)
			}
			seen[d] = b
		}
	}
	if !(seen[1] > seen[2] && seen[2] > seen[3]) {
		t.Errorf("blend should decrease with distance, got %v", seen)
	}
}

func TestTransitionsInteriorHasNoBand(t *testing.T) {
	doc := mustParse(t, twoZoneStraightBorder)
	tr := Resolve(doc).Transitions(3)
	// The far-west column is deep inside the west zone, well beyond a 3-tile band.
	if tr.InBand(0, 20) {
		t.Error("far interior tile should not be in the transition band")
	}
}

func TestSingleZoneHasNoTransitions(t *testing.T) {
	doc := mustParse(t, `{
		"map":{"width":20,"length":20,"name":"t","biome":"desert"},
		"zones":[{"id":"only","anchor":{"x":10,"y":10},"relative_size":1.0}]}`)
	tr := Resolve(doc).Transitions(3)
	for x := 0; x < tr.Width; x++ {
		for y := 0; y < tr.Length; y++ {
			if tr.InBand(x, y) {
				t.Fatalf("single-zone map should have no transition band, but (%d,%d) is in it", x, y)
			}
		}
	}
}

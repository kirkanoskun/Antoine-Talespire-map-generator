package prefab

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/slab"
)

// cellarCode encodes a small building with a cellar: a 5x5 ground floor 400
// units (8 terrain steps) above two cellar pieces. The ground floor is the
// busiest level, so it — not the cellar — is what must sit on the terrain.
func cellarCode(t *testing.T) string {
	t.Helper()
	var floor []slab.Instance
	for x := 0; x < 5; x++ {
		for z := 0; z < 5; z++ {
			floor = append(floor, slab.Instance{X: x * 100, Y: 400, Z: z * 100})
		}
	}
	cellar := []slab.Instance{{X: 100, Y: 0, Z: 100}, {X: 200, Y: 0, Z: 100}}
	code, err := slab.Encode(&slab.Slab{Layouts: []slab.Layout{
		{ID: [16]byte{1}, Instances: floor},
		{ID: [16]byte{2}, Instances: cellar},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func TestGroundLevelIsTheBusiestLevel(t *testing.T) {
	b, err := Validate(Entry{ID: "inn", Name: "Inn", Code: cellarCode(t)})
	if err != nil {
		t.Fatal(err)
	}
	if b.Info.GroundY != 400 {
		t.Errorf("ground_y = %d, want 400 (the ground floor, not the cellar)", b.Info.GroundY)
	}
}

// With one piece per level, nothing marks a floor: keep the lowest piece, the
// behaviour seating had before ground levels existed.
func TestGroundLevelTieKeepsTheLowestPiece(t *testing.T) {
	b, err := Validate(sample(t))
	if err != nil {
		t.Fatal(err)
	}
	if b.Info.GroundY != 0 {
		t.Errorf("ground_y = %d, want 0", b.Info.GroundY)
	}
}

func TestGroundLevelOverride(t *testing.T) {
	zero, inside, above := 0, 200, 401
	for _, c := range []struct {
		ground *int
		want   int
		ok     bool
	}{{&zero, 0, true}, {&inside, 200, true}, {&above, 0, false}} {
		b, err := Validate(Entry{ID: "inn", Name: "Inn", Code: cellarCode(t), GroundY: c.ground})
		if !c.ok {
			if err == nil {
				t.Errorf("ground_y %d accepted outside the building's height", *c.ground)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ground_y %d: %v", *c.ground, err)
		}
		if b.Info.GroundY != c.want {
			t.Errorf("ground_y = %d, want %d", b.Info.GroundY, c.want)
		}
	}
}

func TestProvenanceRules(t *testing.T) {
	for _, c := range []struct {
		license, author string
		ok              bool
	}{
		{"", "someone", false},                 // no licence
		{"CC BY-NC 4.0", "", false},            // attribution licence, no author
		{"CC BY-NC 4.0", "someone", true},      // complete
		{"CC0", "", true},                      // no attribution required
		{"Permission écrite", "someone", true}, // explicit permission, recorded
	} {
		err := CheckProvenance(Entry{ID: "x", License: c.license, Author: c.author})
		if (err == nil) != c.ok {
			t.Errorf("licence %q, author %q: err = %v, want ok=%v", c.license, c.author, err, c.ok)
		}
	}
}

// Built-in entries ship in the repository and the app, so a missing licence
// must stop the catalogue from loading (TestEmbeddedPrefabCatalogue then fails
// in CI). The user's own local imports are not redistributed and stay free.
func TestBuiltinsRequireALicenceButLocalImportsDoNot(t *testing.T) {
	entry := Entry{ID: "inn", Name: "Inn", Code: cellarCode(t), Author: "someone"}
	builtins, _ := json.Marshal([]Entry{entry})
	if _, err := Open("", builtins); err == nil || !strings.Contains(err.Error(), "licence") {
		t.Fatalf("built-in without a licence: err = %v, want a licence error", err)
	}

	entry.License = "CC BY-NC 4.0"
	builtins, _ = json.Marshal([]Entry{entry})
	store, err := Open("", builtins)
	if err != nil {
		t.Fatalf("built-in with a licence: %v", err)
	}
	if b, ok := store.Get("inn"); !ok || b.Info.License != "CC BY-NC 4.0" {
		t.Errorf("licence not carried into Info: %+v", b)
	}

	local := sample(t) // no licence
	if _, err := store.Add(local); err != nil {
		t.Errorf("a local import without a licence must stay allowed: %v", err)
	}
}

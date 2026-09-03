package buildings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/chimera"
)

// sampleCode builds a small but structurally real slab: a wide "ground floor"
// at rawZ 400 plus a couple of pieces below it, so the ground level is the
// busiest one and there is something buried under it.
func sampleCode(t *testing.T) string {
	t.Helper()
	var floor []chimera.Placement
	for x := 0; x < 6; x++ {
		for y := 0; y < 5; y++ {
			floor = append(floor, chimera.Placement{
				RawX: uint32(x * 100), RawY: uint32(y * 100), RawZ: 400,
			})
		}
	}
	cellar := []chimera.Placement{
		{RawX: 100, RawY: 100, RawZ: 0},
		{RawX: 200, RawY: 100, RawZ: 0},
	}
	slab := &chimera.Slab{
		Version:    2,
		MagicBytes: chimera.DefaultMagicBytes,
		Assets: []chimera.Asset{
			{IDBase64: "AAAQosMB+5SfRIxHmT7aPnEm", Placements: floor},
			{IDBase64: "AABKUWVmd8+rTK08RXqSSmjZ", Placements: cellar},
		},
	}
	code, err := chimera.Encode(slab)
	if err != nil {
		t.Fatalf("encoding sample: %v", err)
	}
	return code
}

func TestAddMeasuresAndPersists(t *testing.T) {
	dir := t.TempDir()
	lib, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	e, err := lib.Add(AddOptions{
		Name:    "Smiling Goat Inn",
		Code:    sampleCode(t),
		Tags:    []string{"Tavern", "inn", "tavern"}, // dupes and case are normalised
		Source:  "https://example.test/inn",
		Author:  "someone",
		License: "CC BY-NC",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if e.ID != "smiling-goat-inn" {
		t.Errorf("id = %q, want smiling-goat-inn", e.ID)
	}
	if got := strings.Join(e.Tags, ","); got != "inn,tavern" {
		t.Errorf("tags = %q, want inn,tavern", got)
	}
	// Footprint spans 6x5 tiles; ground level is the busy floor at 400, with
	// 400 units (8 steps) of cellar under it.
	if e.WidthTiles != 5 || e.LengthTiles != 4 {
		t.Errorf("footprint = %dx%d, want 5x4", e.WidthTiles, e.LengthTiles)
	}
	if e.GroundZ != 400 {
		t.Errorf("ground = %d, want 400 (the busiest level)", e.GroundZ)
	}
	if e.BuriedSteps != 8 {
		t.Errorf("buried = %.1f steps, want 8", e.BuriedSteps)
	}
	if e.Placements != 32 {
		t.Errorf("placements = %d, want 32", e.Placements)
	}

	// The slab file and manifest must survive a reopen.
	if _, err := os.Stat(filepath.Join(dir, "smiling-goat-inn.slab")); err != nil {
		t.Errorf("slab file missing: %v", err)
	}
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := again.Get("smiling-goat-inn")
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got.License != "CC BY-NC" || got.Author != "someone" {
		t.Errorf("provenance lost on reload: %+v", got)
	}
	if _, err := again.Slab(got); err != nil {
		t.Errorf("stored slab does not decode: %v", err)
	}
}

func TestAddRejectsDuplicateUnlessReplacing(t *testing.T) {
	dir := t.TempDir()
	lib, _ := Open(dir)
	code := sampleCode(t)
	if _, err := lib.Add(AddOptions{Name: "Inn", Code: code}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Add(AddOptions{Name: "Inn", Code: code}); err == nil {
		t.Error("expected a duplicate id to be refused")
	}
	if _, err := lib.Add(AddOptions{Name: "Inn", Code: code, Replace: true}); err != nil {
		t.Errorf("-replace should overwrite: %v", err)
	}
	if len(lib.Entries) != 1 {
		t.Errorf("replace should not add a second entry, got %d", len(lib.Entries))
	}
}

func TestAddRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	lib, _ := Open(dir)

	if _, err := lib.Add(AddOptions{Name: "Broken", Code: "not-a-slab"}); err == nil {
		t.Error("expected an undecodable slab to be refused")
	}
	if _, err := lib.Add(AddOptions{Code: sampleCode(t)}); err == nil {
		t.Error("expected a missing name/id to be refused")
	}
	if _, err := lib.Add(AddOptions{ID: "Bad Id!", Name: "x", Code: sampleCode(t)}); err == nil {
		t.Error("expected an invalid id to be refused")
	}
	// A ground level outside the slab cannot be seated on terrain.
	bad := uint32(9999)
	if _, err := lib.Add(AddOptions{Name: "x", Code: sampleCode(t), GroundZ: &bad}); err == nil {
		t.Error("expected an out-of-range ground level to be refused")
	}
}

func TestGroundLevelOverride(t *testing.T) {
	dir := t.TempDir()
	lib, _ := Open(dir)
	g := uint32(0)
	e, err := lib.Add(AddOptions{Name: "Cellarless", Code: sampleCode(t), GroundZ: &g})
	if err != nil {
		t.Fatal(err)
	}
	if e.GroundZ != 0 || e.BuriedSteps != 0 {
		t.Errorf("override ignored: ground=%d buried=%.1f", e.GroundZ, e.BuriedSteps)
	}
}

func TestFindAndRemove(t *testing.T) {
	dir := t.TempDir()
	lib, _ := Open(dir)
	code := sampleCode(t)
	_, _ = lib.Add(AddOptions{Name: "Forest Watchtower", Code: code, Tags: []string{"tower"}})
	_, _ = lib.Add(AddOptions{Name: "Smiling Goat Inn", Code: code, Tags: []string{"inn"}})

	if got := lib.Find(""); len(got) != 2 {
		t.Errorf("empty query should return all, got %d", len(got))
	}
	if got := lib.Find("tower"); len(got) != 1 || got[0].ID != "forest-watchtower" {
		t.Errorf("tag search failed: %+v", got)
	}
	if got := lib.Find("goat"); len(got) != 1 {
		t.Errorf("name search failed: %+v", got)
	}
	if got := lib.Find("nothing-like-this"); len(got) != 0 {
		t.Errorf("expected no matches, got %d", len(got))
	}

	if err := lib.Remove("forest-watchtower"); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Get("forest-watchtower"); err == nil {
		t.Error("entry should be gone")
	}
	if _, err := os.Stat(filepath.Join(dir, "forest-watchtower.slab")); !os.IsNotExist(err) {
		t.Error("slab file should be deleted with the entry")
	}
	if err := lib.Remove("not-there"); err == nil {
		t.Error("removing an unknown id should error")
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Smiling Goat Inn":   "smiling-goat-inn",
		"  Tour de Garde  ":  "tour-de-garde",
		"Ruined Keep (v2)":   "ruined-keep-v2",
		"L'Auberge du Chêne": "l-auberge-du-chene",
		"Cœur de Forêt":      "coeur-de-foret",
		"already-a-slug":     "already-a-slug",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

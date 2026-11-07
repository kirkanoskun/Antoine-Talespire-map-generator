package catalogs

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var update = flag.Bool("update", false, "rewrite configs/assets.json from the pack indexes")

func repoFile(t *testing.T, parts ...string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func importRepoIndexes(t *testing.T) []Asset {
	t.Helper()
	assets, err := ImportIndexes(
		repoFile(t, "configs", "taleweaver", "medieval-fantasy.index.json"),
		repoFile(t, "configs", "taleweaver", "scifi.index.json"))
	if err != nil {
		t.Fatal(err)
	}
	return assets
}

// configs/assets.json must be exactly what the importer makes of the indexes in
// the repository: re-run cmd/import-index (or this test with -update) after
// replacing an index.
func TestAssetCatalogueIsCurrent(t *testing.T) {
	want, err := EncodeAssets(importRepoIndexes(t))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("..", "..", "configs", "assets.json")
	if *update {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := repoFile(t, "configs", "assets.json"); !bytes.Equal(got, want) {
		t.Fatal("configs/assets.json is stale: run go run ./cmd/import-index (see its doc comment)")
	}
	again, _ := EncodeAssets(importRepoIndexes(t))
	if !bytes.Equal(again, want) {
		t.Fatal("two imports of the same indexes differ")
	}
}

func TestAssetCatalogueContents(t *testing.T) {
	cat, err := ParseAssets(repoFile(t, "configs", "assets.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Every non-deprecated tile and prop of the two packs (2025 + 850).
	if cat.Len() != 2875 {
		t.Errorf("catalogue has %d assets, want 2875", cat.Len())
	}
	fire, ok := cat.ByID("AAA0wD/Pk8fsSpNFmDCNyR1L")
	if !ok || fire.UUID != "cf3fc034-c793-4aec-9345-98308dc91d4b" || fire.Name != "Bonfire" || fire.Folder != "Lights" {
		t.Errorf("props.json's fire should be the index's Bonfire, got %+v", fire)
	}
	fountain, ok := cat.BySlug("fountain_01")
	if !ok || fountain.Footprint != [2]int{2, 2} || fountain.Kind != "prop" {
		t.Errorf("fountain_01 = %+v, want a 2x2 prop", fountain)
	}
	if wagon, _ := cat.BySlug("wagon_01"); wagon == nil || wagon.Footprint != [2]int{2, 5} {
		t.Errorf("wagon_01 footprint = %v, want 2x5", wagon)
	}
	if stall, _ := cat.BySlug("stall_fruit_01"); stall == nil || stall.Kind != "tile" || stall.Center != [3]float64{0.5, 0.5, 0.5} {
		t.Errorf("stall_fruit_01 = %+v, want a corner-origin tile", stall)
	}
	id, err := fountain.LayoutID()
	if err != nil || id[0] != 0x29 || id[1] != 0x54 {
		t.Errorf("fountain layout id = % x, want the UUID in .NET order (29 54 45 08 …)", id)
	}
}

// Every asset the old catalogue uses must still exist: the full catalogue is a
// superset, never a replacement.
func TestOldCatalogueIsInTheFullOne(t *testing.T) {
	cat, err := ParseAssets(repoFile(t, "configs", "assets.json"))
	if err != nil {
		t.Fatal(err)
	}
	ids := regexp.MustCompile(`"([A-Za-z0-9+/]{22}[A-Za-z0-9+/=]{2})"`)
	seen := map[string]bool{}
	for _, file := range []string{"props.json", "biomes.json"} {
		for _, m := range ids.FindAllSubmatch(repoFile(t, "configs", file), -1) {
			seen[string(m[1])] = true
		}
	}
	if len(seen) < 59 {
		t.Fatalf("found %d asset ids in props.json and biomes.json, expected at least 59", len(seen))
	}
	for id := range seen {
		if _, ok := cat.ByID(id); !ok {
			t.Errorf("asset %s from the old catalogue is missing from assets.json", id)
		}
	}
}

func TestSlugs(t *testing.T) {
	for in, want := range map[string]string{
		"Fountain_01":              "fountain_01",
		"Doors - Graveyard fence":  "doors_graveyard_fence",
		"  stall  Fruit--01 ":      "stall_fruit_01",
		"haunted wall 1x1 broken!": "haunted_wall_1x1_broken",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	assets := []Asset{
		{UUID: "aaaa1111", Name: "Rock", Folder: "Nature"},
		{UUID: "bbbb2222", Name: "Rock", Folder: "Dungeon"},
		{UUID: "cccc3333", Name: "Rock", Folder: "Dungeon"},
		{UUID: "dddd4444-0000", Name: ""},
	}
	if err := assignSlugs(assets); err != nil {
		t.Fatal(err)
	}
	got := []string{assets[0].Slug, assets[1].Slug, assets[2].Slug, assets[3].Slug}
	want := []string{"rock_nature", "rock_dungeon_bbbb", "rock_dungeon_cccc", "asset_dddd4444"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("slugs = %v, want %v", got, want)
			break
		}
	}
}

func TestThemesResolve(t *testing.T) {
	cat, err := ParseAssets(repoFile(t, "configs", "assets.json"))
	if err != nil {
		t.Fatal(err)
	}
	themes, err := ParseThemes(repoFile(t, "configs", "themes.json"), cat)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int{}
	for _, th := range themes {
		ids[th.ID] = len(th.Objects)
		for _, o := range th.Objects {
			if o.Role == "" {
				t.Errorf("theme %s: %s has no role", th.ID, o.Asset.Slug)
			}
		}
	}
	for _, id := range []string{"cimetiere", "place_village", "marais", "village_abandonne"} {
		if ids[id] == 0 {
			t.Errorf("theme %s is missing or empty", id)
		}
	}
	if _, err := ParseThemes([]byte(`[{"id":"x","label":"x","objects":[{"uuid":"00000000-0000-0000-0000-000000000000","role":"r"}]}]`), cat); err == nil {
		t.Error("an unknown UUID in a theme must be an error")
	}
}

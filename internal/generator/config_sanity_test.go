package generator

import (
	"encoding/json"
	"os"
	"testing"
)

// These are non-regression "sanity" checks on the hand-maintained catalogues
// (configs/biomes.json, configs/props.json). They guard against the classes of
// mistake we actually hit: props referenced by a typo, a prop pool quietly
// shrinking to near-nothing (temperate_forest/ground was once 3 pines), and a
// misc pool that is almost entirely one theme (the ruins relief was once 100%
// bones).

type propCatalog []struct {
	ID    string `json:"id"`
	Parts []struct {
		ID string `json:"id"`
	} `json:"asset_parts"`
}

type biomeCatalog []struct {
	BiomeType string `json:"biome_type"`
	Reliefs   []struct {
		Key            string   `json:"key"`
		BuildingBlocks []string `json:"building_blocks"`
		PropBlocks     map[string]struct {
			Weight float64  `json:"weight"`
			Props  []string `json:"props"`
		} `json:"prop_blocks"`
	} `json:"reliefs"`
}

func loadJSON(t *testing.T, name string, v any) {
	t.Helper()
	b, err := os.ReadFile(configPath(name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
}

func propIDs(t *testing.T) map[string]bool {
	t.Helper()
	var props propCatalog
	loadJSON(t, "props.json", &props)
	ids := make(map[string]bool, len(props))
	for _, p := range props {
		ids[p.ID] = true
	}
	return ids
}

// TestConfigReferentialIntegrity: every prop / building block referenced by a
// biome exists in props.json, biome types are unique, and no prop_block is empty
// or has a duplicate. Catches typos and the accidental duplicate biome entry.
func TestConfigReferentialIntegrity(t *testing.T) {
	ids := propIDs(t)
	var biomes biomeCatalog
	loadJSON(t, "biomes.json", &biomes)

	seenBiome := map[string]bool{}
	for _, b := range biomes {
		if seenBiome[b.BiomeType] {
			t.Errorf("duplicate biome_type %q", b.BiomeType)
		}
		seenBiome[b.BiomeType] = true

		for _, r := range b.Reliefs {
			for _, bb := range r.BuildingBlocks {
				if !ids[bb] {
					t.Errorf("%s/%s: building_block %q not found in props.json", b.BiomeType, r.Key, bb)
				}
			}
			for cat, pb := range r.PropBlocks {
				if len(pb.Props) == 0 {
					t.Errorf("%s/%s/%s: empty prop pool", b.BiomeType, r.Key, cat)
				}
				seen := map[string]bool{}
				for _, p := range pb.Props {
					if !ids[p] {
						t.Errorf("%s/%s/%s: prop %q not found in props.json", b.BiomeType, r.Key, cat, p)
					}
					if seen[p] {
						t.Errorf("%s/%s/%s: duplicate prop %q", b.BiomeType, r.Key, cat, p)
					}
					seen[p] = true
				}
			}
		}
	}
}

const (
	// A prop category that is a *primary* feature of a relief (drawn often) must
	// offer real variety. "Primary" = weight at or above richWeightFloor.
	richWeightFloor = 0.15
	richMinProps    = 4
)

// grandfatheredThinPools are (biome/relief/category) primary pools that are
// currently below richMinProps because the asset catalogue has few coherent
// options for that biome. New narrow pools are NOT allowed — this list is the
// documented set of accepted exceptions, each a candidate for future enrichment.
var grandfatheredThinPools = map[string]bool{
	"beach/ground/vegetation":                   true, // coconut + pine only
	"swamp/base_ground/vegetation":              true, // a single swamp tree exists
	"swamp/mountain/vegetation":                 true,
	"subtropical_forest/base_ground/vegetation": true, // 3 large trees
	"subtropical_forest/mountain/vegetation":    true,
}

// TestConfigPrimaryPoolsAreVaried fails if a heavily-weighted vegetation or misc
// pool has fewer than richMinProps distinct props (unless grandfathered). This
// is what stops a pool from shrinking back to something as narrow as the old
// temperate_forest/ground "3 pines".
func TestConfigPrimaryPoolsAreVaried(t *testing.T) {
	var biomes biomeCatalog
	loadJSON(t, "biomes.json", &biomes)

	for _, b := range biomes {
		for _, r := range b.Reliefs {
			for _, cat := range []string{"vegetation", "misc"} {
				pb, ok := r.PropBlocks[cat]
				if !ok || pb.Weight < richWeightFloor {
					continue
				}
				key := b.BiomeType + "/" + r.Key + "/" + cat
				if grandfatheredThinPools[key] {
					continue
				}
				if n := len(pb.Props); n < richMinProps {
					t.Errorf("%s: primary %s pool (weight %.2f) has only %d distinct props, want >= %d — pool too narrow",
						key, cat, pb.Weight, n, richMinProps)
				}
			}
		}
	}
}

// boneProps are the funerary/skeleton props. A relief may use them as an accent,
// but a misc pool should not be almost entirely bones.
var boneProps = map[string]bool{
	"pile_of_skulls": true, "down_skeleton": true, "up_skeleton": true,
	"down_rib": true, "up_rib": true, "bull_skull": true,
}

const (
	// Only police misc pools that are actually visible (drawn with a
	// non-trivial weight); a 0.01-weight pool is placed too rarely to matter.
	boneWeightFloor = 0.08
	maxBoneFraction = 0.5
)

// TestConfigMiscNotBoneDominated fails if a visible misc pool is majority bones,
// as the ruins relief once was (weight 0.12, 3/3 skeletons). Keeps bones an
// occasional accent rather than the systematic decor.
func TestConfigMiscNotBoneDominated(t *testing.T) {
	var biomes biomeCatalog
	loadJSON(t, "biomes.json", &biomes)

	for _, b := range biomes {
		for _, r := range b.Reliefs {
			pb, ok := r.PropBlocks["misc"]
			if !ok || len(pb.Props) == 0 || pb.Weight < boneWeightFloor {
				continue
			}
			bones := 0
			for _, p := range pb.Props {
				if boneProps[p] {
					bones++
				}
			}
			if frac := float64(bones) / float64(len(pb.Props)); frac > maxBoneFraction {
				t.Errorf("%s/%s: misc pool is %.0f%% bones (%d/%d) at weight %.2f — over %.0f%%; add non-funerary decor",
					b.BiomeType, r.Key, frac*100, bones, len(pb.Props), pb.Weight, maxBoneFraction*100)
			}
		}
	}
}

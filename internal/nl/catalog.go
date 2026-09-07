// Package nl is the natural-language layer (Phase 2 of the brief): it turns a
// free-text scene description into a validated IR document via a Claude API
// call, with strict schema validation and retry.
//
// The package is deliberately split so the retry/validation logic can be tested
// without hitting the network: an Interpreter drives a Completer (an interface),
// and only AnthropicCompleter actually calls the model.
package nl

import (
	"fmt"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/prefab"
	"sort"

	"github.com/johnfercher/taleslab/pkg/taleslab/taleslabrepositories"
)

// Catalog is the set of engine facts the model must respect: the biomes it may
// choose from, the relief keys valid for each (for relief_override), and the
// friendly point-of-interest names that resolve to real props. It is injected
// into the system prompt and used to validate the model's output before the
// generator ever sees it.
type Catalog struct {
	Prefabs *prefab.Store
	// Biomes maps a biome id to its sorted relief keys.
	Biomes map[string][]string
	// POINames is the sorted list of understood point-of-interest names.
	POINames []string
}

// LoadCatalog reads the biome catalogue and records each biome's relief keys.
// poiNames is the friendly POI vocabulary (see generator.POIAliasNames).
func LoadCatalog(biomesPath string, poiNames []string) (*Catalog, error) {
	repo, err := taleslabrepositories.NewBiomeRepository(biomesPath)
	if err != nil {
		return nil, fmt.Errorf("loading biomes: %w", err)
	}
	biomes := map[string][]string{}
	for biomeType, biome := range repo.GetBiomes() {
		reliefs := make([]string, 0, len(biome.Reliefs))
		for key := range biome.Reliefs {
			reliefs = append(reliefs, string(key))
		}
		sort.Strings(reliefs)
		biomes[string(biomeType)] = reliefs
	}
	names := append([]string(nil), poiNames...)
	sort.Strings(names)
	return &Catalog{Biomes: biomes, POINames: names}, nil
}

// biomeNames returns the sorted biome ids.
func (c *Catalog) biomeNames() []string {
	names := make([]string, 0, len(c.Biomes))
	for b := range c.Biomes {
		names = append(names, b)
	}
	sort.Strings(names)
	return names
}

// reliefKnown reports whether relief is a valid relief of biome.
func (c *Catalog) reliefKnown(biome, relief string) bool {
	for _, r := range c.Biomes[biome] {
		if r == relief {
			return true
		}
	}
	return false
}

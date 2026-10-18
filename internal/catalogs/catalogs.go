// Package catalogs parses the biome and prop catalogues from bytes.
//
// taleslab's JSON repositories only read from a file path, and the browser build
// (cmd/web) has no filesystem. These parsers build the same repository values
// from in-memory JSON, mirroring taleslab's loaders exactly — the parity test
// holds them to that — so the desktop app and the website generate identical
// maps from identical catalogues.
package catalogs

import (
	"encoding/json"
	"fmt"

	"github.com/johnfercher/taleslab/pkg/taleslab/taleslabdomain/taleslabconsts/biometype"
	"github.com/johnfercher/taleslab/pkg/taleslab/taleslabdomain/taleslabconsts/elementtype"
	"github.com/johnfercher/taleslab/pkg/taleslab/taleslabdomain/taleslabentities"
	domainrepos "github.com/johnfercher/taleslab/pkg/taleslab/taleslabdomain/taleslabrepositories"
	"github.com/johnfercher/taleslab/pkg/taleslab/taleslabrepositories"
)

// ParseBiomes builds a biome repository from the contents of biomes.json.
func ParseBiomes(data []byte) (domainrepos.BiomeRepository, error) {
	var daos []*taleslabrepositories.BiomeDao
	if err := json.Unmarshal(data, &daos); err != nil {
		return nil, fmt.Errorf("parsing biomes: %w", err)
	}
	biomes := make(map[biometype.BiomeType]*taleslabentities.Biome, len(daos))
	for _, dao := range daos {
		reliefs := make(map[elementtype.ElementType]*taleslabentities.Relief, len(dao.Reliefs))
		for _, relief := range dao.Reliefs {
			reliefs[elementtype.ElementType(relief.Key)] = relief
		}
		biomes[dao.Type] = &taleslabentities.Biome{
			Type:      dao.Type,
			Reliefs:   reliefs,
			StoneWall: dao.StoneWall,
		}
	}
	return biomeRepo(biomes), nil
}

// ParseProps builds a prop repository from the contents of props.json.
func ParseProps(data []byte) (domainrepos.PropRepository, error) {
	var list []*taleslabentities.Prop
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parsing props: %w", err)
	}
	props := make(map[string]*taleslabentities.Prop, len(list))
	for _, prop := range list {
		props[prop.ID] = prop
	}
	return propRepo(props), nil
}

type biomeRepo map[biometype.BiomeType]*taleslabentities.Biome

func (r biomeRepo) GetBiome(t biometype.BiomeType) *taleslabentities.Biome { return r[t] }
func (r biomeRepo) GetBiomes() map[biometype.BiomeType]*taleslabentities.Biome {
	return r
}

type propRepo map[string]*taleslabentities.Prop

func (r propRepo) GetProp(id string) *taleslabentities.Prop    { return r[id] }
func (r propRepo) GetProps() map[string]*taleslabentities.Prop { return r }

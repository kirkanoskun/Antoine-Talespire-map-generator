package generator

import (
	"github.com/johnfercher/talescoder/pkg/models"
	"github.com/johnfercher/taleslab/pkg/taleslab/taleslabdomain/taleslabconsts"
	"github.com/johnfercher/taleslab/pkg/taleslab/taleslabdomain/taleslabentities"
)

// taleSpireSlabFromSlab converts an internal slab to the talescoder model.
//
// It mirrors taleslab's taleslabmappers.TaleSpireSlabFromSlab but groups assets
// in deterministic *first-seen* order instead of Go map iteration order. That
// makes the exported base64 reproducible for a given seed, which matters for
// the conversational iteration loop and any output caching (brief sections 5.8
// and 8). Since our slab.Assets slice is already built deterministically, the
// resulting code is stable.
func taleSpireSlabFromSlab(slab *taleslabentities.Slab) *models.Slab {
	order := make([]string, 0)
	layouts := make(map[string][]*models.Layout)

	for _, asset := range slab.Assets {
		key := string(asset.ID)
		if _, seen := layouts[key]; !seen {
			order = append(order, key)
		}
		layouts[key] = append(layouts[key], &models.Layout{
			Coordinates: &models.Vector3d{
				X: uint16(asset.Coordinates.X),
				Y: uint16(asset.Coordinates.Y),
				Z: uint16(asset.Coordinates.Z),
			},
			Rotation: uint16(asset.Rotation),
		})
	}

	out := &models.Slab{
		MagicBytes:  taleslabconsts.MagicBytes,
		Version:     taleslabconsts.SlabVersion,
		AssetsCount: int16(len(order)),
	}
	for _, key := range order {
		out.Assets = append(out.Assets, &models.Asset{
			Id:           []byte(key),
			LayoutsCount: int16(len(layouts[key])),
			Layouts:      layouts[key],
		})
	}
	return out
}

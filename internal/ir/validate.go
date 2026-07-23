package ir

import (
	"fmt"

	"github.com/johnfercher/taleslab/pkg/taleslab/taleslabdomain/taleslabconsts/biometype"
)

// KnownBiomes is the set of biome identifiers the generation engine supports.
// It mirrors taleslab's biometype constants so the IR can be validated without
// loading the biome config file.
var KnownBiomes = map[string]bool{
	string(biometype.SubTropicalForest): true,
	string(biometype.TemperateForest):   true,
	string(biometype.DeadForest):        true,
	string(biometype.Swamp):             true,
	string(biometype.Lava):              true,
	string(biometype.Desert):            true,
	string(biometype.Tundra):            true,
	string(biometype.Beach):             true,
}

var knownElevations = map[Elevation]bool{
	ElevationFlat:       true,
	ElevationHill:       true,
	ElevationDepression: true,
	ElevationMountain:   true,
}

// Maximum map dimension. TaleSpire slabs get unwieldy well before this, and the
// coordinate encoding is uint16, so we keep a conservative bound. Revisit
// against TaleSpire's documented limits (brief section 8).
const MaxDimension = 200

// Validate checks structural and semantic invariants. It is intentionally
// strict: a malformed IR should fail loudly here rather than produce a broken
// map or panic deep in generation (brief section 8, "validation de schéma
// stricte côté serveur").
func (doc *IR) Validate() error {
	if doc.Map.Width <= 0 || doc.Map.Length <= 0 {
		return fmt.Errorf("map dimensions must be positive, got %dx%d", doc.Map.Width, doc.Map.Length)
	}
	if doc.Map.Width > MaxDimension || doc.Map.Length > MaxDimension {
		return fmt.Errorf("map dimensions %dx%d exceed maximum %d", doc.Map.Width, doc.Map.Length, MaxDimension)
	}
	if len(doc.Zones) == 0 {
		return fmt.Errorf("at least one zone is required")
	}

	ids := make(map[string]bool, len(doc.Zones))
	for i := range doc.Zones {
		z := &doc.Zones[i]
		where := fmt.Sprintf("zone[%d]", i)
		if z.ID == "" {
			return fmt.Errorf("%s: id is required", where)
		}
		where = fmt.Sprintf("zone %q", z.ID)
		if ids[z.ID] {
			return fmt.Errorf("%s: duplicate zone id", where)
		}
		ids[z.ID] = true

		if !KnownBiomes[z.Biome] {
			return fmt.Errorf("%s: unknown biome %q", where, z.Biome)
		}
		if z.RelativeSize <= 0 || z.RelativeSize > 1 {
			return fmt.Errorf("%s: relative_size must be in (0,1], got %g", where, z.RelativeSize)
		}
		if z.Elevation == "" {
			z.Elevation = ElevationFlat
		}
		if !knownElevations[z.Elevation] {
			return fmt.Errorf("%s: unknown elevation %q", where, z.Elevation)
		}
		if !doc.inBounds(z.Anchor.X, z.Anchor.Y) {
			return fmt.Errorf("%s: anchor (%d,%d) is outside map bounds %dx%d",
				where, z.Anchor.X, z.Anchor.Y, doc.Map.Width, doc.Map.Length)
		}
		for k, v := range z.DensityOverrides {
			if k != "vegetation" && k != "stones" && k != "misc" {
				return fmt.Errorf("%s: unknown density override category %q", where, k)
			}
			if v < 0 || v > 1 {
				return fmt.Errorf("%s: density override %q must be in [0,1], got %g", where, k, v)
			}
		}
		for j := range z.PointsOfInterest {
			poi := &z.PointsOfInterest[j]
			if poi.Prop == "" {
				return fmt.Errorf("%s: point_of_interest[%d] missing prop", where, j)
			}
			if !poi.Position.Scattered && !doc.inBounds(poi.Position.X, poi.Position.Y) {
				return fmt.Errorf("%s: point_of_interest[%d] position (%d,%d) out of bounds",
					where, j, poi.Position.X, poi.Position.Y)
			}
			if poi.Position.Scattered && poi.Count <= 0 {
				return fmt.Errorf("%s: point_of_interest[%d] scattered placement needs count > 0", where, j)
			}
		}
	}

	for i := range doc.Connections {
		c := &doc.Connections[i]
		if !ids[c.From] {
			return fmt.Errorf("connection[%d]: unknown 'from' zone %q", i, c.From)
		}
		if !ids[c.To] {
			return fmt.Errorf("connection[%d]: unknown 'to' zone %q", i, c.To)
		}
	}
	return nil
}

func (doc *IR) inBounds(x, y int) bool {
	return x >= 0 && x < doc.Map.Width && y >= 0 && y < doc.Map.Length
}

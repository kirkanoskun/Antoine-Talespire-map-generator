package generator

import (
	"fmt"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/prefab"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/slab"
)

// SetPrefabs wires the shared catalogue before generation starts.
func (g *Generator) SetPrefabs(store *prefab.Store) { g.prefabs = store }

// BuildingAt identifies the reserved prefab footprint for scatter and preview.
func (f *HeightField) BuildingAt(x, y int) bool {
	return f.buildings != nil && f.buildings[x][y]
}

func (g *Generator) placeBuildings(doc *ir.IR, f *HeightField) ([]placement, error) {
	if len(doc.Buildings) == 0 {
		return nil, nil
	}
	f.buildings = make([][]bool, f.Width)
	for x := range f.buildings {
		f.buildings[x] = make([]bool, f.Length)
	}
	var out []placement
	total := 0
	for _, placementIR := range doc.Buildings {
		b, ok := g.prefabs.Get(placementIR.Prefab)
		if !ok {
			return nil, fmt.Errorf("unknown building prefab %q; import its Slab first", placementIR.Prefab)
		}
		if placementIR.Position == nil {
			return nil, fmt.Errorf("building position is required")
		}
		x0, y0 := placementIR.Position.X, placementIR.Position.Y
		w, l := b.Info.Width, b.Info.Length
		if placementIR.Rotation == 90 || placementIR.Rotation == 270 {
			w, l = l, w
		}
		if x0 < 0 || y0 < 0 || x0+w > f.Width || y0+l > f.Length {
			return nil, fmt.Errorf("building %q needs %dx%d tiles at (%d,%d), outside the map", b.Info.ID, w, l, x0, y0)
		}
		height := f.HeightAt(x0, y0)
		for x := x0; x < x0+w; x++ {
			for y := y0; y < y0+l; y++ {
				if f.buildings[x][y] {
					return nil, fmt.Errorf("building %q overlaps another reserved footprint", b.Info.ID)
				}
				if f.IsWaterAt(x, y) {
					return nil, fmt.Errorf("building %q overlaps water; choose a dry area", b.Info.ID)
				}
				height = max(height, f.HeightAt(x, y))
			}
		}
		for x := x0; x < x0+w; x++ {
			for y := y0; y < y0+l; y++ {
				f.buildings[x][y] = true
				f.tiles[x][y].height = height
				f.tiles[x][y].relief = pathRelief
			}
		}
		total += b.Info.AssetCount
		if total > slab.MaxAssets/2 {
			return nil, fmt.Errorf("buildings exceed the map's 50000 imported asset budget")
		}
		// Reserve one tile before the minimum asset origin. Bounding boxes describe
		// origins only, not mesh extents. Users may enlarge the footprint at import.
		// Terrain blocks are 0.5 game units tall in the current generator.
		for _, layout := range b.Slab.Layouts {
			for _, p := range layout.Instances {
				switch placementIR.Rotation {
				case 0:
				case 90:
					p.X, p.Z = p.Z, b.SpanX-p.X
				case 180:
					p.X, p.Z = b.SpanX-p.X, b.SpanZ-p.Z
				case 270:
					p.X, p.Z = b.SpanZ-p.Z, p.X
				default:
					return nil, fmt.Errorf("invalid building rotation")
				}
				p.Rotation = (p.Rotation + uint8(placementIR.Rotation/15)) % 24
				p.X += (x0 + 1) * 100
				p.Z += (y0 + 1) * 100
				p.Y += (height + 1) * 50
				if p.X > slab.MaxCoordinate || p.Y > slab.MaxCoordinate || p.Z > slab.MaxCoordinate {
					return nil, fmt.Errorf("building %q exceeds Slab coordinate limits", b.Info.ID)
				}
				one := &slab.Layout{ID: layout.ID, Reserved: layout.Reserved, Instances: []slab.Instance{p}}
				// Bucket the entire prefab by its anchor so slicing never cuts a building
				// apart. A building can extend beyond that slice's nominal tile rectangle.
				out = append(out, placement{tileX: x0, tileY: y0, imported: one})
			}
		}
	}
	return out, nil
}

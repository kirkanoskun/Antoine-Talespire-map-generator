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

// stepUnits is the height of one terrain step in Slab units (hundredths of a
// game unit): a terrain block at height h spans [h*50, (h+1)*50).
const stepUnits = 50

// placedBuilding is a building whose footprint has been reserved and levelled.
type placedBuilding struct {
	b        *prefab.Building
	x0, y0   int
	rotation int
	height   int // levelled terrain height under the footprint
}

// placeBuildings seats every building by its ground level (Info.GroundY), not
// by its lowest piece: builds often carry a cellar or footings meant to end up
// underground, and anchoring their bottom would lift the whole build into the
// air by exactly that depth. TaleSpire has no negative heights, so when a
// buried part would go below zero the whole scene is raised to make room, and
// the map's own ground is dug out on the tiles where a build goes underground,
// so a cellar is not filled with terrain.
func (g *Generator) placeBuildings(doc *ir.IR, f *HeightField) ([]placement, error) {
	if len(doc.Buildings) == 0 {
		return nil, nil
	}
	f.buildings = make([][]bool, f.Width)
	f.dug = make([][]bool, f.Width)
	for x := range f.buildings {
		f.buildings[x] = make([]bool, f.Length)
		f.dug[x] = make([]bool, f.Length)
	}

	// Pass 1: reserve and level each footprint, and find how far the scene must
	// rise so that no building's buried part goes below zero.
	var planned []placedBuilding
	total, lift := 0, 0
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
		// The ground level sits on the top of the levelled terrain; whatever is
		// below it needs that much room above zero.
		if need := b.Info.GroundY - surface(height); need > lift {
			lift = need
		}
		planned = append(planned, placedBuilding{b: b, x0: x0, y0: y0, rotation: placementIR.Rotation, height: height})
	}

	// Raise the whole scene (terrain, scatter, POIs, every building) by whole
	// terrain steps, so the deepest cellar still lands at or above zero.
	liftSteps := (lift + stepUnits - 1) / stepUnits
	if liftSteps > 0 {
		for x := range f.tiles {
			for y := range f.tiles[x] {
				f.tiles[x][y].height += liftSteps
			}
		}
	}

	// Pass 2: emit the pieces, seating each building's ground level on its
	// footprint's surface.
	var out []placement
	for _, pb := range planned {
		b := pb.b
		shift := surface(pb.height+liftSteps) - b.Info.GroundY
		// Reserve one tile before the minimum asset origin. Bounding boxes describe
		// origins only, not mesh extents. Users may enlarge the footprint at import.
		for _, layout := range b.Slab.Layouts {
			for _, p := range layout.Instances {
				switch pb.rotation {
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
				p.Rotation = (p.Rotation + uint8(pb.rotation/15)) % 24
				p.X += (pb.x0 + 1) * 100
				p.Z += (pb.y0 + 1) * 100
				buried := p.Y < b.Info.GroundY
				p.Y += shift
				if p.Y < 0 || p.X > slab.MaxCoordinate || p.Y > slab.MaxCoordinate || p.Z > slab.MaxCoordinate {
					return nil, fmt.Errorf("building %q exceeds Slab coordinate limits", b.Info.ID)
				}
				if buried {
					// Dig the map's own ground out of the tile this piece sits in,
					// within the reserved footprint only.
					if tx, ty := p.X/100, p.Z/100; tx < f.Width && ty < f.Length && f.buildings[tx][ty] {
						f.dug[tx][ty] = true
					}
				}
				one := &slab.Layout{ID: layout.ID, Reserved: layout.Reserved, Instances: []slab.Instance{p}}
				// Bucket the entire prefab by its anchor so slicing never cuts a building
				// apart. A building can extend beyond that slice's nominal tile rectangle.
				out = append(out, placement{tileX: pb.x0, tileY: pb.y0, imported: one})
			}
		}
	}
	return out, nil
}

// surface is the Slab height of the top of a terrain column of the given
// height: where a building's ground floor rests.
func surface(height int) int { return (height + 1) * stepUnits }

// dugAt reports whether the map's own ground was removed at (x,y) to make room
// for a building's underground part.
func (f *HeightField) dugAt(x, y int) bool {
	return f.dug != nil && f.dug[x][y]
}

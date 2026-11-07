package generator

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/johnfercher/taleslab/pkg/taleslab/taleslabdomain/taleslabentities"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/catalogs"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/slab"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

// Assets returns the embedded full catalogue (configs/assets.json).
func Assets() (*catalogs.AssetCatalog, error) {
	assets, _, err := catalogs.Embedded()
	return assets, err
}

// resolvePOI finds what a point of interest names, in order: a props.json id or
// an alias (the original small catalogue, encoded by talescoder as before), then
// a slug of the full catalogue (placed exactly, see placeObjects).
func (g *Generator) resolvePOI(name string) (*taleslabentities.Prop, *catalogs.Asset) {
	if prop := g.props.GetProp(resolvePropAlias(name)); prop != nil {
		return prop, nil
	}
	if g.assets != nil {
		if a, ok := g.assets.BySlug(name); ok {
			return nil, a
		}
	}
	return nil, nil
}

// KnownProp reports whether a point-of-interest name resolves to something the
// generator can place, so the NL layer can send an unknown name back to the
// model instead of letting it be skipped.
func (g *Generator) KnownProp(name string) bool {
	prop, asset := g.resolvePOI(name)
	return prop != nil || asset != nil
}

// PlacedObject is a full-catalogue object on the map: its reserved footprint,
// for the preview.
type PlacedObject struct {
	X, Y, W, D int
	Slug       string
}

// Objects lists the catalogue objects placed on the map.
func (f *HeightField) Objects() []PlacedObject { return f.objects }

// ObjectAt reports whether (x,y) lies in a catalogue object's footprint.
func (f *HeightField) ObjectAt(x, y int) bool {
	return f.objectTiles != nil && f.objectTiles[x][y]
}

// objectSeedSalt derives the random source that scatters catalogue objects from
// the map's seed, so placing them never shifts the main sequence: the rest of
// the map comes out the same with or without them.
const objectSeedSalt = 0x0b1ec7

// placeObjects places the points of interest that name full-catalogue assets,
// treating each as a one-piece building: it reserves the footprint (no scatter,
// no other object on it), levels it to its highest tile, and seats the object's
// origin on that surface, unrotated, through the exact Slab encoding. It runs
// after the buildings and before the scatter.
//
// An object that would leave the map or overlap a building or another object is
// skipped with a warning, as the original points of interest are.
func (g *Generator) placeObjects(doc *ir.IR, mask *spatial.Mask, f *HeightField, seed int64, res *Result) ([]placement, error) {
	var out []placement
	rng := rand.New(rand.NewSource(seed ^ objectSeedSalt))
	for zi := range doc.Zones {
		zone := &doc.Zones[zi]
		for _, poi := range zone.PointsOfInterest {
			prop, a := g.resolvePOI(poi.Prop)
			if prop != nil || a == nil {
				continue // the original catalogue, or unknown: placePOIs reports it
			}
			w, d := a.Footprint[0], a.Footprint[1]
			if !poi.Position.Scattered {
				x, y := poi.Position.X, poi.Position.Y
				if reason := f.objectBlocked(x, y, w, d); reason != "" {
					res.Warnings = append(res.Warnings, fmt.Sprintf("zone %q: POI %q %s (skipped)", zone.ID, poi.Prop, reason))
					continue
				}
				p, err := f.seatObject(a, x, y)
				if err != nil {
					return nil, err
				}
				out = append(out, p)
				continue
			}
			spots := objectSpots(mask, zi, w, d)
			rng.Shuffle(len(spots), func(i, j int) { spots[i], spots[j] = spots[j], spots[i] })
			placed := 0
			for _, s := range spots {
				if placed == poi.Count {
					break
				}
				if f.objectBlocked(s[0], s[1], w, d) != "" {
					continue
				}
				p, err := f.seatObject(a, s[0], s[1])
				if err != nil {
					return nil, err
				}
				out = append(out, p)
				placed++
			}
			if placed < poi.Count {
				res.Warnings = append(res.Warnings, fmt.Sprintf("zone %q: only %d of %d %q fit (skipped the rest)", zone.ID, placed, poi.Count, poi.Prop))
			}
		}
	}
	return out, nil
}

// objectBlocked explains why a w x d footprint at (x,y) cannot be used, or
// returns "".
func (f *HeightField) objectBlocked(x, y, w, d int) string {
	if x < 0 || y < 0 || x+w > f.Width || y+d > f.Length {
		return fmt.Sprintf("needs %dx%d tiles at (%d,%d), outside the map", w, d, x, y)
	}
	for i := x; i < x+w; i++ {
		for j := y; j < y+d; j++ {
			if f.BuildingAt(i, j) {
				return "overlaps a reserved building footprint"
			}
			if f.ObjectAt(i, j) {
				return "overlaps another object"
			}
		}
	}
	return ""
}

// objectSpots lists, in a fixed order, the north-west corners where a w x d
// footprint lies entirely inside zone zi.
func objectSpots(mask *spatial.Mask, zi, w, d int) [][2]int {
	var spots [][2]int
	for x := 0; x+w <= mask.Width; x++ {
		for y := 0; y+d <= mask.Length; y++ {
			inside := true
			for i := x; i < x+w && inside; i++ {
				for j := y; j < y+d; j++ {
					if mask.ZoneIndexAt(i, j) != zi {
						inside = false
						break
					}
				}
			}
			if inside {
				spots = append(spots, [2]int{x, y})
			}
		}
	}
	return spots
}

// seatObject reserves and levels the footprint at (x,y) and returns the
// object's exact placement. Horizontally, the centre of its bounds goes to the
// centre of the footprint, whatever its origin (tiles have theirs at a corner,
// props at their centre, a few elsewhere). Vertically, its origin rests on the
// levelled surface: some objects reach below their origin on purpose (a
// gravestone is half buried).
func (f *HeightField) seatObject(a *catalogs.Asset, x, y int) (placement, error) {
	w, d := a.Footprint[0], a.Footprint[1]
	if f.objectTiles == nil {
		f.objectTiles = make([][]bool, f.Width)
		for i := range f.objectTiles {
			f.objectTiles[i] = make([]bool, f.Length)
		}
	}
	height := f.HeightAt(x, y)
	for i := x; i < x+w; i++ {
		for j := y; j < y+d; j++ {
			height = max(height, f.HeightAt(i, j))
		}
	}
	for i := x; i < x+w; i++ {
		for j := y; j < y+d; j++ {
			f.objectTiles[i][j] = true
			f.tiles[i][j].height = height
		}
	}
	f.objects = append(f.objects, PlacedObject{X: x, Y: y, W: w, D: d, Slug: a.Slug})

	id, err := a.LayoutID()
	if err != nil {
		return placement{}, err
	}
	inst := slab.Instance{
		X: max(0, int(math.Round((float64(x)+float64(w)/2-a.Center[0])*100))),
		Z: max(0, int(math.Round((float64(y)+float64(d)/2-a.Center[1])*100))),
		Y: surface(height),
	}
	if inst.X > slab.MaxCoordinate || inst.Y > slab.MaxCoordinate || inst.Z > slab.MaxCoordinate {
		return placement{}, fmt.Errorf("object %q exceeds Slab coordinate limits", a.Slug)
	}
	return placement{tileX: x, tileY: y, imported: &slab.Layout{ID: id, Instances: []slab.Instance{inst}}}, nil
}

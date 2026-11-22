package generator

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/catalogs"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/slab"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

// Ruined buildings, built the way TaleSpire players build them. The rules come
// from 95 community slabs studied on 22 Nov 2026 (TalesTavern; the slabs
// themselves are not in the repository, see docs/analyse-slabs-ruines.md):
//
//   - a ruin is a rectangle of 4 to 8 tiles a side, its stone floor laid on top
//     of the ground and broken in places, the grass showing through;
//   - its walls are a single course of the Castle Ruins wall, half a tile thick
//     on the inner half of the ring around the floor, with a doorway and gaps;
//   - about half the wall pieces sit on the ground, the rest are sunk one to
//     three steps into it, which is what makes the top line jagged; a few carry
//     a second, sunk course, the last tall remnants;
//   - rubble is stacked where two walls meet and lies in the gaps and inside,
//     and plants (ferns, dead bushes, sometimes a tree) grow where the floor
//     is gone.
//
// Ruins are placed in every zone whose relief_override is "ruins", after the
// buildings and objects and before the scatter, from their own random source:
// the rest of the map comes out the same with or without them.

const ruinSeedSalt = 0x2e1a5

// ruinCover is the share of a ruins zone that ruined buildings aim to cover;
// the rest is the zone's own ground and scatter, the streets between them.
const ruinCover = 0.4

// Ruin is a ruined building's outer footprint, wall ring included, in tiles.
type Ruin struct{ X, Y, W, D int }

// RuinWall is one wall piece seen from above, in tiles (map space).
type RuinWall struct{ X1, Y1, X2, Y2 float64 }

// Ruins lists the ruined buildings placed on the map.
func (f *HeightField) Ruins() []Ruin { return f.ruins }

// RuinWalls lists the wall pieces of the ruined buildings, for the preview.
func (f *HeightField) RuinWalls() []RuinWall { return f.ruinWalls }

var (
	ruinFloors = []string{"castle_ruins_floor_small", "castle_ruins_floor_stone_1x1", "castle_ruins_stone_floor_2"}
	ruinPlants = []string{"fern_01", "fern_02", "dead_bush_01", "mushroom_patch_forest_01"}
)

const (
	ruinWall    = "castle_ruins_wall_01"
	ruinRubble  = "castle_ruins_rockpile_misc_exterior"
	ruinRocks   = "rubble_rocks_pile"
	ruinPebbles = "rubble_pebbles_spread_out"
	ruinTree    = "tree_01"
)

// Wall rotations, in 15° steps, by the side of the floor the wall stands on.
// Measured on the community ruins: west 0, east 12, north 6, south 18 (north
// is the game's, which is the map's north once mirrored on the way out).
const (
	rotWest  = 0
	rotNorth = 6
	rotEast  = 12
	rotSouth = 18
)

// placeRuins builds the ruined buildings of every ruins zone.
func (g *Generator) placeRuins(doc *ir.IR, mask *spatial.Mask, f *HeightField, paths *spatial.Paths, seed int64) ([]placement, error) {
	if g.assets == nil {
		return nil, nil
	}
	var out []placement
	rng := rand.New(rand.NewSource(seed ^ ruinSeedSalt))
	// A point of interest placed by hand (an altar among the ruins) keeps its
	// tile: no ruin is built on it or right next to it.
	kept := map[[2]int]bool{}
	for _, z := range doc.Zones {
		for _, poi := range z.PointsOfInterest {
			if !poi.Position.Scattered {
				kept[[2]int{poi.Position.X, poi.Position.Y}] = true
			}
		}
	}
	for zi := range doc.Zones {
		if doc.Zones[zi].ReliefOverride != "ruins" {
			continue
		}
		var tiles [][2]int
		for x := 0; x < mask.Width; x++ {
			for y := 0; y < mask.Length; y++ {
				if mask.ZoneIndexAt(x, y) == zi {
					tiles = append(tiles, [2]int{x, y})
				}
			}
		}
		rng.Shuffle(len(tiles), func(i, j int) { tiles[i], tiles[j] = tiles[j], tiles[i] })
		target := int(float64(len(tiles)) * ruinCover)
		covered := 0
		for _, t := range tiles {
			if covered >= target {
				break
			}
			iw, id := 4+rng.Intn(5), 4+rng.Intn(5)
			if !f.ruinFits(mask, zi, paths, kept, t[0], t[1], iw+2, id+2) {
				continue
			}
			ps, err := g.buildRuin(f, rng, t[0], t[1], iw, id)
			if err != nil {
				return nil, err
			}
			out = append(out, ps...)
			covered += (iw + 2) * (id + 2)
		}
	}
	return out, nil
}

// ruinFits reports whether a w x d footprint at (x,y) lies inside zone zi, on
// dry land, off the paths, clear of buildings, objects and points of interest
// placed by hand, with a free tile all round so two ruins never share a wall.
func (f *HeightField) ruinFits(mask *spatial.Mask, zi int, paths *spatial.Paths, kept map[[2]int]bool, x, y, w, d int) bool {
	if x < 0 || y < 0 || x+w > f.Width || y+d > f.Length {
		return false
	}
	for i := x - 1; i <= x+w; i++ {
		for j := y - 1; j <= y+d; j++ {
			if i < 0 || j < 0 || i >= f.Width || j >= f.Length {
				continue
			}
			if f.BuildingAt(i, j) || f.ObjectAt(i, j) || kept[[2]int{i, j}] {
				return false
			}
			inside := i >= x && i < x+w && j >= y && j < y+d
			if inside && (mask.ZoneIndexAt(i, j) != zi || f.IsWaterAt(i, j) || (paths != nil && paths.On(i, j))) {
				return false
			}
		}
	}
	return true
}

// buildRuin reserves and levels a ruin's footprint (outer corner x,y, an
// iw x id floor inside a ring of walls) and returns its pieces.
func (g *Generator) buildRuin(f *HeightField, rng *rand.Rand, x, y, iw, id int) ([]placement, error) {
	w, d := iw+2, id+2
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
	f.ruins = append(f.ruins, Ruin{X: x, Y: y, W: w, D: d})
	ground := surface(height)

	b := ruinBuilder{g: g, f: f}
	x0, y0 := float64(x+1), float64(y+1) // the floor's north-west corner
	fx, fy := float64(x+1+iw), float64(y+1+id)

	// The floor, on top of the ground, broken in places.
	floored := make([][]bool, iw)
	for i := range floored {
		floored[i] = make([]bool, id)
		for j := range floored[i] {
			if rng.Float64() < 0.72 {
				floored[i][j] = true
				tx, ty := x0+float64(i), y0+float64(j)
				b.tile(ruinFloors[rng.Intn(len(ruinFloors))], tx, ty, tx+1, ty+1, ground, uint8(6*rng.Intn(4)))
			}
		}
	}
	// The walls: one doorway on one side, then gaps where the wall fell.
	sides := []struct {
		n      int
		rot    uint8
		rect   func(k int) (float64, float64, float64, float64)
		middle func(k int) (float64, float64)
	}{
		{iw, rotNorth, func(k int) (float64, float64, float64, float64) {
			return x0 + float64(k), y0 - 0.5, x0 + float64(k) + 1, y0
		}, func(k int) (float64, float64) { return x0 + float64(k) + 0.5, y0 - 0.25 }},
		{iw, rotSouth, func(k int) (float64, float64, float64, float64) {
			return x0 + float64(k), fy, x0 + float64(k) + 1, fy + 0.5
		}, func(k int) (float64, float64) { return x0 + float64(k) + 0.5, fy + 0.25 }},
		{id, rotWest, func(k int) (float64, float64, float64, float64) {
			return x0 - 0.5, y0 + float64(k), x0, y0 + float64(k) + 1
		}, func(k int) (float64, float64) { return x0 - 0.25, y0 + float64(k) + 0.5 }},
		{id, rotEast, func(k int) (float64, float64, float64, float64) {
			return fx, y0 + float64(k), fx + 0.5, y0 + float64(k) + 1
		}, func(k int) (float64, float64) { return fx + 0.25, y0 + float64(k) + 0.5 }},
	}
	doorSide := rng.Intn(4)
	for s, side := range sides {
		door, doorW := -1, 0
		if s == doorSide {
			doorW = 1
			if side.n >= 7 && rng.Float64() < 0.5 {
				doorW = 2
			}
			door = 1 + rng.Intn(side.n-1-doorW)
		}
		gap := 0
		for k := 0; k < side.n; k++ {
			if door >= 0 && k >= door && k < door+doorW {
				continue // the doorway stays clear
			}
			if gap == 0 && rng.Float64() < 0.13 {
				gap = 1 + rng.Intn(2)
			}
			if gap > 0 {
				gap--
				if rng.Float64() < 0.5 {
					cx, cy := side.middle(k)
					b.prop(ruinRubble, cx, cy, ground, uint8(rng.Intn(24)))
				}
				continue
			}
			x1, y1, x2, y2 := side.rect(k)
			sink := ruinSink(rng)
			b.tile(ruinWall, x1, y1, x2, y2, max(0, ground-sink), side.rot)
			if sink == 0 && rng.Float64() < 0.12 {
				// A taller remnant: a second course, sunk into the first.
				b.tile(ruinWall, x1, y1, x2, y2, ground+wallUnits-stepUnits*(1+rng.Intn(3)), side.rot)
			}
			f.ruinWalls = append(f.ruinWalls, RuinWall{x1, y1, x2, y2})
		}
	}
	// Rubble stacked where two walls meet: the corner square they leave open.
	for _, c := range [][2]float64{{x0 - 0.25, y0 - 0.25}, {fx + 0.25, y0 - 0.25}, {x0 - 0.25, fy + 0.25}, {fx + 0.25, fy + 0.25}} {
		for k, n := 0, 1+rng.Intn(3); k < n; k++ {
			b.prop(ruinRubble, c[0], c[1], ground+k*rubbleStep, uint8(rng.Intn(24)))
		}
	}
	// Inside: rubble, a spread of pebbles, and plants where the floor is gone.
	top := func(i, j int) int {
		if floored[i][j] {
			return ground + stepUnits
		}
		return ground
	}
	for k, n := 0, 1+rng.Intn(3); k < n; k++ {
		i, j := rng.Intn(iw), rng.Intn(id)
		slug := ruinRubble
		if rng.Float64() < 0.4 {
			slug = ruinRocks
		}
		b.prop(slug, x0+float64(i)+0.25+rng.Float64()/2, y0+float64(j)+0.25+rng.Float64()/2, top(i, j), uint8(rng.Intn(24)))
	}
	if rng.Float64() < 0.5 {
		i, j := rng.Intn(iw), rng.Intn(id)
		b.prop(ruinPebbles, x0+float64(i)+0.5, y0+float64(j)+0.5, top(i, j), uint8(rng.Intn(24)))
	}
	var bare [][2]int
	for i := 0; i < iw; i++ {
		for j := 0; j < id; j++ {
			if !floored[i][j] {
				bare = append(bare, [2]int{i, j})
			}
		}
	}
	tree := -1
	if iw >= 5 && id >= 5 && len(bare) > 0 && rng.Float64() < 0.35 {
		tree = rng.Intn(len(bare))
	}
	for k, t := range bare {
		cx, cy := x0+float64(t[0])+0.5, y0+float64(t[1])+0.5
		switch {
		case k == tree:
			b.prop(ruinTree, cx, cy, ground, uint8(rng.Intn(24)))
		case rng.Float64() < 0.3:
			b.prop(ruinPlants[rng.Intn(len(ruinPlants))], cx, cy, ground, uint8(rng.Intn(24)))
		}
	}
	return b.out, b.err
}

// Heights in Slab units: a step (half a tile) is 50, a ruin wall four steps.
const (
	wallUnits  = 4 * stepUnits
	rubbleStep = 22 // the rise between stacked rubble piles in the community slabs
)

// ruinSink draws how deep a wall piece is sunk into the ground, in Slab units:
// on the ground half the time, else one, two or three steps down, as measured
// on the community ruins.
func ruinSink(rng *rand.Rand) int {
	switch r := rng.Float64(); {
	case r < 0.5:
		return 0
	case r < 0.75:
		return stepUnits
	case r < 0.9:
		return 2 * stepUnits
	default:
		return 3 * stepUnits
	}
}

// ruinBuilder turns catalogue pieces into exact placements.
type ruinBuilder struct {
	g   *Generator
	f   *HeightField
	out []placement
	err error
}

func (b *ruinBuilder) asset(slug string) *catalogs.Asset {
	a, ok := b.g.assets.BySlug(slug)
	if !ok && b.err == nil {
		b.err = fmt.Errorf("ruins: %q is not in the catalogue", slug)
	}
	return a
}

// tile places a tile whose rotated footprint covers [x1,x2) x [y1,y2) in map
// tiles, its bottom at level (Slab units). A tile's stored position is the
// corner of its rotated footprint (measured on the community slabs: floor
// tiles turned by any quarter keep the same position), so the mirror on the
// way out shifts it by the footprint's depth.
func (b *ruinBuilder) tile(slug string, x1, y1, x2, y2 float64, level int, rot uint8) {
	a := b.asset(slug)
	if a == nil {
		return
	}
	b.emit(a, x1, y1, int(math.Round((y2-y1)*100)), level, rot, int(math.Floor(x1)), int(math.Ceil(y2))-1)
}

// prop places a prop centred on (cx,cy) in map tiles: a prop's origin is its
// centre, which the mirror maps onto itself.
func (b *ruinBuilder) prop(slug string, cx, cy float64, level int, rot uint8) {
	a := b.asset(slug)
	if a == nil {
		return
	}
	b.emit(a, cx-a.Center[0], cy-a.Center[1], int(math.Round(2*a.Center[1]*100)), level, rot, int(math.Floor(cx)), int(math.Floor(cy)))
}

func (b *ruinBuilder) emit(a *catalogs.Asset, x, y float64, mirror, level int, rot uint8, tileX, tileY int) {
	id, err := a.LayoutID()
	if err != nil {
		if b.err == nil {
			b.err = err
		}
		return
	}
	inst := slab.Instance{
		X:        max(0, int(math.Round(x*100))),
		Z:        max(0, int(math.Round(y*100))),
		Y:        max(0, level),
		Rotation: rot % 24,
	}
	if inst.X > slab.MaxCoordinate || inst.Y > slab.MaxCoordinate || inst.Z > slab.MaxCoordinate {
		if b.err == nil {
			b.err = fmt.Errorf("ruins: %q exceeds Slab coordinate limits", a.Slug)
		}
		return
	}
	b.out = append(b.out, placement{
		tileX:    min(max(tileX, 0), b.f.Width-1),
		tileY:    min(max(tileY, 0), b.f.Length-1),
		imported: &slab.Layout{ID: id, Instances: []slab.Instance{inst}},
		mirror:   mirror,
	})
}

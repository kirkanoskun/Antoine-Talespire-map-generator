// Package generator turns a resolved zone mask into a TaleSpire slab.
//
// This is the heart of Phase 1 (brief section 5.4). A map has ONE dominant
// biome, fixed for the whole map. Zones never change biome: they modulate,
// within that single biome, the prop density, the relief used (via an optional
// relief_override), and the explicit points of interest. The generator reuses
// taleslab's biome repository, prop catalogue, slab entities and encoder; the
// seam it adds is a per-tile relief lookup driven by the zone mask, instead of
// taleslab's single relief-per-map fill. Smooth density/height transitions
// between zones of the same biome (brief 5.5) are out of scope for Phase 1.
package generator

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/johnfercher/talescoder/pkg/encoder"
	"github.com/johnfercher/taleslab/pkg/taleslab/taleslabdomain/taleslabconsts/biometype"
	"github.com/johnfercher/taleslab/pkg/taleslab/taleslabdomain/taleslabconsts/elementtype"
	"github.com/johnfercher/taleslab/pkg/taleslab/taleslabdomain/taleslabentities"
	domainrepos "github.com/johnfercher/taleslab/pkg/taleslab/taleslabdomain/taleslabrepositories"
	"github.com/johnfercher/taleslab/pkg/taleslab/taleslabrepositories"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/catalogs"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/prefab"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/slab"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

// Elevation shaping constants (in TaleSpire height units, i.e. stacked blocks).
const (
	baseHeight      = 1
	hillGain        = 4
	mountainGain    = 9
	depressionDepth = 3
	// mountainThreshold: tiles raised at least this much above base take the
	// "mountain" relief (rockier building blocks/props) unless a zone overrides
	// the relief explicitly.
	mountainThreshold = 5
	groundRotation    = 768
)

// defaultTransitionHalfWidth is the band half-width (in tiles) over which zone
// borders are smoothed. Two zones therefore blend across ~2*this tiles.
const defaultTransitionHalfWidth = 3

// Generator holds the shared, stateless taleslab resources.
type Generator struct {
	biomes          domainrepos.BiomeRepository
	props           domainrepos.PropRepository
	enc             encoder.Encoder
	transitionHalf  int
	smoothingPasses int
	sliceSize       int
	prefabs         *prefab.Store
}

// New loads the biome and prop catalogues from the given config paths.
func New(biomesPath, propsPath string) (*Generator, error) {
	biomes, err := taleslabrepositories.NewBiomeRepository(biomesPath)
	if err != nil {
		return nil, fmt.Errorf("loading biomes: %w", err)
	}
	props, err := taleslabrepositories.NewPropRepository(propsPath)
	if err != nil {
		return nil, fmt.Errorf("loading props: %w", err)
	}
	return newGenerator(biomes, props), nil
}

// NewFromJSON is New for catalogues already in memory: the browser build has
// no filesystem to load them from.
func NewFromJSON(biomesJSON, propsJSON []byte) (*Generator, error) {
	biomes, err := catalogs.ParseBiomes(biomesJSON)
	if err != nil {
		return nil, err
	}
	props, err := catalogs.ParseProps(propsJSON)
	if err != nil {
		return nil, err
	}
	return newGenerator(biomes, props), nil
}

func newGenerator(biomes domainrepos.BiomeRepository, props domainrepos.PropRepository) *Generator {
	return &Generator{
		biomes:          biomes,
		props:           props,
		enc:             encoder.NewEncoder(),
		transitionHalf:  defaultTransitionHalfWidth,
		smoothingPasses: 2,
	}
}

// SetTransitionHalfWidth sets the zone-border stitching band half-width in
// tiles. Zero disables stitching (hard borders). This is the Phase 3 knob.
func (g *Generator) SetTransitionHalfWidth(tiles int) {
	if tiles < 0 {
		tiles = 0
	}
	g.transitionHalf = tiles
}

// SetSliceSize sets the tile size of each exported slab slice. A positive value
// slices the map into a grid of slabs of at most that many tiles per side, each
// with its own local origin so they paste adjacent in TaleSpire. Zero (default)
// slices automatically, and only when the whole map is over TaleSpire's ~30 kB
// per-slab limit (a larger slab pastes but fails on save / board switch): the
// largest slice size whose every slab fits is picked.
func (g *Generator) SetSliceSize(tiles int) {
	if tiles < 0 {
		tiles = 0
	}
	g.sliceSize = tiles
}

// TaleSpireSlabLimitBytes is TaleSpire's documented per-slab size limit: a slab
// larger than this pastes but fails when saved or when switching boards.
// (https://talespire.com/faq)
const TaleSpireSlabLimitBytes = 30000

// Result is the output of a generation run.
type Result struct {
	Code       string       // base64 TaleSpire slab for the whole map (paste into the game)
	Slices     [][]string   // grid of per-slice codes ([x][y]) when the map is sliced; nil otherwise
	SliceSize  int          // tiles per side of each slice (the last row/column may be smaller); 0 if not sliced
	AssetCount int          // total assets placed
	Height     *HeightField // per-tile height/relief, for preview and inspection
	Warnings   []string     // non-fatal issues (unknown POI props, oversized slabs)
}

// HeightField exposes the shaped terrain so the preview renderer can shade it.
type HeightField struct {
	Width     int
	Length    int
	tiles     [][]tile
	buildings [][]bool
	dug       [][]bool // ground removed under a building's underground part
	pois      [][2]int // tiles where a point of interest was actually placed
	props     [][]PropKind
}

// PropKind is the scatter a tile carries on top of its ground (at most one
// prop per tile), recorded so the preview can show density, not just terrain.
type PropKind uint8

const (
	PropNone  PropKind = iota
	PropTree           // the "vegetation" category on land: trees, cacti
	PropStone          // the "stones" category
	PropMisc           // the "misc" category (bushes, flowers, ferns…) and coral
)

// PropAt returns the scatter placed on tile (x,y).
func (f *HeightField) PropAt(x, y int) PropKind {
	if f.props == nil {
		return PropNone
	}
	return f.props[x][y]
}

// POIs returns the tiles where points of interest were placed, scattered ones
// included, so the preview shows what the slab really contains.
func (f *HeightField) POIs() [][2]int { return f.pois }

// HeightAt returns the terrain height at (x,y).
func (h *HeightField) HeightAt(x, y int) int { return h.tiles[x][y].height }

// ReliefAt returns the resolved relief key at (x,y).
func (h *HeightField) ReliefAt(x, y int) string { return h.tiles[x][y].relief }

// IsWaterAt reports whether (x,y) resolves to the water relief.
func (h *HeightField) IsWaterAt(x, y int) bool {
	return h.tiles[x][y].relief == string(elementtype.Water)
}

type tile struct {
	height int
	relief string // resolved relief key (relief_override, else height-derived)
	zone   int
}

// Generate produces a TaleSpire slab for the resolved mask. The seed makes the
// procedural scatter (and scattered POIs) fully reproducible.
func (g *Generator) Generate(doc *ir.IR, mask *spatial.Mask, seed int64) (*Result, error) {
	biome := g.biomes.GetBiome(biometype.BiomeType(doc.Map.Biome))
	if biome == nil {
		return nil, fmt.Errorf("map biome %q is not present in the biome catalogue", doc.Map.Biome)
	}
	// Config-aware validation: every relief_override must exist in the biome.
	for i := range doc.Zones {
		if ov := doc.Zones[i].ReliefOverride; ov != "" {
			if biome.Reliefs[elementtype.ElementType(ov)] == nil {
				return nil, fmt.Errorf("zone %q: relief_override %q is not a relief of biome %q",
					doc.Zones[i].ID, ov, doc.Map.Biome)
			}
		}
	}

	rng := rand.New(rand.NewSource(seed))

	field := shapeTerrain(doc, mask)

	// Phase 3 stitching: smooth height and prop density across zone borders.
	// Only meaningful with more than one zone.
	var trans *spatial.Transitions
	if g.transitionHalf > 0 && len(doc.Zones) > 1 {
		trans = mask.Transitions(g.transitionHalf)
		smoothHeights(field, trans, g.smoothingPasses)
	}

	// Carve connections into bare, leveled corridors that ramp between zones.
	var paths *spatial.Paths
	if len(doc.Connections) > 0 {
		paths = spatial.BuildPaths(doc)
		g.carvePaths(doc, field, paths, mask)
	}

	buildingParts, err := g.placeBuildings(doc, field)
	if err != nil {
		return nil, err
	}
	res := &Result{Height: field}
	var placements []placement
	placements = g.placeGround(placements, biome, field, rng)
	placements = g.placeProps(placements, biome, doc, field, trans, paths, rng)
	placements = g.placePOIs(placements, doc, mask, field, rng, res)
	placements = append(placements, buildingParts...)
	res.AssetCount = len(placements)

	// Whole-map slab (always produced, for a single copy-paste).
	code, err := g.encodeRegion(placements, 0, 0)
	if err != nil {
		return nil, err
	}
	res.Code = code

	// Sliced grid: at the configured size, or automatically when the whole map
	// is too big for one slab.
	size := g.sliceSize
	if size == 0 && len(code) > TaleSpireSlabLimitBytes {
		size = firstSliceGuess(len(code), field.Width, field.Length)
	}
	if size == 0 {
		return res, nil
	}
	for {
		slices, err := g.encodeSlices(placements, field.Width, field.Length, size)
		if err != nil {
			return nil, err
		}
		// Automatic slicing shrinks until every slab fits with a margin, as the
		// limit is only documented as "about 30 kB"; a slice too dense to fit
		// even when small (a big building, say) is reported instead.
		if g.sliceSize == 0 && len(oversizedSlices(slices, autoSliceTarget)) > 0 && size > minAutoSlice {
			size = max(minAutoSlice, size*3/4)
			continue
		}
		res.Slices, res.SliceSize = slices, size
		for _, o := range oversizedSlices(slices, TaleSpireSlabLimitBytes) {
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"slice [%d,%d] is %d bytes, over TaleSpire's ~%d byte limit; use a smaller -slice",
				o[0], o[1], o[2], TaleSpireSlabLimitBytes))
		}
		return res, nil
	}
}

// minAutoSlice is the smallest slice side automatic slicing will try, and
// autoSliceTarget the slab size it aims below.
const (
	minAutoSlice    = 8
	autoSliceTarget = TaleSpireSlabLimitBytes * 9 / 10
)

// oversizedSlices lists the slices over limit bytes, as {x, y, bytes}.
func oversizedSlices(slices [][]string, limit int) [][3]int {
	var out [][3]int
	for sx := range slices {
		for sy, code := range slices[sx] {
			if len(code) > limit {
				out = append(out, [3]int{sx, sy, len(code)})
			}
		}
	}
	return out
}

// firstSliceGuess estimates the slice side at which each slab fits the limit,
// assuming bytes spread evenly over the map, with a margin for dense spots.
func firstSliceGuess(codeBytes, w, l int) int {
	side := float64(max(w, l)) * math.Sqrt(0.8*autoSliceTarget/float64(codeBytes))
	return max(minAutoSlice, int(side))
}

// placement is a deferred asset: a part at a tile with a z and base rotation.
// Coordinates are finalized per slice so each slab can use a local origin.
type placement struct {
	part         *taleslabentities.Part
	tileX, tileY int
	z            int
	baseRotation int
	imported     *slab.Layout
}

// encodeRegion builds and encodes a single slab from the placements whose tile
// falls in the region [originX, originX+size) — or all placements when size is
// non-positive — with coordinates rebased to (originX, originY).
func (g *Generator) encodeRegion(placements []placement, originX, originY int) (string, error) {
	terrain := &taleslabentities.Slab{}
	var imported []slab.Layout
	importedIndex := map[string]int{}
	for i := range placements {
		p := &placements[i]
		if p.imported == nil {
			terrain.Assets = append(terrain.Assets, buildAsset(p, originX, originY))
		} else {
			l := *p.imported
			l.Instances = append([]slab.Instance(nil), l.Instances...)
			for j := range l.Instances {
				l.Instances[j].X -= originX * 100
				l.Instances[j].Z -= originY * 100
			}
			key := fmt.Sprintf("%x/%d", l.ID, l.Reserved)
			if idx, ok := importedIndex[key]; ok {
				imported[idx].Instances = append(imported[idx].Instances, l.Instances...)
			} else {
				importedIndex[key] = len(imported)
				imported = append(imported, l)
			}
		}
	}
	code, err := g.enc.Encode(taleSpireSlabFromSlab(terrain))
	if err != nil {
		return "", fmt.Errorf("encoding terrain: %w", err)
	}
	if len(imported) > 0 {
		// Preserve the existing terrain pipeline byte-for-byte when no buildings
		// are present. Imported layouts bypass talescoder's lossy axis adapter.
		combined, err := slab.DecodeGenerated(code)
		if err != nil {
			return "", fmt.Errorf("decoding terrain for prefab merge: %w", err)
		}
		combined.Layouts = append(combined.Layouts, imported...)
		code, err = slab.Encode(combined)
		if err != nil {
			return "", fmt.Errorf("encoding buildings: %w", err)
		}
	}
	return code, nil
}

// encodeSlices partitions the map into a grid of slabs of at most size tiles
// per side, each with a local origin, and encodes each.
func (g *Generator) encodeSlices(placements []placement, w, l, size int) ([][]string, error) {
	nx := (w + size - 1) / size
	ny := (l + size - 1) / size

	// Bucket placements by slice for a single pass.
	buckets := make([][][]placement, nx)
	for sx := 0; sx < nx; sx++ {
		buckets[sx] = make([][]placement, ny)
	}
	for _, p := range placements {
		sx := p.tileX / size
		sy := p.tileY / size
		buckets[sx][sy] = append(buckets[sx][sy], p)
	}

	slices := make([][]string, nx)
	for sx := 0; sx < nx; sx++ {
		slices[sx] = make([]string, ny)
		for sy := 0; sy < ny; sy++ {
			code, err := g.encodeRegion(buckets[sx][sy], sx*size, sy*size)
			if err != nil {
				return nil, err
			}
			slices[sx][sy] = code
		}
	}
	return slices, nil
}

// shapeTerrain computes per-tile height and the resolved relief key from each
// zone's elevation intent and optional relief_override, using a Gaussian
// mound/pit centred on the zone anchor.
//
// Precedence (confirmed design): relief_override wins for the material (which
// relief/blocks/props a tile uses); elevation still controls height. A deep
// depression becomes water only when the zone does not override the relief.
func shapeTerrain(doc *ir.IR, mask *spatial.Mask) *HeightField {
	w, l := doc.Map.Width, doc.Map.Length
	counts := mask.TileCounts()

	// sigma per zone: derived from the zone's area (radius of an equivalent
	// disc), so the mound/pit fills roughly the zone.
	sigma := make([]float64, len(doc.Zones))
	for i, c := range counts {
		radius := math.Sqrt(float64(c) / math.Pi)
		sigma[i] = math.Max(radius/2.0, 1.0)
	}

	f := &HeightField{Width: w, Length: l, tiles: make([][]tile, w)}
	for x := 0; x < w; x++ {
		f.tiles[x] = make([]tile, l)
		for y := 0; y < l; y++ {
			zi := mask.ZoneIndexAt(x, y)
			z := &doc.Zones[zi]
			dx := float64(x - z.Anchor.X)
			dy := float64(y - z.Anchor.Y)
			falloff := math.Exp(-(dx*dx + dy*dy) / (2 * sigma[zi] * sigma[zi]))

			h := baseHeight
			elem := elementtype.Ground
			switch z.Elevation {
			case ir.ElevationHill:
				h = baseHeight + int(math.Round(hillGain*falloff))
			case ir.ElevationMountain:
				bump := int(math.Round(mountainGain * falloff))
				h = baseHeight + bump
				if bump >= mountainThreshold {
					elem = elementtype.Mountain
				}
			case ir.ElevationDepression:
				h = baseHeight - int(math.Round(depressionDepth*falloff))
				if h <= 0 {
					h = 0
					elem = elementtype.Water
				}
			default: // flat
				h = baseHeight
			}

			relief := string(elem)
			if z.ReliefOverride != "" {
				relief = z.ReliefOverride
			}
			f.tiles[x][y] = tile{height: h, relief: relief, zone: zi}
		}
	}
	return f
}

// placeGround stacks building blocks for every tile, filling vertical gaps down
// to the lowest 4-neighbour so cliffs between heights are solid (mirrors
// taleslab's wall-filling), using the map biome and each tile's resolved relief.
func (g *Generator) placeGround(out []placement, biome *taleslabentities.Biome, f *HeightField, rng *rand.Rand) []placement {
	for x := 0; x < f.Width; x++ {
		for y := 0; y < f.Length; y++ {
			t := f.tiles[x][y]
			block := g.buildingBlock(biome, t.relief, rng)
			// A dug tile still draws its block, so digging a cellar leaves the
			// random sequence — and so the rest of the map — unchanged.
			if block == nil || f.dugAt(x, y) {
				continue
			}

			minH := t.height
			for _, n := range neighbours(f, x, y) {
				if n < minH {
					minH = n
				}
			}

			for _, part := range block.Parts {
				for k := minH; k <= t.height; k++ {
					out = append(out, placement{part: part, tileX: x, tileY: y, z: k + part.OffsetZ, baseRotation: groundRotation})
				}
			}
		}
	}
	return out
}

// placeProps scatters vegetation / stones / misc using the map biome's per-relief
// weights, modulated by each zone's density_overrides, with taleslab-style
// spacing. When trans is non-nil, the effective density near a zone border is
// blended toward the neighbouring zone's density so the change is gradual rather
// than a hard line.
func (g *Generator) placeProps(out []placement, biome *taleslabentities.Biome, doc *ir.IR, f *HeightField, trans *spatial.Transitions, paths *spatial.Paths, rng *rand.Rand) []placement {
	occupied := make([][]bool, f.Width)
	for x := range occupied {
		occupied[x] = make([]bool, f.Length)
	}

	categories := []struct {
		elem     elementtype.ElementType
		override string
		kind     PropKind
	}{
		{elementtype.Tree, "vegetation", PropTree},
		{elementtype.Stone, "stones", PropStone},
		{elementtype.Misc, "misc", PropMisc},
	}
	f.props = make([][]PropKind, f.Width)
	for x := range f.props {
		f.props[x] = make([]PropKind, f.Length)
	}

	for x := 1; x < f.Width-1; x++ {
		for y := 1; y < f.Length-1; y++ {
			if f.BuildingAt(x, y) || tooClose(occupied, x, y) {
				continue
			}
			if paths != nil && paths.On(x, y) {
				continue // keep carved paths clear of scatter
			}
			t := f.tiles[x][y]
			zone := &doc.Zones[t.zone]
			relief := biome.Reliefs[elementtype.ElementType(t.relief)]
			if relief == nil || relief.PropBlocks == nil {
				continue
			}

			for _, cat := range categories {
				dist := distributionFor(relief.PropBlocks, cat.elem)
				if len(dist.Props) == 0 {
					continue
				}
				weight := categoryWeight(zone, cat.override, dist.Weight)
				// Blend toward the neighbouring zone's density near a border. The
				// neighbour's fallback default must come from *its* relief, not this
				// tile's — otherwise a ground/mountain seam blends toward the wrong
				// biome default. When the neighbour overrides its relief we can look
				// up that relief's weight; otherwise this tile's default is the best
				// proxy we have (the neighbour's relief is height-derived per tile).
				if trans != nil {
					if other, blend := trans.At(x, y); other >= 0 && blend > 0 {
						otherDefault := dist.Weight
						if ov := doc.Zones[other].ReliefOverride; ov != "" {
							if orelief := biome.Reliefs[elementtype.ElementType(ov)]; orelief != nil && orelief.PropBlocks != nil {
								otherDefault = distributionFor(orelief.PropBlocks, cat.elem).Weight
							}
						}
						ow := categoryWeight(&doc.Zones[other], cat.override, otherDefault)
						weight += (ow - weight) * blend
					}
				}
				if rng.Float64() >= weight {
					continue
				}

				propID := dist.Props[rng.Intn(len(dist.Props))]
				prop := g.props.GetProp(propID)
				if prop == nil {
					continue
				}
				rot := randomRotation(rng)
				for _, part := range prop.Parts {
					out = append(out, placement{part: part, tileX: x, tileY: y, z: t.height + part.OffsetZ, baseRotation: rot})
				}
				occupied[x][y] = true
				f.props[x][y] = cat.kind
				if cat.kind == PropTree && t.relief == string(elementtype.Water) {
					f.props[x][y] = PropMisc // underwater "vegetation" is coral, not a tree
				}
				break // one prop per tile
			}
		}
	}
	return out
}

// placePOIs positions explicit points of interest deterministically.
func (g *Generator) placePOIs(out []placement, doc *ir.IR, mask *spatial.Mask, f *HeightField, rng *rand.Rand, res *Result) []placement {
	for zi := range doc.Zones {
		zone := &doc.Zones[zi]
		for _, poi := range zone.PointsOfInterest {
			propID := resolvePropAlias(poi.Prop)
			prop := g.props.GetProp(propID)
			if prop == nil {
				res.Warnings = append(res.Warnings,
					fmt.Sprintf("zone %q: unknown POI prop %q (skipped)", zone.ID, poi.Prop))
				continue
			}

			var positions [][2]int
			if poi.Position.Scattered {
				positions = scatterInZone(mask, zi, poi.Count, rng)
			} else {
				positions = [][2]int{{poi.Position.X, poi.Position.Y}}
			}

			for _, p := range positions {
				x, y := p[0], p[1]
				if f.BuildingAt(x, y) {
					res.Warnings = append(res.Warnings, fmt.Sprintf("zone %q: POI %q overlaps a reserved building footprint (skipped)", zone.ID, poi.Prop))
					continue
				}
				f.pois = append(f.pois, [2]int{x, y})
				rot := randomRotation(rng)
				for _, part := range prop.Parts {
					out = append(out, placement{part: part, tileX: x, tileY: y, z: f.tiles[x][y].height + part.OffsetZ, baseRotation: rot})
				}
			}
		}
	}
	return out
}

// pathRelief is the relief carved paths use: a bare, walkable ground present in
// every biome, so a corridor reads as a distinct trail through the scene.
const pathRelief = "base_ground"

// carvePaths turns the path overlay into leveled corridors. Each path tile's
// height is set to a linear ramp between the two connected zones' anchor heights
// (so the trail climbs from one to the other) and its relief to bare ground.
// Endpoint heights are snapshotted before any tile is rewritten. A path never
// paves over water: it stops at the shore, so a pond stays a pond.
func (g *Generator) carvePaths(doc *ir.IR, f *HeightField, paths *spatial.Paths, mask *spatial.Mask) {
	if !paths.Any() {
		return
	}
	var labels [][2]int
	anchorHeight := make(map[string]int, len(doc.Zones))
	for i := range doc.Zones {
		z := &doc.Zones[i]
		x, y := z.Anchor.X, z.Anchor.Y
		// A zone with another nested at its centre (a courtyard around its
		// pond) does not own its own anchor: take the height of a tile it does.
		if mask.ZoneIndexAt(x, y) != i {
			if labels == nil {
				labels = mask.LabelPoints()
			}
			x, y = labels[i][0], labels[i][1]
		}
		anchorHeight[z.ID] = f.tiles[x][y].height
	}
	type endpoints struct {
		h0, h1 int
		ok     bool
	}
	eps := make([]endpoints, len(doc.Connections))
	for ci := range doc.Connections {
		c := &doc.Connections[ci]
		h0, ok0 := anchorHeight[c.From]
		h1, ok1 := anchorHeight[c.To]
		eps[ci] = endpoints{h0: h0, h1: h1, ok: ok0 && ok1}
	}

	for x := 0; x < f.Width; x++ {
		for y := 0; y < f.Length; y++ {
			if !paths.On(x, y) || f.IsWaterAt(x, y) {
				continue
			}
			if ci := paths.Conn(x, y); ci >= 0 && eps[ci].ok {
				t := paths.Param(x, y)
				h := float64(eps[ci].h0) + (float64(eps[ci].h1)-float64(eps[ci].h0))*t
				f.tiles[x][y].height = int(h + 0.5)
			}
			f.tiles[x][y].relief = pathRelief
		}
	}
}

// buildingBlock picks a building block for a relief key, falling back to the
// biome's "ground" relief when the resolved relief has no building blocks.
func (g *Generator) buildingBlock(b *taleslabentities.Biome, reliefKey string, rng *rand.Rand) *taleslabentities.Prop {
	relief := b.Reliefs[elementtype.ElementType(reliefKey)]
	if relief == nil || len(relief.BuildingBlocks) == 0 {
		relief = b.Reliefs[elementtype.Ground]
	}
	if relief == nil || len(relief.BuildingBlocks) == 0 {
		return nil
	}
	key := relief.BuildingBlocks[rng.Intn(len(relief.BuildingBlocks))]
	return g.props.GetProp(key)
}

// --- helpers ---

// categoryWeight returns a zone's effective placement weight for a prop category:
// the zone's density_override if present, else the biome's default weight for the
// tile's relief.
func categoryWeight(zone *ir.Zone, override string, defaultWeight float64) float64 {
	if ov, ok := zone.DensityOverrides[override]; ok {
		return ov
	}
	return defaultWeight
}

// smoothHeights averages heights within the transition band so cliffs between
// zones of different elevation become gradual slopes. It runs a few passes,
// writing each pass into a fresh buffer for order-independence. Water tiles keep
// their height (a pond stays a pond) but still pull their neighbours down, so a
// hill slopes toward the water's edge.
func smoothHeights(f *HeightField, trans *spatial.Transitions, passes int) {
	for p := 0; p < passes; p++ {
		type update struct {
			x, y, h int
		}
		var updates []update
		for x := 0; x < f.Width; x++ {
			for y := 0; y < f.Length; y++ {
				if !trans.InBand(x, y) || f.tiles[x][y].relief == string(elementtype.Water) {
					continue
				}
				sum := f.tiles[x][y].height
				cnt := 1
				for _, h := range neighbours(f, x, y) {
					sum += h
					cnt++
				}
				updates = append(updates, update{x, y, (sum + cnt/2) / cnt})
			}
		}
		for _, u := range updates {
			f.tiles[u.x][u.y].height = u.h
		}
	}
}

func neighbours(f *HeightField, x, y int) []int {
	var out []int
	if x > 0 {
		out = append(out, f.tiles[x-1][y].height)
	}
	if x < f.Width-1 {
		out = append(out, f.tiles[x+1][y].height)
	}
	if y > 0 {
		out = append(out, f.tiles[x][y-1].height)
	}
	if y < f.Length-1 {
		out = append(out, f.tiles[x][y+1].height)
	}
	return out
}

func tooClose(occupied [][]bool, x, y int) bool {
	if x > 1 && (occupied[x-1][y] || occupied[x-2][y]) {
		return true
	}
	if y > 1 && (occupied[x][y-1] || occupied[x][y-2]) {
		return true
	}
	return false
}

func distributionFor(pb *taleslabentities.PropBlocks, e elementtype.ElementType) taleslabentities.PropDistribution {
	switch e {
	case elementtype.Tree:
		return pb.Vegetation
	case elementtype.Stone:
		return pb.Stones
	default:
		return pb.Misc
	}
}

// buildAsset finalizes a placement into a TaleSpire asset, rebasing its tile to
// the given slice origin. Grid indices are multiplied by the asset's footprint
// (taleslab's convention); the rotation is nudged by the global row so tiling
// props don't look uniform (using the global tileY keeps a single-slab map's
// output identical whether or not slicing is enabled).
func buildAsset(p *placement, originX, originY int) *taleslabentities.Asset {
	part := p.part
	x := p.tileX - originX
	y := p.tileY - originY
	return &taleslabentities.Asset{
		ID:         part.ID,
		Name:       part.Name,
		Dimensions: part.Dimensions,
		OffsetZ:    part.OffsetZ,
		// A coordinate is a grid position, NOT a position scaled by the asset's
		// size. taleslab multiplies each axis by the asset's dimensions here,
		// which is a no-op for the 1x1x1 assets it happens to use but displaces
		// anything larger: a width-2 bush lands at twice its X, and a height-2
		// wall (big_stone_wall, used by the ruins relief) floats at twice its
		// height. We store the position itself.
		Coordinates: &taleslabentities.Vector3d{X: x, Y: y, Z: p.z},
		// Overflow compensation, and the reason taleslab's odd "/41" term exists:
		// the encoder packs Y as y*1600 into a uint16, which wraps every 41 tiles
		// (41*1600 > 65535). Each wrap drops 65536 from that field — 4096 once
		// shifted into the packed Y — and because the rotation word's low bits
		// overlap Y's high bits, adding 1 per wrap puts exactly that back.
		Rotation: p.baseRotation + y/41,
	}
}

func randomRotation(rng *rand.Rand) int {
	// Four cardinal orientations (TaleSpire uses 0..1535 for a full turn).
	return rng.Intn(4) * 384
}

func scatterInZone(mask *spatial.Mask, zoneIdx, count int, rng *rand.Rand) [][2]int {
	var tiles [][2]int
	for x := 0; x < mask.Width; x++ {
		for y := 0; y < mask.Length; y++ {
			if mask.ZoneIndexAt(x, y) == zoneIdx {
				tiles = append(tiles, [2]int{x, y})
			}
		}
	}
	if len(tiles) == 0 {
		return nil
	}
	rng.Shuffle(len(tiles), func(i, j int) { tiles[i], tiles[j] = tiles[j], tiles[i] })
	if count > len(tiles) {
		count = len(tiles)
	}
	return tiles[:count]
}

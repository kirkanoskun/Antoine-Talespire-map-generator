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
 prefabs *prefab.Store
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
	return &Generator{
		biomes:          biomes,
		props:           props,
		enc:             encoder.NewEncoder(),
		transitionHalf:  defaultTransitionHalfWidth,
		smoothingPasses: 2,
	}, nil
}

// SetTransitionHalfWidth sets the zone-border stitching band half-width in
// tiles. Zero disables stitching (hard borders). This is the Phase 3 knob.
func (g *Generator) SetTransitionHalfWidth(tiles int) {
	if tiles < 0 {
		tiles = 0
	}
	g.transitionHalf = tiles
}

// SetSliceSize sets the tile size of each exported slab slice. Zero (default)
// emits the whole map as a single slab. A positive value slices the map into a
// grid of slabs of at most that many tiles per side, each with its own local
// origin so they paste adjacent in TaleSpire. Slicing is the Phase 5 answer to
// TaleSpire's ~30 kB per-slab limit (a large single slab fails on save / board
// switch).
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
	Slices     [][]string   // grid of per-slice codes when slicing is enabled ([x][y]); nil otherwise
	AssetCount int          // total assets placed
	Height     *HeightField // per-tile height/relief, for preview and inspection
	Warnings   []string     // non-fatal issues (unknown POI props, oversized slabs)
}

// HeightField exposes the shaped terrain so the preview renderer can shade it.
type HeightField struct {
	Width  int
	Length int
	tiles  [][]tile
 buildings [][]bool
}

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
		g.carvePaths(doc, field, paths)
	}

 buildingParts, err := g.placeBuildings(doc, field)
 if err != nil { return nil, err }
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
	if len(code) > TaleSpireSlabLimitBytes && g.sliceSize == 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"slab is %d bytes, over TaleSpire's ~%d byte limit; enable slicing (-slice) to export it in pieces",
			len(code), TaleSpireSlabLimitBytes))
	}

	// Optional sliced grid for large maps.
	if g.sliceSize > 0 {
		slices, err := g.encodeSlices(placements, field.Width, field.Length, res)
		if err != nil {
			return nil, err
		}
		res.Slices = slices
	}
	return res, nil
}

// placement is a deferred asset: a part at a tile with a z and base rotation.
// Coordinates are finalized per slice so each slab can use a local origin.
type placement struct {
	part         *taleslabentities.Part
	tileX, tileY int
	z            int
	baseRotation int
 imported *slab.Layout
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
    l.Instances[j].X -= originX*100
    l.Instances[j].Z -= originY*100
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
 if err != nil { return "", fmt.Errorf("encoding terrain: %w", err) }
 if len(imported) > 0 {
  // Preserve the existing terrain pipeline byte-for-byte when no buildings
  // are present. Imported layouts bypass talescoder's lossy axis adapter.
  combined, err := slab.DecodeGenerated(code)
  if err != nil { return "", fmt.Errorf("decoding terrain for prefab merge: %w", err) }
  combined.Layouts = append(combined.Layouts, imported...)
  code, err = slab.Encode(combined)
  if err != nil { return "", fmt.Errorf("encoding buildings: %w", err) }
 }
	return code, nil
}

// encodeSlices partitions the map into a grid of slabs of at most sliceSize
// tiles per side, each with a local origin, and encodes each. Oversized slices
// are reported as warnings.
func (g *Generator) encodeSlices(placements []placement, w, l int, res *Result) ([][]string, error) {
	size := g.sliceSize
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
			if len(code) > TaleSpireSlabLimitBytes {
				res.Warnings = append(res.Warnings, fmt.Sprintf(
					"slice [%d,%d] is %d bytes, over TaleSpire's ~%d byte limit; use a smaller -slice",
					sx, sy, len(code), TaleSpireSlabLimitBytes))
			}
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
			if block == nil {
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
	}{
		{elementtype.Tree, "vegetation"},
		{elementtype.Stone, "stones"},
		{elementtype.Misc, "misc"},
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
				// Blend toward the neighbouring zone's density near a border.
				if trans != nil {
					if other, blend := trans.At(x, y); other >= 0 && blend > 0 {
						ow := categoryWeight(&doc.Zones[other], cat.override, dist.Weight)
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
// Endpoint heights are snapshotted before any tile is rewritten.
func (g *Generator) carvePaths(doc *ir.IR, f *HeightField, paths *spatial.Paths) {
	if !paths.Any() {
		return
	}
	anchorHeight := make(map[string]int, len(doc.Zones))
	for i := range doc.Zones {
		z := &doc.Zones[i]
		anchorHeight[z.ID] = f.tiles[z.Anchor.X][z.Anchor.Y].height
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
			if !paths.On(x, y) {
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
	return &taleslabentities.Asset{
		ID:         part.ID,
		Name:       part.Name,
		Dimensions: part.Dimensions,
		OffsetZ:    part.OffsetZ,
		Coordinates: &taleslabentities.Vector3d{
			X: (p.tileX - originX) * part.Dimensions.Width,
			Y: (p.tileY - originY) * part.Dimensions.Length,
			Z: p.z * part.Dimensions.Height,
		},
		Rotation: p.baseRotation + (p.tileY * part.Dimensions.Length / 41),
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

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

// Generator holds the shared, stateless taleslab resources.
type Generator struct {
	biomes domainrepos.BiomeRepository
	props  domainrepos.PropRepository
	enc    encoder.Encoder
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
	return &Generator{biomes: biomes, props: props, enc: encoder.NewEncoder()}, nil
}

// Result is the output of a generation run.
type Result struct {
	Code       string       // base64 TaleSpire slab, paste into the game
	AssetCount int          // total assets placed
	Height     *HeightField // per-tile height/relief, for preview and inspection
	Warnings   []string     // non-fatal issues (e.g. unknown POI prop ids)
}

// HeightField exposes the shaped terrain so the preview renderer can shade it.
type HeightField struct {
	Width  int
	Length int
	tiles  [][]tile
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
	slab := &taleslabentities.Slab{}
	res := &Result{Height: field}

	g.placeGround(slab, biome, field, rng)
	g.placeProps(slab, biome, doc, field, rng)
	g.placePOIs(slab, doc, mask, field, rng, res)

	res.AssetCount = len(slab.Assets)

	taleSpireSlab := taleSpireSlabFromSlab(slab)
	code, err := g.enc.Encode(taleSpireSlab)
	if err != nil {
		return nil, fmt.Errorf("encoding slab: %w", err)
	}
	res.Code = code
	return res, nil
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
func (g *Generator) placeGround(slab *taleslabentities.Slab, biome *taleslabentities.Biome, f *HeightField, rng *rand.Rand) {
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
					asset := newAsset(part)
					setCoordinates(asset, x, y, k+part.OffsetZ, groundRotation)
					slab.Assets = append(slab.Assets, asset)
				}
			}
		}
	}
}

// placeProps scatters vegetation / stones / misc using the map biome's per-relief
// weights, modulated by each zone's density_overrides, with taleslab-style
// spacing.
func (g *Generator) placeProps(slab *taleslabentities.Slab, biome *taleslabentities.Biome, doc *ir.IR, f *HeightField, rng *rand.Rand) {
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
			if tooClose(occupied, x, y) {
				continue
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
				weight := dist.Weight
				if ov, ok := zone.DensityOverrides[cat.override]; ok {
					weight = ov
				}
				if rng.Float64() >= weight {
					continue
				}

				propID := dist.Props[rng.Intn(len(dist.Props))]
				prop := g.props.GetProp(propID)
				if prop == nil {
					continue
				}
				for _, part := range prop.Parts {
					asset := newAsset(part)
					setCoordinates(asset, x, y, t.height+part.OffsetZ, randomRotation(rng))
					slab.Assets = append(slab.Assets, asset)
				}
				occupied[x][y] = true
				break // one prop per tile
			}
		}
	}
}

// placePOIs positions explicit points of interest deterministically.
func (g *Generator) placePOIs(slab *taleslabentities.Slab, doc *ir.IR, mask *spatial.Mask, f *HeightField, rng *rand.Rand, res *Result) {
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
				for _, part := range prop.Parts {
					asset := newAsset(part)
					setCoordinates(asset, x, y, f.tiles[x][y].height+part.OffsetZ, randomRotation(rng))
					slab.Assets = append(slab.Assets, asset)
				}
			}
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

func newAsset(part *taleslabentities.Part) *taleslabentities.Asset {
	return &taleslabentities.Asset{
		ID:         part.ID,
		Name:       part.Name,
		Dimensions: part.Dimensions,
		OffsetZ:    part.OffsetZ,
	}
}

// setCoordinates matches taleslab's convention: grid indices are multiplied by
// the asset's footprint, and rotation is nudged by row so tiling props don't
// look uniform.
func setCoordinates(asset *taleslabentities.Asset, x, y, z, rotation int) {
	asset.Coordinates = &taleslabentities.Vector3d{
		X: x * asset.Dimensions.Width,
		Y: y * asset.Dimensions.Length,
		Z: z * asset.Dimensions.Height,
	}
	asset.Rotation = rotation + (y * asset.Dimensions.Length / 41)
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

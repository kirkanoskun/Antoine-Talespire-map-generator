// Command composemap is an EXPERIMENT: it generates a map from an IR and drops
// a decoded community building (a "Chimera" slab, e.g. from Tales Tavern) onto
// it, then writes a single combined TaleSpire code.
//
// It deliberately does not live inside internal/generator: it composes the
// generator and internal/chimera from the outside, so the generation pipeline
// stays untouched while we validate that imported buildings land correctly.
//
//	go run ./cmd/composemap -ir testdata/forest_edge.json \
//	    -building inn.txt -at 30,7 -out out/map.txt -preview out/map.png
//
// Vertical placement: the generator stacks ground so a tile's top block sits at
// height h, and props rest at h+1. The building is therefore translated so its
// lowest element lands on h+1, where h is the highest terrain under its
// footprint (using the highest point means the building is never half-buried).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/chimera"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/preview"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

const (
	unitsPerTile = 100 // horizontal game units per tile
	unitsPerStep = 50  // vertical game units per height step
)

func main() {
	irPath := flag.String("ir", "", "IR JSON describing the map (required)")
	buildPath := flag.String("building", "", "file holding the building's base64 slab code (required)")
	at := flag.String("at", "0,0", "tile position for the building's north-west corner: x,y")
	biomes := flag.String("biomes", "configs/biomes.json", "biome catalogue")
	props := flag.String("props", "configs/props.json", "prop catalogue")
	seed := flag.Int64("seed", 1, "generation seed")
	transition := flag.Int("transition", 3, "zone-border stitching half-width")
	outPath := flag.String("out", "", "write the combined TaleSpire code here")
	previewPath := flag.String("preview", "", "write a 2D preview PNG here")
	keepProps := flag.Bool("keep-props", false, "keep generated props inside the building footprint")
	sliceSize := flag.Int("slice", 0, "also write the map cut into slabs of at most N tiles per side (0 = single slab)")
	flag.Parse()

	if *irPath == "" || *buildPath == "" {
		flag.Usage()
		os.Exit(2)
	}
	ax, ay, err := parseAt(*at)
	if err != nil {
		log.Fatal(err)
	}

	// --- 1. generate the map -------------------------------------------------
	raw, err := os.ReadFile(*irPath)
	if err != nil {
		log.Fatalf("reading IR: %v", err)
	}
	doc, err := ir.Parse(raw)
	if err != nil {
		log.Fatalf("parsing IR: %v", err)
	}
	gen, err := generator.New(*biomes, *props)
	if err != nil {
		log.Fatalf("generator: %v", err)
	}
	gen.SetTransitionHalfWidth(*transition)

	mask := spatial.Resolve(doc)
	res, err := gen.Generate(doc, mask, *seed)
	if err != nil {
		log.Fatalf("generating map: %v", err)
	}
	for _, w := range res.Warnings {
		log.Printf("warning: %s", w)
	}

	mapSlab, err := chimera.Decode(res.Code)
	if err != nil {
		log.Fatalf("decoding generated map: %v", err)
	}

	// --- 2. decode the building ---------------------------------------------
	bcode, err := os.ReadFile(*buildPath)
	if err != nil {
		log.Fatalf("reading building: %v", err)
	}
	building, err := chimera.Decode(strings.TrimSpace(string(bcode)))
	if err != nil {
		log.Fatalf("decoding building: %v", err)
	}

	minX, minY, minZ := extent(building)
	wTiles := int((maxOf(building, func(p chimera.Placement) uint32 { return p.RawX }) - minX)) / unitsPerTile
	lTiles := int((maxOf(building, func(p chimera.Placement) uint32 { return p.RawY }) - minY)) / unitsPerTile

	// --- 3. terrain under the footprint --------------------------------------
	loH, hiH := terrainRange(res.Height, ax, ay, wTiles, lTiles)
	if loH < 0 {
		log.Fatalf("building footprint (%d..%d, %d..%d) falls outside the %dx%d map",
			ax, ax+wTiles, ay, ay+lTiles, doc.Map.Width, doc.Map.Length)
	}
	if loH != hiH {
		log.Printf("note: terrain under the footprint is not level (heights %d..%d); "+
			"seating the building on the highest point so nothing is buried", loH, hiH)
	}
	baseStep := hiH + 1 // props rest one step above the top ground block

	// --- 4. translate the building into place --------------------------------
	dx := int(ax*unitsPerTile) - int(minX)
	dy := int(ay*unitsPerTile) - int(minY)
	dz := baseStep*unitsPerStep - int(minZ)
	for ai := range building.Assets {
		for pi := range building.Assets[ai].Placements {
			p := &building.Assets[ai].Placements[pi]
			p.RawX = uint32(int(p.RawX) + dx)
			p.RawY = uint32(int(p.RawY) + dy)
			p.RawZ = uint32(int(p.RawZ) + dz)
		}
	}

	// --- 5. clear generated props under the building --------------------------
	// Ground blocks fill up to h (rawZ <= h*unitsPerStep); props sit above. So
	// dropping anything higher than the ground inside the footprint removes the
	// scatter (trees, rocks) without touching the terrain itself.
	removed := 0
	if !*keepProps {
		for ai := range mapSlab.Assets {
			kept := mapSlab.Assets[ai].Placements[:0]
			for _, p := range mapSlab.Assets[ai].Placements {
				tx, ty := int(p.RawX)/unitsPerTile, int(p.RawY)/unitsPerTile
				inside := tx >= ax && tx <= ax+wTiles && ty >= ay && ty <= ay+lTiles
				if inside {
					if h := res.Height.HeightAt(clamp(tx, 0, doc.Map.Width-1), clamp(ty, 0, doc.Map.Length-1)); int(p.RawZ) > h*unitsPerStep {
						removed++
						continue
					}
				}
				kept = append(kept, p)
			}
			mapSlab.Assets[ai].Placements = kept
		}
	}

	// --- 6. merge and encode --------------------------------------------------
	combined := merge(mapSlab, building)
	code, err := chimera.Encode(combined)
	if err != nil {
		log.Fatalf("encoding combined slab: %v", err)
	}

	total := 0
	for _, a := range combined.Assets {
		total += len(a.Placements)
	}
	fmt.Printf("map:      %dx%d, %d zones\n", doc.Map.Width, doc.Map.Length, len(doc.Zones))
	fmt.Printf("building: %d assets, %d placements, footprint %dx%d tiles\n",
		len(building.Assets), countPlacements(building), wTiles, lTiles)
	fmt.Printf("placed at tile (%d,%d), terrain height %d, building base step %d\n", ax, ay, hiH, baseStep)
	if removed > 0 {
		fmt.Printf("cleared %d generated props inside the footprint\n", removed)
	}
	fmt.Printf("combined: %d distinct assets, %d placements\n", len(combined.Assets), total)
	fmt.Printf("code size: %d chars\n", len(code))
	if len(code) > generator.TaleSpireSlabLimitBytes {
		fmt.Printf("WARNING: over TaleSpire's ~%d byte slab limit — it may paste but fail to save\n",
			generator.TaleSpireSlabLimitBytes)
	}

	if *outPath != "" {
		if err := os.WriteFile(*outPath, []byte(code), 0o644); err != nil {
			log.Fatalf("writing code: %v", err)
		}
		fmt.Printf("wrote %s\n", *outPath)
	}
	if *sliceSize > 0 {
		if err := writeSlices(combined, *sliceSize, *outPath); err != nil {
			log.Fatalf("slicing: %v", err)
		}
	}
	if *previewPath != "" {
		f, err := os.Create(*previewPath)
		if err != nil {
			log.Fatalf("creating preview: %v", err)
		}
		defer f.Close()
		if err := preview.WritePNG(f, doc, mask, res.Height, preview.Options{Scale: 8}); err != nil {
			log.Fatalf("writing preview: %v", err)
		}
		fmt.Printf("wrote %s (terrain only; the building is not drawn)\n", *previewPath)
	}
}

// merge concatenates two slabs, combining placements of assets they share.
// Order is map-first, then new building assets, so output is deterministic.
func merge(a, b *chimera.Slab) *chimera.Slab {
	out := &chimera.Slab{Version: a.Version, MagicBytes: a.MagicBytes}
	index := map[string]int{}
	add := func(s *chimera.Slab) {
		for _, asset := range s.Assets {
			if len(asset.Placements) == 0 {
				continue
			}
			if i, ok := index[asset.IDBase64]; ok {
				out.Assets[i].Placements = append(out.Assets[i].Placements, asset.Placements...)
				continue
			}
			index[asset.IDBase64] = len(out.Assets)
			out.Assets = append(out.Assets, chimera.Asset{
				IDBase64:   asset.IDBase64,
				Placements: append([]chimera.Placement(nil), asset.Placements...),
			})
		}
	}
	add(a)
	add(b)
	return out
}

func extent(s *chimera.Slab) (minX, minY, minZ uint32) {
	first := true
	for _, a := range s.Assets {
		for _, p := range a.Placements {
			if first {
				minX, minY, minZ = p.RawX, p.RawY, p.RawZ
				first = false
				continue
			}
			if p.RawX < minX {
				minX = p.RawX
			}
			if p.RawY < minY {
				minY = p.RawY
			}
			if p.RawZ < minZ {
				minZ = p.RawZ
			}
		}
	}
	return
}

func maxOf(s *chimera.Slab, sel func(chimera.Placement) uint32) uint32 {
	var m uint32
	for _, a := range s.Assets {
		for _, p := range a.Placements {
			if v := sel(p); v > m {
				m = v
			}
		}
	}
	return m
}

func countPlacements(s *chimera.Slab) int {
	n := 0
	for _, a := range s.Assets {
		n += len(a.Placements)
	}
	return n
}

// terrainRange returns the lowest and highest terrain height under the
// footprint, or (-1,-1) if the footprint leaves the map.
func terrainRange(h *generator.HeightField, ax, ay, w, l int) (int, int) {
	if ax < 0 || ay < 0 || ax+w >= h.Width || ay+l >= h.Length {
		return -1, -1
	}
	lo, hi := h.HeightAt(ax, ay), h.HeightAt(ax, ay)
	for x := ax; x <= ax+w; x++ {
		for y := ay; y <= ay+l; y++ {
			v := h.HeightAt(x, y)
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
	}
	return lo, hi
}

func parseAt(s string) (int, int, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("-at must be x,y (got %q)", s)
	}
	x, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("-at x: %w", err)
	}
	y, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("-at y: %w", err)
	}
	return x, y, nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// writeSlices cuts the combined slab into a grid of at most size tiles per side,
// rebasing each piece to its own local origin so the pieces paste adjacently.
// TaleSpire's per-slab limit means a large map plus a whole building cannot ship
// as one code; these pieces can.
func writeSlices(s *chimera.Slab, size int, outPath string) error {
	base := strings.TrimSuffix(outPath, ".txt")
	if base == "" {
		base = "slab"
	}
	type key struct{ cx, cy int }
	buckets := map[key]*chimera.Slab{}
	order := []key{}
	for _, a := range s.Assets {
		for _, p := range a.Placements {
			k := key{int(p.RawX) / unitsPerTile / size, int(p.RawY) / unitsPerTile / size}
			b, ok := buckets[k]
			if !ok {
				b = &chimera.Slab{Version: s.Version, MagicBytes: s.MagicBytes}
				buckets[k] = b
				order = append(order, k)
			}
			// Rebase to the slice's own origin.
			q := p
			q.RawX = uint32(int(p.RawX) - k.cx*size*unitsPerTile)
			q.RawY = uint32(int(p.RawY) - k.cy*size*unitsPerTile)
			appendPlacement(b, a.IDBase64, q)
		}
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].cx != order[j].cx {
			return order[i].cx < order[j].cx
		}
		return order[i].cy < order[j].cy
	})
	fmt.Printf("\nsliced into %d slabs of <= %d tiles per side:\n", len(order), size)
	for _, k := range order {
		code, err := chimera.Encode(buckets[k])
		if err != nil {
			return err
		}
		name := fmt.Sprintf("%s_x%d_y%d.txt", base, k.cx, k.cy)
		if err := os.WriteFile(name, []byte(code), 0o644); err != nil {
			return err
		}
		flag := ""
		if len(code) > generator.TaleSpireSlabLimitBytes {
			flag = "  <-- still over the limit"
		}
		fmt.Printf("  %-40s %6d chars%s\n", name, len(code), flag)
	}
	return nil
}

func appendPlacement(s *chimera.Slab, id string, p chimera.Placement) {
	for i := range s.Assets {
		if s.Assets[i].IDBase64 == id {
			s.Assets[i].Placements = append(s.Assets[i].Placements, p)
			return
		}
	}
	s.Assets = append(s.Assets, chimera.Asset{IDBase64: id, Placements: []chimera.Placement{p}})
}

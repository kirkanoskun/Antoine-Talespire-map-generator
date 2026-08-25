// Command slabdecode decodes a TaleSpire (Chimera) slab code with the corrected
// Y axis and reports whether the coordinates look sane. Use it to vet a real
// community slab before considering it for a building library.
//
//	pbpaste | go run ./cmd/slabdecode              # macOS: paste a copied slab
//	go run ./cmd/slabdecode -in slab.txt           # or from a file
//	go run ./cmd/slabdecode -in slab.txt -json     # full decoded placements
//
// It is intentionally not connected to the map generator.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/chimera"
)

func main() {
	in := flag.String("in", "", "file containing the base64 slab code (default: stdin)")
	asJSON := flag.Bool("json", false, "print the full decoded slab as JSON")
	levels := flag.Bool("levels", false, "list the slab's horizontal levels — use it to spot which one is ground level (e.g. where a palisade or yard sits) before placing a building")
	flag.Parse()

	var data []byte
	var err error
	if *in != "" {
		data, err = os.ReadFile(*in)
	} else {
		data, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
	code := strings.TrimSpace(string(data))
	if code == "" {
		fmt.Fprintln(os.Stderr, "no slab code provided (pass -in FILE or pipe it on stdin)")
		os.Exit(1)
	}

	slab, err := chimera.Decode(code)
	if err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}

	if *levels {
		printLevels(slab)
		return
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(slab)
		return
	}

	// Summary: ranges are the quick sanity check — a whole building should span
	// a few dozen tiles on each axis, never hundreds.
	var placements int
	minX, minY, minZ := 1e18, 1e18, 1e18
	maxX, maxY, maxZ := -1e18, -1e18, -1e18
	for _, a := range slab.Assets {
		for _, p := range a.Placements {
			placements++
			minX, maxX = min(minX, p.TileX), max(maxX, p.TileX)
			minY, maxY = min(minY, p.TileY), max(maxY, p.TileY)
			minZ, maxZ = min(minZ, p.Height), max(maxZ, p.Height)
		}
	}

	fmt.Printf("version:    %d\n", slab.Version)
	fmt.Printf("assets:     %d distinct\n", len(slab.Assets))
	fmt.Printf("placements: %d\n", placements)
	if placements > 0 {
		fmt.Printf("X range:    %.2f .. %.2f tiles\n", minX, maxX)
		fmt.Printf("Y range:    %.2f .. %.2f tiles   <- the axis this fixes\n", minY, maxY)
		fmt.Printf("Z range:    %.2f .. %.2f (height)\n", minZ, maxZ)
	}
	fmt.Printf("\nnote: %s\n", chimera.CalibrationNote)
}

// printLevels summarises the slab one horizontal level at a time: how many
// pieces sit there and how much ground they span. The level with the widest
// span and the most pieces is usually the building's ground floor — the one to
// line up with the terrain. Anything listed below it is meant to be buried.
func printLevels(slab *chimera.Slab) {
	type lvl struct {
		n                      int
		minX, maxX, minY, maxY float64
	}
	m := map[uint32]*lvl{}
	for _, a := range slab.Assets {
		for _, p := range a.Placements {
			l, ok := m[p.RawZ]
			if !ok {
				l = &lvl{minX: math.Inf(1), minY: math.Inf(1), maxX: math.Inf(-1), maxY: math.Inf(-1)}
				m[p.RawZ] = l
			}
			l.n++
			l.minX, l.maxX = math.Min(l.minX, p.TileX), math.Max(l.maxX, p.TileX)
			l.minY, l.maxY = math.Min(l.minY, p.TileY), math.Max(l.maxY, p.TileY)
		}
	}
	zs := make([]uint32, 0, len(m))
	busiest, busiestZ := 0, uint32(0)
	for z, l := range m {
		zs = append(zs, z)
		if l.n > busiest {
			busiest, busiestZ = l.n, z
		}
	}
	sort.Slice(zs, func(i, j int) bool { return zs[i] < zs[j] })

	fmt.Printf("%-8s %-7s %-7s %-26s %s\n", "rawZ", "step", "pieces", "span (tiles)", "")
	for _, z := range zs {
		l := m[z]
		if l.n < 25 { // hide incidental clutter
			continue
		}
		mark := ""
		if z == busiestZ {
			mark = "  <- busiest level (likely ground floor)"
		}
		fmt.Printf("%-8d %-7.1f %-7d x %5.1f..%-5.1f y %5.1f..%-5.1f%s\n",
			z, float64(z)/50, l.n, l.minX, l.maxX, l.minY, l.maxY, mark)
	}
	fmt.Printf("\nPass the chosen rawZ to composemap as -building-ground to seat that level on the terrain.\n")
}
